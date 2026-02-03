package types

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"negitoro/internal/ast"
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
	TypeAliases map[string]*Type
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
}

func NewChecker() *Checker {
	return &Checker{
		Modules:       map[string]*ModuleInfo{},
		ExprTypes:     map[ast.Expr]*Type{},
		IdentSymbols:  map[*ast.IdentExpr]*Symbol{},
		TypeExprTypes: map[ast.TypeExpr]*Type{},
		Tables:        map[string]*TableInfo{},
		JSXComponents: map[*ast.JSXElement]*JSXComponentInfo{},
	}
}

func (c *Checker) AddModule(mod *ast.Module) {
	c.Modules[mod.Path] = &ModuleInfo{AST: mod, Exports: map[string]*Symbol{}, Top: map[string]*Symbol{}, TypeAliases: map[string]*Type{}}
}

func (c *Checker) Check() bool {
	// First: process imports (including type aliases from prelude)
	for _, mod := range c.Modules {
		c.processImports(mod)
	}
	// Second: collect top-level declarations
	for _, mod := range c.Modules {
		c.collectTop(mod)
	}
	// Third: type check the module
	for _, mod := range c.Modules {
		c.checkModule(mod)
	}
	return len(c.Errors) == 0
}

// processImports handles import statements, including type alias imports from prelude
func (c *Checker) processImports(mod *ModuleInfo) {
	for _, imp := range mod.AST.Imports {
		if imp.From == "prelude" {
			for _, item := range imp.Items {
				if item.IsType {
					// Check if it's a prelude type
					if preludeType := getPreludeType(item.Name); preludeType != nil {
						mod.TypeAliases[item.Name] = preludeType
					}
				}
			}
		}
		if imp.From == "http" {
			for _, item := range imp.Items {
				if item.IsType {
					if httpType := getHTTPType(item.Name); httpType != nil {
						mod.TypeAliases[item.Name] = httpType
					}
				}
			}
		}
		if imp.From == "sqlite" {
			for _, item := range imp.Items {
				if item.IsType {
					if sqliteType := getSQLiteType(item.Name); sqliteType != nil {
						mod.TypeAliases[item.Name] = sqliteType
					}
				}
			}
		}
	}
}

func (c *Checker) collectTop(mod *ModuleInfo) {
	// First pass: collect type aliases
	for _, decl := range mod.AST.Decls {
		if d, ok := decl.(*ast.TypeAliasDecl); ok {
			resolvedType := c.resolveType(d.Type, mod)
			mod.TypeAliases[d.Name] = resolvedType
			if d.Export {
				sym := &Symbol{Name: d.Name, Kind: SymType, Type: resolvedType, Decl: d}
				mod.Exports[d.Name] = sym
			}
		}
	}

	// Second pass: collect other declarations
	for _, decl := range mod.AST.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			declType := c.resolveType(d.Type, mod)
			sym := &Symbol{Name: d.Name, Kind: SymVar, Type: declType, StorageType: declType, Decl: d}
			if declType != nil && declType.Kind == KindFunc {
				sym.Kind = SymFunc
			}
			mod.Top[d.Name] = sym
			if d.Export {
				mod.Exports[d.Name] = sym
			}
		case *ast.FuncDecl:
			sig := c.funcTypeFromDecl(d, mod)
			sym := &Symbol{Name: d.Name, Kind: SymFunc, Type: sig, Decl: d}
			mod.Top[d.Name] = sym
			if d.Export {
				mod.Exports[d.Name] = sym
			}
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
			mod.TypeAliases[d.Name] = rowType
		}
	}
}

func (c *Checker) checkModule(mod *ModuleInfo) {
	env := &Env{checker: c, mod: mod, vars: map[string]*Symbol{}}
	for name, sym := range mod.Top {
		env.vars[name] = sym
	}
	for _, imp := range mod.AST.Imports {
		if imp.From == "prelude" {
			for _, item := range imp.Items {
				if item.IsType {
					// Type imports are already handled in processImports
					if preludeType := getPreludeType(item.Name); preludeType == nil {
						c.errorf(imp.Span, "%s is not a type in prelude", item.Name)
					}
					continue
				}
				if !isPreludeName(item.Name) {
					c.errorf(imp.Span, "%s is not in prelude", item.Name)
					continue
				}
				env.vars[item.Name] = &Symbol{Name: item.Name, Kind: SymBuiltin}
			}
			continue
		}
		if imp.From == "http" {
			for _, item := range imp.Items {
				if item.IsType {
					if httpType := getHTTPType(item.Name); httpType == nil {
						c.errorf(imp.Span, "%s is not a type in http", item.Name)
					}
					continue
				}
				if !isHTTPName(item.Name) {
					c.errorf(imp.Span, "%s is not in http", item.Name)
					continue
				}
				env.vars[item.Name] = &Symbol{Name: item.Name, Kind: SymBuiltin}
			}
			continue
		}
		if imp.From == "sqlite" {
			for _, item := range imp.Items {
				if item.IsType {
					if sqliteType := getSQLiteType(item.Name); sqliteType == nil {
						c.errorf(imp.Span, "%s is not a type in sqlite", item.Name)
					}
					continue
				}
				if !isSQLiteName(item.Name) {
					c.errorf(imp.Span, "%s is not in sqlite", item.Name)
					continue
				}
				env.vars[item.Name] = &Symbol{Name: item.Name, Kind: SymBuiltin}
			}
			continue
		}
		dep, ok := c.Modules[imp.From]
		if !ok {
			c.errorf(imp.Span, "%s not found", imp.From)
			continue
		}
		for _, item := range imp.Items {
			exp := dep.Exports[item.Name]
			if exp == nil {
				c.errorf(imp.Span, "%s is not exported from %s", item.Name, imp.From)
				continue
			}
			// Handle type alias imports
			if item.IsType {
				if exp.Kind != SymType {
					c.errorf(imp.Span, "%s is not a type", item.Name)
					continue
				}
				mod.TypeAliases[item.Name] = exp.Type
				continue
			}
			if exp.Kind == SymType {
				c.errorf(imp.Span, "%s is a type, use 'type %s' to import", item.Name, item.Name)
				continue
			}
			env.vars[item.Name] = exp
		}
	}

	for _, decl := range mod.AST.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			c.checkConstDecl(env, d)
		case *ast.FuncDecl:
			c.checkFuncDecl(env, d)
		}
	}
}

func (c *Checker) checkConstDecl(env *Env, d *ast.ConstDecl) {
	declType := c.resolveType(d.Type, env.mod)
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
	sig := c.funcTypeFromDecl(d, env.mod)
	if sig == nil {
		return
	}
	c.checkFuncBody(env, d.Params, d.Body, sig)
}

func (c *Checker) checkFuncLiteral(env *Env, fn *ast.ArrowFunc, expected *Type) *Type {
	if expected != nil && expected.Kind != KindFunc {
		c.errorf(fn.Span, "function type required")
		return nil
	}
	var expectedParams []*Type
	var expectedRet *Type
	if expected != nil && expected.Kind == KindFunc {
		if len(expected.Params) != len(fn.Params) {
			c.errorf(fn.Span, "param count mismatch")
			return nil
		}
		expectedParams = expected.Params
		expectedRet = expected.Ret
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
		params[i] = c.resolveType(p.Type, env.mod)
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
		ret := c.resolveType(fn.Ret, env.mod)
		if ret == nil {
			return nil
		}
		if expectedRet != nil && expectedRet.Kind != KindTypeParam && !ret.AssignableTo(expectedRet) {
			c.errorf(fn.Span, "return type mismatch")
			return nil
		}
		sig := NewFunc(params, ret)
		c.checkFuncBody(env, fn.Params, body, sig)
		return sig
	}

	if expectedRet != nil {
		sig := NewFunc(params, expectedRet)
		c.checkFuncBody(env, fn.Params, body, sig)
		return sig
	}

	ret := c.checkFuncBodyInfer(env, fn.Params, body, params, nil)
	if ret == nil {
		return nil
	}
	return NewFunc(params, ret)
}

func (c *Checker) funcTypeFromDecl(d *ast.FuncDecl, mod *ModuleInfo) *Type {
	params := make([]*Type, len(d.Params))
	for i, p := range d.Params {
		if p.Type == nil {
			c.errorf(p.Span, "param type required")
			return nil
		}
		params[i] = c.resolveType(p.Type, mod)
		if params[i] == nil {
			return nil
		}
	}
	ret := c.resolveType(d.Ret, mod)
	if ret == nil {
		return nil
	}
	return NewFunc(params, ret)
}

func (c *Checker) checkFuncBody(env *Env, params []ast.Param, body *ast.BlockStmt, sig *Type) {
	fnEnv := env.child()
	for i, p := range params {
		fnEnv.vars[p.Name] = &Symbol{Name: p.Name, Kind: SymVar, Type: sig.Params[i], StorageType: sig.Params[i]}
	}
	c.checkBlock(fnEnv, body, sig.Ret)
	if sig.Ret.Kind != KindVoid && !returns(body) {
		c.errorf(body.Span, "return required")
	}
}

type returnInfo struct {
	expected *Type
	inferred *Type
}

func (c *Checker) checkFuncBodyInfer(env *Env, params []ast.Param, body *ast.BlockStmt, paramTypes []*Type, expectedRet *Type) *Type {
	fnEnv := env.child()
	for i, p := range params {
		fnEnv.vars[p.Name] = &Symbol{Name: p.Name, Kind: SymVar, Type: paramTypes[i], StorageType: paramTypes[i]}
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
	if ret.Kind != KindVoid && !returns(body) {
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
			declType = c.resolveType(s.Type, env.mod)
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
		env.vars[s.Name] = &Symbol{Name: s.Name, Kind: SymVar, Type: declType, StorageType: declType}
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
				declType = c.resolveType(s.Types[i], env.mod)
				if declType != nil && elemTypes[i] != nil && !elemTypes[i].AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", name)
				}
			} else {
				declType = elemTypes[i]
			}
			env.vars[name] = &Symbol{Name: name, Kind: SymVar, Type: declType, StorageType: declType}
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
				declType = c.resolveType(s.Types[i], env.mod)
				if declType != nil && propType != nil && !propType.AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", key)
				}
			} else {
				declType = propType
			}
			env.vars[key] = &Symbol{Name: key, Kind: SymVar, Type: declType, StorageType: declType}
		}
	case *ast.ExprStmt:
		c.checkExpr(env, s.Expr, nil)
	case *ast.ReturnStmt:
		c.checkReturnInfer(env, s, info)
	case *ast.IfStmt:
		condType := c.checkExpr(env, s.Cond, Bool())
		if condType != nil && condType.Kind != KindBool {
			c.errorf(s.Cond.GetSpan(), "boolean required")
		}
		c.checkBlockInfer(env, s.Then, info)
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
			loopEnv.vars[lv.Name] = &Symbol{Name: lv.Name, Kind: SymVar, Type: lv.Type, StorageType: lv.Type}
		}
		c.checkBlockInfer(loopEnv, s.Body, info)
	case *ast.BlockStmt:
		c.checkBlockInfer(env, s, info)
	}
}

func (c *Checker) checkReturnInfer(env *Env, s *ast.ReturnStmt, info *returnInfo) {
	if s.Value == nil {
		if info.expected != nil && info.expected.Kind != KindVoid {
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
	if !info.inferred.Equals(valType) {
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
			declType = c.resolveType(s.Type, env.mod)
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
		env.vars[s.Name] = &Symbol{Name: s.Name, Kind: SymVar, Type: declType, StorageType: declType}
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
				declType = c.resolveType(s.Types[i], env.mod)
				if declType != nil && elemTypes[i] != nil && !elemTypes[i].AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", name)
				}
			} else {
				// Type inference from array/tuple element
				declType = elemTypes[i]
			}
			env.vars[name] = &Symbol{Name: name, Kind: SymVar, Type: declType, StorageType: declType}
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
				declType = c.resolveType(s.Types[i], env.mod)
				if declType != nil && propType != nil && !propType.AssignableTo(declType) {
					c.errorf(s.Span, "destructuring type mismatch for %s", key)
				}
			} else {
				// Type inference from object property
				declType = propType
			}
			env.vars[key] = &Symbol{Name: key, Kind: SymVar, Type: declType, StorageType: declType}
		}
	case *ast.ExprStmt:
		c.checkExpr(env, s.Expr, nil)
	case *ast.ReturnStmt:
		if s.Value == nil {
			if retType.Kind != KindVoid {
				c.errorf(s.Span, "return required")
			}
			return
		}
		valType := c.checkExpr(env, s.Value, retType)
		if valType != nil && !valType.AssignableTo(retType) {
			c.errorf(s.Span, "return type mismatch")
		}
	case *ast.IfStmt:
		condType := c.checkExpr(env, s.Cond, Bool())
		if condType != nil && condType.Kind != KindBool {
			c.errorf(s.Cond.GetSpan(), "boolean required")
		}
		c.checkBlock(env, s.Then, retType)
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
			loopEnv.vars[lv.Name] = &Symbol{Name: lv.Name, Kind: SymVar, Type: lv.Type, StorageType: lv.Type}
		}
		c.checkBlock(loopEnv, s.Body, retType)
	case *ast.BlockStmt:
		c.checkBlock(env, s, retType)
	}
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
			varType = c.resolveType(b.Type, env.mod)
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
				declType = c.resolveType(b.Types[i], env.mod)
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
				declType = c.resolveType(b.Types[i], env.mod)
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
		c.ExprTypes[expr] = I64()
		return I64()
	case *ast.FloatLit:
		c.ExprTypes[expr] = F64()
		return F64()
	case *ast.BoolLit:
		c.ExprTypes[expr] = Bool()
		return Bool()
	case *ast.StringLit:
		c.ExprTypes[expr] = String()
		return String()
	case *ast.IdentExpr:
		sym := env.lookup(e.Name)
		if sym == nil {
			c.errorf(e.Span, "undefined: %s", e.Name)
			return nil
		}
		if sym.Kind == SymBuiltin {
			if expected == nil || expected.Kind != KindFunc {
				c.errorf(e.Span, "builtin cannot be used as value")
				return nil
			}
			c.ExprTypes[expr] = expected
			return expected
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
		targetType := c.resolveType(e.Type, env.mod)
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
	case *ast.TernaryExpr:
		condType := c.checkExpr(env, e.Cond, Bool())
		if condType == nil || condType.Kind != KindBool {
			c.errorf(e.Cond.GetSpan(), "ternary condition must be boolean")
			return nil
		}
		thenType := c.checkExpr(env, e.Then, expected)
		elseType := c.checkExpr(env, e.Else, expected)
		if thenType == nil || elseType == nil {
			return nil
		}
		if thenType.Equals(elseType) {
			c.ExprTypes[expr] = thenType
			return thenType
		}
		if expected != nil && thenType.AssignableTo(expected) && elseType.AssignableTo(expected) {
			c.ExprTypes[expr] = expected
			return expected
		}
		c.errorf(e.Span, "ternary branches must have same type")
		return nil
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
				ident, ok := asExpr.Expr.(*ast.IdentExpr)
				if !ok {
					c.errorf(asExpr.Span, "as pattern requires identifier")
				} else {
					if switchIdent, ok := e.Value.(*ast.IdentExpr); ok {
						if ident.Name != switchIdent.Name {
							c.errorf(asExpr.Span, "as pattern must match switch value")
						}
					} else {
						c.errorf(asExpr.Span, "as pattern requires identifier switch value")
					}
					targetType := c.resolveType(asExpr.Type, env.mod)
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
						if sym := env.lookup(ident.Name); sym != nil {
							caseEnv = env.child()
							storageType := sym.StorageType
							if storageType == nil {
								storageType = sym.Type
							}
							caseEnv.vars[ident.Name] = &Symbol{Name: sym.Name, Kind: sym.Kind, Type: targetType, StorageType: storageType, Decl: sym.Decl}
						}
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
			if expected != nil {
				if !bodyType.AssignableTo(expected) {
					c.errorf(cas.Body.GetSpan(), "switch case body type mismatch")
				}
				resultType = expected
			} else if resultType == nil {
				resultType = bodyType
			} else if !bodyType.Equals(resultType) {
				c.errorf(cas.Body.GetSpan(), "switch case body type mismatch")
			}
		}
		// Check default case
		if e.Default != nil {
			defaultType := c.checkExpr(env, e.Default, expected)
			if defaultType != nil {
				if expected != nil {
					if !defaultType.AssignableTo(expected) {
						c.errorf(e.Default.GetSpan(), "switch default type mismatch")
					}
					resultType = expected
				} else if resultType == nil {
					resultType = defaultType
				} else if !defaultType.Equals(resultType) {
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
			c.errorf(e.Index.GetSpan(), "index must be integer")
			return nil
		}
		if arrType.Kind == KindArray {
			c.ExprTypes[expr] = arrType.Elem
			return arrType.Elem
		}
		if arrType.Kind == KindTuple {
			if lit, ok := e.Index.(*ast.IntLit); ok {
				if lit.Value < 0 || int(lit.Value) >= len(arrType.Tuple) {
					c.errorf(e.Span, "tuple index out of range")
					return nil
				}
				t := arrType.Tuple[int(lit.Value)]
				c.ExprTypes[expr] = t
				return t
			}
			if elem := arrayElemType(arrType); elem != nil {
				c.ExprTypes[expr] = elem
				return elem
			}
			c.errorf(e.Span, "tuple element types differ")
			return nil
		}
		c.errorf(e.Span, "array required")
		return nil
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
		// Block expression executes statements and returns void
		blockEnv := env.child()
		for _, stmt := range e.Stmts {
			c.checkStmt(blockEnv, stmt, Void())
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
			// Parameters must be primitive types (string, integer, number, bool)
			switch paramType.Kind {
			case KindString, KindI64, KindF64, KindBool:
				// OK
			default:
				c.errorf(param.GetSpan(), "SQL parameter must be a primitive type (string, integer, number, or bool)")
			}
		}
		// Validate SQL query against table definitions
		c.validateSQLQuery(e)

		// Determine row type based on SELECT columns
		rowType := c.inferSQLRowType(e)

		// Return type depends on the query kind
		var resultType *Type
		switch e.Kind {
		case ast.SQLQueryExecute:
			// execute returns void (no result)
			resultType = Void()
		case ast.SQLQueryFetchOptional:
			// fetch_optional returns RowType | null (for now, just RowType)
			// TODO: Add proper nullable/optional type support
			resultType = rowType
		case ast.SQLQueryFetchOne:
			// fetch_one returns RowType directly
			resultType = rowType
		case ast.SQLQueryFetch, ast.SQLQueryFetchAll:
			// fetch and fetch_all return RowType[] directly
			resultType = NewArray(rowType)
		default:
			// Default: same as fetch_all
			resultType = NewArray(rowType)
		}
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
			if attrType != nil && !isPrimitiveKind(attrType.Kind) {
				c.errorf(attr.Span, "JSX attribute value must be a primitive type")
			}
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
		if !left.Equals(right) {
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
		c.errorf(call.Span, "undefined: %s", ident.Name)
		return nil
	}
	if sym.Kind == SymBuiltin {
		return c.checkBuiltinCall(env, ident.Name, call, expected)
	}
	if sym.Kind != SymFunc || sym.Type == nil || sym.Type.Kind != KindFunc {
		c.errorf(call.Span, "%s is not function", ident.Name)
		return nil
	}
	c.IdentSymbols[ident] = sym
	sig := sym.Type
	if len(call.Args) != len(sig.Params) {
		c.errorf(call.Span, "argument count mismatch")
		return nil
	}
	bindings := map[*Type]*Type{}
	for i, arg := range call.Args {
		argType := c.checkExpr(env, arg, sig.Params[i])
		if argType == nil {
			return nil
		}
		expectedParam := sig.Params[i]
		if typeContainsTypeParam(expectedParam) {
			if !c.matchType(expectedParam, argType, bindings) {
				c.errorf(call.Span, "argument type mismatch")
				return nil
			}
		} else if !argType.AssignableTo(expectedParam) {
			c.errorf(call.Span, "argument type mismatch")
			return nil
		}
	}
	retType := c.substituteTypeParams(sig.Ret, bindings)
	c.ExprTypes[call] = retType
	return retType
}

func (c *Checker) matchType(expected, actual *Type, bindings map[*Type]*Type) bool {
	if expected == nil || actual == nil {
		return false
	}
	if expected.Kind == KindTypeParam {
		if bound, ok := bindings[expected]; ok {
			return actual.Equals(bound)
		}
		bindings[expected] = actual
		return true
	}
	if expected.Kind != actual.Kind {
		return false
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
	default:
		return actual.Equals(expected)
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

	// Check the receiver object type first
	objType := c.checkExpr(env, member.Object, nil)
	if objType == nil {
		return nil
	}

	sym := env.lookup(funcName)
	if sym == nil {
		c.errorf(call.Span, "undefined: %s", funcName)
		return nil
	}

	if sym.Kind == SymBuiltin {
		// For builtin calls, prepend the object as first argument
		allArgs := append([]ast.Expr{member.Object}, call.Args...)
		syntheticCall := &ast.CallExpr{
			Callee: &ast.IdentExpr{Name: funcName, Span: member.Span},
			Args:   allArgs,
			Span:   call.Span,
		}
		result := c.checkBuiltinCall(env, funcName, syntheticCall, expected)
		c.ExprTypes[call] = result
		return result
	}

	if sym.Kind != SymFunc || sym.Type == nil || sym.Type.Kind != KindFunc {
		c.errorf(call.Span, "%s is not function", funcName)
		return nil
	}

	sig := sym.Type
	// Check that argument count matches (object + args == params)
	if len(call.Args)+1 != len(sig.Params) {
		c.errorf(call.Span, "argument count mismatch")
		return nil
	}

	// Check object type matches first parameter
	if !objType.AssignableTo(sig.Params[0]) {
		c.errorf(call.Span, "receiver type mismatch")
		return nil
	}

	// Check remaining arguments
	for i, arg := range call.Args {
		argType := c.checkExpr(env, arg, sig.Params[i+1])
		if argType != nil && !argType.AssignableTo(sig.Params[i+1]) {
			c.errorf(call.Span, "argument type mismatch")
			return nil
		}
	}

	c.ExprTypes[call] = sig.Ret
	return sig.Ret
}

func (c *Checker) checkBuiltinCall(env *Env, name string, call *ast.CallExpr, expected *Type) *Type {
	switch name {
	case "log":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "log expects 1 arg")
			return nil
		}
		c.checkExpr(env, call.Args[0], nil)
		c.ExprTypes[call] = Void()
		return Void()
	case "stringify":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "stringify expects 1 arg")
			return nil
		}
		c.checkExpr(env, call.Args[0], nil)
		c.ExprTypes[call] = String()
		return String()
	case "parse":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "parse expects 1 arg")
			return nil
		}
		argType := c.checkExpr(env, call.Args[0], String())
		if argType == nil || argType.Kind != KindString {
			c.errorf(call.Span, "parse expects string")
			return nil
		}
		if expected == nil {
			c.errorf(call.Span, "parse needs expected type")
			return nil
		}
		c.ExprTypes[call] = expected
		return expected
	case "toString":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "toString expects 1 arg")
			return nil
		}
		argType := c.checkExpr(env, call.Args[0], nil)
		if argType == nil {
			return nil
		}
		switch argType.Kind {
		case KindI64, KindF64, KindBool, KindString:
			c.ExprTypes[call] = String()
			return String()
		default:
			c.errorf(call.Span, "toString expects primitive")
			return nil
		}
	case "range":
		if len(call.Args) != 2 {
			c.errorf(call.Span, "range expects 2 args")
			return nil
		}
		startType := c.checkExpr(env, call.Args[0], I64())
		endType := c.checkExpr(env, call.Args[1], I64())
		if startType == nil || endType == nil {
			return nil
		}
		if startType.Kind != KindI64 || endType.Kind != KindI64 {
			c.errorf(call.Span, "range expects integer args")
			return nil
		}
		arr := NewArray(I64())
		c.ExprTypes[call] = arr
		return arr
	case "length":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "length expects 1 arg")
			return nil
		}
		arrType := c.checkExpr(env, call.Args[0], nil)
		if arrType == nil || arrType.Kind != KindArray {
			c.errorf(call.Span, "length expects array")
			return nil
		}
		c.ExprTypes[call] = I64()
		return I64()
	case "map":
		if len(call.Args) != 2 {
			c.errorf(call.Span, "map expects 2 args")
			return nil
		}
		arrType := c.checkExpr(env, call.Args[0], nil)
		if arrType == nil {
			return nil
		}
		if arrType.Kind != KindArray {
			c.errorf(call.Span, "map expects array")
			return nil
		}
		elemType := arrType.Elem
		if elemType == nil {
			c.errorf(call.Span, "map element type required")
			return nil
		}
		expectedFn := &Type{Kind: KindFunc, Params: []*Type{elemType}}
		fnType := c.checkExpr(env, call.Args[1], expectedFn)
		if fnType == nil || fnType.Kind != KindFunc || len(fnType.Params) != 1 {
			c.errorf(call.Span, "map expects function")
			return nil
		}
		if !fnType.Params[0].Equals(elemType) {
			c.errorf(call.Span, "map callback type mismatch")
			return nil
		}
		ret := NewArray(fnType.Ret)
		c.ExprTypes[call] = ret
		return ret
	case "filter":
		if len(call.Args) != 2 {
			c.errorf(call.Span, "filter expects 2 args")
			return nil
		}
		arrType := c.checkExpr(env, call.Args[0], nil)
		if arrType == nil {
			return nil
		}
		if arrType.Kind != KindArray {
			c.errorf(call.Span, "filter expects array")
			return nil
		}
		elemType := arrType.Elem
		if elemType == nil {
			c.errorf(call.Span, "filter element type required")
			return nil
		}
		expectedFn := &Type{Kind: KindFunc, Params: []*Type{elemType}}
		fnType := c.checkExpr(env, call.Args[1], expectedFn)
		if fnType == nil || fnType.Kind != KindFunc || len(fnType.Params) != 1 {
			c.errorf(call.Span, "filter expects function")
			return nil
		}
		if !fnType.Params[0].Equals(elemType) {
			c.errorf(call.Span, "filter callback type mismatch")
			return nil
		}
		if fnType.Ret == nil || fnType.Ret.Kind != KindBool {
			c.errorf(call.Span, "filter expects boolean return")
			return nil
		}
		ret := NewArray(elemType)
		c.ExprTypes[call] = ret
		return ret
	case "reduce":
		if len(call.Args) != 3 {
			c.errorf(call.Span, "reduce expects 3 args")
			return nil
		}
		arrType := c.checkExpr(env, call.Args[0], nil)
		if arrType == nil {
			return nil
		}
		if arrType.Kind != KindArray {
			c.errorf(call.Span, "reduce expects array")
			return nil
		}
		elemType := arrType.Elem
		if elemType == nil {
			c.errorf(call.Span, "reduce element type required")
			return nil
		}
		initType := c.checkExpr(env, call.Args[2], nil)
		if initType == nil {
			return nil
		}
		expectedFn := &Type{Kind: KindFunc, Params: []*Type{initType, elemType}}
		fnType := c.checkExpr(env, call.Args[1], expectedFn)
		if fnType == nil || fnType.Kind != KindFunc || len(fnType.Params) != 2 {
			c.errorf(call.Span, "reduce expects function")
			return nil
		}
		if !fnType.Params[0].Equals(initType) || !fnType.Ret.Equals(fnType.Params[0]) {
			c.errorf(call.Span, "reduce accumulator type mismatch")
			return nil
		}
		if !fnType.Params[1].Equals(elemType) {
			c.errorf(call.Span, "reduce element type mismatch")
			return nil
		}
		c.ExprTypes[call] = fnType.Ret
		return fnType.Ret
	case "dbOpen":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "dbOpen expects 1 arg")
			return nil
		}
		argType := c.checkExpr(env, call.Args[0], String())
		if argType == nil || argType.Kind != KindString {
			c.errorf(call.Span, "dbOpen expects string")
			return nil
		}
		c.ExprTypes[call] = Void()
		return Void()
	case "getArgs":
		if len(call.Args) != 0 {
			c.errorf(call.Span, "getArgs expects 0 args")
			return nil
		}
		arr := NewArray(String())
		c.ExprTypes[call] = arr
		return arr
	case "sqlQuery":
		if len(call.Args) != 2 {
			c.errorf(call.Span, "sqlQuery expects 2 args: query string and params array")
			return nil
		}
		queryType := c.checkExpr(env, call.Args[0], String())
		if queryType == nil || queryType.Kind != KindString {
			c.errorf(call.Span, "sqlQuery expects string as first arg")
			return nil
		}
		paramsType := c.checkExpr(env, call.Args[1], nil)
		if paramsType == nil || paramsType.Kind != KindArray {
			c.errorf(call.Span, "sqlQuery expects array as second arg")
			return nil
		}
		// Return type is object with columns and rows
		resultType := NewObject([]Prop{
			{Name: "columns", Type: NewArray(String())},
			{Name: "rows", Type: NewArray(NewArray(String()))},
		})
		c.ExprTypes[call] = resultType
		return resultType
	// HTTP Server builtins
	case "createServer":
		if len(call.Args) != 0 {
			c.errorf(call.Span, "createServer expects 0 args")
			return nil
		}
		// Return an opaque Server handle (represented as i32 object handle)
		serverType := NewObject([]Prop{})
		c.ExprTypes[call] = serverType
		return serverType
	case "addRoute":
		if len(call.Args) != 3 {
			c.errorf(call.Span, "addRoute expects 3 args: server, path, handler")
			return nil
		}
		// First arg: server handle (opaque object)
		serverType := c.checkExpr(env, call.Args[0], nil)
		if serverType == nil {
			return nil
		}
		// Second arg: path string
		pathType := c.checkExpr(env, call.Args[1], String())
		if pathType == nil || pathType.Kind != KindString {
			c.errorf(call.Span, "addRoute expects string as path")
			return nil
		}
		// Third arg: handler function
		// Handler should take a request object and return a response object
		handlerType := c.checkExpr(env, call.Args[2], nil)
		if handlerType == nil || handlerType.Kind != KindFunc {
			c.errorf(call.Span, "addRoute expects function as handler")
			return nil
		}
		c.ExprTypes[call] = Void()
		return Void()
	case "listen":
		if len(call.Args) != 2 {
			c.errorf(call.Span, "listen expects 2 args: server, port")
			return nil
		}
		// First arg: server handle (opaque object)
		serverType := c.checkExpr(env, call.Args[0], nil)
		if serverType == nil {
			return nil
		}
		// Second arg: port string
		portType := c.checkExpr(env, call.Args[1], String())
		if portType == nil || portType.Kind != KindString {
			c.errorf(call.Span, "listen expects string as port")
			return nil
		}
		c.ExprTypes[call] = Void()
		return Void()
	case "responseText":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "responseText expects 1 arg")
			return nil
		}
		textType := c.checkExpr(env, call.Args[0], String())
		if textType == nil || textType.Kind != KindString {
			c.errorf(call.Span, "responseText expects string")
			return nil
		}
		// Return response object
		responseType := NewObject([]Prop{
			{Name: "body", Type: String()},
		})
		c.ExprTypes[call] = responseType
		return responseType
	case "responseHtml":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "responseHtml expects 1 arg")
			return nil
		}
		htmlType := c.checkExpr(env, call.Args[0], String())
		if htmlType == nil || htmlType.Kind != KindString {
			c.errorf(call.Span, "responseHtml expects string")
			return nil
		}
		// Return response object
		responseType := NewObject([]Prop{
			{Name: "body", Type: String()},
			{Name: "contentType", Type: String()},
		})
		c.ExprTypes[call] = responseType
		return responseType
	case "getPath":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "getPath expects 1 arg")
			return nil
		}
		// First arg: request object
		c.checkExpr(env, call.Args[0], nil)
		c.ExprTypes[call] = String()
		return String()
	case "getMethod":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "getMethod expects 1 arg")
			return nil
		}
		// First arg: request object
		c.checkExpr(env, call.Args[0], nil)
		c.ExprTypes[call] = String()
		return String()
	case "responseJson":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "responseJson expects 1 arg")
			return nil
		}
		// Accept any type for JSON serialization
		c.checkExpr(env, call.Args[0], nil)
		// Return response object
		responseType := NewObject([]Prop{
			{Name: "body", Type: String()},
			{Name: "contentType", Type: String()},
		})
		c.ExprTypes[call] = responseType
		return responseType
	case "responseRedirect":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "responseRedirect expects 1 arg")
			return nil
		}
		urlType := c.checkExpr(env, call.Args[0], String())
		if urlType == nil || urlType.Kind != KindString {
			c.errorf(call.Span, "responseRedirect expects string")
			return nil
		}
		// Return response object with redirect info
		responseType := NewObject([]Prop{
			{Name: "body", Type: String()},
			{Name: "contentType", Type: String()},
		})
		c.ExprTypes[call] = responseType
		return responseType
	default:
		c.errorf(call.Span, "unknown builtin")
		return nil
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
		var elemType *Type
		for _, entry := range lit.Entries {
			if entry.Kind == ast.ArrayValue {
				et := c.checkExpr(env, entry.Value, nil)
				if et == nil {
					return nil
				}
				if elemType == nil {
					elemType = et
				} else if !et.Equals(elemType) {
					c.errorf(entry.Span, "array element type mismatch")
					return nil
				}
			} else {
				spreadType := c.checkExpr(env, entry.Value, nil)
				if spreadType == nil || spreadType.Kind != KindArray {
					c.errorf(entry.Span, "array spread requires array")
					return nil
				}
				if elemType == nil {
					elemType = spreadType.Elem
				} else if !spreadType.Elem.Equals(elemType) {
					c.errorf(entry.Span, "array element type mismatch")
					return nil
				}
			}
		}
		if elemType == nil {
			c.errorf(lit.Span, "array literal requires elements")
			return nil
		}
		arr := NewArray(elemType)
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
	allSame := true
	first := elemTypes[0]
	for _, t := range elemTypes[1:] {
		if !t.Equals(first) {
			allSame = false
			break
		}
	}
	if allSame {
		arr := NewArray(first)
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
				if !existing.Equals(valType) {
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
		case "integer":
			return c.recordType(expr, I64())
		case "boolean":
			return c.recordType(expr, Bool())
		case "string":
			return c.recordType(expr, String())
		case "void":
			return c.recordType(expr, Void())
		case "number":
			return c.recordType(expr, Number())
		default:
			if aliasType, ok := mod.TypeAliases[t.Name]; ok {
				return c.recordType(expr, aliasType)
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
			c.errorf(t.Span, "unknown generic type %s", t.Name)
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

func isPreludeName(name string) bool {
	switch name {
	case "log", "stringify", "parse", "toString", "range", "length", "map", "filter", "reduce", "getArgs", "sqlQuery",
		"responseText", "getPath", "getMethod":
		return true
	default:
		return false
	}
}

func isHTTPName(name string) bool {
	switch name {
	case "createServer", "listen", "addRoute", "responseHtml", "responseJson", "responseRedirect":
		return true
	default:
		return false
	}
}

func isSQLiteName(name string) bool {
	switch name {
	case "dbOpen":
		return true
	default:
		return false
	}
}

// getPreludeType returns the type for a prelude type alias, or nil if the name is not a prelude type
func getPreludeType(name string) *Type {
	switch name {
	case "JSX":
		// JSX is a string alias for server-rendered fragments
		return String()
	default:
		return nil
	}
}

// getHTTPType returns the type for an http module type alias
func getHTTPType(name string) *Type {
	switch name {
	case "Request":
		// Request = { path: string, method: string, query: Map<string>, form: Map<string> }
		mapOfString := NewObjectWithIndex(nil, String())
		return NewObject([]Prop{
			{Name: "form", Type: mapOfString},
			{Name: "method", Type: String()},
			{Name: "path", Type: String()},
			{Name: "query", Type: mapOfString},
		})
	case "Response":
		// Response = { "body": string, "contentType": string }
		return NewObject([]Prop{
			{Name: "body", Type: String()},
			{Name: "contentType", Type: String()},
		})
	default:
		return nil
	}
}

func getSQLiteType(name string) *Type {
	return nil
}

type Env struct {
	checker *Checker
	mod     *ModuleInfo
	parent  *Env
	vars    map[string]*Symbol
}

func (e *Env) child() *Env {
	return &Env{checker: e.checker, mod: e.mod, parent: e, vars: map[string]*Symbol{}}
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
	return &Env{checker: e.checker, mod: e.mod, vars: vars}
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
