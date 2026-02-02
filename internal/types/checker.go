package types

import (
	"fmt"
	"sort"
	"strings"

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
	Name string
	Kind SymbolKind
	Type *Type
	Decl ast.Decl
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

type Checker struct {
	Modules      map[string]*ModuleInfo
	ExprTypes    map[ast.Expr]*Type
	IdentSymbols map[*ast.IdentExpr]*Symbol
	Tables       map[string]*TableInfo // table name -> table info
	Errors       []error
}

func NewChecker() *Checker {
	return &Checker{
		Modules:      map[string]*ModuleInfo{},
		ExprTypes:    map[ast.Expr]*Type{},
		IdentSymbols: map[*ast.IdentExpr]*Symbol{},
		Tables:       map[string]*TableInfo{},
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
			sym := &Symbol{Name: d.Name, Kind: SymVar, Type: declType, Decl: d}
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
	if arrow, ok := d.Init.(*ast.ArrowFunc); ok {
		if declType.Kind != KindFunc {
			c.errorf(d.Span, "function type annotation required")
			return
		}
		c.checkArrowFunc(env, arrow, declType)
		return
	}
	if declType.Kind == KindFunc {
		c.errorf(d.Span, "function initializer required")
		return
	}
	initType := c.checkExpr(env, d.Init, declType)
	if initType != nil && !initType.Equals(declType) {
		c.errorf(d.Span, "type mismatch")
	}
}

func (c *Checker) checkFuncDecl(env *Env, d *ast.FuncDecl) {
	sig := c.funcTypeFromDecl(d, env.mod)
	c.checkFuncBody(env, d.Params, d.Body, sig)
}

func (c *Checker) checkArrowFunc(env *Env, fn *ast.ArrowFunc, declType *Type) {
	if declType.Kind != KindFunc {
		c.errorf(fn.Span, "function type required")
		return
	}
	params := make([]*Type, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = c.resolveType(p.Type, env.mod)
	}
	if len(params) != len(declType.Params) {
		c.errorf(fn.Span, "param count mismatch")
		return
	}
	for i := range params {
		if !params[i].Equals(declType.Params[i]) {
			c.errorf(fn.Span, "param type mismatch")
			return
		}
	}
	ret := c.resolveType(fn.Ret, env.mod)
	if !ret.Equals(declType.Ret) {
		c.errorf(fn.Span, "return type mismatch")
		return
	}
	body := fn.Body
	if body == nil && fn.Expr != nil {
		body = &ast.BlockStmt{Stmts: []ast.Stmt{&ast.ReturnStmt{Value: fn.Expr, Span: fn.Expr.GetSpan()}}, Span: fn.Span}
	}
	c.checkFuncBody(env, fn.Params, body, declType)
}

func (c *Checker) funcTypeFromDecl(d *ast.FuncDecl, mod *ModuleInfo) *Type {
	params := make([]*Type, len(d.Params))
	for i, p := range d.Params {
		params[i] = c.resolveType(p.Type, mod)
	}
	ret := c.resolveType(d.Ret, mod)
	return NewFunc(params, ret)
}

func (c *Checker) funcTypeFromArrow(fn *ast.ArrowFunc, mod *ModuleInfo) *Type {
	params := make([]*Type, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = c.resolveType(p.Type, mod)
	}
	ret := c.resolveType(fn.Ret, mod)
	return NewFunc(params, ret)
}

func (c *Checker) checkFuncBody(env *Env, params []ast.Param, body *ast.BlockStmt, sig *Type) {
	fnEnv := env.child()
	for i, p := range params {
		fnEnv.vars[p.Name] = &Symbol{Name: p.Name, Kind: SymVar, Type: sig.Params[i]}
	}
	c.checkBlock(fnEnv, body, sig.Ret)
	if sig.Ret.Kind != KindVoid && !returns(body) {
		c.errorf(body.Span, "return required")
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
			if initType != nil && !initType.Equals(declType) {
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
		env.vars[s.Name] = &Symbol{Name: s.Name, Kind: SymVar, Type: declType}
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
				if declType != nil && !declType.Equals(elemTypes[i]) {
					c.errorf(s.Span, "destructuring type mismatch for %s", name)
				}
			} else {
				// Type inference from array/tuple element
				declType = elemTypes[i]
			}
			env.vars[name] = &Symbol{Name: name, Kind: SymVar, Type: declType}
		}
	case *ast.ObjectDestructureStmt:
		// Check the initializer expression
		initType := c.checkExpr(env, s.Init, nil)
		if initType == nil {
			return
		}

		// Must be an object type or any type
		if initType.Kind != KindObject && initType.Kind != KindAny {
			c.errorf(s.Span, "object destructuring requires object type")
			return
		}

		// Bind each variable
		for i, key := range s.Keys {
			var declType *Type

			if initType.Kind == KindAny {
				// any型の場合、各プロパティもany型になる（明示的な型があればそれを使う）
				if s.Types[i] != nil {
					declType = c.resolveType(s.Types[i], env.mod)
				} else {
					declType = Any()
				}
			} else {
				// Find the property type in the object
				propType := initType.PropType(key)
				if propType == nil {
					c.errorf(s.Span, "property '%s' not found in object", key)
					continue
				}

				if s.Types[i] != nil {
					// Explicit type annotation
					declType = c.resolveType(s.Types[i], env.mod)
					if declType != nil && !declType.Equals(propType) {
						c.errorf(s.Span, "destructuring type mismatch for %s", key)
					}
				} else {
					// Type inference from object property
					declType = propType
				}
			}
			env.vars[key] = &Symbol{Name: key, Kind: SymVar, Type: declType}
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
		// any型への代入は任意の型を許可
		if valType != nil && retType.Kind != KindAny && !valType.Equals(retType) {
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
		var varType *Type
		if s.VarType != nil {
			// 型注釈がある場合
			varType = c.resolveType(s.VarType, env.mod)
			if varType == nil {
				return
			}
			if !elemType.Equals(varType) {
				c.errorf(s.Span, "for-of element type mismatch")
				return
			}
		} else {
			// 型推論：イテラブルの要素型を使用
			varType = elemType
		}
		loopEnv := env.child()
		loopEnv.vars[s.VarName] = &Symbol{Name: s.VarName, Kind: SymVar, Type: varType}
		c.checkBlock(loopEnv, s.Body, retType)
	case *ast.BlockStmt:
		c.checkBlock(env, s, retType)
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
		if !thenType.Equals(elseType) {
			c.errorf(e.Span, "ternary branches must have same type")
			return nil
		}
		c.ExprTypes[expr] = thenType
		return thenType
	case *ast.SwitchExpr:
		// Check the value being switched on
		valueType := c.checkExpr(env, e.Value, nil)
		if valueType == nil {
			return nil
		}

		// Determine result type from cases
		var resultType *Type
		for _, cas := range e.Cases {
			// Check pattern type matches value type
			patternType := c.checkExpr(env, cas.Pattern, valueType)
			if patternType != nil && !patternType.Equals(valueType) {
				c.errorf(cas.Pattern.GetSpan(), "switch case pattern type mismatch")
			}
			// Check body expression
			bodyType := c.checkExpr(env, cas.Body, expected)
			if bodyType == nil {
				continue
			}
			if resultType == nil {
				resultType = bodyType
			} else if !bodyType.Equals(resultType) {
				c.errorf(cas.Body.GetSpan(), "switch case body type mismatch")
			}
		}
		// Check default case
		if e.Default != nil {
			defaultType := c.checkExpr(env, e.Default, expected)
			if defaultType != nil {
				if resultType == nil {
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
		// any型のメンバーアクセスはany型を返す
		if objType.Kind == KindAny {
			c.ExprTypes[expr] = Any()
			return Any()
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
		sig := c.funcTypeFromArrow(e, env.mod)
		if sig == nil {
			return nil
		}
		rootEnv := env.root()
		moduleEnv := rootEnv.clone()
		c.checkArrowFunc(moduleEnv, e, sig)
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
			// Parameters must be primitive types (string, integer, float, bool) or any
			switch paramType.Kind {
			case KindString, KindI64, KindF64, KindBool, KindAny:
				// OK
			default:
				c.errorf(param.GetSpan(), "SQL parameter must be a primitive type (string, integer, float, or bool)")
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
		// JSX element returns string
		// Check attribute expressions
		for _, attr := range e.Attributes {
			if attr.Value != nil {
				attrType := c.checkExpr(env, attr.Value, String())
				// Store the attribute value type for code generation
				if attrType != nil {
					c.ExprTypes[attr.Value] = attrType
				}
				if attrType != nil && attrType.Kind != KindString && attrType.Kind != KindI64 && attrType.Kind != KindF64 && attrType.Kind != KindBool && attrType.Kind != KindAny {
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
		// Expression must return a primitive type (string, number, bool), any, or array of strings
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
			if exprType.Kind != KindString && exprType.Kind != KindI64 && exprType.Kind != KindF64 && exprType.Kind != KindBool && exprType.Kind != KindAny {
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
		// any型との比較は常に許可
		if left.Kind == KindAny || right.Kind == KindAny {
			return Bool()
		}
		if !left.Equals(right) {
			c.errorf(e.Span, "type mismatch")
			return nil
		}
		return Bool()
	case "<", "<=", ">", ">=":
		// any型との比較は許可
		if left.Kind == KindAny || right.Kind == KindAny {
			return Bool()
		}
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
	for i, arg := range call.Args {
		argType := c.checkExpr(env, arg, sig.Params[i])
		// any型の引数は任意の型に変換可能
		if argType != nil && argType.Kind != KindAny && !argType.Equals(sig.Params[i]) {
			c.errorf(call.Span, "argument type mismatch")
			return nil
		}
	}
	c.ExprTypes[call] = sig.Ret
	return sig.Ret
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
		if objType.Kind == KindAny && funcName != "map" {
			// any型のレシーバーは従来通りany型として扱う
			for _, arg := range call.Args {
				c.checkExpr(env, arg, nil)
			}
			c.ExprTypes[call] = Any()
			return Any()
		}
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

	// any型のメソッド呼び出しはany型を返す（型チェックをスキップ）
	if objType.Kind == KindAny {
		// 引数も一応チェックする
		for _, arg := range call.Args {
			c.checkExpr(env, arg, nil)
		}
		c.ExprTypes[call] = Any()
		return Any()
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
	if !objType.Equals(sig.Params[0]) {
		c.errorf(call.Span, "receiver type mismatch")
		return nil
	}

	// Check remaining arguments
	for i, arg := range call.Args {
		argType := c.checkExpr(env, arg, sig.Params[i+1])
		if argType != nil && !argType.Equals(sig.Params[i+1]) {
			c.errorf(call.Span, "argument type mismatch")
			return nil
		}
	}

	c.ExprTypes[call] = sig.Ret
	return sig.Ret
}

func (c *Checker) checkBuiltinCall(env *Env, name string, call *ast.CallExpr, expected *Type) *Type {
	switch name {
	case "print":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "print expects 1 arg")
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
		fnType := c.checkExpr(env, call.Args[1], nil)
		if fnType == nil || fnType.Kind != KindFunc || len(fnType.Params) != 1 {
			c.errorf(call.Span, "map expects function")
			return nil
		}

		elemType := arrType.Elem
		if arrType.Kind == KindAny {
			// Infer array element type from callback parameter
			elemType = fnType.Params[0]
		} else if arrType.Kind != KindArray {
			c.errorf(call.Span, "map expects array")
			return nil
		}
		if elemType == nil {
			c.errorf(call.Span, "map element type required")
			return nil
		}
		if !fnType.Params[0].Equals(elemType) {
			c.errorf(call.Span, "map callback type mismatch")
			return nil
		}
		ret := NewArray(fnType.Ret)
		c.ExprTypes[call] = ret
		return ret
	case "reduce":
		if len(call.Args) != 3 {
			c.errorf(call.Span, "reduce expects 3 args")
			return nil
		}
		arrType := c.checkExpr(env, call.Args[0], nil)
		if arrType == nil || arrType.Kind != KindArray {
			c.errorf(call.Span, "reduce expects array")
			return nil
		}
		fnType := c.checkExpr(env, call.Args[1], nil)
		if fnType == nil || fnType.Kind != KindFunc || len(fnType.Params) != 2 {
			c.errorf(call.Span, "reduce expects function")
			return nil
		}
		initType := c.checkExpr(env, call.Args[2], fnType.Params[0])
		if initType == nil {
			return nil
		}
		if !fnType.Params[0].Equals(initType) || !fnType.Ret.Equals(fnType.Params[0]) {
			c.errorf(call.Span, "reduce accumulator type mismatch")
			return nil
		}
		if !fnType.Params[1].Equals(arrType.Elem) {
			c.errorf(call.Span, "reduce element type mismatch")
			return nil
		}
		c.ExprTypes[call] = fnType.Ret
		return fnType.Ret
	case "dbSave":
		if len(call.Args) != 1 {
			c.errorf(call.Span, "dbSave expects 1 arg")
			return nil
		}
		argType := c.checkExpr(env, call.Args[0], String())
		if argType == nil || argType.Kind != KindString {
			c.errorf(call.Span, "dbSave expects string")
			return nil
		}
		c.ExprTypes[call] = Void()
		return Void()
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
		if urlType == nil || (urlType.Kind != KindString && urlType.Kind != KindAny) {
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
					if et != nil && !et.Equals(expected.Elem) {
						c.errorf(entry.Span, "array element type mismatch")
						return nil
					}
				} else {
					spreadType := c.checkExpr(env, entry.Value, nil)
					if spreadType == nil || spreadType.Kind != KindArray {
						c.errorf(entry.Span, "array spread requires array")
						return nil
					}
					if !spreadType.Elem.Equals(expected.Elem) {
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
				if et != nil && !et.Equals(expected.Tuple[i]) {
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
		if !objType.Equals(expected) {
			c.errorf(lit.Span, "object type mismatch")
			return nil
		}
		c.ExprTypes[lit] = expected
		return expected
	}
	c.ExprTypes[lit] = objType
	return objType
}

func (c *Checker) resolveType(expr ast.TypeExpr, mod *ModuleInfo) *Type {
	switch t := expr.(type) {
	case *ast.NamedType:
		switch t.Name {
		case "integer":
			return I64()
		case "float":
			return F64()
		case "boolean":
			return Bool()
		case "string", "JSX":
			return String()
		case "void":
			return Void()
		case "any":
			return Any()
		default:
			// Check if it's a type alias in the current module
			if aliasType, ok := mod.TypeAliases[t.Name]; ok {
				return aliasType
			}
			c.errorf(t.Span, "unknown type %s", t.Name)
			return nil
		}
	case *ast.ArrayType:
		elem := c.resolveType(t.Elem, mod)
		return NewArray(elem)
	case *ast.TupleType:
		var elems []*Type
		for _, e := range t.Elems {
			elems = append(elems, c.resolveType(e, mod))
		}
		return NewTuple(elems)
	case *ast.ObjectType:
		var props []Prop
		for _, p := range t.Props {
			props = append(props, Prop{Name: p.Key, Type: c.resolveType(p.Type, mod)})
		}
		return NewObject(props)
	case *ast.FuncType:
		params := make([]*Type, len(t.Params))
		for i, p := range t.Params {
			params[i] = c.resolveType(p.Type, mod)
		}
		ret := c.resolveType(t.Ret, mod)
		return NewFunc(params, ret)
	default:
		return nil
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

	tableInfo, exists := c.Tables[tableName]
	if !exists {
		// If no table definitions exist at all, skip validation (for backward compatibility)
		if len(c.Tables) == 0 {
			return
		}
		c.errorf(e.Span, "table '%s' is not defined", tableName)
		return
	}

	// Validate column references
	for _, col := range columns {
		if col == "*" {
			continue // SELECT * is always valid
		}
		if _, exists := tableInfo.Columns[col]; !exists {
			c.errorf(e.Span, "column '%s' does not exist in table '%s'", col, tableName)
		}
	}
}

// parseSQLQueryInfo extracts table name and referenced columns from SQL query
func (c *Checker) parseSQLQueryInfo(query string) (tableName string, columns []string) {
	upper := strings.ToUpper(query)
	words := strings.Fields(query)
	upperWords := strings.Fields(upper)

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
			tableName = strings.ToLower(words[fromIdx+1])
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
					// Handle table.column format
					if dotIdx := strings.Index(col, "."); dotIdx >= 0 {
						col = col[dotIdx+1:]
					}
					columns = append(columns, strings.ToLower(col))
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
						columns = append(columns, strings.ToLower(col))
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
						columns = append(columns, strings.ToLower(col))
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
				columns = append(columns, strings.ToLower(col))
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
	case "print", "stringify", "parse", "toString", "range", "length", "map", "reduce", "dbSave", "dbOpen", "getArgs", "sqlQuery",
		"createServer", "listen", "addRoute", "responseText", "responseHtml", "responseJson", "responseRedirect", "getPath", "getMethod":
		return true
	default:
		return false
	}
}

// getPreludeType returns the type for a prelude type alias, or nil if the name is not a prelude type
func getPreludeType(name string) *Type {
	switch name {
	case "Request":
		// Request = { path: string, method: string, query: any, form: any }
		// query and form are dynamic string-keyed objects (any型として扱う)
		return NewObject([]Prop{
			{Name: "form", Type: Any()},
			{Name: "method", Type: String()},
			{Name: "path", Type: String()},
			{Name: "query", Type: Any()},
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
