package types

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"tuna/internal/ast"
)

type SymbolKind int

const (
	SymVar SymbolKind = iota
	SymFunc
	SymBuiltin
	SymType
)

type Symbol struct {
	Name        string
	Kind        SymbolKind
	Type        *Type
	StorageType *Type
	Decl        ast.Decl
	Alias       *Symbol
}

type JSXComponentInfo struct {
	Symbol    *Symbol
	PropsType *Type
	ParamType *Type
}

type ModuleInfo struct {
	AST         *ast.Module
	Exports     map[string]*Symbol
	Top         map[string]*Symbol
	TypeAliases map[string]*TypeAlias
}

type TypeAlias struct {
	Params            []string
	Template          *Type
	ParamPlaceholders map[string]*Type
}

func newTypeAlias(params []string, template *Type, placeholders map[string]*Type) *TypeAlias {
	alias := &TypeAlias{
		Params:            append([]string(nil), params...),
		Template:          template,
		ParamPlaceholders: map[string]*Type{},
	}
	for name, placeholder := range placeholders {
		alias.ParamPlaceholders[name] = placeholder
	}
	return alias
}

// TableInfo stores information about a table definition
type TableInfo struct {
	Name    string
	Columns map[string]*ColumnInfo // column name -> column info
}

// ColumnInfo stores information about a column in a table
type ColumnInfo struct {
	Name        string
	Type        string
	Constraints string
}

type sqlColumnRef struct {
	Table  string
	Column string
}

type Checker struct {
	Modules       map[string]*ModuleInfo
	ExprTypes     map[ast.Expr]*Type
	IdentSymbols  map[*ast.IdentExpr]*Symbol
	TypeExprTypes map[ast.TypeExpr]*Type
	Tables        map[string]*TableInfo // table name -> table info
	Errors        []error
	JSXComponents map[*ast.JSXElement]*JSXComponentInfo
	symbolModule  map[*Symbol]*ModuleInfo
}

func NewChecker() *Checker {
	return &Checker{
		Modules:       map[string]*ModuleInfo{},
		ExprTypes:     map[ast.Expr]*Type{},
		IdentSymbols:  map[*ast.IdentExpr]*Symbol{},
		TypeExprTypes: map[ast.TypeExpr]*Type{},
		Tables:        map[string]*TableInfo{},
		JSXComponents: map[*ast.JSXElement]*JSXComponentInfo{},
		symbolModule:  map[*Symbol]*ModuleInfo{},
	}
}

func (c *Checker) AddModule(mod *ast.Module) {
	c.Modules[mod.Path] = &ModuleInfo{AST: mod, Exports: map[string]*Symbol{}, Top: map[string]*Symbol{}, TypeAliases: map[string]*TypeAlias{}}
}

func (c *Checker) Check() bool {
	preludeMod := c.Modules["prelude"]

	// First: process imports for prelude, then collect its top-level declarations.
	if preludeMod != nil {
		c.processImports(preludeMod)
		c.collectTop(preludeMod)
	}

	var builtinMods []*ModuleInfo
	for _, mod := range c.Modules {
		if mod == preludeMod {
			continue
		}
		if isBuiltinModulePath(mod.AST.Path) {
			builtinMods = append(builtinMods, mod)
		}
	}
	sort.Slice(builtinMods, func(i, j int) bool {
		return builtinMods[i].AST.Path < builtinMods[j].AST.Path
	})

	// Second: process imports for builtin modules (so their type aliases are available)
	for _, mod := range builtinMods {
		c.processImports(mod)
	}
	// Third: collect top-level declarations for builtin modules
	for _, mod := range builtinMods {
		c.collectTop(mod)
	}
	// Fourth: process imports for user modules
	for _, mod := range c.Modules {
		if mod == preludeMod || isBuiltinModulePath(mod.AST.Path) {
			continue
		}
		c.processImports(mod)
	}
	// Fifth: collect top-level declarations for user modules
	for _, mod := range c.Modules {
		if mod == preludeMod || isBuiltinModulePath(mod.AST.Path) {
			continue
		}
		c.collectTop(mod)
	}
	// Sixth: type check the module
	for _, mod := range c.Modules {
		c.checkModule(mod)
	}
	return len(c.Errors) == 0
}

// processImports handles import statements, including type aliases from built-in modules
func (c *Checker) processImports(mod *ModuleInfo) {
	for _, imp := range mod.AST.Imports {
		if !isBuiltinModulePath(imp.From) {
			continue
		}
		dep := c.Modules[imp.From]
		if dep == nil {
			continue
		}
		for _, item := range imp.Items {
			if !item.IsType {
				continue
			}
			if aliasInfo := dep.TypeAliases[item.Name]; aliasInfo != nil {
				mod.TypeAliases[item.Name] = aliasInfo
				continue
			}
			if exp := dep.Exports[item.Name]; exp != nil && exp.Kind == SymType {
				mod.TypeAliases[item.Name] = &TypeAlias{Template: exp.Type}
			}
		}
	}
}

func (c *Checker) collectTop(mod *ModuleInfo) {
	// First pass: collect type aliases
	for _, decl := range mod.AST.Decls {
		if d, ok := decl.(*ast.TypeAliasDecl); ok {
			alias := c.buildTypeAlias(d, mod)
			mod.TypeAliases[d.Name] = alias
			if d.Export {
				sym := &Symbol{Name: d.Name, Kind: SymType, Type: alias.Template, Decl: d}
				mod.Exports[d.Name] = sym
				c.symbolModule[sym] = mod
			}
		}
	}

	// Second pass: collect other declarations
	for _, decl := range mod.AST.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			if _, exists := mod.Top[d.Name]; exists {
				c.errorf(d.Span, "shadowing is not allowed: %s", d.Name)
				continue
			}
			declType := c.resolveType(d.Type, mod)
			sym := &Symbol{Name: d.Name, Kind: SymVar, Type: declType, StorageType: declType, Decl: d}
			if declType != nil && declType.Kind == KindFunc {
				sym.Kind = SymFunc
			}
			mod.Top[d.Name] = sym
			if d.Export {
				mod.Exports[d.Name] = sym
			}
			c.symbolModule[sym] = mod
		case *ast.FuncDecl:
			if _, exists := mod.Top[d.Name]; exists {
				c.errorf(d.Span, "shadowing is not allowed: %s", d.Name)
				continue
			}
			sig, _ := c.funcTypeFromDecl(d, mod)
			sym := &Symbol{Name: d.Name, Kind: SymFunc, Type: sig, Decl: d}
			mod.Top[d.Name] = sym
			if d.Export {
				mod.Exports[d.Name] = sym
			}
			c.symbolModule[sym] = mod
		case *ast.ExternFuncDecl:
			if _, exists := mod.Top[d.Name]; exists {
				c.errorf(d.Span, "shadowing is not allowed: %s", d.Name)
				continue
			}
			if !isBuiltinModulePath(mod.AST.Path) {
				c.errorf(d.Span, "extern function is only supported in builtin modules")
				continue
			}
			sig, _ := c.funcTypeFromExternDecl(d, mod)
			sym := &Symbol{Name: d.Name, Kind: SymFunc, Type: sig, Decl: d}
			mod.Top[d.Name] = sym
			if d.Export {
				mod.Exports[d.Name] = sym
			}
			c.symbolModule[sym] = mod
		case *ast.TableDecl:
			// Collect table definition
			tableInfo := &TableInfo{
				Name:    d.Name,
				Columns: map[string]*ColumnInfo{},
			}
			for _, col := range d.Columns {
				tableInfo.Columns[col.Name] = &ColumnInfo{
					Name:        col.Name,
					Type:        col.Type,
					Constraints: col.Constraints,
				}
			}
			c.Tables[d.Name] = tableInfo

			// Generate type alias for table row type
			// All columns are treated as string type (SQLite returns text)
			var props []Prop
			for _, col := range d.Columns {
				props = append(props, Prop{Name: col.Name, Type: String()})
			}
			rowType := NewObject(props)
			mod.TypeAliases[d.Name] = newTypeAlias(nil, rowType, nil)
		}
	}
}

func (c *Checker) buildTypeAlias(decl *ast.TypeAliasDecl, mod *ModuleInfo) *TypeAlias {
	params := append([]string(nil), decl.TypeParams...)
	typeParams := map[string]*Type{}
	paramPlaceholders := map[string]*Type{}
	for _, name := range params {
		placeholder := NewTypeParam(name)
		typeParams[name] = placeholder
		paramPlaceholders[name] = placeholder
	}
	template := c.resolveTypeRec(decl.Type, mod, typeParams)
	return newTypeAlias(params, template, paramPlaceholders)
}

func (c *Checker) checkModule(mod *ModuleInfo) {
	env := &Env{checker: c, mod: mod, vars: map[string]*Symbol{}}
	for name, sym := range mod.Top {
		env.vars[name] = sym
	}
	for _, imp := range mod.AST.Imports {
		dep, ok := c.Modules[imp.From]
		if !ok {
			c.errorf(imp.Span, "%s not found", imp.From)
			continue
		}
		if imp.DefaultName != "" {
			exp := dep.Exports["default"]
			if exp == nil {
				c.errorf(imp.Span, "default export not found in %s", imp.From)
			} else {
				c.bindImportedValue(env, imp.DefaultName, exp, imp.Span)
			}
		}
		for _, item := range imp.Items {
			exp := dep.Exports[item.Name]
			if exp == nil {
				c.errorf(imp.Span, "%s is not exported from %s", item.Name, imp.From)
				continue
			}
			if item.IsType {
				if exp.Kind != SymType {
					c.errorf(imp.Span, "%s is not a type", item.Name)
					continue
				}
				if aliasInfo := dep.TypeAliases[item.Name]; aliasInfo != nil {
					mod.TypeAliases[item.Name] = aliasInfo
				} else {
					mod.TypeAliases[item.Name] = &TypeAlias{Template: exp.Type}
				}
				continue
			}
			if exp.Kind == SymType {
				c.errorf(imp.Span, "%s is a type, use 'type %s' to import", item.Name, item.Name)
				continue
			}
			c.bindImportedValue(env, item.Name, exp, imp.Span)
		}
	}

	for _, decl := range mod.AST.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			c.checkConstDecl(env, d)
		case *ast.FuncDecl:
			c.checkFuncDecl(env, d)
		case *ast.ExternFuncDecl:
			// Extern functions have no body and are type-checked by signature only.
		}
	}
}

func (c *Checker) checkConstDecl(env *Env, d *ast.ConstDecl) {
	declType := c.resolveTypeInEnv(d.Type, env)
	if declType == nil {
		return
	}
	sym := env.lookup(d.Name)
	if sym != nil {
		if ident, ok := d.Init.(*ast.IdentExpr); ok {
			if target := env.lookup(ident.Name); target != nil {
				sym.Alias = target
			}
		}
	}
	if arrow, ok := d.Init.(*ast.ArrowFunc); ok {
		if declType.Kind != KindFunc {
			c.errorf(d.Span, "function type annotation required")
			return
		}
		sig := c.checkFuncLiteral(env, arrow, declType)
		if sig != nil {
			c.ExprTypes[arrow] = sig
		}
		return
	}
	if declType.Kind == KindFunc {
		initType := c.checkExpr(env, d.Init, declType)
		if initType != nil && !initType.AssignableTo(declType) {
			c.errorf(d.Span, "type mismatch")
		}
		return
	}
	initType := c.checkExpr(env, d.Init, declType)
	if initType != nil && !initType.AssignableTo(declType) {
		c.errorf(d.Span, "type mismatch")
	}
}

func (c *Checker) checkFuncDecl(env *Env, d *ast.FuncDecl) {
	sig, typeParams := c.funcTypeFromDecl(d, env.mod)
	if sig == nil {
		return
	}
	c.checkFuncBody(env, d.Params, d.Body, sig, typeParams)
}

func (c *Checker) checkFuncLiteral(env *Env, fn *ast.ArrowFunc, expected *Type) *Type {
	if expected != nil && expected.Kind != KindFunc {
		c.errorf(fn.Span, "function type required")
		return nil
	}
	var expectedParams []*Type
	var expectedRet *Type
	var typeParams map[string]*Type
	if expected != nil && expected.Kind == KindFunc {
		if len(expected.Params) != len(fn.Params) {
			c.errorf(fn.Span, "param count mismatch")
			return nil
		}
		expectedParams = expected.Params
		expectedRet = expected.Ret
		if len(expected.TypeParams) > 0 {
			typeParams = funcTypeParamMap(expected)
		}
	}
	typeEnv := env
	if typeParams != nil {
		typeEnv = env.child()
		typeEnv.typeParams = typeParams
	}

	params := make([]*Type, len(fn.Params))
	for i, p := range fn.Params {
		if p.Type == nil {
			if expectedParams == nil || expectedParams[i] == nil {
				c.errorf(p.Span, "param type required")
				return nil
			}
			params[i] = expectedParams[i]
			continue
		}
		params[i] = c.resolveTypeInEnv(p.Type, typeEnv)
		if params[i] == nil {
			return nil
		}
		if expectedParams != nil && expectedParams[i] != nil && expectedParams[i].Kind != KindTypeParam && !params[i].AssignableTo(expectedParams[i]) {
			c.errorf(p.Span, "param type mismatch")
			return nil
		}
	}

	body := fn.Body
	if body == nil && fn.Expr != nil {
		body = &ast.BlockStmt{Stmts: []ast.Stmt{&ast.ReturnStmt{Value: fn.Expr, Span: fn.Expr.GetSpan()}}, Span: fn.Span}
	}
	if body == nil {
		c.errorf(fn.Span, "function body required")
		return nil
	}

	if fn.Ret != nil {
		ret := c.resolveTypeInEnv(fn.Ret, typeEnv)
		if ret == nil {
			return nil
		}
		if expectedRet != nil && expectedRet.Kind != KindTypeParam && !ret.AssignableTo(expectedRet) {
			c.errorf(fn.Span, "return type mismatch")
			return nil
		}
		sig := NewFunc(params, ret)
		c.checkFuncBody(env, fn.Params, body, sig, typeParams)
		return sig
	}

	if expectedRet != nil && !typeContainsTypeParam(expectedRet) {
		sig := NewFunc(params, expectedRet)
		c.checkFuncBody(env, fn.Params, body, sig, typeParams)
		return sig
	}

	inferExpected := expectedRet
	if expectedRet != nil && typeContainsTypeParam(expectedRet) {
		inferExpected = nil
	}
	ret := c.checkFuncBodyInfer(env, fn.Params, body, params, inferExpected)
	if ret == nil {
		return nil
	}
	return NewFunc(params, ret)
}

func (c *Checker) funcTypeFromDecl(d *ast.FuncDecl, mod *ModuleInfo) (*Type, map[string]*Type) {
	return c.funcTypeFromSignature(d.TypeParams, d.Params, d.Ret, mod)
}

func (c *Checker) funcTypeFromExternDecl(d *ast.ExternFuncDecl, mod *ModuleInfo) (*Type, map[string]*Type) {
	return c.funcTypeFromSignature(d.TypeParams, d.Params, d.Ret, mod)
}

func (c *Checker) funcTypeFromSignature(typeParamNames []string, paramsDecl []ast.Param, retDecl ast.TypeExpr, mod *ModuleInfo) (*Type, map[string]*Type) {
	var typeParams map[string]*Type
	if len(typeParamNames) > 0 {
		typeParams = make(map[string]*Type, len(typeParamNames))
		for _, name := range typeParamNames {
			typeParams[name] = NewTypeParam(name)
		}
	}
	params := make([]*Type, len(paramsDecl))
	for i, p := range paramsDecl {
		if p.Type == nil {
			c.errorf(p.Span, "param type required")
			return nil, nil
		}
		params[i] = c.resolveTypeRec(p.Type, mod, typeParams)
		if params[i] == nil {
			return nil, nil
		}
	}
	ret := c.resolveTypeRec(retDecl, mod, typeParams)
	if ret == nil {
		return nil, nil
	}
	sig := NewFunc(params, ret)
	if len(typeParamNames) > 0 {
		sig.TypeParams = append([]string(nil), typeParamNames...)
	}
	return sig, typeParams
}

func (c *Checker) checkFuncBody(env *Env, params []ast.Param, body *ast.BlockStmt, sig *Type, typeParams map[string]*Type) {
	fnEnv := env.child()
	if typeParams != nil {
		fnEnv.typeParams = typeParams
	}
	fnEnv.retType = sig.Ret
	for i, p := range params {
		c.declareVar(fnEnv, p.Name, sig.Params[i], p.Span)
	}
	c.checkBlock(fnEnv, body, sig.Ret)
	if !allowsImplicitVoidReturn(sig.Ret) && !returns(body) {
		c.errorf(body.Span, "return required")
	}
}

type returnInfo struct {
	expected *Type
	inferred *Type
}

func (c *Checker) checkFuncBodyInfer(env *Env, params []ast.Param, body *ast.BlockStmt, paramTypes []*Type, expectedRet *Type) *Type {
	fnEnv := env.child()
	fnEnv.retType = expectedRet
	for i, p := range params {
		c.declareVar(fnEnv, p.Name, paramTypes[i], p.Span)
	}
	info := &returnInfo{expected: expectedRet}
	c.checkBlockInfer(fnEnv, body, info)

	ret := expectedRet
	if ret == nil {
		if info.inferred == nil {
			ret = Void()
		} else {
			ret = info.inferred
		}
	}
	if !allowsImplicitVoidReturn(ret) && !returns(body) {
		c.errorf(body.Span, "return required")
	}
	return ret
}

func (c *Checker) checkBlockInfer(env *Env, block *ast.BlockStmt, info *returnInfo) {
	local := env.child()
	for _, stmt := range block.Stmts {
		c.checkStmtInfer(local, stmt, info)
	}
}

func (c *Checker) checkStmtInfer(env *Env, stmt ast.Stmt, info *returnInfo) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		var declType *Type
		if s.Type != nil {
			declType = c.resolveTypeInEnv(s.Type, env)
			initType := c.checkExpr(env, s.Init, declType)
			if initType != nil && !initType.AssignableTo(declType) {
				c.errorf(s.Span, "type mismatch")
			}
		} else {
			initType := c.checkExpr(env, s.Init, nil)
			if initType == nil {
				c.errorf(s.Span, "cannot infer type")
				return
			}
			declType = initType
		}
		c.declareVar(env, s.Name, declType, s.Span)
	case *ast.DestructureStmt:
		initType := c.checkExpr(env, s.Init, nil)
		if initType == nil {
			return
		}

		var elemTypes []*Type
		if initType.Kind == KindArray {
			for range s.Names {
				elemTypes = append(elemTypes, initType.Elem)
			}
		} else if initType.Kind == KindTuple {
			if len(s.Names) > len(initType.Tuple) {
				c.errorf(s.Span, "destructuring has more elements than tuple")
				return
			}
			elemTypes = initType.Tuple[:len(s.Names)]
		} else {
			c.errorf(s.Span, "destructuring requires array or tuple")
			return
		}

		for i, name := range s.Names {
			var declType *Type
			if s.Types[i] != nil {
				declType = c.resolveTypeInEnv(s.Types[i], env)
				if declType != nil && elemTypes[i] != nil && !elemTypes[i].AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", name)
				}
			} else {
				declType = elemTypes[i]
			}
			c.declareVar(env, name, declType, s.Span)
		}
	case *ast.ObjectDestructureStmt:
		initType := c.checkExpr(env, s.Init, nil)
		if initType == nil {
			return
		}
		if initType.Kind != KindObject {
			c.errorf(s.Span, "object destructuring requires object type")
			return
		}
		for i, key := range s.Keys {
			var declType *Type
			propType := initType.PropType(key)
			if propType == nil {
				c.errorf(s.Span, "property '%s' not found in object", key)
				continue
			}
			if s.Types[i] != nil {
				declType = c.resolveTypeInEnv(s.Types[i], env)
				if declType != nil && propType != nil && !propType.AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", key)
				}
			} else {
				declType = propType
			}
			c.declareVar(env, key, declType, s.Span)
		}
	case *ast.ExprStmt:
		c.checkExpr(env, s.Expr, nil)
	case *ast.ReturnStmt:
		c.checkReturnInfer(env, s, info)
	case *ast.IfStmt:
		thenEnv := c.checkIfCond(env, s.Cond)
		c.checkBlockInfer(thenEnv, s.Then, info)
		if s.Else != nil {
			c.checkBlockInfer(env, s.Else, info)
		}
	case *ast.ForOfStmt:
		iterType := c.checkExpr(env, s.Iter, nil)
		if iterType == nil {
			return
		}
		elemType := arrayElemType(iterType)
		if elemType == nil {
			c.errorf(s.Span, "for-of requires array")
			return
		}
		loopVars, ok := c.resolveForOfBindings(env, s.Var, elemType)
		if !ok {
			return
		}
		loopEnv := env.child()
		for _, lv := range loopVars {
			if lv.Type == nil {
				continue
			}
			c.declareVar(loopEnv, lv.Name, lv.Type, s.Span)
		}
		c.checkBlockInfer(loopEnv, s.Body, info)
	case *ast.BlockStmt:
		c.checkBlockInfer(env, s, info)
	}
}

func (c *Checker) checkReturnInfer(env *Env, s *ast.ReturnStmt, info *returnInfo) {
	if s.Value == nil {
		if info.expected != nil && !allowsImplicitVoidReturn(info.expected) {
			c.errorf(s.Span, "return required")
		}
		if info.inferred == nil {
			info.inferred = Void()
		} else if info.inferred.Kind != KindVoid {
			c.errorf(s.Span, "return type mismatch")
		}
		return
	}

	valType := c.checkExpr(env, s.Value, info.expected)
	if valType == nil {
		return
	}
	if info.expected != nil {
		if !valType.AssignableTo(info.expected) {
			c.errorf(s.Span, "return type mismatch")
		}
		return
	}
	if info.inferred == nil {
		info.inferred = valType
		return
	}
	if !typesEqual(baseType(info.inferred), baseType(valType)) {
		c.errorf(s.Span, "return type mismatch")
	}
}

func returns(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		if len(s.Stmts) == 0 {
			return false
		}
		return returns(s.Stmts[len(s.Stmts)-1])
	case *ast.ReturnStmt:
		return true
	case *ast.IfStmt:
		if s.Else == nil {
			return false
		}
		return returns(s.Then) && returns(s.Else)
	default:
		return false
	}
}

func blockExprReturns(expr *ast.BlockExpr) bool {
	if expr == nil || len(expr.Stmts) == 0 {
		return false
	}
	return returns(&ast.BlockStmt{Stmts: expr.Stmts, Span: expr.Span})
}

func (c *Checker) checkBlock(env *Env, block *ast.BlockStmt, retType *Type) {
	local := env.child()
	for _, stmt := range block.Stmts {
		c.checkStmt(local, stmt, retType)
	}
}

func (c *Checker) checkStmt(env *Env, stmt ast.Stmt, retType *Type) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		var declType *Type
		if s.Type != nil {
			// 型注釈がある場合
			declType = c.resolveTypeInEnv(s.Type, env)
			initType := c.checkExpr(env, s.Init, declType)
			if initType != nil && !initType.AssignableTo(declType) {
				c.errorf(s.Span, "type mismatch")
			}
		} else {
			// 型推論：初期化式から型を推論
			initType := c.checkExpr(env, s.Init, nil)
			if initType == nil {
				c.errorf(s.Span, "cannot infer type")
				return
			}
			declType = initType
		}
		c.declareVar(env, s.Name, declType, s.Span)
	case *ast.DestructureStmt:
		// Check the initializer expression
		initType := c.checkExpr(env, s.Init, nil)
		if initType == nil {
			return
		}

		// Determine element types based on array or tuple
		var elemTypes []*Type
		if initType.Kind == KindArray {
			// For arrays, all elements have the same type
			for range s.Names {
				elemTypes = append(elemTypes, initType.Elem)
			}
		} else if initType.Kind == KindTuple {
			// For tuples, use the tuple element types
			if len(s.Names) > len(initType.Tuple) {
				c.errorf(s.Span, "destructuring has more elements than tuple")
				return
			}
			elemTypes = initType.Tuple[:len(s.Names)]
		} else {
			c.errorf(s.Span, "destructuring requires array or tuple")
			return
		}

		// Bind each variable
		for i, name := range s.Names {
			var declType *Type
			if s.Types[i] != nil {
				// Explicit type annotation
				declType = c.resolveTypeInEnv(s.Types[i], env)
				if declType != nil && elemTypes[i] != nil && !elemTypes[i].AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", name)
				}
			} else {
				// Type inference from array/tuple element
				declType = elemTypes[i]
			}
			c.declareVar(env, name, declType, s.Span)
		}
	case *ast.ObjectDestructureStmt:
		// Check the initializer expression
		initType := c.checkExpr(env, s.Init, nil)
		if initType == nil {
			return
		}

		// Must be an object type
		if initType.Kind != KindObject {
			c.errorf(s.Span, "object destructuring requires object type")
			return
		}

		// Bind each variable
		for i, key := range s.Keys {
			var declType *Type

			// Find the property type in the object
			propType := initType.PropType(key)
			if propType == nil {
				c.errorf(s.Span, "property '%s' not found in object", key)
				continue
			}

			if s.Types[i] != nil {
				// Explicit type annotation
				declType = c.resolveTypeInEnv(s.Types[i], env)
				if declType != nil && propType != nil && !propType.AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", key)
				}
			} else {
				// Type inference from object property
				declType = propType
			}
			c.declareVar(env, key, declType, s.Span)
		}
	case *ast.ExprStmt:
		c.checkExpr(env, s.Expr, nil)
	case *ast.ReturnStmt:
		if s.Value == nil {
			if !allowsImplicitVoidReturn(retType) {
				c.errorf(s.Span, "return required")
			}
			return
		}
		valType := c.checkExpr(env, s.Value, retType)
		if valType != nil && !valType.AssignableTo(retType) {
			c.errorf(s.Span, "return type mismatch")
		}
	case *ast.IfStmt:
		thenEnv := c.checkIfCond(env, s.Cond)
		c.checkBlock(thenEnv, s.Then, retType)
		if s.Else != nil {
			c.checkBlock(env, s.Else, retType)
		}
	case *ast.ForOfStmt:
		iterType := c.checkExpr(env, s.Iter, nil)
		if iterType == nil {
			return
		}
		elemType := arrayElemType(iterType)
		if elemType == nil {
			c.errorf(s.Span, "for-of requires array")
			return
		}
		loopVars, ok := c.resolveForOfBindings(env, s.Var, elemType)
		if !ok {
			return
		}
		loopEnv := env.child()
		for _, lv := range loopVars {
			if lv.Type == nil {
				continue
			}
			c.declareVar(loopEnv, lv.Name, lv.Type, s.Span)
		}
		c.checkBlock(loopEnv, s.Body, retType)
	case *ast.BlockStmt:
		c.checkBlock(env, s, retType)
	}
}

func (c *Checker) checkIfCond(env *Env, cond ast.Expr) *Env {
	if asExpr, ok := cond.(*ast.AsExpr); ok {
		targetType := c.checkExpr(env, asExpr, nil)
		if targetType == nil {
			return env
		}
		ident, ok := asExpr.Expr.(*ast.IdentExpr)
		if !ok {
			return env
		}
		sym := env.lookup(ident.Name)
		if sym == nil {
			return env
		}
		narrowedEnv := env.child()
		storageType := sym.StorageType
		if storageType == nil {
			storageType = sym.Type
		}
		narrowedEnv.vars[ident.Name] = &Symbol{
			Name:        sym.Name,
			Kind:        sym.Kind,
			Type:        targetType,
			StorageType: storageType,
			Decl:        sym.Decl,
		}
		return narrowedEnv
	}

	condType := c.checkExpr(env, cond, Bool())
	if condType != nil && condType.Kind != KindBool {
		c.errorf(cond.GetSpan(), "boolean required")
	}
	return env
}

func (c *Checker) declareVar(env *Env, name string, typ *Type, span ast.Span) {
	if existing := env.lookup(name); existing != nil {
		c.errorf(span, "shadowing is not allowed: %s", name)
		return
	}
	env.vars[name] = &Symbol{Name: name, Kind: SymVar, Type: typ, StorageType: typ}
}

func (c *Checker) bindImportedValue(env *Env, name string, sym *Symbol, span ast.Span) {
	if existing := env.lookup(name); existing != nil {
		c.errorf(span, "shadowing is not allowed: %s", name)
		return
	}
	env.vars[name] = sym
}

func allowsImplicitVoidReturn(t *Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case KindVoid, KindUndefined:
		return true
	case KindUnion:
		for _, member := range t.Union {
			if allowsImplicitVoidReturn(member) {
				return true
			}
		}
	}
	return false
}

type forOfVarBinding struct {
	Name string
	Type *Type
}

func (c *Checker) resolveForOfBindings(env *Env, binding ast.ForOfVar, elemType *Type) ([]forOfVarBinding, bool) {
	switch b := binding.(type) {
	case *ast.ForOfIdentVar:
		if elemType == nil {
			c.errorf(b.Span, "for-of binding requires iterable element")
			return nil, false
		}
		var varType *Type
		if b.Type != nil {
			varType = c.resolveTypeInEnv(b.Type, env)
			if varType == nil {
				return nil, false
			}
			if elemType != nil && !elemType.AssignableTo(varType) {
				c.errorf(b.Span, "for-of element type mismatch")
				return nil, false
			}
		} else {
			varType = elemType
		}
		return []forOfVarBinding{{Name: b.Name, Type: varType}}, true
	case *ast.ForOfArrayDestructureVar:
		if elemType == nil {
			c.errorf(b.Span, "for-of destructuring requires iterable element")
			return nil, false
		}
		var elemTypes []*Type
		switch elemType.Kind {
		case KindArray:
			for range b.Names {
				elemTypes = append(elemTypes, elemType.Elem)
			}
		case KindTuple:
			if len(b.Names) > len(elemType.Tuple) {
				c.errorf(b.Span, "destructuring has more elements than tuple")
				return nil, false
			}
			elemTypes = append(elemTypes, elemType.Tuple[:len(b.Names)]...)
		default:
			c.errorf(b.Span, "for-of destructuring requires array or tuple element")
			return nil, false
		}
		var bindings []forOfVarBinding
		for i, name := range b.Names {
			var elem *Type
			if i < len(elemTypes) {
				elem = elemTypes[i]
			}
			var declType *Type
			if i < len(b.Types) && b.Types[i] != nil {
				declType = c.resolveTypeInEnv(b.Types[i], env)
				if declType == nil {
					return nil, false
				}
				if elem != nil && !elem.AssignableTo(declType) {
					c.errorf(b.Span, "destructuring type mismatch for %s", name)
				}
			} else {
				declType = elem
			}
			bindings = append(bindings, forOfVarBinding{Name: name, Type: declType})
		}
		return bindings, true
	case *ast.ForOfObjectDestructureVar:
		if elemType == nil || elemType.Kind != KindObject {
			c.errorf(b.Span, "for-of object destructuring requires object element")
			return nil, false
		}
		var bindings []forOfVarBinding
		for i, key := range b.Keys {
			propType := elemType.PropType(key)
			if propType == nil {
				c.errorf(b.Span, "property '%s' not found in object", key)
				return nil, false
			}
			var declType *Type
			if i < len(b.Types) && b.Types[i] != nil {
				declType = c.resolveTypeInEnv(b.Types[i], env)
				if declType == nil {
					return nil, false
				}
				if !propType.AssignableTo(declType) {
					c.errorf(b.Span, "destructuring type mismatch for %s", key)
				}
			} else {
				declType = propType
			}
			bindings = append(bindings, forOfVarBinding{Name: key, Type: declType})
		}
		return bindings, true
	default:
		c.errorf(ast.Span{}, "unsupported for-of binding")
		return nil, false
	}
}

func arrayElemType(t *Type) *Type {
	switch t.Kind {
	case KindArray:
		return t.Elem
	default:
		return nil
	}
}

func (c *Checker) checkExpr(env *Env, expr ast.Expr, expected *Type) *Type {
	if expr == nil {
		return Void()
	}
	switch e := expr.(type) {
	case *ast.IntLit:
		typ := LiteralI64(e.Value)
		c.ExprTypes[expr] = typ
		return typ
	case *ast.FloatLit:
		typ := LiteralF64(e.Value)
		c.ExprTypes[expr] = typ
		return typ
	case *ast.BoolLit:
		typ := LiteralBool(e.Value)
		c.ExprTypes[expr] = typ
		return typ
	case *ast.NullLit:
		typ := Null()
		c.ExprTypes[expr] = typ
		return typ
	case *ast.UndefinedLit:
		typ := Undefined()
		c.ExprTypes[expr] = typ
		return typ
	case *ast.StringLit:
		typ := LiteralString(e.Value)
		c.ExprTypes[expr] = typ
		return typ
	case *ast.TemplateLit:
		for _, part := range e.Exprs {
			partType := c.checkExpr(env, part, nil)
			if partType == nil {
				return nil
			}
			if !isTemplateStringConvertible(partType) {
				c.errorf(part.GetSpan(), "template interpolation must be string, i64, number, or boolean")
				return nil
			}
		}
		if len(e.Exprs) == 0 && len(e.Segments) == 1 {
			typ := LiteralString(e.Segments[0])
			c.ExprTypes[expr] = typ
			return typ
		}
		typ := String()
		c.ExprTypes[expr] = typ
		return typ
	case *ast.IdentExpr:
		sym := env.lookup(e.Name)
		if sym == nil {
			c.errorf(e.Span, "undefined: %s", e.Name)
			return nil
		}
		if c.isIntrinsicSymbol(sym) && !c.isIntrinsicValueAllowed(sym) {
			c.errorf(e.Span, "builtin cannot be used as value")
			return nil
		}
		c.IdentSymbols[e] = sym
		c.ExprTypes[expr] = sym.Type
		return sym.Type
	case *ast.ArrayLit:
		return c.checkArrayLit(env, e, expected)
	case *ast.ObjectLit:
		return c.checkObjectLit(env, e, expected)
	case *ast.UnaryExpr:
		inner := c.checkExpr(env, e.Expr, nil)
		if inner == nil {
			return nil
		}
		if inner.Kind != KindI64 && inner.Kind != KindF64 {
			c.errorf(e.Span, "number required")
			return nil
		}
		c.ExprTypes[expr] = inner
		return inner
	case *ast.AsExpr:
		targetType := c.resolveTypeInEnv(e.Type, env)
		if targetType == nil {
			return nil
		}
		if targetType.Kind == KindUnion {
			c.errorf(e.Span, "as target must be non-union")
			return nil
		}
		exprType := c.checkExpr(env, e.Expr, nil)
		if exprType == nil {
			return nil
		}
		if exprType.Kind != KindUnion {
			c.errorf(e.Span, "as requires union type")
			return nil
		}
		if !targetType.AssignableTo(exprType) {
			c.errorf(e.Span, "as target not in union")
			return nil
		}
		c.ExprTypes[expr] = targetType
		return targetType
	case *ast.BinaryExpr:
		left := c.checkExpr(env, e.Left, nil)
		right := c.checkExpr(env, e.Right, nil)
		if left == nil || right == nil {
			return nil
		}
		result := c.checkBinary(e, left, right)
		c.ExprTypes[expr] = result
		return result
	case *ast.IfExpr:
		thenEnv := c.checkIfCond(env, e.Cond)
		thenType := c.checkExpr(thenEnv, e.Then, expected)
		if thenType == nil {
			return nil
		}
		thenValueType := baseType(thenType)
		if thenValueType.Kind == KindVoid {
			thenValueType = Undefined()
		}

		elseValueType := Undefined()
		if e.Else != nil {
			elseType := c.checkExpr(env, e.Else, expected)
			if elseType == nil {
				return nil
			}
			elseValueType = baseType(elseType)
			if elseValueType.Kind == KindVoid {
				elseValueType = Undefined()
			}
		}

		resultType := NewUnion([]*Type{thenValueType, elseValueType})
		if expected != nil && resultType != nil && !typeContainsTypeParam(expected) && !resultType.AssignableTo(expected) {
			c.errorf(e.Span, "if expression type mismatch")
			return nil
		}
		c.ExprTypes[expr] = resultType
		return resultType
	case *ast.SwitchExpr:
		// Check the value being switched on
		valueType := c.checkExpr(env, e.Value, nil)
		if valueType == nil {
			return nil
		}

		// Determine result type from cases
		var resultType *Type
		for _, cas := range e.Cases {
			caseEnv := env
			if asExpr, ok := cas.Pattern.(*ast.AsExpr); ok {
				targetType := c.resolveTypeInEnv(asExpr.Type, env)
				if targetType != nil {
					if targetType.Kind == KindUnion {
						c.errorf(asExpr.Span, "as target must be non-union")
					}
					if valueType.Kind != KindUnion {
						c.errorf(asExpr.Span, "as pattern requires union switch value")
					} else if !targetType.AssignableTo(valueType) {
						c.errorf(asExpr.Span, "as target not in union")
					}
					c.ExprTypes[asExpr] = targetType

					// Create a scope for bindings within this case.
					caseEnv = env.child()

					// Narrow the switch variable itself when switching on an identifier.
					if switchIdent, ok := e.Value.(*ast.IdentExpr); ok {
						if sym := env.lookup(switchIdent.Name); sym != nil {
							storageType := sym.StorageType
							if storageType == nil {
								storageType = sym.Type
							}
							caseEnv.vars[switchIdent.Name] = &Symbol{Name: sym.Name, Kind: sym.Kind, Type: targetType, StorageType: storageType, Decl: sym.Decl}
						}
					}

					// Bind a new name or destructure from the narrowed value.
					switch bind := asExpr.Expr.(type) {
					case *ast.IdentExpr:
						if switchIdent, ok := e.Value.(*ast.IdentExpr); ok && bind.Name == switchIdent.Name {
							// Already narrowed above.
							break
						}
						c.declareVar(caseEnv, bind.Name, targetType, asExpr.Span)
					case *ast.ObjectPatternExpr:
						if targetType.Kind != KindObject {
							c.errorf(asExpr.Span, "object destructuring requires object type")
							break
						}
						for i, key := range bind.Keys {
							propType := targetType.PropType(key)
							if propType == nil {
								c.errorf(asExpr.Span, "property '%s' not found in object", key)
								continue
							}
							declType := propType
							if i < len(bind.Types) && bind.Types[i] != nil {
								declType = c.resolveTypeInEnv(bind.Types[i], env)
								if declType != nil && !propType.AssignableTo(declType) {
									c.errorf(asExpr.Span, "destructuring type mismatch for %s", key)
								}
							}
							c.declareVar(caseEnv, key, declType, asExpr.Span)
						}
					case *ast.ArrayPatternExpr:
						var elemTypes []*Type
						switch targetType.Kind {
						case KindArray:
							for range bind.Names {
								elemTypes = append(elemTypes, targetType.Elem)
							}
						case KindTuple:
							if len(bind.Names) > len(targetType.Tuple) {
								c.errorf(asExpr.Span, "destructuring has more elements than tuple")
								break
							}
							elemTypes = targetType.Tuple[:len(bind.Names)]
						default:
							c.errorf(asExpr.Span, "destructuring requires array or tuple type")
							break
						}

						for i, name := range bind.Names {
							if i >= len(elemTypes) {
								break
							}
							var declType *Type
							if i < len(bind.Types) && bind.Types[i] != nil {
								declType = c.resolveTypeInEnv(bind.Types[i], env)
								if declType != nil && elemTypes[i] != nil && !elemTypes[i].AssignableTo(declType) {
									c.errorf(asExpr.Span, "destructuring type mismatch for %s", name)
								}
							} else {
								declType = elemTypes[i]
							}
							c.declareVar(caseEnv, name, declType, asExpr.Span)
						}
					default:
						c.errorf(asExpr.Span, "as pattern requires identifier or destructuring pattern")
					}
				}
			} else {
				// Check pattern type matches value type
				patternType := c.checkExpr(env, cas.Pattern, valueType)
				if patternType != nil && !patternType.AssignableTo(valueType) {
					c.errorf(cas.Pattern.GetSpan(), "switch case pattern type mismatch")
				}
			}

			// Check body expression
			bodyType := c.checkExpr(caseEnv, cas.Body, expected)
			if bodyType == nil {
				continue
			}
			if block, ok := cas.Body.(*ast.BlockExpr); ok && blockExprReturns(block) {
				continue
			}
			if expected != nil {
				if !bodyType.AssignableTo(expected) {
					c.errorf(cas.Body.GetSpan(), "switch case body type mismatch")
				}
				resultType = expected
			} else if resultType == nil {
				resultType = bodyType
			} else if !typesEqual(baseType(bodyType), baseType(resultType)) {
				c.errorf(cas.Body.GetSpan(), "switch case body type mismatch")
			}
		}
		// Check default case
		if e.Default != nil {
			defaultType := c.checkExpr(env, e.Default, expected)
			if defaultType != nil {
				if block, ok := e.Default.(*ast.BlockExpr); ok && blockExprReturns(block) {
					// Default returns from the function; it does not participate in switch result typing.
				} else if expected != nil {
					if !defaultType.AssignableTo(expected) {
						c.errorf(e.Default.GetSpan(), "switch default type mismatch")
					}
					resultType = expected
				} else if resultType == nil {
					resultType = defaultType
				} else if !typesEqual(baseType(defaultType), baseType(resultType)) {
					c.errorf(e.Default.GetSpan(), "switch default type mismatch")
				}
			}
		}
		if resultType == nil {
			resultType = Void()
		}
		c.ExprTypes[expr] = resultType
		return resultType
	case *ast.CallExpr:
		return c.checkCall(env, e, expected)
	case *ast.MemberExpr:
		objType := c.checkExpr(env, e.Object, nil)
		if objType == nil {
			return nil
		}
		if objType.Kind != KindObject {
			c.errorf(e.Span, "object required")
			return nil
		}
		propType := objType.PropType(e.Property)
		if propType == nil {
			c.errorf(e.Span, "property not found: %s", e.Property)
			return nil
		}
		c.ExprTypes[expr] = propType
		return propType
	case *ast.IndexExpr:
		arrType := c.checkExpr(env, e.Array, nil)
		if arrType == nil {
			return nil
		}
		idxType := c.checkExpr(env, e.Index, I64())
		if idxType == nil || idxType.Kind != KindI64 {
			c.errorf(e.Index.GetSpan(), "index must be i64")
			return nil
		}
		if arrType.Kind == KindArray {
			ret := NewUnion([]*Type{arrType.Elem, resultErrorType()})
			c.ExprTypes[expr] = ret
			return ret
		}
		if arrType.Kind == KindTuple {
			if lit, ok := e.Index.(*ast.IntLit); ok {
				if lit.Value < 0 || int(lit.Value) >= len(arrType.Tuple) {
					c.errorf(e.Span, "tuple index out of range")
					return nil
				}
				t := arrType.Tuple[int(lit.Value)]
				ret := NewUnion([]*Type{t, resultErrorType()})
				c.ExprTypes[expr] = ret
				return ret
			}
			if elem := arrayElemType(arrType); elem != nil {
				ret := NewUnion([]*Type{elem, resultErrorType()})
				c.ExprTypes[expr] = ret
				return ret
			}
			c.errorf(e.Span, "tuple element types differ")
			return nil
		}
		c.errorf(e.Span, "array required")
		return nil
	case *ast.TryExpr:
		resultType := c.checkExpr(env, e.Expr, nil)
		if resultType == nil {
			return nil
		}
		successType, errType := splitResultType(resultType)
		if successType == nil || errType == nil {
			c.errorf(e.Span, "? expects (T | error) expression")
			return nil
		}
		if env.retType == nil || !errType.AssignableTo(env.retType) {
			c.errorf(e.Span, "? requires function return type to include error")
			return nil
		}
		if expected != nil && !successType.AssignableTo(expected) {
			c.errorf(e.Span, "type mismatch")
			return nil
		}
		c.ExprTypes[expr] = successType
		return successType
	case *ast.ArrowFunc:
		rootEnv := env.root()
		moduleEnv := rootEnv.clone()
		sig := c.checkFuncLiteral(moduleEnv, e, expected)
		if sig == nil {
			return nil
		}
		c.ExprTypes[expr] = sig
		return sig
	case *ast.BlockExpr:
		// Block expression executes statements and returns the value of the last expression statement.
		// If the last statement is not an expression statement, the block evaluates to void.
		blockEnv := env.child()
		if len(e.Stmts) == 0 {
			c.ExprTypes[expr] = Void()
			return Void()
		}
		for i, stmt := range e.Stmts {
			if i == len(e.Stmts)-1 {
				// Last statement decides the value.
				if es, ok := stmt.(*ast.ExprStmt); ok {
					valType := c.checkExpr(blockEnv, es.Expr, expected)
					if valType == nil {
						return nil
					}
					c.ExprTypes[expr] = valType
					return valType
				}
				c.checkStmt(blockEnv, stmt, env.retType)
				c.ExprTypes[expr] = Void()
				return Void()
			}
			c.checkStmt(blockEnv, stmt, env.retType)
		}
		c.ExprTypes[expr] = Void()
		return Void()
	case *ast.SQLExpr:
		// Check parameter expressions
		for _, param := range e.Params {
			paramType := c.checkExpr(env, param, nil)
			if paramType == nil {
				continue
			}
			// Parameters must be primitive types (string, i64, number, bool)
			switch paramType.Kind {
			case KindString, KindI64, KindF64, KindBool:
				// OK
			default:
				c.errorf(param.GetSpan(), "SQL parameter must be a primitive type (string, i64, number, or bool)")
			}
		}
		// Validate SQL query against table definitions
		c.validateSQLQuery(e)

		// Determine row type based on SELECT columns
		rowType := c.inferSQLRowType(e)

		// Return type depends on the query kind.
		// SQL 実行時エラーは error 値として返すため、常に (... | error) になる。
		var successType *Type
		switch e.Kind {
		case ast.SQLQueryExecute:
			// execute returns undefined on success
			successType = Undefined()
		case ast.SQLQueryFetchOptional:
			if rowType == nil {
				successType = Null()
			} else {
				successType = NewUnion([]*Type{rowType, Null()})
			}
		case ast.SQLQueryFetchOne:
			// fetch_one returns RowType directly on success
			successType = rowType
		case ast.SQLQueryFetch, ast.SQLQueryFetchAll:
			// fetch and fetch_all return RowType[] directly on success
			successType = NewArray(rowType)
		default:
			// Default: same as fetch_all
			successType = NewArray(rowType)
		}
		resultType := NewUnion([]*Type{successType, resultErrorType()})
		c.ExprTypes[expr] = resultType
		return resultType
	case *ast.JSXElement:
		if isJSXComponentTag(e.Tag) {
			return c.checkJSXComponent(env, e)
		}
		// JSX element returns string
		// Check attribute expressions
		for _, attr := range e.Attributes {
			if attr.Value != nil {
				attrType := c.checkExpr(env, attr.Value, String())
				// Store the attribute value type for code generation
				if attrType != nil {
					c.ExprTypes[attr.Value] = attrType
				}
				if attrType != nil && attrType.Kind != KindString && attrType.Kind != KindI64 && attrType.Kind != KindF64 && attrType.Kind != KindBool {
					c.errorf(attr.Span, "JSX attribute value must be a primitive type")
				}
			}
		}
		// Check children
		for _, child := range e.Children {
			c.checkJSXChild(env, &child)
		}
		c.ExprTypes[expr] = String()
		return String()
	case *ast.JSXFragment:
		// JSX fragment returns string
		for _, child := range e.Children {
			c.checkJSXChild(env, &child)
		}
		c.ExprTypes[expr] = String()
		return String()
	default:
		return nil
	}
}

func (c *Checker) checkJSXComponent(env *Env, e *ast.JSXElement) *Type {
	tag := e.Tag
	sym := env.lookup(tag)
	if sym == nil {
		c.errorf(e.Span, "undefined component: %s", tag)
		c.ExprTypes[e] = String()
		return String()
	}
	if sym.Kind != SymFunc || sym.Type == nil || sym.Type.Kind != KindFunc {
		c.errorf(e.Span, "%s is not a component function", tag)
		c.ExprTypes[e] = String()
		return String()
	}
	if len(sym.Type.Params) == 0 {
		for _, attr := range e.Attributes {
			c.errorf(attr.Span, "component %s does not accept attributes", tag)
		}
		if len(e.Children) > 0 {
			c.errorf(e.Span, "component %s does not accept children", tag)
		}
		c.JSXComponents[e] = &JSXComponentInfo{
			Symbol: sym,
		}
		c.ExprTypes[e] = String()
		return String()
	}
	paramType := sym.Type.Params[0]
	if paramType.Kind != KindObject {
		c.errorf(e.Span, "component %s props must be an object", tag)
		c.ExprTypes[e] = String()
		return String()
	}

	props := []Prop{}
	seen := map[string]bool{}

	for _, attr := range e.Attributes {
		attrType := Bool()
		if attr.Value != nil {
			attrType = c.checkExpr(env, attr.Value, nil)
		}

		targetType := paramType.PropType(attr.Name)
		if targetType == nil {
			if paramType.Index != nil {
				targetType = paramType.Index
			} else {
				c.errorf(attr.Span, "component %s does not accept attribute %q", tag, attr.Name)
				continue
			}
		}

		if attrType != nil && !attrType.AssignableTo(targetType) {
			c.errorf(attr.Span, "attribute %s has incompatible type", attr.Name)
		}

		props = append(props, Prop{Name: attr.Name, Type: targetType})
		seen[attr.Name] = true
	}

	childType := paramType.PropType("children")
	if len(e.Children) > 0 {
		if childType == nil {
			childType = paramType.Index
		}
		if childType == nil {
			c.errorf(e.Span, "component %s does not accept children", tag)
		} else {
			if !String().AssignableTo(childType) {
				c.errorf(e.Span, "children must be a string-compatible type")
			}
			props = append(props, Prop{Name: "children", Type: childType})
			seen["children"] = true
		}
	} else if childType != nil {
		props = append(props, Prop{Name: "children", Type: childType})
		seen["children"] = true
	}

	for _, prop := range paramType.Props {
		if !seen[prop.Name] {
			c.errorf(e.Span, "component %s missing property %q", tag, prop.Name)
		}
	}

	c.JSXComponents[e] = &JSXComponentInfo{
		Symbol:    sym,
		PropsType: NewObject(props),
		ParamType: paramType,
	}
	c.ExprTypes[e] = String()
	return String()
}

func isPrimitiveKind(kind Kind) bool {
	switch kind {
	case KindString, KindI64, KindF64, KindBool:
		return true
	default:
		return false
	}
}

func isJSXComponentTag(tag string) bool {
	for _, r := range tag {
		return unicode.IsUpper(r)
	}
	return false
}

// checkJSXChild checks the type of a JSX child element
func (c *Checker) checkJSXChild(env *Env, child *ast.JSXChild) {
	switch child.Kind {
	case ast.JSXChildText:
		// Text is always valid
	case ast.JSXChildElement:
		// Recursively check nested element
		if child.Element != nil {
			c.checkExpr(env, child.Element, String())
		}
	case ast.JSXChildExpr:
		// Expression must return a primitive type (string, number, bool) or array of strings
		if child.Expr != nil {
			exprType := c.checkExpr(env, child.Expr, nil)
			if exprType == nil {
				return
			}
			// 配列型の場合、要素がstringならOK（map結果のJSX配列を許可）
			if exprType.Kind == KindArray {
				if exprType.Elem != nil && exprType.Elem.Kind == KindString {
					return // string[] is OK for JSX children
				}
			}
			if exprType.Kind != KindString && exprType.Kind != KindI64 && exprType.Kind != KindF64 && exprType.Kind != KindBool {
				c.errorf(child.Span, "JSX expression must return a primitive type (string, number, or bool)")
			}
		}
	}
}

func (c *Checker) checkBinary(e *ast.BinaryExpr, left, right *Type) *Type {
	switch e.Op {
	case "+":
		if left.Kind == KindString && right.Kind == KindString {
			return String()
		}
		if left.Kind == KindI64 && right.Kind == KindI64 {
			return I64()
		}
		if left.Kind == KindF64 && right.Kind == KindF64 {
			return F64()
		}
		c.errorf(e.Span, "type mismatch")
		return nil
	case "-", "*", "/", "%":
		if left.Kind == KindI64 && right.Kind == KindI64 {
			return I64()
		}
		if left.Kind == KindF64 && right.Kind == KindF64 {
			return F64()
		}
		c.errorf(e.Span, "number required")
		return nil
	case "==", "!=":
		if !typesEqual(baseType(left), baseType(right)) {
			c.errorf(e.Span, "type mismatch")
			return nil
		}
		return Bool()
	case "<", "<=", ">", ">=":
		if left.Kind == KindI64 && right.Kind == KindI64 {
			return Bool()
		}
		if left.Kind == KindF64 && right.Kind == KindF64 {
			return Bool()
		}
		c.errorf(e.Span, "number required")
		return nil
	case "&", "|":
		if left.Kind == KindBool && right.Kind == KindBool {
			return Bool()
		}
		c.errorf(e.Span, "boolean required")
		return nil
	default:
		c.errorf(e.Span, "unknown operator")
		return nil
	}
}

func isTemplateStringConvertible(t *Type) bool {
	if t == nil {
		return false
	}
	t = baseType(t)
	switch t.Kind {
	case KindI64, KindF64, KindBool, KindString:
		return true
	case KindUnion:
		if len(t.Union) == 0 {
			return false
		}
		for _, member := range t.Union {
			if !isTemplateStringConvertible(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func collectTypeParamPlaceholders(t *Type, placeholders map[string]*Type) {
	if t == nil {
		return
	}
	switch t.Kind {
	case KindTypeParam:
		if _, ok := placeholders[t.Name]; !ok {
			placeholders[t.Name] = t
		}
	case KindArray:
		collectTypeParamPlaceholders(t.Elem, placeholders)
	case KindFunc:
		for _, param := range t.Params {
			collectTypeParamPlaceholders(param, placeholders)
		}
		collectTypeParamPlaceholders(t.Ret, placeholders)
	case KindTuple:
		for _, elem := range t.Tuple {
			collectTypeParamPlaceholders(elem, placeholders)
		}
	case KindObject:
		for _, prop := range t.Props {
			collectTypeParamPlaceholders(prop.Type, placeholders)
		}
		if t.Index != nil {
			collectTypeParamPlaceholders(t.Index, placeholders)
		}
	case KindUnion:
		for _, member := range t.Union {
			collectTypeParamPlaceholders(member, placeholders)
		}
	}
}

func (c *Checker) checkCallWithSymbol(env *Env, sym *Symbol, call *ast.CallExpr, expected *Type) *Type {
	if sym.Type == nil || sym.Type.Kind != KindFunc {
		c.errorf(call.Span, "%s is not function", sym.Name)
		return nil
	}
	sig := sym.Type
	if c.isIntrinsicSymbol(sym) && sym.Name == "add_route" && len(call.Args) == 4 && len(sig.Params) == 3 {
		if c.checkExpr(env, call.Args[0], sig.Params[0]) == nil {
			return nil
		}
		if c.checkExpr(env, call.Args[1], String()) == nil {
			return nil
		}
		if c.checkExpr(env, call.Args[2], sig.Params[1]) == nil {
			return nil
		}
		if c.checkExpr(env, call.Args[3], sig.Params[2]) == nil {
			return nil
		}
		c.ExprTypes[call] = sig.Ret
		return sig.Ret
	}
	if len(call.Args) != len(sig.Params) {
		c.errorf(call.Span, "argument count mismatch")
		return nil
	}
	bindings := map[*Type]*Type{}
	explicitTypeArgs := len(call.TypeArgs) > 0
	if explicitTypeArgs {
		if len(sig.TypeParams) == 0 {
			c.errorf(call.Span, "type arguments are only supported for generic functions")
			return nil
		}
		if len(call.TypeArgs) != len(sig.TypeParams) {
			c.errorf(call.Span, "type argument count mismatch")
			return nil
		}
		placeholders := map[string]*Type{}
		collectTypeParamPlaceholders(sig, placeholders)
		for i, name := range sig.TypeParams {
			argType := c.resolveTypeInEnv(call.TypeArgs[i], env)
			if argType == nil {
				return nil
			}
			if placeholder := placeholders[name]; placeholder != nil {
				bindings[placeholder] = argType
			}
			if c.isDecodeLikeSymbol(sym) {
				if !isDecodableType(argType, nil) {
					c.errorf(call.Span, "%s target type not supported", sym.Name)
					return nil
				}
			}
		}
	} else if c.isDecodeLikeSymbol(sym) && len(sig.TypeParams) > 0 {
		c.errorf(call.Span, "%s expects 1 type argument", sym.Name)
		return nil
	}

	var argTypes []*Type
	if !explicitTypeArgs {
		argTypes = make([]*Type, len(call.Args))
		for i, arg := range call.Args {
			if _, ok := arg.(*ast.ArrowFunc); ok {
				continue
			}
			expectedParam := c.substituteTypeParams(sig.Params[i], bindings)
			argType := c.checkExpr(env, arg, expectedParam)
			if argType == nil {
				return nil
			}
			argTypes[i] = argType
			if typeContainsTypeParam(expectedParam) {
				if !c.matchType(expectedParam, argType, bindings) {
					c.errorf(call.Span, "argument type mismatch: expect %s, found %s", typeNameForError(expectedParam), typeNameForError(argType))
					return nil
				}
			}
		}
	}

	for i, arg := range call.Args {
		expectedParam := c.substituteTypeParams(sig.Params[i], bindings)
		var argType *Type
		if explicitTypeArgs {
			argType = c.checkExpr(env, arg, expectedParam)
		} else {
			argType = argTypes[i]
			if argType == nil {
				argType = c.checkExpr(env, arg, expectedParam)
			} else {
				argType = c.substituteTypeParams(argType, bindings)
				c.ExprTypes[arg] = argType
			}
		}
		if argType == nil {
			return nil
		}
		if explicitTypeArgs {
			if expectedParam != nil && !argType.AssignableTo(expectedParam) {
				c.errorf(call.Span, "argument type mismatch: expect %s, found %s", typeNameForError(expectedParam), typeNameForError(argType))
				return nil
			}
			continue
		}
		if typeContainsTypeParam(expectedParam) {
			if !c.matchType(expectedParam, argType, bindings) {
				c.errorf(call.Span, "argument type mismatch: expect %s, found %s", typeNameForError(expectedParam), typeNameForError(argType))
				return nil
			}
		} else if !argType.AssignableTo(expectedParam) {
			c.errorf(call.Span, "argument type mismatch: expect %s, found %s", typeNameForError(expectedParam), typeNameForError(argType))
			return nil
		}
	}
	retType := c.substituteTypeParams(sig.Ret, bindings)
	c.ExprTypes[call] = retType
	return retType
}

func (c *Checker) checkCall(env *Env, call *ast.CallExpr, expected *Type) *Type {
	// Handle method-style call: obj.func(args) => func(obj, args)
	if member, ok := call.Callee.(*ast.MemberExpr); ok {
		return c.checkMethodCall(env, call, member, expected)
	}

	ident, ok := call.Callee.(*ast.IdentExpr)
	if !ok {
		c.errorf(call.Span, "call requires identifier")
		return nil
	}
	sym := env.lookup(ident.Name)
	if sym == nil {
		if ident.Name == "error" {
			return c.checkBuiltinErrorCall(env, call)
		}
		c.errorf(call.Span, "undefined: %s", ident.Name)
		return nil
	}
	c.IdentSymbols[ident] = sym
	return c.checkCallWithSymbol(env, sym, call, expected)
}

func (c *Checker) checkBuiltinErrorCall(env *Env, call *ast.CallExpr) *Type {
	if len(call.TypeArgs) > 0 {
		c.errorf(call.Span, "error does not accept type arguments")
		return nil
	}
	if len(call.Args) != 1 {
		c.errorf(call.Span, "argument count mismatch")
		return nil
	}
	argType := c.checkExpr(env, call.Args[0], String())
	if argType == nil {
		return nil
	}
	if !argType.AssignableTo(String()) {
		c.errorf(call.Span, "argument type mismatch: expect %s, found %s", typeNameForError(String()), typeNameForError(argType))
		return nil
	}
	retType := resultErrorType()
	c.ExprTypes[call] = retType
	return retType
}

func normalizeTypeParamBinding(typ *Type) *Type {
	if typ == nil {
		return nil
	}
	if typ.Kind == KindUnion {
		members := make([]*Type, 0, len(typ.Union))
		for _, member := range typ.Union {
			normalized := normalizeTypeParamBinding(member)
			if normalized != nil {
				members = append(members, normalized)
			}
		}
		return NewUnion(members)
	}
	return baseType(typ)
}

func (c *Checker) matchType(expected, actual *Type, bindings map[*Type]*Type) bool {
	if expected == nil || actual == nil {
		return false
	}
	if expected.Kind == KindTypeParam {
		actual = normalizeTypeParamBinding(actual)
		if bound, ok := bindings[expected]; ok {
			if actual.AssignableTo(bound) {
				return true
			}
			if bound.AssignableTo(actual) {
				bindings[expected] = actual
				return true
			}
			bindings[expected] = normalizeTypeParamBinding(NewUnion([]*Type{bound, actual}))
			return true
		}
		bindings[expected] = actual
		return true
	}
	if expected.Kind == KindUnion {
		var typeParams []*Type
		var fixed []*Type
		for _, member := range expected.Union {
			if member.Kind == KindTypeParam {
				typeParams = append(typeParams, member)
			} else {
				fixed = append(fixed, member)
			}
		}
		if len(typeParams) == 0 {
			return actual.AssignableTo(expected)
		}
		if len(typeParams) > 1 {
			return false
		}
		param := typeParams[0]
		actualMembers := []*Type{actual}
		if actual.Kind == KindUnion {
			actualMembers = actual.Union
		}
		unmatched := make([]*Type, 0, len(actualMembers))
		for _, act := range actualMembers {
			if act.Kind == KindTypeParam && act == param {
				// "expected" 由来の自己参照型パラメータは推論の拘束に使わない。
				continue
			}
			matchedFixed := false
			for _, exp := range fixed {
				if act.AssignableTo(exp) {
					matchedFixed = true
					break
				}
			}
			if !matchedFixed {
				unmatched = append(unmatched, act)
			}
		}
		if len(unmatched) == 0 {
			return true
		}
		target := normalizeTypeParamBinding(NewUnion(unmatched))
		if bound, ok := bindings[param]; ok {
			if target.AssignableTo(bound) {
				return true
			}
			if bound.AssignableTo(target) {
				bindings[param] = target
				return true
			}
			bindings[param] = normalizeTypeParamBinding(NewUnion([]*Type{bound, target}))
			return true
		}
		bindings[param] = target
		return true
	}
	if expected.Kind != actual.Kind {
		return actual.AssignableTo(expected)
	}
	switch expected.Kind {
	case KindArray:
		return c.matchType(expected.Elem, actual.Elem, bindings)
	case KindFunc:
		if len(expected.Params) != len(actual.Params) {
			return false
		}
		for i := range expected.Params {
			if !c.matchType(expected.Params[i], actual.Params[i], bindings) {
				return false
			}
		}
		return c.matchType(expected.Ret, actual.Ret, bindings)
	case KindTuple:
		if len(expected.Tuple) != len(actual.Tuple) {
			return false
		}
		for i := range expected.Tuple {
			if !c.matchType(expected.Tuple[i], actual.Tuple[i], bindings) {
				return false
			}
		}
		return true
	case KindObject:
		if len(expected.Props) != len(actual.Props) {
			return false
		}
		if (expected.Index == nil) != (actual.Index == nil) {
			return false
		}
		if expected.Index != nil && !c.matchType(expected.Index, actual.Index, bindings) {
			return false
		}
		for i := range expected.Props {
			if expected.Props[i].Name != actual.Props[i].Name {
				return false
			}
			if !c.matchType(expected.Props[i].Type, actual.Props[i].Type, bindings) {
				return false
			}
		}
		return true
	default:
		return actual.AssignableTo(expected)
	}
}

func (c *Checker) substituteTypeParams(typ *Type, bindings map[*Type]*Type) *Type {
	if typ == nil {
		return nil
	}
	if typ.Kind == KindTypeParam {
		if actual, ok := bindings[typ]; ok {
			return actual
		}
		return typ
	}
	switch typ.Kind {
	case KindArray:
		return NewArray(c.substituteTypeParams(typ.Elem, bindings))
	case KindFunc:
		params := make([]*Type, len(typ.Params))
		for i, param := range typ.Params {
			params[i] = c.substituteTypeParams(param, bindings)
		}
		return NewFunc(params, c.substituteTypeParams(typ.Ret, bindings))
	case KindTuple:
		elems := make([]*Type, len(typ.Tuple))
		for i, elem := range typ.Tuple {
			elems[i] = c.substituteTypeParams(elem, bindings)
		}
		return NewTuple(elems)
	case KindObject:
		var props []Prop
		for _, prop := range typ.Props {
			props = append(props, Prop{Name: prop.Name, Type: c.substituteTypeParams(prop.Type, bindings)})
		}
		if typ.Index != nil {
			return NewObjectWithIndex(props, c.substituteTypeParams(typ.Index, bindings))
		}
		return NewObject(props)
	case KindUnion:
		var members []*Type
		for _, member := range typ.Union {
			members = append(members, c.substituteTypeParams(member, bindings))
		}
		return NewUnion(members)
	default:
		return typ
	}
}

func typeContainsTypeParam(typ *Type) bool {
	if typ == nil {
		return false
	}
	if typ.Kind == KindTypeParam {
		return true
	}
	switch typ.Kind {
	case KindArray:
		return typeContainsTypeParam(typ.Elem)
	case KindFunc:
		for _, param := range typ.Params {
			if typeContainsTypeParam(param) {
				return true
			}
		}
		return typeContainsTypeParam(typ.Ret)
	case KindTuple:
		for _, elem := range typ.Tuple {
			if typeContainsTypeParam(elem) {
				return true
			}
		}
		return false
	case KindObject:
		for _, prop := range typ.Props {
			if typeContainsTypeParam(prop.Type) {
				return true
			}
		}
		if typ.Index != nil {
			return typeContainsTypeParam(typ.Index)
		}
		return false
	case KindUnion:
		for _, member := range typ.Union {
			if typeContainsTypeParam(member) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// checkMethodCall handles method-style calls: obj.func(args) => func(obj, args)
func (c *Checker) checkMethodCall(env *Env, call *ast.CallExpr, member *ast.MemberExpr, expected *Type) *Type {
	funcName := member.Property

	sym := env.lookup(funcName)
	if sym == nil {
		c.errorf(call.Span, "undefined: %s", funcName)
		return nil
	}

	allArgs := append([]ast.Expr{member.Object}, call.Args...)
	syntheticCall := &ast.CallExpr{
		Callee:   &ast.IdentExpr{Name: funcName, Span: member.Span},
		Args:     allArgs,
		TypeArgs: call.TypeArgs,
		Span:     call.Span,
	}
	result := c.checkCallWithSymbol(env, sym, syntheticCall, expected)
	c.ExprTypes[call] = result
	return result
}

func isDecodableType(t *Type, stack map[*Type]bool) bool {
	if t == nil {
		return false
	}
	if t.Literal {
		switch t.Kind {
		case KindI64, KindF64, KindBool, KindString, KindNull:
			return true
		default:
			return false
		}
	}
	if stack == nil {
		stack = map[*Type]bool{}
	}
	if stack[t] {
		return false
	}
	stack[t] = true
	defer delete(stack, t)

	switch t.Kind {
	case KindI64, KindF64, KindBool, KindString, KindNull, KindUndefined, KindJSON:
		return true
	case KindArray:
		return isDecodableType(t.Elem, stack)
	case KindTuple:
		for _, e := range t.Tuple {
			if !isDecodableType(e, stack) {
				return false
			}
		}
		return true
	case KindObject:
		for _, p := range t.Props {
			if !isDecodableType(p.Type, stack) {
				return false
			}
		}
		if t.Index != nil && !isDecodableType(t.Index, stack) {
			return false
		}
		return true
	case KindUnion:
		for _, m := range t.Union {
			if !isDecodableType(m, stack) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (c *Checker) checkArrayLit(env *Env, lit *ast.ArrayLit, expected *Type) *Type {
	hasSpread := false
	for _, entry := range lit.Entries {
		if entry.Kind == ast.ArraySpread {
			hasSpread = true
			break
		}
	}
	if expected != nil {
		switch expected.Kind {
		case KindArray:
			for _, entry := range lit.Entries {
				if entry.Kind == ast.ArrayValue {
					et := c.checkExpr(env, entry.Value, expected.Elem)
					if et != nil && !et.AssignableTo(expected.Elem) {
						c.errorf(entry.Span, "array element type mismatch")
						return nil
					}
				} else {
					spreadType := c.checkExpr(env, entry.Value, nil)
					if spreadType == nil || spreadType.Kind != KindArray {
						c.errorf(entry.Span, "array spread requires array")
						return nil
					}
					if !spreadType.Elem.AssignableTo(expected.Elem) {
						c.errorf(entry.Span, "array element type mismatch")
						return nil
					}
				}
			}
			c.ExprTypes[lit] = expected
			return expected
		case KindTuple:
			if hasSpread {
				c.errorf(lit.Span, "tuple literal cannot use spread")
				return nil
			}
			if len(lit.Entries) != len(expected.Tuple) {
				c.errorf(lit.Span, "tuple length mismatch")
				return nil
			}
			for i, entry := range lit.Entries {
				et := c.checkExpr(env, entry.Value, expected.Tuple[i])
				if et != nil && !et.AssignableTo(expected.Tuple[i]) {
					c.errorf(entry.Span, "tuple element type mismatch")
					return nil
				}
			}
			c.ExprTypes[lit] = expected
			return expected
		}
	}
	if hasSpread {
		var elemBase *Type
		for _, entry := range lit.Entries {
			if entry.Kind == ast.ArrayValue {
				et := c.checkExpr(env, entry.Value, nil)
				if et == nil {
					return nil
				}
				general := baseType(et)
				if general == nil {
					return nil
				}
				if elemBase == nil {
					elemBase = general
				} else if !typesEqual(elemBase, general) {
					c.errorf(entry.Span, "array element type mismatch")
					return nil
				}
			} else {
				spreadType := c.checkExpr(env, entry.Value, nil)
				if spreadType == nil || spreadType.Kind != KindArray {
					c.errorf(entry.Span, "array spread requires array")
					return nil
				}
				if spreadType.Elem == nil {
					c.errorf(entry.Span, "array spread requires element type")
					return nil
				}
				general := baseType(spreadType.Elem)
				if general == nil {
					return nil
				}
				if elemBase == nil {
					elemBase = general
				} else if !typesEqual(elemBase, general) {
					c.errorf(entry.Span, "array element type mismatch")
					return nil
				}
			}
		}
		if elemBase == nil {
			c.errorf(lit.Span, "array literal requires elements")
			return nil
		}
		arr := NewArray(elemBase)
		c.ExprTypes[lit] = arr
		return arr
	}
	var elemTypes []*Type
	for _, entry := range lit.Entries {
		elemTypes = append(elemTypes, c.checkExpr(env, entry.Value, nil))
	}
	if len(elemTypes) == 0 {
		c.errorf(lit.Span, "empty array needs type")
		return nil
	}
	first := elemTypes[0]
	firstBase := baseType(first)
	allSame := true
	for _, t := range elemTypes[1:] {
		if !typesEqual(baseType(t), firstBase) {
			allSame = false
			break
		}
	}
	if allSame {
		arr := NewArray(firstBase)
		c.ExprTypes[lit] = arr
		return arr
	}
	tuple := NewTuple(elemTypes)
	c.ExprTypes[lit] = tuple
	return tuple
}

func (c *Checker) checkObjectLit(env *Env, lit *ast.ObjectLit, expected *Type) *Type {
	props := map[string]*Type{}
	order := []string{}
	for _, entry := range lit.Entries {
		switch entry.Kind {
		case ast.ObjectSpread:
			spreadType := c.checkExpr(env, entry.Value, nil)
			if spreadType == nil || spreadType.Kind != KindObject {
				c.errorf(entry.Span, "spread requires object")
				return nil
			}
			for _, prop := range spreadType.Props {
				if _, ok := props[prop.Name]; !ok {
					order = append(order, prop.Name)
				}
				props[prop.Name] = prop.Type
			}
		case ast.ObjectProp:
			valType := c.checkExpr(env, entry.Value, nil)
			if valType == nil {
				return nil
			}
			if existing, ok := props[entry.Key]; ok {
				if !typesEqual(baseType(existing), baseType(valType)) {
					c.errorf(entry.Span, "property type mismatch")
					return nil
				}
			} else {
				order = append(order, entry.Key)
				props[entry.Key] = valType
			}
		}
	}
	var list []Prop
	for _, name := range order {
		list = append(list, Prop{Name: name, Type: props[name]})
	}
	for i := range list {
		if list[i].Name != "type" {
			list[i].Type = baseType(list[i].Type)
		}
	}
	objType := NewObject(list)
	if expected != nil {
		if !objType.AssignableTo(expected) {
			c.errorf(lit.Span, "object type mismatch")
			return nil
		}
		c.ExprTypes[lit] = expected
		return expected
	}
	c.ExprTypes[lit] = objType
	return objType
}

func (c *Checker) recordType(expr ast.TypeExpr, typ *Type) *Type {
	if expr != nil && typ != nil {
		c.TypeExprTypes[expr] = typ
	}
	return typ
}

func (c *Checker) resolveTypeInEnv(expr ast.TypeExpr, env *Env) *Type {
	if env == nil {
		return nil
	}
	if env.typeParams != nil {
		return c.resolveTypeRec(expr, env.mod, env.typeParams)
	}
	return c.resolveType(expr, env.mod)
}

func (c *Checker) resolveType(expr ast.TypeExpr, mod *ModuleInfo) *Type {
	return c.resolveTypeRec(expr, mod, nil)
}

func (c *Checker) resolveTypeRec(expr ast.TypeExpr, mod *ModuleInfo, typeParams map[string]*Type) *Type {
	if expr == nil {
		return nil
	}
	switch t := expr.(type) {
	case *ast.NamedType:
		if typeParams != nil {
			if paramType, ok := typeParams[t.Name]; ok {
				return c.recordType(expr, paramType)
			}
		}
		switch t.Name {
		case "i64":
			return c.recordType(expr, I64())
		case "i32":
			return c.recordType(expr, I32())
		case "error":
			return c.recordType(expr, resultErrorType())
		case "boolean":
			return c.recordType(expr, Bool())
		case "string":
			return c.recordType(expr, String())
		case "json":
			return c.recordType(expr, JSON())
		case "void":
			return c.recordType(expr, Void())
		case "null":
			return c.recordType(expr, Null())
		case "undefined":
			return c.recordType(expr, Undefined())
		case "number":
			return c.recordType(expr, Number())
		default:
			if aliasType, ok := mod.TypeAliases[t.Name]; ok {
				if len(aliasType.Params) > 0 {
					c.errorf(t.Span, "type alias %s requires %d type arguments", t.Name, len(aliasType.Params))
					return nil
				}
				if aliasType.Template == nil {
					return nil
				}
				return c.recordType(expr, aliasType.Template)
			}
			c.errorf(t.Span, "unknown type %s", t.Name)
			return nil
		}
	case *ast.GenericType:
		switch t.Name {
		case "Array":
			if len(t.Args) != 1 {
				c.errorf(t.Span, "Array expects 1 type argument")
				return nil
			}
			elemType := c.resolveTypeRec(t.Args[0], mod, typeParams)
			if elemType == nil {
				return nil
			}
			return c.recordType(expr, NewArray(elemType))
		case "Map":
			if len(t.Args) != 1 {
				c.errorf(t.Span, "Map expects 1 type argument")
				return nil
			}
			valType := c.resolveTypeRec(t.Args[0], mod, typeParams)
			if valType == nil {
				return nil
			}
			return c.recordType(expr, NewObjectWithIndex(nil, valType))
		default:
			if aliasType, ok := mod.TypeAliases[t.Name]; ok {
				if len(aliasType.Params) == 0 {
					c.errorf(t.Span, "type alias %s does not take type arguments", t.Name)
					return nil
				}
				if len(t.Args) != len(aliasType.Params) {
					c.errorf(t.Span, "type alias %s expects %d type arguments", t.Name, len(aliasType.Params))
					return nil
				}
				argTypes := make([]*Type, len(t.Args))
				for i, arg := range t.Args {
					argType := c.resolveTypeRec(arg, mod, typeParams)
					if argType == nil {
						return nil
					}
					argTypes[i] = argType
				}
				if aliasType.Template == nil {
					return nil
				}
				bindings := map[*Type]*Type{}
				for i, name := range aliasType.Params {
					if placeholder, ok := aliasType.ParamPlaceholders[name]; ok && placeholder != nil {
						bindings[placeholder] = argTypes[i]
					}
				}
				specialized := c.substituteTypeParams(aliasType.Template, bindings)
				return c.recordType(expr, specialized)
			}
			c.errorf(t.Span, "unknown generic type %s", t.Name)
			return nil
		}
	case *ast.LiteralType:
		if t.Value == nil {
			c.errorf(t.Span, "invalid literal type")
			return nil
		}
		switch lit := t.Value.(type) {
		case *ast.IntLit:
			return c.recordType(expr, LiteralI64(lit.Value))
		case *ast.FloatLit:
			return c.recordType(expr, LiteralF64(lit.Value))
		case *ast.BoolLit:
			return c.recordType(expr, LiteralBool(lit.Value))
		case *ast.StringLit:
			return c.recordType(expr, LiteralString(lit.Value))
		default:
			c.errorf(t.Span, "unsupported literal type")
			return nil
		}
	case *ast.ArrayType:
		elem := c.resolveTypeRec(t.Elem, mod, typeParams)
		if elem == nil {
			return nil
		}
		return c.recordType(expr, NewArray(elem))
	case *ast.TupleType:
		var elems []*Type
		for _, e := range t.Elems {
			elemType := c.resolveTypeRec(e, mod, typeParams)
			if elemType == nil {
				return nil
			}
			elems = append(elems, elemType)
		}
		return c.recordType(expr, NewTuple(elems))
	case *ast.UnionType:
		var members []*Type
		for _, member := range t.Types {
			memberType := c.resolveTypeRec(member, mod, typeParams)
			if memberType == nil {
				return nil
			}
			members = append(members, memberType)
		}
		return c.recordType(expr, NewUnion(members))
	case *ast.ObjectType:
		var props []Prop
		for _, p := range t.Props {
			propType := c.resolveTypeRec(p.Type, mod, typeParams)
			if propType == nil {
				return nil
			}
			props = append(props, Prop{Name: p.Key, Type: propType})
		}
		return c.recordType(expr, NewObject(props))
	case *ast.FuncType:
		combined := typeParams
		if len(t.TypeParams) > 0 {
			combined = cloneTypeParams(typeParams)
			for _, name := range t.TypeParams {
				combined[name] = NewTypeParam(name)
			}
		}
		params := make([]*Type, len(t.Params))
		for i, p := range t.Params {
			paramType := c.resolveTypeRec(p.Type, mod, combined)
			if paramType == nil {
				return nil
			}
			params[i] = paramType
		}
		ret := c.resolveTypeRec(t.Ret, mod, combined)
		if ret == nil {
			return nil
		}
		funcType := NewFunc(params, ret)
		if len(t.TypeParams) > 0 {
			funcType.TypeParams = append([]string(nil), t.TypeParams...)
		}
		return c.recordType(expr, funcType)
	default:
		return nil
	}
}

func cloneTypeParams(src map[string]*Type) map[string]*Type {
	if src == nil {
		return map[string]*Type{}
	}
	clone := make(map[string]*Type, len(src))
	for k, v := range src {
		clone[k] = v
	}
	return clone
}

func funcTypeParamMap(t *Type) map[string]*Type {
	if t == nil || len(t.TypeParams) == 0 {
		return nil
	}
	params := map[string]*Type{}
	var visit func(*Type)
	visit = func(cur *Type) {
		if cur == nil {
			return
		}
		switch cur.Kind {
		case KindTypeParam:
			if _, ok := params[cur.Name]; !ok && cur.Name != "" {
				params[cur.Name] = cur
			}
		case KindArray:
			visit(cur.Elem)
		case KindFunc:
			for _, p := range cur.Params {
				visit(p)
			}
			visit(cur.Ret)
		case KindTuple:
			for _, elem := range cur.Tuple {
				visit(elem)
			}
		case KindObject:
			for _, prop := range cur.Props {
				visit(prop.Type)
			}
			if cur.Index != nil {
				visit(cur.Index)
			}
		case KindUnion:
			for _, member := range cur.Union {
				visit(member)
			}
		}
	}
	visit(t)
	return params
}

func typeNameForError(t *Type) string {
	if t == nil {
		return "unknown"
	}
	if isResultErrorType(t) {
		return "error"
	}
	if t.Literal {
		switch t.Kind {
		case KindI64:
			return fmt.Sprintf("%d", t.LiteralValue.(int64))
		case KindF64:
			return fmt.Sprintf("%g", t.LiteralValue.(float64))
		case KindBool:
			if t.LiteralValue.(bool) {
				return "true"
			}
			return "false"
		case KindString:
			return fmt.Sprintf("%q", t.LiteralValue.(string))
		}
	}
	switch t.Kind {
	case KindI64:
		return "i64"
	case KindI32:
		return "i32"
	case KindF64:
		return "number"
	case KindBool:
		return "boolean"
	case KindString:
		return "string"
	case KindJSON:
		return "json"
	case KindVoid:
		return "void"
	case KindNull:
		return "null"
	case KindUndefined:
		return "undefined"
	case KindTypeParam:
		if t.Name != "" {
			return t.Name
		}
		return "typeparam"
	case KindArray:
		if t.Elem == nil {
			return "unknown[]"
		}
		return typeNameForError(t.Elem) + "[]"
	case KindTuple:
		parts := make([]string, len(t.Tuple))
		for i, elem := range t.Tuple {
			parts[i] = typeNameForError(elem)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case KindFunc:
		params := make([]string, len(t.Params))
		for i, p := range t.Params {
			params[i] = typeNameForError(p)
		}
		ret := "unknown"
		if t.Ret != nil {
			ret = typeNameForError(t.Ret)
		}
		return "(" + strings.Join(params, ", ") + ") => " + ret
	case KindObject:
		if len(t.Props) == 0 && t.Index == nil {
			return "{}"
		}
		parts := make([]string, 0, len(t.Props)+1)
		for _, prop := range t.Props {
			parts = append(parts, prop.Name+": "+typeNameForError(prop.Type))
		}
		if t.Index != nil {
			parts = append(parts, "[key: string]: "+typeNameForError(t.Index))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case KindUnion:
		parts := make([]string, len(t.Union))
		for i, member := range t.Union {
			parts[i] = typeNameForError(member)
		}
		return strings.Join(parts, " | ")
	default:
		return "unknown"
	}
}

func (c *Checker) errorf(span ast.Span, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.Errors = append(c.Errors, fmt.Errorf("%d:%d: %s", span.Start.Line, span.Start.Col, msg))
}

// validateSQLQuery validates SQL query against table definitions
func (c *Checker) validateSQLQuery(e *ast.SQLExpr) {
	query := strings.ToUpper(strings.TrimSpace(e.Query))

	// Skip validation for CREATE TABLE statements
	if strings.Contains(query, "CREATE TABLE") {
		return
	}

	// Extract table name and validate
	tableName, columns := c.parseSQLQueryInfo(e.Query)
	if tableName == "" {
		return // Can't determine table name, skip validation
	}

	// Validate column references
	for _, col := range columns {
		if col.Column == "*" {
			continue // SELECT * is always valid
		}
		actualTable := col.Table
		if actualTable == "" {
			actualTable = tableName
		}
		tableInfo, exists := c.Tables[actualTable]
		if !exists {
			// If no table definitions exist at all, skip validation (for backward compatibility)
			if len(c.Tables) == 0 {
				continue
			}
			c.errorf(e.Span, "table '%s' is not defined", actualTable)
			continue
		}
		if _, exists := tableInfo.Columns[col.Column]; !exists {
			c.errorf(e.Span, "column '%s' does not exist in table '%s'", col.Column, actualTable)
		}
	}
}

// parseSQLQueryInfo extracts table name and referenced columns from SQL query
func (c *Checker) parseSQLQueryInfo(query string) (tableName string, columns []sqlColumnRef) {
	upper := strings.ToUpper(query)
	words := strings.Fields(query)
	upperWords := strings.Fields(upper)

	_, tableAliases := parseSQLTableReferences(words, upperWords)

	// Handle SELECT
	if strings.HasPrefix(upper, "SELECT") {
		// Find FROM clause
		fromIdx := -1
		for i, w := range upperWords {
			if w == "FROM" {
				fromIdx = i
				break
			}
		}
		if fromIdx > 0 && fromIdx+1 < len(words) {
			if tableName == "" {
				tableName = strings.ToLower(words[fromIdx+1])
			}
			// Extract column names (between SELECT and FROM)
			i := 1
			for i < fromIdx {
				col := strings.Trim(words[i], ",")
				upperCol := strings.ToUpper(col)

				// Skip AS keyword and the alias that follows
				if upperCol == "AS" {
					i += 2 // skip AS and alias
					continue
				}

				// Skip SQL functions like COUNT(*), SUM(), etc.
				if strings.Contains(col, "(") {
					// Skip until we find a word not part of the function
					for i < fromIdx && !strings.HasSuffix(words[i], ")") {
						i++
					}
					i++
					continue
				}

				if col != "" {
					refTable := ""
					// Handle table.column format
					if dotIdx := strings.Index(col, "."); dotIdx >= 0 {
						prefix := strings.ToLower(strings.Trim(col[:dotIdx], ","))
						col = col[dotIdx+1:]
						if actual, ok := tableAliases[prefix]; ok {
							refTable = actual
						} else {
							refTable = prefix
						}
					}
					columns = append(columns, sqlColumnRef{Table: refTable, Column: strings.ToLower(col)})
				}
				i++
			}
		}
		return
	}

	// Handle INSERT
	if strings.HasPrefix(upper, "INSERT") {
		// INSERT INTO table (columns) VALUES ...
		intoIdx := -1
		for i, w := range upperWords {
			if w == "INTO" {
				intoIdx = i
				break
			}
		}
		if intoIdx >= 0 && intoIdx+1 < len(words) {
			tableName = strings.ToLower(words[intoIdx+1])
			// Extract columns between ( and )
			parenStart := strings.Index(query, "(")
			parenEnd := strings.Index(query, ")")
			if parenStart >= 0 && parenEnd > parenStart {
				colPart := query[parenStart+1 : parenEnd]
				for _, col := range strings.Split(colPart, ",") {
					col = strings.TrimSpace(col)
					if col != "" {
						columns = append(columns, sqlColumnRef{Column: strings.ToLower(col)})
					}
				}
			}
		}
		return
	}

	// Handle UPDATE
	if strings.HasPrefix(upper, "UPDATE") {
		if len(words) >= 2 {
			tableName = strings.ToLower(words[1])
			// Extract columns from SET clause
			setIdx := -1
			for i, w := range upperWords {
				if w == "SET" {
					setIdx = i
					break
				}
			}
			if setIdx >= 0 {
				// Find WHERE clause or end
				whereIdx := len(words)
				for i, w := range upperWords {
					if w == "WHERE" {
						whereIdx = i
						break
					}
				}
				// Extract column names from SET ... WHERE
				setPart := strings.Join(words[setIdx+1:whereIdx], " ")
				for _, assign := range strings.Split(setPart, ",") {
					assign = strings.TrimSpace(assign)
					if eqIdx := strings.Index(assign, "="); eqIdx >= 0 {
						col := strings.TrimSpace(assign[:eqIdx])
						columns = append(columns, sqlColumnRef{Column: strings.ToLower(col)})
					}
				}
			}
			// Also check WHERE clause for column references
			whereIdx := -1
			for i, w := range upperWords {
				if w == "WHERE" {
					whereIdx = i
					break
				}
			}
			if whereIdx >= 0 && whereIdx+1 < len(words) {
				// Simple: extract first identifier after WHERE
				col := words[whereIdx+1]
				columns = append(columns, sqlColumnRef{Column: strings.ToLower(col)})
			}
		}
		return
	}

	// Handle DELETE
	if strings.HasPrefix(upper, "DELETE") {
		fromIdx := -1
		for i, w := range upperWords {
			if w == "FROM" {
				fromIdx = i
				break
			}
		}
		if fromIdx >= 0 && fromIdx+1 < len(words) {
			tableName = strings.ToLower(words[fromIdx+1])
		}
		return
	}

	return "", nil
}

func parseSQLTableReferences(words, upperWords []string) (string, map[string]string) {
	aliasMap := map[string]string{}
	defaultTable := ""
	i := 0
	for i < len(words) {
		upper := upperWords[i]
		if upper == "FROM" || upper == "JOIN" {
			i++
			if i >= len(words) {
				break
			}
			tableToken := strings.Trim(words[i], ",")
			if tableToken == "" {
				continue
			}
			tableName := strings.ToLower(tableToken)
			aliasMap[tableName] = tableName
			if defaultTable == "" && upper == "FROM" {
				defaultTable = tableName
			}
			nextIdx := i + 1
			if nextIdx < len(words) {
				nextUpper := upperWords[nextIdx]
				if nextUpper == "AS" && nextIdx+1 < len(words) {
					alias := strings.ToLower(strings.Trim(words[nextIdx+1], ","))
					if alias != "" {
						aliasMap[alias] = tableName
					}
					i = nextIdx + 1
				} else if shouldTreatAsAlias(nextUpper) {
					alias := strings.ToLower(strings.Trim(words[nextIdx], ","))
					if alias != "" {
						aliasMap[alias] = tableName
					}
					i = nextIdx
				} else {
					i = nextIdx - 1
				}
			}
			i++
			continue
		}
		i++
	}
	return defaultTable, aliasMap
}

func shouldTreatAsAlias(word string) bool {
	switch word {
	case "ON", "JOIN", "WHERE", "GROUP", "ORDER", "LIMIT", "INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS", "NATURAL", ",":
		return false
	default:
		return word != ""
	}
}

// inferSQLRowType determines the row type for a SQL expression based on SELECT columns
func (c *Checker) inferSQLRowType(e *ast.SQLExpr) *Type {
	query := strings.TrimSpace(e.Query)
	upper := strings.ToUpper(query)

	// Only SELECT queries return rows with column data
	if !strings.HasPrefix(upper, "SELECT") {
		// For non-SELECT queries, return empty object type for rows
		return NewObject([]Prop{})
	}

	// Extract column names from SELECT query
	columns := c.extractSelectColumns(query)
	if len(columns) == 0 {
		// Fallback to empty object if we can't parse columns
		return NewObject([]Prop{})
	}

	// Build object type with column names as keys, all values are strings
	props := make([]Prop, len(columns))
	for i, col := range columns {
		props[i] = Prop{Name: col, Type: String()}
	}

	return NewObject(props)
}

// extractSelectColumns extracts column names from a SELECT query
func (c *Checker) extractSelectColumns(query string) []string {
	upper := strings.ToUpper(query)
	words := strings.Fields(query)
	upperWords := strings.Fields(upper)

	// Find FROM clause (may not exist for queries like "SELECT last_insert_rowid()")
	fromIdx := -1
	for i, w := range upperWords {
		if w == "FROM" {
			fromIdx = i
			break
		}
	}

	// Determine the end of the SELECT clause
	selectEndIdx := fromIdx
	if selectEndIdx <= 0 {
		// No FROM clause - SELECT clause extends to the end
		selectEndIdx = len(words)
	}

	if selectEndIdx <= 1 {
		return nil
	}

	// Extract column part (between SELECT and FROM, or to end if no FROM)
	selectPart := strings.Join(words[1:selectEndIdx], " ")

	// Handle SELECT * case - need to expand from table definition
	if strings.TrimSpace(selectPart) == "*" {
		// Try to get table name and expand columns
		tableName := ""
		if fromIdx >= 0 && fromIdx+1 < len(words) {
			tableName = strings.ToLower(words[fromIdx+1])
		}
		if tableInfo, exists := c.Tables[tableName]; exists {
			var cols []string
			for colName := range tableInfo.Columns {
				cols = append(cols, colName)
			}
			// Sort for consistent ordering
			sort.Strings(cols)
			return cols
		}
		return nil
	}

	// Parse comma-separated column list
	var columns []string
	for _, part := range strings.Split(selectPart, ",") {
		col := strings.TrimSpace(part)
		if col == "" {
			continue
		}

		// Handle aliases: "column AS alias" or "column alias"
		upperCol := strings.ToUpper(col)
		if asIdx := strings.Index(upperCol, " AS "); asIdx >= 0 {
			col = strings.TrimSpace(col[asIdx+4:])
		} else if spaceIdx := strings.LastIndex(col, " "); spaceIdx >= 0 {
			// Check if it's not a function like "last_insert_rowid()"
			if !strings.Contains(col, "(") {
				col = strings.TrimSpace(col[spaceIdx+1:])
			}
		}

		// Handle table.column format
		if dotIdx := strings.Index(col, "."); dotIdx >= 0 {
			col = col[dotIdx+1:]
		}

		// Handle function calls like "last_insert_rowid()" - use the function name
		if parenIdx := strings.Index(col, "("); parenIdx >= 0 {
			col = col[:parenIdx]
		}

		columns = append(columns, strings.ToLower(col))
	}

	return columns
}

var intrinsicFuncNames = map[string]bool{
	"log":               true,
	"stringify":         true,
	"toJSON":            true,
	"parse":             true,
	"decode":            true,
	"to_string":         true,
	"range":             true,
	"length":            true,
	"map":               true,
	"filter":            true,
	"reduce":            true,
	"db_open":           true,
	"get_args":          true,
	"get_env":           true,
	"gc":                true,
	"run_formatter":     true,
	"run_sandbox":       true,
	"read_text":         true,
	"write_text":        true,
	"append_text":       true,
	"read_dir":          true,
	"exists":            true,
	"sqlQuery":          true,
	"create_server":     true,
	"listen":            true,
	"add_route":         true,
	"response_text":     true,
	"response_html":     true,
	"response_json":     true,
	"response_redirect": true,
	"get_path":          true,
	"get_method":        true,
}

var intrinsicValueDenied = map[string]bool{
	"add_route": true,
	"parse":     true,
	"decode":    true,
	"range":     true,
	"sqlQuery":  true,
}

func isBuiltinModulePath(path string) bool {
	if path == "" {
		return false
	}
	if strings.Contains(path, "/") || strings.Contains(path, "\\") {
		return false
	}
	if filepath.Ext(path) != "" {
		return false
	}
	return true
}

func (c *Checker) isIntrinsicSymbol(sym *Symbol) bool {
	if sym == nil {
		return false
	}
	mod := c.symbolModule[sym]
	if mod == nil {
		return false
	}
	if !isBuiltinModulePath(mod.AST.Path) {
		return false
	}
	return intrinsicFuncNames[sym.Name]
}

func (c *Checker) isIntrinsicValueAllowed(sym *Symbol) bool {
	if sym == nil || !c.isIntrinsicSymbol(sym) {
		return false
	}
	return !intrinsicValueDenied[sym.Name]
}

func (c *Checker) isDecodeLikeSymbol(sym *Symbol) bool {
	if sym == nil {
		return false
	}
	if sym.Name != "decode" && sym.Name != "parse" {
		return false
	}
	mod := c.symbolModule[sym]
	if mod == nil {
		return false
	}
	return mod.AST.Path == "json"
}

func resultErrorType() *Type {
	return NewObject([]Prop{
		{Name: "message", Type: String()},
		{Name: "stacktrace", Type: NewArray(String())},
		{Name: "type", Type: LiteralString("error")},
	})
}

func isResultErrorType(t *Type) bool {
	if t == nil || t.Kind != KindObject {
		return false
	}
	typ := t.PropType("type")
	msg := t.PropType("message")
	trace := t.PropType("stacktrace")
	if typ == nil || msg == nil || trace == nil {
		return false
	}
	if !typ.Equals(LiteralString("error")) {
		return false
	}
	if !msg.AssignableTo(String()) {
		return false
	}
	return trace.AssignableTo(NewArray(String()))
}

func splitResultType(t *Type) (success *Type, err *Type) {
	if t == nil || t.Kind != KindUnion {
		return nil, nil
	}
	var successMembers []*Type
	var errMembers []*Type
	for _, member := range t.Union {
		if isResultErrorType(member) {
			errMembers = append(errMembers, member)
		} else {
			successMembers = append(successMembers, member)
		}
	}
	if len(errMembers) == 0 || len(successMembers) == 0 {
		return nil, nil
	}
	return NewUnion(successMembers), NewUnion(errMembers)
}

type Env struct {
	checker *Checker
	mod     *ModuleInfo
	parent  *Env
	vars    map[string]*Symbol
	retType *Type
	// typeParams holds function type parameter bindings in scope.
	typeParams map[string]*Type
}

func (e *Env) child() *Env {
	return &Env{
		checker:    e.checker,
		mod:        e.mod,
		parent:     e,
		vars:       map[string]*Symbol{},
		retType:    e.retType,
		typeParams: e.typeParams,
	}
}

func (e *Env) root() *Env {
	cur := e
	for cur.parent != nil {
		cur = cur.parent
	}
	return cur
}

func (e *Env) clone() *Env {
	vars := map[string]*Symbol{}
	for k, v := range e.vars {
		vars[k] = v
	}
	return &Env{
		checker:    e.checker,
		mod:        e.mod,
		vars:       vars,
		retType:    e.retType,
		typeParams: e.typeParams,
	}
}

func (e *Env) lookup(name string) *Symbol {
	if sym, ok := e.vars[name]; ok {
		return sym
	}
	if e.parent != nil {
		return e.parent.lookup(name)
	}
	return nil
}
