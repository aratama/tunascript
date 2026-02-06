package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"tuna/internal/ast"
	"tuna/internal/types"
)

type Generator struct {
	checker *types.Checker
	modules []*types.ModuleInfo
	modIDs  map[string]int

	stringIDs   map[string]int
	stringOrder []string
	stringData  []stringDatum

	funcNames     map[*types.Symbol]string
	funcExports   map[*types.Symbol]bool
	globalNames   map[*types.Symbol]string
	globalExports map[*types.Symbol]bool

	lambdaFuncs map[*ast.ArrowFunc]*lambdaInfo
	lambdaOrder []*lambdaInfo

	tableDefs []*ast.TableDecl

	// HTTP handler functions that need to be exported
	httpHandlerFuncs   map[*types.Symbol]bool
	httpHandlerLambdas map[*ast.ArrowFunc]bool
}

type stringDatum struct {
	value  string
	offset int
	length int
	name   string
}

type lambdaInfo struct {
	fn   *ast.ArrowFunc
	typ  *types.Type
	name string
}

func NewGenerator(checker *types.Checker) *Generator {
	return &Generator{
		checker:            checker,
		lambdaFuncs:        map[*ast.ArrowFunc]*lambdaInfo{},
		httpHandlerFuncs:   map[*types.Symbol]bool{},
		httpHandlerLambdas: map[*ast.ArrowFunc]bool{},
	}
}

func (g *Generator) Generate(entry string) (string, error) {
	g.initModules()
	g.assignSymbols(entry)
	g.collectStrings()
	g.collectFunctionNames()
	g.assignStringData()

	w := &watBuilder{}
	w.line("(module")
	w.indent++
	g.emitImports(w)
	g.emitMemory(w)
	g.emitGlobals(w)
	g.emitFunctions(w, entry)
	w.indent--
	w.line(")")
	return w.String(), nil
}

func (g *Generator) initModules() {
	var paths []string
	for path := range g.checker.Modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	g.modIDs = map[string]int{}
	for i, path := range paths {
		g.modIDs[path] = i
		g.modules = append(g.modules, g.checker.Modules[path])
	}
}

func (g *Generator) collectStrings() {
	g.stringIDs = map[string]int{}
	for _, mod := range g.modules {
		for _, decl := range mod.AST.Decls {
			g.collectStringsDecl(decl)
		}
	}
	g.internString("")
	// Built-in Error handling relies on these strings even if user code doesn't reference them directly.
	g.internString("type")
	g.internString("Error")
	// addRoute のメソッド省略時に使うワイルドカード。
	g.internString("*")
	// Generate and intern table definitions JSON if any tables exist
	if len(g.tableDefs) > 0 {
		jsonStr := g.generateTableDefsJSON()
		g.internString(jsonStr)
	}
}

type decodeSchema struct {
	Kind    string           `json:"kind"`
	Literal *decodeSchemaLit `json:"literal,omitempty"`
	Elem    *decodeSchema    `json:"elem,omitempty"`
	Tuple   []*decodeSchema  `json:"tuple,omitempty"`
	Props   []decodeSchemaKV `json:"props,omitempty"`
	Index   *decodeSchema    `json:"index,omitempty"`
	Union   []*decodeSchema  `json:"union,omitempty"`
}

type decodeSchemaLit struct {
	Kind  string      `json:"kind"`
	Value interface{} `json:"value"`
}

type decodeSchemaKV struct {
	Name string        `json:"name"`
	Type *decodeSchema `json:"type"`
}

func decodeSchemaString(t *types.Type) string {
	s, err := decodeSchemaFromType(t)
	if err != nil {
		panic(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func decodeSchemaFromType(t *types.Type) (*decodeSchema, error) {
	if t == nil {
		return nil, fmt.Errorf("decode schema: nil type")
	}

	// Literal types (e.g. "ok", 1, true) are encoded as base kind + literal constraint.
	if t.Literal {
		base := &types.Type{Kind: t.Kind}
		s, err := decodeSchemaFromType(base)
		if err != nil {
			return nil, err
		}
		s.Literal = &decodeSchemaLit{Kind: schemaKindForTypeKind(t.Kind), Value: t.LiteralValue}
		return s, nil
	}

	switch t.Kind {
	case types.KindI64:
		return &decodeSchema{Kind: "integer"}, nil
	case types.KindF64:
		return &decodeSchema{Kind: "number"}, nil
	case types.KindBool:
		return &decodeSchema{Kind: "boolean"}, nil
	case types.KindString:
		return &decodeSchema{Kind: "string"}, nil
	case types.KindNull:
		return &decodeSchema{Kind: "null"}, nil
	case types.KindUndefined:
		return &decodeSchema{Kind: "undefined"}, nil
	case types.KindJSON:
		return &decodeSchema{Kind: "json"}, nil
	case types.KindArray:
		elem, err := decodeSchemaFromType(t.Elem)
		if err != nil {
			return nil, err
		}
		return &decodeSchema{Kind: "array", Elem: elem}, nil
	case types.KindTuple:
		elems := make([]*decodeSchema, 0, len(t.Tuple))
		for _, e := range t.Tuple {
			s, err := decodeSchemaFromType(e)
			if err != nil {
				return nil, err
			}
			elems = append(elems, s)
		}
		return &decodeSchema{Kind: "tuple", Tuple: elems}, nil
	case types.KindObject:
		props := make([]decodeSchemaKV, 0, len(t.Props))
		for _, p := range t.Props {
			s, err := decodeSchemaFromType(p.Type)
			if err != nil {
				return nil, err
			}
			props = append(props, decodeSchemaKV{Name: p.Name, Type: s})
		}
		var index *decodeSchema
		if t.Index != nil {
			s, err := decodeSchemaFromType(t.Index)
			if err != nil {
				return nil, err
			}
			index = s
		}
		return &decodeSchema{Kind: "object", Props: props, Index: index}, nil
	case types.KindUnion:
		members := make([]*decodeSchema, 0, len(t.Union))
		for _, m := range t.Union {
			s, err := decodeSchemaFromType(m)
			if err != nil {
				return nil, err
			}
			members = append(members, s)
		}
		return &decodeSchema{Kind: "union", Union: members}, nil
	default:
		return nil, fmt.Errorf("decode schema: unsupported type kind %v", t.Kind)
	}
}

func schemaKindForTypeKind(kind types.Kind) string {
	switch kind {
	case types.KindI64:
		return "integer"
	case types.KindF64:
		return "number"
	case types.KindBool:
		return "boolean"
	case types.KindString:
		return "string"
	case types.KindNull:
		return "null"
	case types.KindUndefined:
		return "undefined"
	default:
		return "unknown"
	}
}

// generateTableDefsJSON generates a JSON string representing all table definitions
func (g *Generator) generateTableDefsJSON() string {
	var b strings.Builder
	b.WriteString("[")
	for i, td := range g.tableDefs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("{\"name\":\"")
		b.WriteString(td.Name)
		b.WriteString("\",\"columns\":[")
		for j, col := range td.Columns {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("{\"name\":\"")
			b.WriteString(col.Name)
			b.WriteString("\",\"type\":\"")
			b.WriteString(col.Type)
			b.WriteString("\"")
			if col.Constraints != "" {
				b.WriteString(",\"constraints\":\"")
				b.WriteString(col.Constraints)
				b.WriteString("\"")
			}
			b.WriteString("}")
		}
		b.WriteString("]}")
	}
	b.WriteString("]")
	return b.String()
}

func (g *Generator) collectStringsDecl(decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.ConstDecl:
		g.collectStringsType(d.Type)
		g.collectStringsExpr(d.Init)
	case *ast.FuncDecl:
		for _, p := range d.Params {
			g.collectStringsType(p.Type)
		}
		g.collectStringsType(d.Ret)
		g.collectStringsBlock(d.Body)
	case *ast.TableDecl:
		// Store table definition for later use
		g.tableDefs = append(g.tableDefs, d)
	}
}

func (g *Generator) collectStringsBlock(block *ast.BlockStmt) {
	for _, stmt := range block.Stmts {
		g.collectStringsStmt(stmt)
	}
}

func (g *Generator) collectTypeGuardStrings(targetType *types.Type) {
	if targetType == nil {
		return
	}
	if targetType.Kind != types.KindObject {
		return
	}
	for _, prop := range targetType.Props {
		if prop.Type == nil || prop.Type.Kind != types.KindString || !prop.Type.Literal {
			continue
		}
		g.internString(prop.Name)
		if lit, ok := prop.Type.LiteralValue.(string); ok {
			g.internString(lit)
		}
	}
}

func (g *Generator) collectStringsStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		g.collectStringsType(s.Type)
		g.collectStringsExpr(s.Init)
	case *ast.DestructureStmt:
		for _, t := range s.Types {
			g.collectStringsType(t)
		}
		g.collectStringsExpr(s.Init)
	case *ast.ObjectDestructureStmt:
		for _, key := range s.Keys {
			g.internString(key)
		}
		for _, t := range s.Types {
			g.collectStringsType(t)
		}
		g.collectStringsExpr(s.Init)
	case *ast.ExprStmt:
		g.collectStringsExpr(s.Expr)
	case *ast.ReturnStmt:
		g.collectStringsExpr(s.Value)
	case *ast.IfStmt:
		g.collectStringsExpr(s.Cond)
		g.collectStringsBlock(s.Then)
		if s.Else != nil {
			g.collectStringsBlock(s.Else)
		}
	case *ast.ForOfStmt:
		switch v := s.Var.(type) {
		case *ast.ForOfIdentVar:
			g.collectStringsType(v.Type)
		case *ast.ForOfArrayDestructureVar:
			for _, t := range v.Types {
				g.collectStringsType(t)
			}
		case *ast.ForOfObjectDestructureVar:
			for _, key := range v.Keys {
				g.internString(key)
			}
			for _, t := range v.Types {
				g.collectStringsType(t)
			}
		}
		g.collectStringsExpr(s.Iter)
		g.collectStringsBlock(s.Body)
	case *ast.BlockStmt:
		g.collectStringsBlock(s)
	}
}

func (g *Generator) collectStringsExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.StringLit:
		g.internString(e.Value)
	case *ast.TemplateLit:
		for _, segment := range e.Segments {
			g.internString(segment)
		}
		for _, part := range e.Exprs {
			g.collectStringsExpr(part)
		}
	case *ast.SQLExpr:
		g.internString(e.Query)
		// Also collect strings from parameter expressions
		for _, param := range e.Params {
			g.collectStringsExpr(param)
		}
	case *ast.ObjectLit:
		for _, entry := range e.Entries {
			if entry.Kind == ast.ObjectProp {
				g.internString(entry.Key)
				g.collectStringsExpr(entry.Value)
			} else {
				g.collectStringsExpr(entry.Value)
			}
		}
	case *ast.ArrayLit:
		for _, entry := range e.Entries {
			g.collectStringsExpr(entry.Value)
		}
	case *ast.CallExpr:
		g.collectStringsExpr(e.Callee)
		for _, ta := range e.TypeArgs {
			g.collectStringsType(ta)
		}
		for _, arg := range e.Args {
			g.collectStringsExpr(arg)
		}
		if ident, ok := e.Callee.(*ast.IdentExpr); ok && ident.Name == "decode" && len(e.TypeArgs) == 1 {
			targetType := g.checker.TypeExprTypes[e.TypeArgs[0]]
			if targetType != nil {
				g.internString(decodeSchemaString(targetType))
			}
		}
		if ident, ok := e.Callee.(*ast.IdentExpr); ok && ident.Name == "Error" {
			g.internString("message")
			g.internString("type")
			g.internString("Error")
		}
	case *ast.MemberExpr:
		g.collectStringsExpr(e.Object)
		g.internString(e.Property)
	case *ast.IndexExpr:
		g.collectStringsExpr(e.Array)
		g.collectStringsExpr(e.Index)
	case *ast.TryExpr:
		g.collectStringsExpr(e.Expr)
	case *ast.UnaryExpr:
		g.collectStringsExpr(e.Expr)
	case *ast.AsExpr:
		g.collectStringsExpr(e.Expr)
		g.collectStringsType(e.Type)
		g.collectTypeGuardStrings(g.checker.ExprTypes[e])
	case *ast.ObjectPatternExpr:
		for _, key := range e.Keys {
			g.internString(key)
		}
		for _, t := range e.Types {
			g.collectStringsType(t)
		}
	case *ast.ArrayPatternExpr:
		for _, t := range e.Types {
			g.collectStringsType(t)
		}
	case *ast.BinaryExpr:
		g.collectStringsExpr(e.Left)
		g.collectStringsExpr(e.Right)
	case *ast.IfExpr:
		g.collectStringsExpr(e.Cond)
		g.collectStringsExpr(e.Then)
		if e.Else != nil {
			g.collectStringsExpr(e.Else)
		}
	case *ast.SwitchExpr:
		g.collectStringsExpr(e.Value)
		for _, cas := range e.Cases {
			g.collectStringsExpr(cas.Pattern)
			g.collectStringsExpr(cas.Body)
		}
		if e.Default != nil {
			g.collectStringsExpr(e.Default)
		}
	case *ast.BlockExpr:
		for _, stmt := range e.Stmts {
			g.collectStringsStmt(stmt)
		}
	case *ast.ArrowFunc:
		if e.Body != nil {
			g.collectStringsBlock(e.Body)
		}
		if e.Expr != nil {
			g.collectStringsExpr(e.Expr)
		}
	case *ast.JSXElement:
		g.collectStringsJSX(e)
	case *ast.JSXFragment:
		for _, child := range e.Children {
			g.collectStringsJSXChild(&child)
		}
	}
}

// collectStringsJSX collects strings from a JSX element
func (g *Generator) collectStringsJSX(e *ast.JSXElement) {
	// Tag name
	g.internString("<" + e.Tag)
	if e.SelfClose {
		g.internString(" />")
	} else {
		g.internString(">")
		g.internString("</" + e.Tag + ">")
	}
	// Attribute names and values
	for _, attr := range e.Attributes {
		// For boolean attributes, intern just the attribute name (without ="")
		attrType := g.checker.ExprTypes[attr.Value]
		if attrType != nil && attrType.Kind == types.KindBool {
			g.internString(" " + attr.Name)
		} else {
			g.internString(" " + attr.Name + "=\"")
			g.internString("\"")
		}
		if attr.Value != nil {
			g.collectStringsExpr(attr.Value)
		}
	}
	// Children
	for _, child := range e.Children {
		g.collectStringsJSXChild(&child)
	}
}

// collectStringsJSXChild collects strings from a JSX child
func (g *Generator) collectStringsJSXChild(child *ast.JSXChild) {
	switch child.Kind {
	case ast.JSXChildText:
		g.internString(child.Text)
	case ast.JSXChildElement:
		if child.Element != nil {
			g.collectStringsJSX(child.Element)
		}
	case ast.JSXChildExpr:
		g.collectStringsExpr(child.Expr)
	}
}

func (g *Generator) collectStringsType(t ast.TypeExpr) {
	switch tt := t.(type) {
	case *ast.ArrayType:
		g.collectStringsType(tt.Elem)
	case *ast.TupleType:
		for _, e := range tt.Elems {
			g.collectStringsType(e)
		}
	case *ast.UnionType:
		for _, e := range tt.Types {
			g.collectStringsType(e)
		}
	case *ast.ObjectType:
		for _, p := range tt.Props {
			g.internString(p.Key)
			g.collectStringsType(p.Type)
		}
	case *ast.FuncType:
		for _, p := range tt.Params {
			g.collectStringsType(p.Type)
		}
		g.collectStringsType(tt.Ret)
	}
}

func (g *Generator) internString(value string) {
	if _, ok := g.stringIDs[value]; ok {
		return
	}
	g.stringIDs[value] = len(g.stringOrder)
	g.stringOrder = append(g.stringOrder, value)
}

// collectFunctionNames interns function names used in HTTP route handlers
func (g *Generator) collectFunctionNames() {
	for _, mod := range g.modules {
		for _, decl := range mod.AST.Decls {
			g.collectFunctionNamesDecl(decl)
		}
	}
}

func (g *Generator) collectFunctionNamesDecl(decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.ConstDecl:
		g.collectFunctionNamesExpr(d.Init)
	case *ast.FuncDecl:
		g.collectFunctionNamesBlock(d.Body)
	}
}

func (g *Generator) collectFunctionNamesBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		g.collectFunctionNamesStmt(stmt)
	}
}

func (g *Generator) collectFunctionNamesStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		g.collectFunctionNamesExpr(s.Init)
	case *ast.ExprStmt:
		g.collectFunctionNamesExpr(s.Expr)
	case *ast.ReturnStmt:
		g.collectFunctionNamesExpr(s.Value)
	case *ast.IfStmt:
		g.collectFunctionNamesExpr(s.Cond)
		g.collectFunctionNamesBlock(s.Then)
		if s.Else != nil {
			g.collectFunctionNamesBlock(s.Else)
		}
	case *ast.ForOfStmt:
		g.collectFunctionNamesExpr(s.Iter)
		g.collectFunctionNamesBlock(s.Body)
	case *ast.BlockStmt:
		g.collectFunctionNamesBlock(s)
	}
}

func (g *Generator) collectFunctionNamesExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		// Check if this is an addRoute call (regular or method-style)
		var isAddRoute bool
		var handlerArg ast.Expr

		if ident, ok := e.Callee.(*ast.IdentExpr); ok && ident.Name == "addRoute" {
			isAddRoute = true
			if len(e.Args) >= 4 {
				handlerArg = e.Args[3]
			} else if len(e.Args) >= 3 {
				handlerArg = e.Args[2]
			}
		} else if member, ok := e.Callee.(*ast.MemberExpr); ok && member.Property == "addRoute" {
			// Method-style: server.addRoute("/", handler)
			isAddRoute = true
			if len(e.Args) >= 3 {
				handlerArg = e.Args[2] // Handler is third arg when method is provided
			} else if len(e.Args) >= 2 {
				handlerArg = e.Args[1] // Handler is second arg in method style
			}
			// Also recurse into the object
			g.collectFunctionNamesExpr(member.Object)
		}

		if isAddRoute && handlerArg != nil {
			if handlerIdent, ok := handlerArg.(*ast.IdentExpr); ok {
				sym := g.checker.IdentSymbols[handlerIdent]
				if sym != nil {
					funcName := g.funcImplName(sym)
					g.internString(funcName)
					// Mark this function as an HTTP handler that needs to be exported
					g.httpHandlerFuncs[sym] = true
				}
			} else if arrow, ok := handlerArg.(*ast.ArrowFunc); ok {
				lambdaName := g.lambdaName(arrow)
				g.internString(lambdaName)
				// Mark this lambda as an HTTP handler that needs to be exported
				g.httpHandlerLambdas[arrow] = true
			}
		}

		// Recurse into arguments
		for _, arg := range e.Args {
			g.collectFunctionNamesExpr(arg)
		}
	case *ast.AsExpr:
		g.collectFunctionNamesExpr(e.Expr)
	case *ast.BinaryExpr:
		g.collectFunctionNamesExpr(e.Left)
		g.collectFunctionNamesExpr(e.Right)
	case *ast.IfExpr:
		g.collectFunctionNamesExpr(e.Cond)
		g.collectFunctionNamesExpr(e.Then)
		if e.Else != nil {
			g.collectFunctionNamesExpr(e.Else)
		}
	case *ast.SwitchExpr:
		g.collectFunctionNamesExpr(e.Value)
		for _, cas := range e.Cases {
			g.collectFunctionNamesExpr(cas.Pattern)
			g.collectFunctionNamesExpr(cas.Body)
		}
		if e.Default != nil {
			g.collectFunctionNamesExpr(e.Default)
		}
	case *ast.TryExpr:
		g.collectFunctionNamesExpr(e.Expr)
	case *ast.BlockExpr:
		for _, stmt := range e.Stmts {
			g.collectFunctionNamesStmt(stmt)
		}
	case *ast.ArrowFunc:
		if e.Body != nil {
			g.collectFunctionNamesBlock(e.Body)
		}
		if e.Expr != nil {
			g.collectFunctionNamesExpr(e.Expr)
		}
	case *ast.JSXElement:
		for _, attr := range e.Attributes {
			g.collectFunctionNamesExpr(attr.Value)
		}
		for _, child := range e.Children {
			if child.Kind == ast.JSXChildExpr {
				g.collectFunctionNamesExpr(child.Expr)
			} else if child.Kind == ast.JSXChildElement && child.Element != nil {
				g.collectFunctionNamesExpr(child.Element)
			}
		}
	case *ast.JSXFragment:
		for _, child := range e.Children {
			if child.Kind == ast.JSXChildExpr {
				g.collectFunctionNamesExpr(child.Expr)
			} else if child.Kind == ast.JSXChildElement && child.Element != nil {
				g.collectFunctionNamesExpr(child.Element)
			}
		}
	}
}

func (g *Generator) assignStringData() {
	offset := 0
	for _, value := range g.stringOrder {
		length := len([]byte(value))
		name := fmt.Sprintf("$str_lit_%d", len(g.stringData))
		g.stringData = append(g.stringData, stringDatum{value: value, offset: offset, length: length, name: name})
		offset += length
	}
}

func (g *Generator) assignSymbols(entry string) {
	g.funcNames = map[*types.Symbol]string{}
	g.funcExports = map[*types.Symbol]bool{}
	g.globalNames = map[*types.Symbol]string{}
	g.globalExports = map[*types.Symbol]bool{}
	entryAbs, _ := filepath.Abs(entry)

	for _, mod := range g.modules {
		prefix := fmt.Sprintf("m%d", g.modIDs[mod.AST.Path])
		for _, sym := range mod.Top {
			name := fmt.Sprintf("$%s_%s", prefix, sym.Name)
			if sym.Kind == types.SymFunc {
				g.funcNames[sym] = name
				if mod.AST.Path == entryAbs {
					if decl, ok := sym.Decl.(*ast.ConstDecl); ok && decl.Export {
						g.funcExports[sym] = true
					}
					if decl, ok := sym.Decl.(*ast.FuncDecl); ok && decl.Export {
						g.funcExports[sym] = true
					}
				}
			} else {
				g.globalNames[sym] = name
				if mod.AST.Path == entryAbs {
					if decl, ok := sym.Decl.(*ast.ConstDecl); ok && decl.Export {
						g.globalExports[sym] = true
					}
				}
			}
		}
	}
}

func (g *Generator) emitImports(w *watBuilder) {
	imports := []struct {
		module string
		name   string
	}{
		{"array", "range"},
		{"file", "append_text"},
		{"file", "exists"},
		{"file", "read_dir"},
		{"file", "read_text"},
		{"file", "write_text"},
		{"http", "http_add_route"},
		{"http", "http_create_server"},
		{"http", "http_listen"},
		{"http", "http_response_html"},
		{"http", "http_response_html_str"},
		{"http", "http_response_json"},
		{"http", "http_response_redirect"},
		{"http", "http_response_redirect_str"},
		{"json", "decode"},
		{"json", "parse"},
		{"json", "stringify"},
		{"prelude", "arr_get"},
		{"prelude", "arr_get_result"},
		{"prelude", "arr_join"},
		{"prelude", "arr_len"},
		{"prelude", "arr_new"},
		{"prelude", "arr_set"},
		{"prelude", "escape_html_attr"},
		{"prelude", "gc"},
		{"prelude", "get_args"},
		{"prelude", "get_env"},
		{"prelude", "http_get_method"},
		{"prelude", "http_get_path"},
		{"prelude", "http_response_text"},
		{"prelude", "http_response_text_str"},
		{"prelude", "intern_string"},
		{"prelude", "log"},
		{"prelude", "obj_get"},
		{"prelude", "obj_new"},
		{"prelude", "obj_set"},
		{"prelude", "register_tables"},
		{"prelude", "sql_exec"},
		{"prelude", "sql_execute"},
		{"prelude", "sql_fetch_one"},
		{"prelude", "sql_fetch_optional"},
		{"prelude", "sql_query"},
		{"prelude", "str_concat"},
		{"prelude", "str_eq"},
		{"prelude", "str_from_utf8"},
		{"prelude", "toString"},
		{"prelude", "val_eq"},
		{"prelude", "val_from_bool"},
		{"prelude", "val_from_f64"},
		{"prelude", "val_from_i64"},
		{"prelude", "val_kind"},
		{"prelude", "val_null"},
		{"prelude", "val_to_bool"},
		{"prelude", "val_to_f64"},
		{"prelude", "val_to_i64"},
		{"prelude", "val_undefined"},
		{"runtime", "run_formatter"},
		{"runtime", "run_sandbox"},
		{"sqlite", "db_open"},
	}
	for _, imp := range imports {
		sig := importSig(imp.module, imp.name)
		w.line(fmt.Sprintf("(import \"%s\" \"%s\" %s)", imp.module, imp.name, sig))
	}
}

func importSig(module, name string) string {
	prefix := fmt.Sprintf("$%s", module)
	switch name {
	case "append_text":
		return fmt.Sprintf("(func %s.append_text (param externref externref) (result externref))", prefix)
	case "exists":
		return fmt.Sprintf("(func %s.exists (param externref) (result i32))", prefix)
	case "read_dir":
		return fmt.Sprintf("(func %s.read_dir (param externref) (result externref))", prefix)
	case "read_text":
		return fmt.Sprintf("(func %s.read_text (param externref) (result externref))", prefix)
	case "range":
		return fmt.Sprintf("(func %s.range (param i64 i64) (result externref))", prefix)
	case "write_text":
		return fmt.Sprintf("(func %s.write_text (param externref externref) (result externref))", prefix)
	case "http_add_route":
		return fmt.Sprintf("(func %s.http_add_route (param externref externref i32 i32 externref))", prefix)
	case "http_create_server":
		return fmt.Sprintf("(func %s.http_create_server (result externref))", prefix)
	case "http_listen":
		return fmt.Sprintf("(func %s.http_listen (param externref externref))", prefix)
	case "http_response_html":
		return fmt.Sprintf("(func %s.http_response_html (param i32 i32) (result externref))", prefix)
	case "http_response_html_str":
		return fmt.Sprintf("(func %s.http_response_html_str (param externref) (result externref))", prefix)
	case "http_response_json":
		return fmt.Sprintf("(func %s.http_response_json (param externref) (result externref))", prefix)
	case "http_response_redirect":
		return fmt.Sprintf("(func %s.http_response_redirect (param i32 i32) (result externref))", prefix)
	case "http_response_redirect_str":
		return fmt.Sprintf("(func %s.http_response_redirect_str (param externref) (result externref))", prefix)
	case "decode":
		return fmt.Sprintf("(func %s.decode (param externref externref) (result externref))", prefix)
	case "parse":
		return fmt.Sprintf("(func %s.parse (param externref) (result externref))", prefix)
	case "stringify":
		return fmt.Sprintf("(func %s.stringify (param externref) (result externref))", prefix)
	case "arr_get":
		return fmt.Sprintf("(func %s.arr_get (param externref i32) (result externref))", prefix)
	case "arr_get_result":
		return fmt.Sprintf("(func %s.arr_get_result (param externref i32) (result externref))", prefix)
	case "arr_join":
		return fmt.Sprintf("(func %s.arr_join (param externref) (result externref))", prefix)
	case "arr_len":
		return fmt.Sprintf("(func %s.arr_len (param externref) (result i32))", prefix)
	case "arr_new":
		return fmt.Sprintf("(func %s.arr_new (param i32) (result externref))", prefix)
	case "arr_set":
		return fmt.Sprintf("(func %s.arr_set (param externref i32 externref))", prefix)
	case "escape_html_attr":
		return fmt.Sprintf("(func %s.escape_html_attr (param externref) (result externref))", prefix)
	case "gc":
		return fmt.Sprintf("(func %s.gc)", prefix)
	case "get_args":
		return fmt.Sprintf("(func %s.get_args (result externref))", prefix)
	case "get_env":
		return fmt.Sprintf("(func %s.get_env (param externref) (result externref))", prefix)
	case "http_get_method":
		return fmt.Sprintf("(func %s.http_get_method (param externref) (result externref))", prefix)
	case "http_get_path":
		return fmt.Sprintf("(func %s.http_get_path (param externref) (result externref))", prefix)
	case "http_response_text":
		return fmt.Sprintf("(func %s.http_response_text (param i32 i32) (result externref))", prefix)
	case "http_response_text_str":
		return fmt.Sprintf("(func %s.http_response_text_str (param externref) (result externref))", prefix)
	case "intern_string":
		return fmt.Sprintf("(func %s.intern_string (param i32 i32) (result externref))", prefix)
	case "log":
		return fmt.Sprintf("(func %s.log (param externref))", prefix)
	case "obj_get":
		return fmt.Sprintf("(func %s.obj_get (param externref externref) (result externref))", prefix)
	case "obj_new":
		return fmt.Sprintf("(func %s.obj_new (param i32) (result externref))", prefix)
	case "obj_set":
		return fmt.Sprintf("(func %s.obj_set (param externref externref externref))", prefix)
	case "register_tables":
		return fmt.Sprintf("(func %s.register_tables (param i32 i32))", prefix)
	case "sql_exec":
		return fmt.Sprintf("(func %s.sql_exec (param i32 i32) (result externref))", prefix)
	case "sql_execute":
		return fmt.Sprintf("(func %s.sql_execute (param i32 i32 externref) (result externref))", prefix)
	case "sql_fetch_one":
		return fmt.Sprintf("(func %s.sql_fetch_one (param i32 i32 externref) (result externref))", prefix)
	case "sql_fetch_optional":
		return fmt.Sprintf("(func %s.sql_fetch_optional (param i32 i32 externref) (result externref))", prefix)
	case "sql_query":
		return fmt.Sprintf("(func %s.sql_query (param i32 i32 externref) (result externref))", prefix)
	case "str_concat":
		return fmt.Sprintf("(func %s.str_concat (param externref externref) (result externref))", prefix)
	case "str_eq":
		return fmt.Sprintf("(func %s.str_eq (param externref externref) (result i32))", prefix)
	case "str_from_utf8":
		return fmt.Sprintf("(func %s.str_from_utf8 (param i32 i32) (result externref))", prefix)
	case "toString":
		return fmt.Sprintf("(func %s.toString (param externref) (result externref))", prefix)
	case "val_eq":
		return fmt.Sprintf("(func %s.val_eq (param externref externref) (result i32))", prefix)
	case "val_from_bool":
		return fmt.Sprintf("(func %s.val_from_bool (param i32) (result externref))", prefix)
	case "val_from_f64":
		return fmt.Sprintf("(func %s.val_from_f64 (param f64) (result externref))", prefix)
	case "val_from_i64":
		return fmt.Sprintf("(func %s.val_from_i64 (param i64) (result externref))", prefix)
	case "val_kind":
		return fmt.Sprintf("(func %s.val_kind (param externref) (result i32))", prefix)
	case "val_null":
		return fmt.Sprintf("(func %s.val_null (result externref))", prefix)
	case "val_to_bool":
		return fmt.Sprintf("(func %s.val_to_bool (param externref) (result i32))", prefix)
	case "val_to_f64":
		return fmt.Sprintf("(func %s.val_to_f64 (param externref) (result f64))", prefix)
	case "val_to_i64":
		return fmt.Sprintf("(func %s.val_to_i64 (param externref) (result i64))", prefix)
	case "val_undefined":
		return fmt.Sprintf("(func %s.val_undefined (result externref))", prefix)
	case "run_formatter":
		return fmt.Sprintf("(func %s.run_formatter (param externref) (result externref))", prefix)
	case "run_sandbox":
		return fmt.Sprintf("(func %s.run_sandbox (param externref) (result externref))", prefix)
	case "db_open":
		return fmt.Sprintf("(func %s.db_open (param externref) (result externref))", prefix)
	default:
		return ""
	}
}

func (g *Generator) emitMemory(w *watBuilder) {
	dataSize := 0
	for _, d := range g.stringData {
		dataSize = d.offset + d.length
	}
	pages := (dataSize + 65535) / 65536
	if pages == 0 {
		pages = 1
	}
	w.line(fmt.Sprintf("(memory $memory %d)", pages))
	w.line("(export \"memory\" (memory $memory))")
	for _, d := range g.stringData {
		w.line(fmt.Sprintf("(data (i32.const %d) \"%s\")", d.offset, escapeData(d.value)))
	}
}

func (g *Generator) emitGlobals(w *watBuilder) {
	w.line("(global $__inited (mut i32) (i32.const 0))")
	// Entry main() result storage.
	// void main の場合は null(ref.null extern) のまま。main が (void | Error) を返す場合に実行結果が格納される。
	w.line("(global $__main_result (mut externref) (ref.null extern))")
	for _, d := range g.stringData {
		w.line(fmt.Sprintf("(global %s (mut externref) (ref.null extern))", d.name))
	}
	for _, mod := range g.modules {
		for _, sym := range mod.Top {
			if sym.Kind != types.SymVar {
				continue
			}
			wasmName := g.globalNames[sym]
			wt := wasmType(sym.Type)
			if wt == "externref" {
				w.line(fmt.Sprintf("(global %s (mut externref) (ref.null extern))", wasmName))
			} else {
				w.line(fmt.Sprintf("(global %s (mut %s) (%s.const 0))", wasmName, wt, wt))
			}
			if g.globalExports[sym] {
				w.line(fmt.Sprintf("(export \"%s\" (global %s))", sym.Name, wasmName))
			}
		}
	}
}

func (g *Generator) emitFunctions(w *watBuilder, entry string) {
	entryAbs, _ := filepath.Abs(entry)
	g.emitInit(w)

	for _, mod := range g.modules {
		for _, decl := range mod.AST.Decls {
			switch d := decl.(type) {
			case *ast.ConstDecl:
				if fn, ok := d.Init.(*ast.ArrowFunc); ok {
					sym := mod.Top[d.Name]
					g.emitFunc(w, sym, fn, mod.AST.Path == entryAbs)
				}
			case *ast.FuncDecl:
				sym := mod.Top[d.Name]
				g.emitFuncDecl(w, sym, d, mod.AST.Path == entryAbs)
			}
		}
	}

	g.emitLambdaFuncs(w)
	g.emitStart(w, entryAbs)
}

func (g *Generator) emitInit(w *watBuilder) {
	emitter := newFuncEmitter(g, types.Void())
	for _, d := range g.stringData {
		emitter.emit(fmt.Sprintf("(i32.const %d)", d.offset))
		emitter.emit(fmt.Sprintf("(i32.const %d)", d.length))
		emitter.emit("(call $prelude.str_from_utf8)")
		emitter.emit(fmt.Sprintf("(global.set %s)", d.name))
	}
	for _, mod := range g.modules {
		for _, decl := range mod.AST.Decls {
			cd, ok := decl.(*ast.ConstDecl)
			if !ok {
				continue
			}
			if _, ok := cd.Init.(*ast.ArrowFunc); ok {
				continue
			}
			if ident, ok := cd.Init.(*ast.IdentExpr); ok {
				if _, ok := builtinModule(ident.Name); ok {
					continue
				}
			}
			sym := mod.Top[cd.Name]
			if sym == nil || sym.Kind != types.SymVar {
				continue
			}
			initType := g.checker.ExprTypes[cd.Init]
			emitter.emitExpr(cd.Init, initType)
			emitter.emitCoerce(initType, sym.Type)
			emitter.emitSetGlobal(sym)
		}
	}

	// Register table definitions with the runtime
	if len(g.tableDefs) > 0 {
		jsonStr := g.generateTableDefsJSON()
		datum := g.stringData[g.stringIDs[jsonStr]]
		emitter.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
		emitter.emit(fmt.Sprintf("(i32.const %d)", datum.length))
		emitter.emit("(call $prelude.register_tables)")
	}

	emitter.emit("(i32.const 1)")
	emitter.emit("(global.set $__inited)")

	w.line("(func $__init")
	w.indent++
	for _, local := range emitter.locals {
		w.line(fmt.Sprintf("(local %s %s)", local.name, local.typ))
	}
	for _, line := range emitter.body {
		w.line(line)
	}
	w.indent--
	w.line(")")

	w.line("(func $__ensure_init")
	w.indent++
	w.line("(if (i32.eqz (global.get $__inited))")
	w.indent++
	w.line("(then")
	w.indent++
	w.line("(call $__init)")
	w.indent--
	w.line(")")
	w.indent--
	w.line(")")
	w.indent--
	w.line(")")
}

func (g *Generator) emitStart(w *watBuilder, entryAbs string) {
	w.line("(func $_start")
	w.indent++
	w.line("(call $__ensure_init)")
	mainSym := g.findExportedMain(entryAbs)
	if mainSym != nil {
		if mainSym.Type.Ret.Kind == types.KindVoid {
			w.line(fmt.Sprintf("(call %s)", g.funcImplName(mainSym)))
		} else {
			w.line(fmt.Sprintf("(call %s)", g.funcImplName(mainSym)))
			w.line("(global.set $__main_result)")
		}
	}
	w.indent--
	w.line(")")
	w.line("(export \"_start\" (func $_start))")

	// Runner が main の (void | Error) を検査するためのアクセサ。
	w.line("(func $__get_main_result (result externref)")
	w.indent++
	w.line("(global.get $__main_result)")
	w.indent--
	w.line(")")
	w.line("(export \"__main_result\" (func $__get_main_result))")
}

func (g *Generator) findExportedMain(entryAbs string) *types.Symbol {
	for _, mod := range g.modules {
		if mod.AST.Path != entryAbs {
			continue
		}
		sym := mod.Top["main"]
		if sym == nil {
			return nil
		}
		if sym.Kind != types.SymFunc {
			return nil
		}
		if sym.Type == nil || sym.Type.Kind != types.KindFunc {
			return nil
		}
		if len(sym.Type.Params) != 0 || !isMainReturnType(sym.Type.Ret) {
			return nil
		}
		if !g.funcExports[sym] {
			return nil
		}
		return sym
	}
	return nil
}

func isMainReturnType(ret *types.Type) bool {
	if ret == nil {
		return false
	}
	if ret.Kind == types.KindVoid {
		return true
	}
	success, err := splitResultMembers(ret)
	if success == nil || err == nil {
		return false
	}
	return isVoidLikeType(success)
}

func isVoidLikeType(t *types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case types.KindVoid, types.KindUndefined:
		return true
	case types.KindUnion:
		if len(t.Union) == 0 {
			return false
		}
		for _, member := range t.Union {
			if !isVoidLikeType(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (g *Generator) emitFunc(w *watBuilder, sym *types.Symbol, fn *ast.ArrowFunc, isEntry bool) {
	funcType := sym.Type
	params := fn.Params
	body := fn.Body
	if body == nil && fn.Expr != nil {
		body = &ast.BlockStmt{Stmts: []ast.Stmt{&ast.ReturnStmt{Value: fn.Expr, Span: fn.Expr.GetSpan()}}, Span: fn.Span}
	}
	g.emitFuncBody(w, sym, params, funcType, body, isEntry)
}

func (g *Generator) emitFuncDecl(w *watBuilder, sym *types.Symbol, fn *ast.FuncDecl, isEntry bool) {
	g.emitFuncBody(w, sym, fn.Params, sym.Type, fn.Body, isEntry)
}

func (g *Generator) emitFuncBody(w *watBuilder, sym *types.Symbol, params []ast.Param, funcType *types.Type, body *ast.BlockStmt, isEntry bool) {
	implName := g.funcImplName(sym)

	// For HTTP handlers, use a different internal name to avoid recursion when the wrapper calls it
	actualImplName := implName
	if g.httpHandlerFuncs[sym] {
		actualImplName = implName + "_inner"
	}

	if isEntry && g.funcExports[sym] {
		g.emitWrapper(w, sym, funcType)
	}

	emitter := newFuncEmitter(g, funcType.Ret)
	for i, p := range params {
		localName := emitter.addParam(p.Name, funcType.Params[i])
		emitter.bindLocal(p.Name, localName)
	}
	emitter.emitBlock(body)
	if funcType.Ret != nil && funcType.Ret.Kind != types.KindVoid && canOmitReturnValue(funcType.Ret) {
		emitter.emit("(call $prelude.val_undefined)")
		emitter.emitCoerce(types.Undefined(), funcType.Ret)
		emitter.emit("return")
	}

	w.line(fmt.Sprintf("(func %s%s", actualImplName, emitter.signature()))
	w.indent++
	for _, local := range emitter.locals {
		w.line(fmt.Sprintf("(local %s %s)", local.name, local.typ))
	}
	for _, line := range emitter.body {
		w.line(line)
	}
	w.indent--
	w.line(")")

	// Export HTTP handler functions with a wrapper that calls __ensure_init
	if g.httpHandlerFuncs[sym] {
		g.emitHttpHandlerWrapper(w, sym, funcType)
	}
}

func canOmitReturnValue(t *types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case types.KindVoid, types.KindUndefined:
		return true
	case types.KindUnion:
		for _, member := range t.Union {
			if canOmitReturnValue(member) {
				return true
			}
		}
	}
	return false
}

// emitHttpHandlerWrapper creates a wrapper function for HTTP handlers that ensures initialization
func (g *Generator) emitHttpHandlerWrapper(w *watBuilder, sym *types.Symbol, funcType *types.Type) {
	implName := g.funcImplName(sym)
	// Use a different internal name for the actual impl to avoid recursion
	internalImplName := implName + "_inner"
	exportName := implName // This is what's stored in the data section and looked up by runtime

	var params []string
	for i, p := range funcType.Params {
		params = append(params, fmt.Sprintf("(param $p%d %s)", i, wasmType(p)))
	}
	result := ""
	if funcType.Ret.Kind != types.KindVoid {
		result = fmt.Sprintf("(result %s)", wasmType(funcType.Ret))
	}

	// Emit the exported wrapper that calls __ensure_init then the inner impl
	w.line(fmt.Sprintf("(func %s %s %s", exportName, strings.Join(params, " "), result))
	w.indent++
	w.line("(call $__ensure_init)")
	for i := range funcType.Params {
		w.line(fmt.Sprintf("(local.get $p%d)", i))
	}
	w.line(fmt.Sprintf("(call %s)", internalImplName))
	w.indent--
	w.line(")")
	// Export directly with the wrapper function
	w.line(fmt.Sprintf("(export \"%s\" (func %s))", exportName, exportName))
}

func (g *Generator) lambdaName(fn *ast.ArrowFunc) string {
	if info, ok := g.lambdaFuncs[fn]; ok {
		return info.name
	}
	typ := g.checker.ExprTypes[fn]
	name := fmt.Sprintf("$lambda_%d", len(g.lambdaOrder))
	info := &lambdaInfo{fn: fn, typ: typ, name: name}
	g.lambdaFuncs[fn] = info
	g.lambdaOrder = append(g.lambdaOrder, info)
	return name
}

func (g *Generator) emitLambdaFuncs(w *watBuilder) {
	for _, info := range g.lambdaOrder {
		g.emitLambda(w, info)
	}
}

func (g *Generator) emitLambda(w *watBuilder, info *lambdaInfo) {
	fn := info.fn
	funcType := info.typ
	emitter := newFuncEmitter(g, funcType.Ret)
	for i, p := range fn.Params {
		localName := emitter.addParam(p.Name, funcType.Params[i])
		emitter.bindLocal(p.Name, localName)
	}
	body := fn.Body
	if body == nil && fn.Expr != nil {
		body = &ast.BlockStmt{Stmts: []ast.Stmt{&ast.ReturnStmt{Value: fn.Expr, Span: fn.Expr.GetSpan()}}, Span: fn.Span}
	}
	emitter.emitBlock(body)

	w.line(fmt.Sprintf("(func %s%s", info.name, emitter.signature()))
	w.indent++
	for _, local := range emitter.locals {
		w.line(fmt.Sprintf("(local %s %s)", local.name, local.typ))
	}
	for _, line := range emitter.body {
		w.line(line)
	}
	w.indent--
	w.line(")")

	// Export HTTP handler lambdas with a wrapper that ensures initialization
	if g.httpHandlerLambdas[fn] {
		g.emitHttpLambdaWrapper(w, info)
	}
}

// emitHttpLambdaWrapper creates a wrapper function for HTTP handler lambdas
func (g *Generator) emitHttpLambdaWrapper(w *watBuilder, info *lambdaInfo) {
	funcType := info.typ
	wrapperName := info.name + "_http"

	var params []string
	for i, p := range funcType.Params {
		params = append(params, fmt.Sprintf("(param $p%d %s)", i, wasmType(p)))
	}
	result := ""
	if funcType.Ret.Kind != types.KindVoid {
		result = fmt.Sprintf("(result %s)", wasmType(funcType.Ret))
	}

	w.line(fmt.Sprintf("(func %s %s %s", wrapperName, strings.Join(params, " "), result))
	w.indent++
	w.line("(call $__ensure_init)")
	for i := range funcType.Params {
		w.line(fmt.Sprintf("(local.get $p%d)", i))
	}
	w.line(fmt.Sprintf("(call %s)", info.name))
	w.indent--
	w.line(")")
	w.line(fmt.Sprintf("(export \"%s\" (func %s))", info.name, wrapperName))
}

func (g *Generator) emitWrapper(w *watBuilder, sym *types.Symbol, funcType *types.Type) {
	wrapName := g.funcNames[sym]
	implName := g.funcImplName(sym)
	var params []string
	for i, p := range funcType.Params {
		params = append(params, fmt.Sprintf("(param $p%d %s)", i, wasmType(p)))
	}
	result := ""
	if funcType.Ret.Kind != types.KindVoid {
		result = fmt.Sprintf("(result %s)", wasmType(funcType.Ret))
	}
	w.line(fmt.Sprintf("(func %s %s %s", wrapName, strings.Join(params, " "), result))
	w.indent++
	w.line("(call $__ensure_init)")
	for i := range funcType.Params {
		w.line(fmt.Sprintf("(local.get $p%d)", i))
	}
	w.line(fmt.Sprintf("(call %s)", implName))
	w.indent--
	w.line(")")
	w.line(fmt.Sprintf("(export \"%s\" (func %s))", sym.Name, wrapName))
}

func (g *Generator) funcImplName(sym *types.Symbol) string {
	return fmt.Sprintf("%s_impl", g.funcNames[sym])
}

func wasmType(t *types.Type) string {
	switch t.Kind {
	case types.KindI64:
		return "i64"
	case types.KindF64:
		return "f64"
	case types.KindBool:
		return "i32"
	case types.KindString, types.KindJSON, types.KindArray, types.KindTuple, types.KindObject, types.KindUnion, types.KindNull, types.KindUndefined:
		return "externref"
	default:
		return "externref"
	}
}

type watBuilder struct {
	sb     strings.Builder
	indent int
}

func (w *watBuilder) line(s string) {
	w.sb.WriteString(strings.Repeat("  ", w.indent))
	w.sb.WriteString(s)
	w.sb.WriteString("\n")
}

func (w *watBuilder) String() string {
	return w.sb.String()
}

func escapeData(s string) string {
	var buf bytes.Buffer
	for _, b := range []byte(s) {
		if b >= 0x20 && b <= 0x7e && b != '\\' && b != '"' {
			buf.WriteByte(b)
			continue
		}
		buf.WriteString(fmt.Sprintf("\\%02x", b))
	}
	return buf.String()
}

type localInfo struct {
	name string
	typ  string
}

type funcEmitter struct {
	g      *Generator
	ret    *types.Type
	params []localInfo
	locals []localInfo
	body   []string
	indent int
	scopes []map[string]string
}

func newFuncEmitter(g *Generator, ret *types.Type) *funcEmitter {
	return &funcEmitter{g: g, ret: ret, scopes: []map[string]string{{}}}
}

func (f *funcEmitter) signature() string {
	var parts []string
	for _, p := range f.params {
		parts = append(parts, fmt.Sprintf("(param %s %s)", p.name, p.typ))
	}
	if f.ret != nil && f.ret.Kind != types.KindVoid {
		parts = append(parts, fmt.Sprintf("(result %s)", wasmType(f.ret)))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func (f *funcEmitter) addParam(name string, t *types.Type) string {
	paramName := fmt.Sprintf("$p%d", len(f.params))
	f.params = append(f.params, localInfo{name: paramName, typ: wasmType(t)})
	return paramName
}

func (f *funcEmitter) bindLocal(name, local string) {
	f.scopes[len(f.scopes)-1][name] = local
}

func (f *funcEmitter) pushScope() {
	f.scopes = append(f.scopes, map[string]string{})
}

func (f *funcEmitter) popScope() {
	f.scopes = f.scopes[:len(f.scopes)-1]
}

func (f *funcEmitter) addLocal(name string, t *types.Type) string {
	localName := fmt.Sprintf("$l%d", len(f.locals))
	f.locals = append(f.locals, localInfo{name: localName, typ: wasmType(t)})
	return localName
}

func (f *funcEmitter) addLocalRaw(typ string) string {
	localName := fmt.Sprintf("$l%d", len(f.locals))
	f.locals = append(f.locals, localInfo{name: localName, typ: typ})
	return localName
}

func (f *funcEmitter) lookup(name string) (string, bool) {
	for i := len(f.scopes) - 1; i >= 0; i-- {
		if local, ok := f.scopes[i][name]; ok {
			return local, true
		}
	}
	return "", false
}

func (f *funcEmitter) emit(line string) {
	f.body = append(f.body, strings.Repeat("  ", f.indent)+line)
}

func (f *funcEmitter) emitBlock(block *ast.BlockStmt) {
	f.pushScope()
	for _, stmt := range block.Stmts {
		f.emitStmt(stmt)
	}
	f.popScope()
}

func (f *funcEmitter) emitStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		initType := f.g.checker.ExprTypes[s.Init]
		declType := initType
		if s.Type != nil {
			if resolved := f.g.checker.TypeExprTypes[s.Type]; resolved != nil {
				declType = resolved
			}
		}
		f.emitExpr(s.Init, initType)
		f.emitCoerce(initType, declType)
		local := f.addLocal(s.Name, declType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
		f.bindLocal(s.Name, local)
	case *ast.DestructureStmt:
		f.emitDestructure(s)
	case *ast.ObjectDestructureStmt:
		f.emitObjectDestructure(s)
	case *ast.ExprStmt:
		f.emitExpr(s.Expr, f.g.checker.ExprTypes[s.Expr])
		if t := f.g.checker.ExprTypes[s.Expr]; t != nil && t.Kind != types.KindVoid {
			f.emit("drop")
		}
	case *ast.ReturnStmt:
		if s.Value != nil {
			valType := f.g.checker.ExprTypes[s.Value]
			f.emitExpr(s.Value, valType)
			f.emitCoerce(valType, f.ret)
		} else if f.ret != nil && f.ret.Kind != types.KindVoid {
			f.emit("(call $prelude.val_undefined)")
			f.emitCoerce(types.Undefined(), f.ret)
		}
		f.emit("return")
	case *ast.IfStmt:
		f.emitIfCond(s.Cond)
		f.emit("(if")
		f.indent++
		f.emit("(then")
		f.indent++
		f.emitBlock(s.Then)
		f.indent--
		f.emit(")")
		if s.Else != nil {
			f.emit("(else")
			f.indent++
			f.emitBlock(s.Else)
			f.indent--
			f.emit(")")
		}
		f.indent--
		f.emit(")")
	case *ast.ForOfStmt:
		f.emitForOf(s)
	case *ast.BlockStmt:
		f.emitBlock(s)
	}
}

func (f *funcEmitter) emitDestructure(s *ast.DestructureStmt) {
	initType := f.g.checker.ExprTypes[s.Init]

	// Store the array/tuple in a temporary local
	arrLocal := f.addLocalRaw("externref")
	f.emitExpr(s.Init, initType)
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))

	// Extract each element and bind to a local
	for i, name := range s.Names {
		// Get element type
		var elemType *types.Type
		if initType.Kind == types.KindArray {
			elemType = initType.Elem
		} else if initType.Kind == types.KindTuple {
			elemType = initType.Tuple[i]
		}
		var varType *types.Type
		if i < len(s.Types) && s.Types[i] != nil {
			if resolved := f.g.checker.TypeExprTypes[s.Types[i]]; resolved != nil {
				varType = resolved
			}
		}
		if varType == nil {
			varType = elemType
		}

		// Get element from array/tuple
		f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
		f.emit(fmt.Sprintf("(i32.const %d)", i))
		f.emit("(call $prelude.arr_get)")
		f.emitUnboxIfPrimitive(varType)

		// Store in local variable
		local := f.addLocal(name, varType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
		f.bindLocal(name, local)
	}
}

func (f *funcEmitter) emitObjectDestructure(s *ast.ObjectDestructureStmt) {
	initType := f.g.checker.ExprTypes[s.Init]

	// Store the object in a temporary local
	objLocal := f.addLocalRaw("externref")
	f.emitExpr(s.Init, initType)
	f.emit(fmt.Sprintf("(local.set %s)", objLocal))

	// Extract each property and bind to a local
	for i, key := range s.Keys {
		// Get property type
		propType := initType.PropType(key)
		var varType *types.Type
		if i < len(s.Types) && s.Types[i] != nil {
			if resolved := f.g.checker.TypeExprTypes[s.Types[i]]; resolved != nil {
				varType = resolved
			}
		}
		if varType == nil {
			varType = propType
		}

		// Get property from object
		f.emit(fmt.Sprintf("(local.get %s)", objLocal))
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(key)))
		f.emit("(call $prelude.obj_get)")
		f.emitUnboxIfPrimitive(varType)

		// Store in local variable
		local := f.addLocal(key, varType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
		f.bindLocal(key, local)
	}
}

func (f *funcEmitter) emitForOf(s *ast.ForOfStmt) {
	iterType := f.g.checker.ExprTypes[s.Iter]
	elem := elemType(iterType)
	arrLocal := f.addLocalRaw("externref")
	lenLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	valLocal := f.addLocalRaw("externref")

	f.emitExpr(s.Iter, iterType)
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit("(call $prelude.arr_len)")
	f.emit(fmt.Sprintf("(local.set %s)", lenLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))

	f.emit("(block $for_end")
	f.indent++
	f.emit("(loop $for_loop")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", lenLocal))
	f.emit("i32.ge_u")
	f.emit("br_if $for_end")
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(call $prelude.arr_get)")
	f.emit(fmt.Sprintf("(local.set %s)", valLocal))

	f.pushScope()
	f.emitForOfBinding(s.Var, valLocal, elem)
	f.emitBlock(s.Body)
	f.popScope()

	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	f.emit("br $for_loop")
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
}

func (f *funcEmitter) emitForOfBinding(binding ast.ForOfVar, valLocal string, elemType *types.Type) {
	switch b := binding.(type) {
	case *ast.ForOfIdentVar:
		var varType *types.Type
		if b.Type != nil {
			varType = f.g.checker.TypeExprTypes[b.Type]
		}
		if varType == nil {
			varType = elemType
		}
		if varType == nil {
			return
		}
		local := f.addLocal(b.Name, varType)
		f.bindLocal(b.Name, local)
		f.emit(fmt.Sprintf("(local.get %s)", valLocal))
		f.emitUnboxIfPrimitive(varType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
	case *ast.ForOfArrayDestructureVar:
		f.emitForOfArrayDestructure(b, valLocal, elemType)
	case *ast.ForOfObjectDestructureVar:
		f.emitForOfObjectDestructure(b, valLocal, elemType)
	}
}

func (f *funcEmitter) emitForOfArrayDestructure(binding *ast.ForOfArrayDestructureVar, valLocal string, elemType *types.Type) {
	if elemType == nil {
		return
	}
	var elemTypes []*types.Type
	switch elemType.Kind {
	case types.KindArray:
		for range binding.Names {
			elemTypes = append(elemTypes, elemType.Elem)
		}
	case types.KindTuple:
		if len(binding.Names) > len(elemType.Tuple) {
			return
		}
		elemTypes = append(elemTypes, elemType.Tuple[:len(binding.Names)]...)
	default:
		return
	}

	for i, name := range binding.Names {
		var varType *types.Type
		if i < len(binding.Types) && binding.Types[i] != nil {
			varType = f.g.checker.TypeExprTypes[binding.Types[i]]
		}
		if varType == nil && i < len(elemTypes) {
			varType = elemTypes[i]
		}
		if varType == nil {
			continue
		}

		local := f.addLocal(name, varType)
		f.bindLocal(name, local)
		f.emit(fmt.Sprintf("(local.get %s)", valLocal))
		f.emit(fmt.Sprintf("(i32.const %d)", i))
		f.emit("(call $prelude.arr_get)")
		f.emitUnboxIfPrimitive(varType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
	}
}

func (f *funcEmitter) emitForOfObjectDestructure(binding *ast.ForOfObjectDestructureVar, valLocal string, elemType *types.Type) {
	if elemType == nil || elemType.Kind != types.KindObject {
		return
	}
	for i, key := range binding.Keys {
		propType := elemType.PropType(key)
		if propType == nil {
			continue
		}
		var varType *types.Type
		if i < len(binding.Types) && binding.Types[i] != nil {
			varType = f.g.checker.TypeExprTypes[binding.Types[i]]
		}
		if varType == nil {
			varType = propType
		}
		if varType == nil {
			continue
		}

		local := f.addLocal(key, varType)
		f.bindLocal(key, local)
		f.emit(fmt.Sprintf("(local.get %s)", valLocal))
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(key)))
		f.emit("(call $prelude.obj_get)")
		f.emitUnboxIfPrimitive(varType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
	}
}

func (f *funcEmitter) emitExpr(expr ast.Expr, t *types.Type) {
	switch e := expr.(type) {
	case *ast.IntLit:
		f.emit(fmt.Sprintf("(i64.const %d)", e.Value))
	case *ast.FloatLit:
		f.emit(fmt.Sprintf("(f64.const %s)", strconv.FormatFloat(e.Value, 'g', -1, 64)))
	case *ast.BoolLit:
		if e.Value {
			f.emit("(i32.const 1)")
		} else {
			f.emit("(i32.const 0)")
		}
	case *ast.NullLit:
		f.emit("(call $prelude.val_null)")
	case *ast.UndefinedLit:
		f.emit("(call $prelude.val_undefined)")
	case *ast.StringLit:
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(e.Value)))
	case *ast.TemplateLit:
		f.emitTemplateLit(e)
	case *ast.IdentExpr:
		sym := f.g.checker.IdentSymbols[e]
		if local, ok := f.lookup(e.Name); ok {
			f.emit(fmt.Sprintf("(local.get %s)", local))
		} else if sym != nil && sym.Kind == types.SymVar {
			f.emit(fmt.Sprintf("(global.get %s)", f.g.globalNames[sym]))
		}
		if sym != nil && sym.Kind == types.SymVar {
			storageType := sym.StorageType
			if storageType == nil {
				storageType = sym.Type
			}
			if storageType != nil && (storageType.Kind == types.KindUnion || storageType.Kind == types.KindTypeParam) {
				f.emitUnboxIfPrimitive(t)
			}
		}
	case *ast.UnaryExpr:
		if e.Op == "+" {
			f.emitExpr(e.Expr, f.g.checker.ExprTypes[e.Expr])
			return
		}
		switch t.Kind {
		case types.KindI64:
			f.emit("(i64.const 0)")
			f.emitExpr(e.Expr, f.g.checker.ExprTypes[e.Expr])
			f.emit("i64.sub")
		case types.KindF64:
			f.emitExpr(e.Expr, f.g.checker.ExprTypes[e.Expr])
			f.emit("f64.neg")
		}
	case *ast.AsExpr:
		exprType := f.g.checker.ExprTypes[e.Expr]
		f.emitExpr(e.Expr, exprType)
		targetType := f.g.checker.ExprTypes[e]
		if targetType == nil && e.Type != nil {
			targetType = f.g.checker.TypeExprTypes[e.Type]
		}
		f.emitUnboxIfPrimitive(targetType)
	case *ast.BinaryExpr:
		f.emitBinaryExpr(e, t)
	case *ast.IfExpr:
		f.emitIfExpr(e, t)
	case *ast.SwitchExpr:
		f.emitSwitchExpr(e, t)
	case *ast.BlockExpr:
		f.emitBlockExpr(e, t)
	case *ast.CallExpr:
		f.emitCallExpr(e, t)
	case *ast.MemberExpr:
		objType := f.g.checker.ExprTypes[e.Object]
		f.emitExpr(e.Object, objType)
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(e.Property)))
		f.emit("(call $prelude.obj_get)")
		f.emitUnboxIfPrimitive(t)
	case *ast.IndexExpr:
		arrType := f.g.checker.ExprTypes[e.Array]
		f.emitExpr(e.Array, arrType)
		f.emitExpr(e.Index, f.g.checker.ExprTypes[e.Index])
		f.emit("(i32.wrap_i64)")
		f.emit("(call $prelude.arr_get_result)")
	case *ast.TryExpr:
		f.emitTryExpr(e, t)
	case *ast.ArrayLit:
		f.emitArrayLit(e, t)
	case *ast.ObjectLit:
		f.emitObjectLit(e, t)
	case *ast.SQLExpr:
		f.emitSQLExpr(e, t)
	case *ast.JSXElement:
		f.emitJSXElement(e)
	case *ast.JSXFragment:
		f.emitJSXFragment(e)
	}
}

func (f *funcEmitter) emitTemplateLit(e *ast.TemplateLit) {
	if len(e.Segments) == 0 {
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("")))
		return
	}

	f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(e.Segments[0])))
	for i, part := range e.Exprs {
		partType := f.g.checker.ExprTypes[part]
		f.emitExpr(part, partType)
		f.emitTemplatePartToString(partType)
		f.emit("(call $prelude.str_concat)")

		nextSegment := ""
		if i+1 < len(e.Segments) {
			nextSegment = e.Segments[i+1]
		}
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(nextSegment)))
		f.emit("(call $prelude.str_concat)")
	}
}

func (f *funcEmitter) emitTemplatePartToString(t *types.Type) {
	if t == nil {
		return
	}
	if t.Kind != types.KindUnion {
		f.emitBoxIfPrimitive(t)
	}
	f.emit("(call $prelude.toString)")
}

func (f *funcEmitter) emitIfExpr(e *ast.IfExpr, t *types.Type) {
	// Condition
	f.emitIfCond(e.Cond)

	isVoid := t != nil && t.Kind == types.KindVoid
	if isVoid {
		f.emit("(if")
	} else {
		f.emit(fmt.Sprintf("(if (result %s)", wasmType(t)))
	}
	f.indent++

	// Then branch
	f.emit("(then")
	f.indent++
	thenType := f.g.checker.ExprTypes[e.Then]
	f.emitExpr(e.Then, thenType)
	if !isVoid {
		if thenType != nil && thenType.Kind == types.KindVoid {
			// No value produced; treat as undefined
			f.emit("(call $prelude.val_undefined)")
			f.emitCoerce(types.Undefined(), t)
		} else {
			f.emitCoerce(thenType, t)
		}
	}
	f.indent--
	f.emit(")")

	// Else branch (else is optional)
	f.emit("(else")
	f.indent++
	if e.Else != nil {
		elseType := f.g.checker.ExprTypes[e.Else]
		f.emitExpr(e.Else, elseType)
		if !isVoid {
			if elseType != nil && elseType.Kind == types.KindVoid {
				f.emit("(call $prelude.val_undefined)")
				f.emitCoerce(types.Undefined(), t)
			} else {
				f.emitCoerce(elseType, t)
			}
		}
	} else if !isVoid {
		// else omitted: undefined
		f.emit("(call $prelude.val_undefined)")
		f.emitCoerce(types.Undefined(), t)
	}
	f.indent--
	f.emit(")")

	f.indent--
	f.emit(")")
}

func (f *funcEmitter) emitIfCond(cond ast.Expr) {
	if asExpr, ok := cond.(*ast.AsExpr); ok {
		valueType := f.g.checker.ExprTypes[asExpr.Expr]
		targetType := f.g.checker.ExprTypes[asExpr]
		valueLocal := f.addLocalRaw(wasmType(valueType))
		f.emitExpr(asExpr.Expr, valueType)
		f.emit(fmt.Sprintf("(local.set %s)", valueLocal))
		f.emitTypeGuard(valueLocal, targetType)
		return
	}
	f.emitExpr(cond, f.g.checker.ExprTypes[cond])
}

func splitResultMembers(t *types.Type) (success *types.Type, err *types.Type) {
	if t == nil || t.Kind != types.KindUnion {
		return nil, nil
	}
	isError := func(member *types.Type) bool {
		if member == nil || member.Kind != types.KindObject {
			return false
		}
		typeProp := member.PropType("type")
		msgProp := member.PropType("message")
		if typeProp == nil || msgProp == nil {
			return false
		}
		return typeProp.Equals(types.LiteralString("Error")) && msgProp.AssignableTo(types.String())
	}
	var successMembers []*types.Type
	var errMembers []*types.Type
	for _, member := range t.Union {
		if isError(member) {
			errMembers = append(errMembers, member)
		} else {
			successMembers = append(successMembers, member)
		}
	}
	if len(successMembers) == 0 || len(errMembers) == 0 {
		return nil, nil
	}
	return types.NewUnion(successMembers), types.NewUnion(errMembers)
}

func (f *funcEmitter) emitTryExpr(e *ast.TryExpr, t *types.Type) {
	resultType := f.g.checker.ExprTypes[e.Expr]
	successType, errType := splitResultMembers(resultType)
	if resultType == nil || successType == nil || errType == nil {
		f.emitExpr(e.Expr, resultType)
		return
	}

	valueLocal := f.addLocalRaw(wasmType(resultType))
	f.emitExpr(e.Expr, resultType)
	f.emit(fmt.Sprintf("(local.set %s)", valueLocal))

	f.emitTypeGuard(valueLocal, errType)
	f.emit(fmt.Sprintf("(if (result %s)", wasmType(successType)))
	f.indent++
	f.emit("(then")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	f.emitCoerce(resultType, f.ret)
	f.emit("return")
	if wasmType(successType) == "i64" {
		f.emit("(i64.const 0)")
	} else if wasmType(successType) == "f64" {
		f.emit("(f64.const 0)")
	} else if wasmType(successType) == "externref" {
		f.emit("(call $prelude.val_null)")
	} else {
		f.emit("(i32.const 0)")
	}
	f.indent--
	f.emit(")")
	f.emit("(else")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	f.emitCoerce(resultType, successType)
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
}

func (f *funcEmitter) emitBlockExpr(e *ast.BlockExpr, t *types.Type) {
	// Execute all statements in the block and return the last expression statement's value.
	f.pushScope()

	if len(e.Stmts) == 0 {
		f.popScope()
		return
	}

	for i, stmt := range e.Stmts {
		isLast := i == len(e.Stmts)-1
		if !isLast || t == nil || t.Kind == types.KindVoid {
			f.emitStmt(stmt)
			continue
		}
		// Last statement decides the value.
		if es, ok := stmt.(*ast.ExprStmt); ok {
			exprType := f.g.checker.ExprTypes[es.Expr]
			f.emitExpr(es.Expr, exprType)
			f.emitCoerce(exprType, t)
			continue
		}
		// Fallback: execute statement and return zero/undefined.
		f.emitStmt(stmt)
		switch wasmType(t) {
		case "i64":
			f.emit("(i64.const 0)")
		case "f64":
			f.emit("(f64.const 0)")
		case "externref":
			f.emit("(call $prelude.val_undefined)")
		default:
			f.emit("(i32.const 0)")
		}
	}

	f.popScope()
}

func (f *funcEmitter) emitSwitchExpr(e *ast.SwitchExpr, t *types.Type) {
	valueType := f.g.checker.ExprTypes[e.Value]

	// Store the switch value in a local variable
	valueLocal := f.addLocalRaw(wasmType(valueType))
	f.emitExpr(e.Value, valueType)
	f.emit(fmt.Sprintf("(local.set %s)", valueLocal))

	switchIdentName := ""
	if ident, ok := e.Value.(*ast.IdentExpr); ok {
		switchIdentName = ident.Name
	}

	// Generate nested if-else chain
	f.emitSwitchCases(e.Cases, e.Default, valueLocal, valueType, t, 0, switchIdentName)
}

func (f *funcEmitter) emitSwitchCases(cases []ast.SwitchCase, defaultExpr ast.Expr, valueLocal string, valueType, resultType *types.Type, idx int, switchIdentName string) {
	isVoid := resultType.Kind == types.KindVoid

	if idx >= len(cases) {
		// No more cases, emit default
		if defaultExpr != nil {
			defaultType := f.g.checker.ExprTypes[defaultExpr]
			f.emitExpr(defaultExpr, defaultType)
			f.emitCoerce(defaultType, resultType)
		} else if !isVoid {
			// No default: emit zero value (this shouldn't happen with proper exhaustiveness)
			wt := wasmType(resultType)
			switch wt {
			case "i32":
				f.emit("(i32.const 0)")
			case "i64":
				f.emit("(i64.const 0)")
			case "f64":
				f.emit("(f64.const 0)")
			case "externref":
				f.emit("(call $prelude.val_undefined)")
			}
		}
		return
	}

	cas := cases[idx]
	if asExpr, ok := cas.Pattern.(*ast.AsExpr); ok {
		targetType := f.g.checker.ExprTypes[asExpr]
		f.emitTypeGuard(valueLocal, targetType)
	} else {
		patternType := f.g.checker.ExprTypes[cas.Pattern]

		// Emit comparison: value == pattern
		f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
		f.emitExpr(cas.Pattern, patternType)

		// Emit equality check based on type
		if valueType.Kind == types.KindUnion {
			f.emitCoerce(patternType, valueType)
			f.emit("(call $prelude.val_eq)")
		} else if valueType.Kind == types.KindString || valueType.Kind == types.KindObject || valueType.Kind == types.KindArray || valueType.Kind == types.KindTuple {
			f.emit("(call $prelude.val_eq)")
		} else {
			f.emit(eqOp(valueType))
		}
	}

	// if-then-else (without result type for void)
	if isVoid {
		f.emit("(if")
	} else {
		f.emit(fmt.Sprintf("(if (result %s)", wasmType(resultType)))
	}
	f.indent++
	f.emit("(then")
	f.indent++
	f.pushScope()
	if asExpr, ok := cas.Pattern.(*ast.AsExpr); ok {
		targetType := f.g.checker.ExprTypes[asExpr]
		f.emitSwitchCaseBindings(asExpr, valueLocal, targetType, switchIdentName)
	}
	bodyType := f.g.checker.ExprTypes[cas.Body]
	f.emitExpr(cas.Body, bodyType)
	f.emitCoerce(bodyType, resultType)
	f.popScope()
	f.indent--
	f.emit(")")
	f.emit("(else")
	f.indent++
	// Recursively emit remaining cases
	f.emitSwitchCases(cases, defaultExpr, valueLocal, valueType, resultType, idx+1, switchIdentName)
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
}

func (f *funcEmitter) emitSwitchCaseBindings(asExpr *ast.AsExpr, valueLocal string, targetType *types.Type, switchIdentName string) {
	if asExpr == nil || targetType == nil {
		return
	}

	switch bind := asExpr.Expr.(type) {
	case *ast.IdentExpr:
		if switchIdentName != "" && bind.Name == switchIdentName {
			// Reuse the switch variable itself (type is already narrowed by checker).
			return
		}
		local := f.addLocal(bind.Name, targetType)
		f.bindLocal(bind.Name, local)
		f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
		f.emitUnboxIfPrimitive(targetType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
	case *ast.ObjectPatternExpr:
		if targetType.Kind != types.KindObject {
			return
		}
		for i, key := range bind.Keys {
			propType := targetType.PropType(key)
			if propType == nil {
				continue
			}
			var varType *types.Type
			if i < len(bind.Types) && bind.Types[i] != nil {
				varType = f.g.checker.TypeExprTypes[bind.Types[i]]
			}
			if varType == nil {
				varType = propType
			}
			if varType == nil {
				continue
			}
			local := f.addLocal(key, varType)
			f.bindLocal(key, local)
			f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
			f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(key)))
			f.emit("(call $prelude.obj_get)")
			f.emitUnboxIfPrimitive(varType)
			f.emit(fmt.Sprintf("(local.set %s)", local))
		}
	case *ast.ArrayPatternExpr:
		var elemTypes []*types.Type
		switch targetType.Kind {
		case types.KindArray:
			for range bind.Names {
				elemTypes = append(elemTypes, targetType.Elem)
			}
		case types.KindTuple:
			if len(bind.Names) > len(targetType.Tuple) {
				return
			}
			elemTypes = targetType.Tuple[:len(bind.Names)]
		default:
			return
		}
		for i, name := range bind.Names {
			if i >= len(elemTypes) {
				break
			}
			var varType *types.Type
			if i < len(bind.Types) && bind.Types[i] != nil {
				varType = f.g.checker.TypeExprTypes[bind.Types[i]]
			}
			if varType == nil {
				varType = elemTypes[i]
			}
			if varType == nil {
				continue
			}
			local := f.addLocal(name, varType)
			f.bindLocal(name, local)
			f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
			f.emit(fmt.Sprintf("(i32.const %d)", i))
			f.emit("(call $prelude.arr_get)")
			f.emitUnboxIfPrimitive(varType)
			f.emit(fmt.Sprintf("(local.set %s)", local))
		}
	}
}

func (f *funcEmitter) emitTypeGuard(valueLocal string, targetType *types.Type) {
	if targetType != nil && targetType.Kind == types.KindTypeParam {
		f.emit("(i32.const 1)")
		return
	}
	if targetType != nil && targetType.Kind == types.KindJSON {
		// json は実行時にあらゆる値を受け入れるため、型ガードは常に真。
		f.emit("(i32.const 1)")
		return
	}

	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	f.emit("(call $prelude.val_kind)")
	if kindConst, okKind := runtimeKindConst(targetType); okKind {
		f.emit(fmt.Sprintf("(i32.const %d)", kindConst))
		f.emit("i32.eq")
	} else {
		f.emit("(i32.const 0)")
		return
	}

	if targetType == nil || targetType.Kind != types.KindObject {
		return
	}
	var literalProps []types.Prop
	for _, prop := range targetType.Props {
		if prop.Type == nil || prop.Type.Kind != types.KindString || !prop.Type.Literal {
			continue
		}
		lit, ok := prop.Type.LiteralValue.(string)
		if !ok {
			continue
		}
		_ = lit
		literalProps = append(literalProps, prop)
	}
	if len(literalProps) == 0 {
		return
	}

	// Only check object properties when kind check succeeded.
	f.emit("(if (result i32)")
	f.indent++
	f.emit("(then")
	f.indent++
	f.emit("(i32.const 1)")
	for _, prop := range literalProps {
		lit, _ := prop.Type.LiteralValue.(string)
		f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(prop.Name)))
		f.emit("(call $prelude.obj_get)")
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(lit)))
		f.emit("(call $prelude.val_eq)")
		f.emit("i32.and")
	}
	f.indent--
	f.emit(")")
	f.emit("(else")
	f.indent++
	f.emit("(i32.const 0)")
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
}

func (f *funcEmitter) emitBinaryExpr(e *ast.BinaryExpr, t *types.Type) {
	leftType := f.g.checker.ExprTypes[e.Left]
	rightType := f.g.checker.ExprTypes[e.Right]
	if e.Op == "+" && leftType.Kind == types.KindString {
		f.emitExpr(e.Left, leftType)
		f.emitExpr(e.Right, rightType)
		f.emit("(call $prelude.str_concat)")
		return
	}
	if e.Op == "==" || e.Op == "!=" {
		if leftType.Kind == types.KindString || leftType.Kind == types.KindObject || leftType.Kind == types.KindArray || leftType.Kind == types.KindTuple || leftType.Kind == types.KindUnion || leftType.Kind == types.KindNull || leftType.Kind == types.KindUndefined || leftType.Kind == types.KindJSON {
			f.emitExpr(e.Left, leftType)
			f.emitExpr(e.Right, rightType)
			f.emit("(call $prelude.val_eq)")
			if e.Op == "!=" {
				f.emit("i32.eqz")
			}
			return
		}
	}
	f.emitExpr(e.Left, leftType)
	f.emitExpr(e.Right, rightType)
	switch e.Op {
	case "+":
		f.emit(binOp(leftType, "add"))
	case "-":
		f.emit(binOp(leftType, "sub"))
	case "*":
		f.emit(binOp(leftType, "mul"))
	case "/":
		if leftType.Kind == types.KindI64 {
			f.emit("i64.div_s")
		} else {
			f.emit("f64.div")
		}
	case "%":
		if leftType.Kind == types.KindI64 {
			f.emit("i64.rem_s")
		} else {
			f.emit("f64.rem")
		}
	case "==":
		f.emit(eqOp(leftType))
	case "!=":
		f.emit(eqOp(leftType))
		f.emit("i32.eqz")
	case "<":
		f.emit(cmpOp(leftType, "lt"))
	case "<=":
		f.emit(cmpOp(leftType, "le"))
	case ">":
		f.emit(cmpOp(leftType, "gt"))
	case ">=":
		f.emit(cmpOp(leftType, "ge"))
	case "&":
		f.emit("i32.and")
	case "|":
		f.emit("i32.or")
	}
}

func (f *funcEmitter) emitCallExpr(call *ast.CallExpr, t *types.Type) {
	// Handle method-style call: obj.func(args) => func(obj, args)
	if member, ok := call.Callee.(*ast.MemberExpr); ok {
		f.emitMethodCallExpr(call, member, t)
		return
	}

	ident, ok := call.Callee.(*ast.IdentExpr)
	if !ok {
		return
	}
	name := ident.Name
	sym := resolveSymbolAlias(f.g.checker.IdentSymbols[ident])
	if sym == nil {
		if module, ok := builtinModule(name); ok {
			f.emitBuiltinCall(module, name, call, t)
		}
		return
	}
	if sym.Kind == types.SymBuiltin {
		if module, ok := builtinModule(sym.Name); ok {
			f.emitBuiltinCall(module, sym.Name, call, t)
		}
		return
	}
	if sym.Kind == types.SymFunc && sym.Type != nil && sym.Type.Kind == types.KindFunc {
		for i, arg := range call.Args {
			argType := f.g.checker.ExprTypes[arg]
			f.emitExpr(arg, argType)
			if i < len(sym.Type.Params) {
				f.emitCoerce(argType, sym.Type.Params[i])
			}
		}
	} else {
		for _, arg := range call.Args {
			f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		}
	}
	f.emit(fmt.Sprintf("(call %s)", f.g.funcImplName(sym)))
	if sym.Kind == types.SymFunc && sym.Type != nil && sym.Type.Ret != nil && sym.Type.Ret.Kind == types.KindTypeParam {
		f.emitUnboxIfPrimitive(t)
	}
}

// emitMethodCallExpr emits code for method-style calls: obj.func(args) => func(obj, args)
func (f *funcEmitter) emitMethodCallExpr(call *ast.CallExpr, member *ast.MemberExpr, t *types.Type) {
	funcName := member.Property
	// Look up the function symbol (non-builtin)
	var targetSym *types.Symbol
	for _, mod := range f.g.modules {
		if sym, ok := mod.Top[funcName]; ok && sym.Kind == types.SymFunc {
			targetSym = sym
			break
		}
	}
	if targetSym == nil {
		if module, ok := builtinModule(funcName); ok {
			// Create a synthetic call with object as first argument
			allArgs := append([]ast.Expr{member.Object}, call.Args...)
			syntheticCall := &ast.CallExpr{
				Callee: &ast.IdentExpr{Name: funcName, Span: member.Span},
				Args:   allArgs,
				Span:   call.Span,
			}
			f.emitBuiltinCall(module, funcName, syntheticCall, t)
		}
		return
	}
	if targetSym.Type == nil || targetSym.Type.Kind != types.KindFunc {
		return
	}

	// Emit object (first argument)
	objType := f.g.checker.ExprTypes[member.Object]
	f.emitExpr(member.Object, objType)
	if len(targetSym.Type.Params) > 0 {
		f.emitCoerce(objType, targetSym.Type.Params[0])
	}

	// Emit remaining arguments
	for i, arg := range call.Args {
		argType := f.g.checker.ExprTypes[arg]
		f.emitExpr(arg, argType)
		if i+1 < len(targetSym.Type.Params) {
			f.emitCoerce(argType, targetSym.Type.Params[i+1])
		}
	}

	f.emit(fmt.Sprintf("(call %s)", f.g.funcImplName(targetSym)))
	if targetSym.Type.Ret != nil && targetSym.Type.Ret.Kind == types.KindTypeParam {
		f.emitUnboxIfPrimitive(t)
	}
}

func (f *funcEmitter) resolveFunctionExpr(expr ast.Expr) (string, *types.Type) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym := f.g.checker.IdentSymbols[e]
		if sym == nil {
			return "", nil
		}
		return f.g.funcImplName(sym), sym.Type
	case *ast.ArrowFunc:
		return f.g.lambdaName(e), f.g.checker.ExprTypes[e]
	default:
		return "", nil
	}
}

func (f *funcEmitter) emitBuiltinCall(module, name string, call *ast.CallExpr, t *types.Type) {
	switch name {
	case "log":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.log)", module))
	case "stringify":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.stringify)", module))
	case "parse":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.parse)", module))
		f.emitUnboxIfPrimitive(t)
	case "decode":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		var schemaStr string
		if len(call.TypeArgs) > 0 {
			if targetType := f.g.checker.TypeExprTypes[call.TypeArgs[0]]; targetType != nil {
				schemaStr = decodeSchemaString(targetType)
			}
		}
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(schemaStr)))
		f.emit(fmt.Sprintf("(call $%s.decode)", module))
	case "toString":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.toString)", module))
	case "range":
		start := call.Args[0]
		end := call.Args[1]
		f.emitExpr(start, f.g.checker.ExprTypes[start])
		f.emitExpr(end, f.g.checker.ExprTypes[end])
		f.emit(fmt.Sprintf("(call $%s.range)", module))
	case "length":
		f.emitLength(call)
	case "map":
		f.emitMap(call, t)
	case "filter":
		f.emitFilter(call, t)
	case "reduce":
		f.emitReduce(call, t)
	case "dbOpen":
		f.emitDbOpen(call, module)
	case "getArgs":
		f.emit(fmt.Sprintf("(call $%s.get_args)", module))
	case "getEnv":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.get_env)", module))
	case "gc":
		f.emit(fmt.Sprintf("(call $%s.gc)", module))
	case "runSandbox":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.run_sandbox)", module))
	case "runFormatter":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.run_formatter)", module))
	case "readText":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.read_text)", module))
	case "writeText":
		pathArg := call.Args[0]
		contentArg := call.Args[1]
		f.emitExpr(pathArg, f.g.checker.ExprTypes[pathArg])
		f.emitExpr(contentArg, f.g.checker.ExprTypes[contentArg])
		f.emit(fmt.Sprintf("(call $%s.write_text)", module))
	case "appendText":
		pathArg := call.Args[0]
		contentArg := call.Args[1]
		f.emitExpr(pathArg, f.g.checker.ExprTypes[pathArg])
		f.emitExpr(contentArg, f.g.checker.ExprTypes[contentArg])
		f.emit(fmt.Sprintf("(call $%s.append_text)", module))
	case "readDir":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.read_dir)", module))
	case "exists":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit(fmt.Sprintf("(call $%s.exists)", module))
	case "sqlQuery":
		f.emitSqlQuery(call)
	case "createServer":
		f.emit(fmt.Sprintf("(call $%s.http_create_server)", module))
	case "listen":
		f.emitHttpListen(call, module)
	case "addRoute":
		f.emitHttpAddRoute(call, module)
	case "responseText":
		f.emitHttpResponseText(call, module)
	case "responseHtml":
		f.emitHttpResponseHtml(call, module)
	case "responseJson":
		f.emitHttpResponseJson(call, module)
	case "responseRedirect":
		f.emitHttpResponseRedirect(call, module)
	case "getPath":
		f.emitHttpGetPath(call, module)
	case "getMethod":
		f.emitHttpGetMethod(call, module)
	}
}

func (f *funcEmitter) emitDbOpen(call *ast.CallExpr, module string) {
	arg := call.Args[0]
	argType := f.g.checker.ExprTypes[arg]
	if argType.Kind == types.KindString {
		// For string literal, use intern_string to convert to a handle
		if strLit, ok := arg.(*ast.StringLit); ok {
			f.g.internString(strLit.Value)
			datum := f.g.stringDataByValue(strLit.Value)
			if datum != nil {
				f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
				f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
				f.emit("(call $prelude.intern_string)")
				f.emit(fmt.Sprintf("(call $%s.db_open)", module))
				return
			}
		}
		// For non-literal strings (already a handle), use directly
		f.emitExpr(arg, argType)
		f.emit(fmt.Sprintf("(call $%s.db_open)", module))
	}
}

func (f *funcEmitter) emitSqlQuery(call *ast.CallExpr) {
	queryArg := call.Args[0]
	paramsArg := call.Args[1]
	queryType := f.g.checker.ExprTypes[queryArg]
	paramsType := f.g.checker.ExprTypes[paramsArg]

	// Handle query string
	if queryType.Kind == types.KindString {
		if strLit, ok := queryArg.(*ast.StringLit); ok {
			f.g.internString(strLit.Value)
			datum := f.g.stringDataByValue(strLit.Value)
			if datum != nil {
				f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
				f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
			}
		} else {
			// For non-literal strings, emit the expression and get raw string
			f.emitExpr(queryArg, queryType)
			// String handle is on stack - need to convert to raw string
			// For now, this is not fully supported
		}
	}

	// Emit params array
	f.emitExpr(paramsArg, paramsType)

	f.emit("(call $prelude.sql_query)")
}

// HTTP Server helper methods

func (f *funcEmitter) emitHttpListen(call *ast.CallExpr, module string) {
	serverArg := call.Args[0]
	portArg := call.Args[1]
	serverType := f.g.checker.ExprTypes[serverArg]

	// Emit server handle
	f.emitExpr(serverArg, serverType)

	// Handle port string
	if strLit, ok := portArg.(*ast.StringLit); ok {
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(strLit.Value)))
	} else {
		portType := f.g.checker.ExprTypes[portArg]
		f.emitExpr(portArg, portType)
	}

	f.emit(fmt.Sprintf("(call $%s.http_listen)", module))
}

func (f *funcEmitter) emitHttpAddRoute(call *ast.CallExpr, module string) {
	serverArg := call.Args[0]
	serverType := f.g.checker.ExprTypes[serverArg]
	var methodArg ast.Expr
	var pathArg ast.Expr
	var handlerArg ast.Expr

	if len(call.Args) == 4 {
		methodArg = call.Args[1]
		pathArg = call.Args[2]
		handlerArg = call.Args[3]
	} else {
		methodArg = nil
		pathArg = call.Args[1]
		handlerArg = call.Args[2]
	}

	var methodType *types.Type
	if methodArg != nil {
		methodType = f.g.checker.ExprTypes[methodArg]
	}
	handlerType := f.g.checker.ExprTypes[handlerArg]

	// Emit server handle
	f.emitExpr(serverArg, serverType)

	// Emit method string handle ("get" / "post" or wildcard)
	if methodArg == nil {
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("*")))
	} else if strLit, ok := methodArg.(*ast.StringLit); ok {
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(strLit.Value)))
	} else {
		f.emitExpr(methodArg, methodType)
	}

	// Handle path string
	if strLit, ok := pathArg.(*ast.StringLit); ok {
		f.g.internString(strLit.Value)
		datum := f.g.stringDataByValue(strLit.Value)
		if datum != nil {
			f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
			f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
		}
	}

	// Emit handler function reference as a string (function name)
	// We need to get the function name and pass it as a string handle
	if ident, ok := handlerArg.(*ast.IdentExpr); ok {
		sym := f.g.checker.IdentSymbols[ident]
		if sym != nil {
			funcName := f.g.funcImplName(sym)
			// Intern the function name as a string
			f.g.internString(funcName)
			datum := f.g.stringDataByValue(funcName)
			if datum != nil {
				f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
				f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
				f.emit("(call $prelude.str_from_utf8)")
			}
		}
	} else if arrow, ok := handlerArg.(*ast.ArrowFunc); ok {
		// Handle anonymous function
		lambdaName := f.g.lambdaName(arrow)
		f.g.internString(lambdaName)
		datum := f.g.stringDataByValue(lambdaName)
		if datum != nil {
			f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
			f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
			f.emit("(call $prelude.str_from_utf8)")
		}
	} else {
		// For other expressions, emit as-is (should be a handle already)
		f.emitExpr(handlerArg, handlerType)
	}

	f.emit(fmt.Sprintf("(call $%s.http_add_route)", module))
}

func (f *funcEmitter) emitHttpResponseText(call *ast.CallExpr, module string) {
	textArg := call.Args[0]
	textType := f.g.checker.ExprTypes[textArg]

	// Handle text string
	if strLit, ok := textArg.(*ast.StringLit); ok {
		// For string literals, use direct memory reference
		f.g.internString(strLit.Value)
		datum := f.g.stringDataByValue(strLit.Value)
		if datum != nil {
			f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
			f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
		}
		f.emit(fmt.Sprintf("(call $%s.http_response_text)", module))
	} else {
		// For non-literal strings (e.g., JSX), use the string handle version
		f.emitExpr(textArg, textType)
		f.emit(fmt.Sprintf("(call $%s.http_response_text_str)", module))
	}
}

func (f *funcEmitter) emitHttpResponseHtml(call *ast.CallExpr, module string) {
	htmlArg := call.Args[0]
	htmlType := f.g.checker.ExprTypes[htmlArg]

	// Handle html string
	if strLit, ok := htmlArg.(*ast.StringLit); ok {
		// For string literals, use direct memory reference
		f.g.internString(strLit.Value)
		datum := f.g.stringDataByValue(strLit.Value)
		if datum != nil {
			f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
			f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
		}
		f.emit(fmt.Sprintf("(call $%s.http_response_html)", module))
	} else {
		// For non-literal strings (e.g., JSX), use the string handle version
		f.emitExpr(htmlArg, htmlType)
		f.emit(fmt.Sprintf("(call $%s.http_response_html_str)", module))
	}
}

func (f *funcEmitter) emitHttpResponseJson(call *ast.CallExpr, module string) {
	dataArg := call.Args[0]
	dataType := f.g.checker.ExprTypes[dataArg]
	f.emitExpr(dataArg, dataType)
	f.emit(fmt.Sprintf("(call $%s.http_response_json)", module))
}

func (f *funcEmitter) emitHttpResponseRedirect(call *ast.CallExpr, module string) {
	urlArg := call.Args[0]
	urlType := f.g.checker.ExprTypes[urlArg]

	// Handle URL string
	if strLit, ok := urlArg.(*ast.StringLit); ok {
		// For string literals, use direct memory reference
		f.g.internString(strLit.Value)
		datum := f.g.stringDataByValue(strLit.Value)
		if datum != nil {
			f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
			f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
		}
		f.emit(fmt.Sprintf("(call $%s.http_response_redirect)", module))
	} else {
		// For non-literal strings, use the string handle version
		f.emitExpr(urlArg, urlType)
		f.emit(fmt.Sprintf("(call $%s.http_response_redirect_str)", module))
	}
}

func (f *funcEmitter) emitHttpGetPath(call *ast.CallExpr, module string) {
	reqArg := call.Args[0]
	reqType := f.g.checker.ExprTypes[reqArg]
	f.emitExpr(reqArg, reqType)
	f.emit(fmt.Sprintf("(call $%s.http_get_path)", module))
}

func (f *funcEmitter) emitHttpGetMethod(call *ast.CallExpr, module string) {
	reqArg := call.Args[0]
	reqType := f.g.checker.ExprTypes[reqArg]
	f.emitExpr(reqArg, reqType)
	f.emit(fmt.Sprintf("(call $%s.http_get_method)", module))
}

func (f *funcEmitter) emitLength(call *ast.CallExpr) {
	arg := call.Args[0]
	f.emitExpr(arg, f.g.checker.ExprTypes[arg])
	f.emit("(call $prelude.arr_len)")
	f.emit("i64.extend_i32_u")
}

func (f *funcEmitter) emitMap(call *ast.CallExpr, t *types.Type) {
	arrExpr := call.Args[0]
	fnExpr := call.Args[1]
	arrType := f.g.checker.ExprTypes[arrExpr]
	fnName, fnType := f.resolveFunctionExpr(fnExpr)
	arrLocal := f.addLocalRaw("externref")
	lenLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	resultLocal := f.addLocalRaw("externref")
	valueLocal := f.addLocalRaw("externref")

	f.emitExpr(arrExpr, arrType)
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit("(call $prelude.arr_len)")
	f.emit(fmt.Sprintf("(local.set %s)", lenLocal))
	f.emit(fmt.Sprintf("(local.get %s)", lenLocal))
	f.emit("(call $prelude.arr_new)")
	f.emit(fmt.Sprintf("(local.set %s)", resultLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))

	f.emit("(block $map_end")
	f.indent++
	f.emit("(loop $map_loop")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", lenLocal))
	f.emit("i32.ge_u")
	f.emit("br_if $map_end")
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(call $prelude.arr_get)")
	if len(fnType.Params) > 0 {
		f.emitUnboxIfPrimitive(fnType.Params[0])
	}
	f.emit(fmt.Sprintf("(call %s)", fnName))
	f.emitBoxIfPrimitive(fnType.Ret)
	f.emit(fmt.Sprintf("(local.set %s)", valueLocal))
	f.emit(fmt.Sprintf("(local.get %s)", resultLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	f.emit("(call $prelude.arr_set)")
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	f.emit("br $map_loop")
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
	f.emit(fmt.Sprintf("(local.get %s)", resultLocal))
}

func (f *funcEmitter) emitFilter(call *ast.CallExpr, t *types.Type) {
	arrExpr := call.Args[0]
	fnExpr := call.Args[1]
	arrType := f.g.checker.ExprTypes[arrExpr]
	fnName, fnType := f.resolveFunctionExpr(fnExpr)
	arrLocal := f.addLocalRaw("externref")
	lenLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	countLocal := f.addLocalRaw("i32")
	resultLocal := f.addLocalRaw("externref")
	outIdxLocal := f.addLocalRaw("i32")
	valueLocal := f.addLocalRaw("externref")

	f.emitExpr(arrExpr, arrType)
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit("(call $prelude.arr_len)")
	f.emit(fmt.Sprintf("(local.set %s)", lenLocal))

	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", countLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))

	f.emit("(block $filter_count_end")
	f.indent++
	f.emit("(loop $filter_count_loop")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", lenLocal))
	f.emit("i32.ge_u")
	f.emit("br_if $filter_count_end")
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(call $prelude.arr_get)")
	f.emit(fmt.Sprintf("(local.set %s)", valueLocal))
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	if len(fnType.Params) > 0 {
		f.emitUnboxIfPrimitive(fnType.Params[0])
	}
	f.emit(fmt.Sprintf("(call %s)", fnName))
	f.emit("(if")
	f.indent++
	f.emit("(then")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", countLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", countLocal))
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	f.emit("br $filter_count_loop")
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")

	f.emit(fmt.Sprintf("(local.get %s)", countLocal))
	f.emit("(call $prelude.arr_new)")
	f.emit(fmt.Sprintf("(local.set %s)", resultLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", outIdxLocal))

	f.emit("(block $filter_end")
	f.indent++
	f.emit("(loop $filter_loop")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", lenLocal))
	f.emit("i32.ge_u")
	f.emit("br_if $filter_end")
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(call $prelude.arr_get)")
	f.emit(fmt.Sprintf("(local.set %s)", valueLocal))
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	if len(fnType.Params) > 0 {
		f.emitUnboxIfPrimitive(fnType.Params[0])
	}
	f.emit(fmt.Sprintf("(call %s)", fnName))
	f.emit("(if")
	f.indent++
	f.emit("(then")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", resultLocal))
	f.emit(fmt.Sprintf("(local.get %s)", outIdxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	f.emit("(call $prelude.arr_set)")
	f.emit(fmt.Sprintf("(local.get %s)", outIdxLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", outIdxLocal))
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	f.emit("br $filter_loop")
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
	f.emit(fmt.Sprintf("(local.get %s)", resultLocal))
}

func (f *funcEmitter) emitReduce(call *ast.CallExpr, t *types.Type) {
	arrExpr := call.Args[0]
	fnExpr := call.Args[1]
	initExpr := call.Args[2]
	arrType := f.g.checker.ExprTypes[arrExpr]
	fnName, fnType := f.resolveFunctionExpr(fnExpr)
	accType := fnType.Ret
	arrLocal := f.addLocalRaw("externref")
	lenLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	accLocal := f.addLocalRaw(valueLocalType(accType))

	f.emitExpr(arrExpr, arrType)
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit("(call $prelude.arr_len)")
	f.emit(fmt.Sprintf("(local.set %s)", lenLocal))
	f.emitExpr(initExpr, accType)
	f.emit(fmt.Sprintf("(local.set %s)", accLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))

	f.emit("(block $reduce_end")
	f.indent++
	f.emit("(loop $reduce_loop")
	f.indent++
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit(fmt.Sprintf("(local.get %s)", lenLocal))
	f.emit("i32.ge_u")
	f.emit("br_if $reduce_end")
	f.emit(fmt.Sprintf("(local.get %s)", accLocal))
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(call $prelude.arr_get)")
	if len(fnType.Params) > 1 {
		f.emitUnboxIfPrimitive(fnType.Params[1])
	}
	f.emit(fmt.Sprintf("(call %s)", fnName))
	f.emit(fmt.Sprintf("(local.set %s)", accLocal))
	f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
	f.emit("(i32.const 1)")
	f.emit("i32.add")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	f.emit("br $reduce_loop")
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
	f.emit(fmt.Sprintf("(local.get %s)", accLocal))
}

func (f *funcEmitter) emitArrayLit(lit *ast.ArrayLit, t *types.Type) {
	if t.Kind == types.KindTuple {
		f.emitTupleArrayLit(lit, t)
		return
	}
	f.emitDynamicArrayLit(lit, t)
}

func (f *funcEmitter) emitTupleArrayLit(lit *ast.ArrayLit, t *types.Type) {
	length := len(lit.Entries)
	arrLocal := f.addLocal("_arr", t)
	f.emit(fmt.Sprintf("(i32.const %d)", length))
	f.emit("(call $prelude.arr_new)")
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))
	for i, entry := range lit.Entries {
		f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
		f.emit(fmt.Sprintf("(i32.const %d)", i))
		f.emitExpr(entry.Value, f.g.checker.ExprTypes[entry.Value])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[entry.Value])
		f.emit("(call $prelude.arr_set)")
	}
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
}

func (f *funcEmitter) emitDynamicArrayLit(lit *ast.ArrayLit, t *types.Type) {
	type entryInfo struct {
		entry       ast.ArrayEntry
		handleLocal string
		lengthLocal string
	}
	var infos []entryInfo
	totalLocal := f.addLocalRaw("i32")
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", totalLocal))
	for _, entry := range lit.Entries {
		info := entryInfo{entry: entry, handleLocal: f.addLocalRaw("externref")}
		if entry.Kind == ast.ArrayValue {
			f.emitExpr(entry.Value, f.g.checker.ExprTypes[entry.Value])
			f.emitBoxIfPrimitive(f.g.checker.ExprTypes[entry.Value])
			f.emit(fmt.Sprintf("(local.set %s)", info.handleLocal))
			f.emit(fmt.Sprintf("(local.get %s)", totalLocal))
			f.emit("(i32.const 1)")
			f.emit("i32.add")
			f.emit(fmt.Sprintf("(local.set %s)", totalLocal))
		} else {
			info.lengthLocal = f.addLocalRaw("i32")
			f.emitExpr(entry.Value, f.g.checker.ExprTypes[entry.Value])
			f.emit(fmt.Sprintf("(local.set %s)", info.handleLocal))
			f.emit(fmt.Sprintf("(local.get %s)", info.handleLocal))
			f.emit("(call $prelude.arr_len)")
			f.emit(fmt.Sprintf("(local.set %s)", info.lengthLocal))
			f.emit(fmt.Sprintf("(local.get %s)", totalLocal))
			f.emit(fmt.Sprintf("(local.get %s)", info.lengthLocal))
			f.emit("i32.add")
			f.emit(fmt.Sprintf("(local.set %s)", totalLocal))
		}
		infos = append(infos, info)
	}
	arrLocal := f.addLocalRaw("externref")
	idxLocal := f.addLocalRaw("i32")
	valueLocal := f.addLocalRaw("externref")
	f.emit(fmt.Sprintf("(local.get %s)", totalLocal))
	f.emit("(call $prelude.arr_new)")
	f.emit(fmt.Sprintf("(local.set %s)", arrLocal))
	f.emit("(i32.const 0)")
	f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
	for i, info := range infos {
		if info.entry.Kind == ast.ArrayValue {
			f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
			f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
			f.emit(fmt.Sprintf("(local.get %s)", info.handleLocal))
			f.emit("(call $prelude.arr_set)")
			f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
			f.emit("(i32.const 1)")
			f.emit("i32.add")
			f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
		} else {
			iterLocal := f.addLocalRaw("i32")
			endLabel := fmt.Sprintf("$spread_end_%d", i)
			loopLabel := fmt.Sprintf("$spread_loop_%d", i)
			f.emit("(i32.const 0)")
			f.emit(fmt.Sprintf("(local.set %s)", iterLocal))
			f.emit(fmt.Sprintf("(block %s", endLabel))
			f.indent++
			f.emit(fmt.Sprintf("(loop %s", loopLabel))
			f.indent++
			f.emit(fmt.Sprintf("(local.get %s)", iterLocal))
			f.emit(fmt.Sprintf("(local.get %s)", info.lengthLocal))
			f.emit("i32.ge_u")
			f.emit(fmt.Sprintf("br_if %s", endLabel))
			f.emit(fmt.Sprintf("(local.get %s)", info.handleLocal))
			f.emit(fmt.Sprintf("(local.get %s)", iterLocal))
			f.emit("(call $prelude.arr_get)")
			f.emit(fmt.Sprintf("(local.set %s)", valueLocal))
			f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
			f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
			f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
			f.emit("(call $prelude.arr_set)")
			f.emit(fmt.Sprintf("(local.get %s)", idxLocal))
			f.emit("(i32.const 1)")
			f.emit("i32.add")
			f.emit(fmt.Sprintf("(local.set %s)", idxLocal))
			f.emit(fmt.Sprintf("(local.get %s)", iterLocal))
			f.emit("(i32.const 1)")
			f.emit("i32.add")
			f.emit(fmt.Sprintf("(local.set %s)", iterLocal))
			f.emit(fmt.Sprintf("br %s", loopLabel))
			f.indent--
			f.emit(")")
			f.indent--
			f.emit(")")
		}
	}
	f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
}

func (f *funcEmitter) emitObjectLit(lit *ast.ObjectLit, t *types.Type) {
	objLocal := f.addLocal("_obj", t)
	propCount := 0
	for _, entry := range lit.Entries {
		if entry.Kind == ast.ObjectProp {
			propCount++
		}
		if entry.Kind == ast.ObjectSpread {
			spreadType := f.g.checker.ExprTypes[entry.Value]
			if spreadType != nil && spreadType.Kind == types.KindObject {
				propCount += len(spreadType.Props)
			}
		}
	}
	f.emit(fmt.Sprintf("(call $prelude.obj_new (i32.const %d))", propCount))
	f.emit(fmt.Sprintf("(local.set %s)", objLocal))
	for _, entry := range lit.Entries {
		switch entry.Kind {
		case ast.ObjectSpread:
			spreadType := f.g.checker.ExprTypes[entry.Value]
			if spreadType == nil || spreadType.Kind != types.KindObject {
				continue
			}
			spreadLocal := f.addLocal("_spread", spreadType)
			f.emitExpr(entry.Value, spreadType)
			f.emit(fmt.Sprintf("(local.set %s)", spreadLocal))
			for _, prop := range spreadType.Props {
				keyGlobal := f.g.stringGlobal(prop.Name)
				f.emit(fmt.Sprintf("(local.get %s)", objLocal))
				f.emit(fmt.Sprintf("(global.get %s)", keyGlobal))
				f.emit(fmt.Sprintf("(local.get %s)", spreadLocal))
				f.emit(fmt.Sprintf("(global.get %s)", keyGlobal))
				f.emit("(call $prelude.obj_get)")
				f.emit("(call $prelude.obj_set)")
			}
		case ast.ObjectProp:
			keyGlobal := f.g.stringGlobal(entry.Key)
			f.emit(fmt.Sprintf("(local.get %s)", objLocal))
			f.emit(fmt.Sprintf("(global.get %s)", keyGlobal))
			f.emitExpr(entry.Value, f.g.checker.ExprTypes[entry.Value])
			f.emitBoxIfPrimitive(f.g.checker.ExprTypes[entry.Value])
			f.emit("(call $prelude.obj_set)")
		}
	}
	f.emit(fmt.Sprintf("(local.get %s)", objLocal))
}

func (f *funcEmitter) emitSQLExpr(e *ast.SQLExpr, t *types.Type) {
	// SQL query is stored as a string in memory
	f.g.internString(e.Query)
	datum := f.g.stringDataByValue(e.Query)
	if datum == nil {
		return
	}

	// Build params array if needed
	paramsLocal := f.addLocalRaw("externref")
	if len(e.Params) == 0 {
		// Empty params array
		f.emit("(i32.const 0)")
		f.emit("(call $prelude.arr_new)")
		f.emit(fmt.Sprintf("(local.set %s)", paramsLocal))
	} else {
		// Create params array
		f.emit(fmt.Sprintf("(i32.const %d)", len(e.Params)))
		f.emit("(call $prelude.arr_new)")
		f.emit(fmt.Sprintf("(local.set %s)", paramsLocal))

		// Populate params array
		for i, param := range e.Params {
			paramType := f.g.checker.ExprTypes[param]
			f.emit(fmt.Sprintf("(local.get %s)", paramsLocal))
			f.emit(fmt.Sprintf("(i32.const %d)", i))
			f.emitExpr(param, paramType)
			f.emitBoxIfPrimitive(paramType)
			f.emit("(call $prelude.arr_set)")
		}
	}

	// Call appropriate runtime function based on query kind
	f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
	f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
	f.emit(fmt.Sprintf("(local.get %s)", paramsLocal))

	switch e.Kind {
	case ast.SQLQueryExecute:
		// execute returns nothing
		f.emit("(call $prelude.sql_execute)")
	case ast.SQLQueryFetchOne:
		// fetch_one returns a single row object
		f.emit("(call $prelude.sql_fetch_one)")
	case ast.SQLQueryFetchOptional:
		// fetch_optional returns a single row object or null
		f.emit("(call $prelude.sql_fetch_optional)")
	case ast.SQLQueryFetch, ast.SQLQueryFetchAll:
		// fetch and fetch_all return { columns: [], rows: [] }
		f.emit("(call $prelude.sql_query)")
	default:
		// Default behavior (same as fetch_all)
		f.emit("(call $prelude.sql_query)")
	}
}

func (f *funcEmitter) emitSetGlobal(sym *types.Symbol) {
	globalName := f.g.globalNames[sym]
	f.emit(fmt.Sprintf("(global.set %s)", globalName))
}

func (f *funcEmitter) emitBoxIfPrimitive(t *types.Type) {
	if t == nil {
		return
	}
	switch t.Kind {
	case types.KindI64:
		f.emit("(call $prelude.val_from_i64)")
	case types.KindF64:
		f.emit("(call $prelude.val_from_f64)")
	case types.KindBool:
		f.emit("(call $prelude.val_from_bool)")
	}
}

func (f *funcEmitter) emitUnboxIfPrimitive(t *types.Type) {
	if t == nil {
		return
	}
	switch t.Kind {
	case types.KindI64:
		f.emit("(call $prelude.val_to_i64)")
	case types.KindF64:
		f.emit("(call $prelude.val_to_f64)")
	case types.KindBool:
		f.emit("(call $prelude.val_to_bool)")
	}
}

func (f *funcEmitter) emitCoerce(from, to *types.Type) {
	if from == nil || to == nil {
		return
	}
	if to.Kind == types.KindTypeParam {
		f.emitBoxIfPrimitive(from)
		return
	}
	if from.Kind == types.KindTypeParam && to.Kind != types.KindUnion {
		f.emitUnboxIfPrimitive(to)
		return
	}
	if to.Kind == types.KindUnion && from.Kind != types.KindUnion {
		f.emitBoxIfPrimitive(from)
		return
	}
	if from.Kind == types.KindUnion && to.Kind != types.KindUnion {
		f.emitUnboxIfPrimitive(to)
	}
}

func valueLocalType(t *types.Type) string {
	if t == nil {
		return "i32"
	}
	switch t.Kind {
	case types.KindI64:
		return "i64"
	case types.KindF64:
		return "f64"
	case types.KindBool:
		return "i32"
	default:
		return "externref"
	}
}

func (g *Generator) stringGlobal(value string) string {
	id := g.stringIDs[value]
	return g.stringData[id].name
}

func (g *Generator) stringDataByValue(value string) *stringDatum {
	id, ok := g.stringIDs[value]
	if !ok {
		return nil
	}
	return &g.stringData[id]
}

func binOp(t *types.Type, op string) string {
	if t.Kind == types.KindI64 {
		return "i64." + op
	}
	return "f64." + op
}

func eqOp(t *types.Type) string {
	if t.Kind == types.KindI64 {
		return "i64.eq"
	}
	if t.Kind == types.KindF64 {
		return "f64.eq"
	}
	if wasmType(t) == "externref" {
		return "ref.eq"
	}
	return "i32.eq"
}

func cmpOp(t *types.Type, op string) string {
	if t.Kind == types.KindI64 {
		return "i64." + op + "_s"
	}
	return "f64." + op
}

func runtimeKindConst(t *types.Type) (int32, bool) {
	if t == nil {
		return 0, false
	}
	switch t.Kind {
	case types.KindI64:
		return 0, true
	case types.KindF64:
		return 1, true
	case types.KindBool:
		return 2, true
	case types.KindString:
		return 3, true
	case types.KindObject:
		return 4, true
	case types.KindArray, types.KindTuple:
		return 5, true
	case types.KindNull:
		return 6, true
	case types.KindUndefined:
		return 7, true
	default:
		return 0, false
	}
}

func elemType(t *types.Type) *types.Type {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case types.KindArray:
		return t.Elem
	default:
		return nil
	}
}

func builtinModule(name string) (string, bool) {
	switch name {
	case "log", "toString", "getArgs", "sqlQuery",
		"gc",
		"getEnv", "responseText", "getPath", "getMethod":
		return "prelude", true
	case "stringify", "parse", "decode":
		return "json", true
	case "range", "length", "map", "filter", "reduce":
		return "array", true
	case "runSandbox", "runFormatter":
		return "runtime", true
	case "readText", "writeText", "appendText", "readDir", "exists":
		return "file", true
	case "dbOpen":
		return "sqlite", true
	case "createServer", "listen", "addRoute", "responseHtml", "responseJson", "responseRedirect":
		return "http", true
	default:
		return "", false
	}
}

func resolveSymbolAlias(sym *types.Symbol) *types.Symbol {
	seen := map[*types.Symbol]bool{}
	for sym != nil {
		if seen[sym] {
			return sym
		}
		seen[sym] = true
		if sym.Alias == nil {
			return sym
		}
		sym = sym.Alias
	}
	return nil
}

// emitJSXElement generates code for a JSX element (converts to string concatenation)
func (f *funcEmitter) emitJSXElement(e *ast.JSXElement) {
	if info, ok := f.g.checker.JSXComponents[e]; ok {
		f.emitJSXComponent(e, info)
		return
	}
	// JSX is compiled to string concatenation
	// <div className="foo">Hello {name}</div>
	// becomes: "<div className=\"foo\">" + "Hello " + name + "</div>"

	// Start with opening tag
	f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("<"+e.Tag)))

	// Emit attributes
	for _, attr := range e.Attributes {
		attrType := f.g.checker.ExprTypes[attr.Value]

		// Handle boolean attributes (checked, disabled, etc.)
		// For boolean type, only emit the attribute if the value is true
		if attrType != nil && attrType.Kind == types.KindBool {
			// We need to conditionally concatenate.
			// Stack before: [str]
			// We want: if (cond) { str + " attrName" } else { str }
			//
			// Approach: Use a local to store the current string, evaluate condition,
			// then conditionally concat
			tempLocal := f.addLocalRaw("externref")
			f.emit(fmt.Sprintf("(local.set %s)", tempLocal)) // save str
			f.emitExpr(attr.Value, attrType)
			f.emit("(if (result externref)")
			f.emit("(then")
			f.emit(fmt.Sprintf("(local.get %s)", tempLocal))
			f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(" "+attr.Name)))
			f.emit("(call $prelude.str_concat)")
			f.emit(")")
			f.emit("(else")
			f.emit(fmt.Sprintf("(local.get %s)", tempLocal))
			f.emit(")")
			f.emit(")")
			continue
		}

		// Attribute name: " attrName=\""
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(" "+attr.Name+"=\"")))
		f.emit("(call $prelude.str_concat)")

		// Attribute value
		if attr.Value != nil {
			f.emitExpr(attr.Value, attrType)
			// Convert to string if not already
			if attrType != nil && attrType.Kind != types.KindString {
				f.emit("(call $prelude.toString)")
			}
			f.emit("(call $prelude.escape_html_attr)")
			f.emit("(call $prelude.str_concat)")
		}

		// Closing quote
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("\"")))
		f.emit("(call $prelude.str_concat)")
	}

	if e.SelfClose {
		// Self-closing tag: " />"
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(" />")))
		f.emit("(call $prelude.str_concat)")
	} else {
		// Close opening tag: ">"
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(">")))
		f.emit("(call $prelude.str_concat)")

		// Emit children
		for _, child := range e.Children {
			f.emitJSXChild(&child)
			f.emit("(call $prelude.str_concat)")
		}

		// Closing tag: "</tag>"
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("</"+e.Tag+">")))
		f.emit("(call $prelude.str_concat)")
	}
}

func (f *funcEmitter) emitJSXComponent(e *ast.JSXElement, info *types.JSXComponentInfo) {
	if info.PropsType == nil {
		f.emit(fmt.Sprintf("(call %s)", f.g.funcImplName(info.Symbol)))
		return
	}

	var childFragment ast.Expr
	if info.PropsType.PropType("children") != nil {
		childFragment = &ast.JSXFragment{Children: e.Children, Span: e.Span}
		f.g.checker.ExprTypes[childFragment] = types.String()
	}

	var entries []ast.ObjectEntry
	for _, attr := range e.Attributes {
		valueExpr := attr.Value
		if valueExpr == nil {
			valueExpr = &ast.BoolLit{Value: true, Span: attr.Span}
			f.g.checker.ExprTypes[valueExpr] = types.Bool()
		}
		entries = append(entries, ast.ObjectEntry{
			Kind:  ast.ObjectProp,
			Key:   attr.Name,
			Value: valueExpr,
			Span:  attr.Span,
		})
	}

	if childFragment != nil {
		entries = append(entries, ast.ObjectEntry{
			Kind:  ast.ObjectProp,
			Key:   "children",
			Value: childFragment,
			Span:  e.Span,
		})
	}

	objLit := &ast.ObjectLit{Entries: entries, Span: e.Span}
	f.emitObjectLit(objLit, info.PropsType)
	f.emit(fmt.Sprintf("(call %s)", f.g.funcImplName(info.Symbol)))
}

// emitJSXFragment generates code for a JSX fragment
func (f *funcEmitter) emitJSXFragment(e *ast.JSXFragment) {
	// Start with empty string
	f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("")))

	// Emit children
	for _, child := range e.Children {
		f.emitJSXChild(&child)
		f.emit("(call $prelude.str_concat)")
	}
}

// emitJSXChild generates code for a JSX child element
func (f *funcEmitter) emitJSXChild(child *ast.JSXChild) {
	switch child.Kind {
	case ast.JSXChildText:
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(child.Text)))
	case ast.JSXChildElement:
		if child.Element != nil {
			f.emitJSXElement(child.Element)
		} else {
			f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("")))
		}
	case ast.JSXChildExpr:
		if child.Expr != nil {
			exprType := f.g.checker.ExprTypes[child.Expr]
			f.emitExpr(child.Expr, exprType)
			// Handle array type (join elements with empty string)
			if exprType != nil && exprType.Kind == types.KindArray {
				f.emit("(call $prelude.arr_join)")
			} else if exprType != nil && exprType.Kind != types.KindString {
				// Convert to string if not already
				f.emit("(call $prelude.toString)")
			}
		} else {
			f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal("")))
		}
	}
}
