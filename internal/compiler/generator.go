package compiler

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"negitoro/internal/ast"
	"negitoro/internal/types"
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
	// Generate and intern table definitions JSON if any tables exist
	if len(g.tableDefs) > 0 {
		jsonStr := g.generateTableDefsJSON()
		g.internString(jsonStr)
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
		g.collectStringsType(s.VarType)
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
		for _, arg := range e.Args {
			g.collectStringsExpr(arg)
		}
	case *ast.MemberExpr:
		g.collectStringsExpr(e.Object)
		g.internString(e.Property)
	case *ast.IndexExpr:
		g.collectStringsExpr(e.Array)
		g.collectStringsExpr(e.Index)
	case *ast.UnaryExpr:
		g.collectStringsExpr(e.Expr)
	case *ast.BinaryExpr:
		g.collectStringsExpr(e.Left)
		g.collectStringsExpr(e.Right)
	case *ast.TernaryExpr:
		g.collectStringsExpr(e.Cond)
		g.collectStringsExpr(e.Then)
		g.collectStringsExpr(e.Else)
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
			if len(e.Args) >= 3 {
				handlerArg = e.Args[2]
			}
		} else if member, ok := e.Callee.(*ast.MemberExpr); ok && member.Property == "addRoute" {
			// Method-style: server.addRoute("/", handler)
			isAddRoute = true
			if len(e.Args) >= 2 {
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
	case *ast.BinaryExpr:
		g.collectFunctionNamesExpr(e.Left)
		g.collectFunctionNamesExpr(e.Right)
	case *ast.TernaryExpr:
		g.collectFunctionNamesExpr(e.Cond)
		g.collectFunctionNamesExpr(e.Then)
		g.collectFunctionNamesExpr(e.Else)
	case *ast.SwitchExpr:
		g.collectFunctionNamesExpr(e.Value)
		for _, cas := range e.Cases {
			g.collectFunctionNamesExpr(cas.Pattern)
			g.collectFunctionNamesExpr(cas.Body)
		}
		if e.Default != nil {
			g.collectFunctionNamesExpr(e.Default)
		}
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
	imports := []string{
		"str_from_utf8",
		"str_concat",
		"str_eq",
		"val_from_i64",
		"val_from_f64",
		"val_from_bool",
		"val_to_i64",
		"val_to_f64",
		"val_to_bool",
		"obj_new",
		"obj_set",
		"obj_get",
		"arr_new",
		"arr_set",
		"arr_get",
		"arr_len",
		"arr_join",
		"range",
		"val_eq",
		"print",
		"stringify",
		"parse",
		"toString",
		"sql_exec",
		"sql_query",
		"sql_fetch_one",
		"sql_fetch_optional",
		"sql_execute",
		"intern_string",
		"db_save",
		"db_open",
		"get_args",
		"register_tables",
		"http_create_server",
		"http_add_route",
		"http_listen",
		"http_response_text",
		"http_response_html",
		"http_response_text_str",
		"http_response_html_str",
		"http_response_json",
		"http_response_redirect",
		"http_response_redirect_str",
		"http_get_path",
		"http_get_method",
	}
	for _, name := range imports {
		sig := importSig(name)
		w.line(fmt.Sprintf("(import \"prelude\" \"%s\" %s)", name, sig))
	}
}

func importSig(name string) string {
	switch name {
	case "str_from_utf8":
		return "(func $prelude.str_from_utf8 (param i32 i32) (result i32))"
	case "str_concat":
		return "(func $prelude.str_concat (param i32 i32) (result i32))"
	case "str_eq":
		return "(func $prelude.str_eq (param i32 i32) (result i32))"
	case "val_from_i64":
		return "(func $prelude.val_from_i64 (param i64) (result i32))"
	case "val_from_f64":
		return "(func $prelude.val_from_f64 (param f64) (result i32))"
	case "val_from_bool":
		return "(func $prelude.val_from_bool (param i32) (result i32))"
	case "val_to_i64":
		return "(func $prelude.val_to_i64 (param i32) (result i64))"
	case "val_to_f64":
		return "(func $prelude.val_to_f64 (param i32) (result f64))"
	case "val_to_bool":
		return "(func $prelude.val_to_bool (param i32) (result i32))"
	case "obj_new":
		return "(func $prelude.obj_new (param i32) (result i32))"
	case "obj_set":
		return "(func $prelude.obj_set (param i32 i32 i32))"
	case "obj_get":
		return "(func $prelude.obj_get (param i32 i32) (result i32))"
	case "arr_new":
		return "(func $prelude.arr_new (param i32) (result i32))"
	case "arr_set":
		return "(func $prelude.arr_set (param i32 i32 i32))"
	case "arr_get":
		return "(func $prelude.arr_get (param i32 i32) (result i32))"
	case "arr_len":
		return "(func $prelude.arr_len (param i32) (result i32))"
	case "arr_join":
		return "(func $prelude.arr_join (param i32) (result i32))"
	case "range":
		return "(func $prelude.range (param i64 i64) (result i32))"
	case "val_eq":
		return "(func $prelude.val_eq (param i32 i32) (result i32))"
	case "print":
		return "(func $prelude.print (param i32))"
	case "stringify":
		return "(func $prelude.stringify (param i32) (result i32))"
	case "parse":
		return "(func $prelude.parse (param i32) (result i32))"
	case "toString":
		return "(func $prelude.toString (param i32) (result i32))"
	case "sql_exec":
		return "(func $prelude.sql_exec (param i32 i32) (result i32))"
	case "sql_query":
		return "(func $prelude.sql_query (param i32 i32 i32) (result i32))"
	case "sql_fetch_one":
		return "(func $prelude.sql_fetch_one (param i32 i32 i32) (result i32))"
	case "sql_fetch_optional":
		return "(func $prelude.sql_fetch_optional (param i32 i32 i32) (result i32))"
	case "sql_execute":
		return "(func $prelude.sql_execute (param i32 i32 i32))"
	case "intern_string":
		return "(func $prelude.intern_string (param i32 i32) (result i32))"
	case "db_save":
		return "(func $prelude.db_save (param i32))"
	case "db_open":
		return "(func $prelude.db_open (param i32))"
	case "get_args":
		return "(func $prelude.get_args (result i32))"
	case "register_tables":
		return "(func $prelude.register_tables (param i32 i32))"
	case "http_create_server":
		return "(func $prelude.http_create_server (result i32))"
	case "http_add_route":
		return "(func $prelude.http_add_route (param i32 i32 i32 i32))"
	case "http_listen":
		return "(func $prelude.http_listen (param i32 i32 i32))"
	case "http_response_text":
		return "(func $prelude.http_response_text (param i32 i32) (result i32))"
	case "http_response_html":
		return "(func $prelude.http_response_html (param i32 i32) (result i32))"
	case "http_response_text_str":
		return "(func $prelude.http_response_text_str (param i32) (result i32))"
	case "http_response_html_str":
		return "(func $prelude.http_response_html_str (param i32) (result i32))"
	case "http_response_json":
		return "(func $prelude.http_response_json (param i32) (result i32))"
	case "http_response_redirect":
		return "(func $prelude.http_response_redirect (param i32 i32) (result i32))"
	case "http_response_redirect_str":
		return "(func $prelude.http_response_redirect_str (param i32) (result i32))"
	case "http_get_path":
		return "(func $prelude.http_get_path (param i32) (result i32))"
	case "http_get_method":
		return "(func $prelude.http_get_method (param i32) (result i32))"
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
	for _, d := range g.stringData {
		w.line(fmt.Sprintf("(global %s (mut i32) (i32.const 0))", d.name))
	}
	for _, mod := range g.modules {
		for _, sym := range mod.Top {
			if sym.Kind != types.SymVar {
				continue
			}
			wasmName := g.globalNames[sym]
			wasmType := wasmType(sym.Type)
			w.line(fmt.Sprintf("(global %s (mut %s) (%s.const 0))", wasmName, wasmType, wasmType))
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
			sym := mod.Top[cd.Name]
			emitter.emitExpr(cd.Init, sym.Type)
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
		w.line(fmt.Sprintf("(call %s)", g.funcImplName(mainSym)))
	}
	w.indent--
	w.line(")")
	w.line("(export \"_start\" (func $_start))")
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
		if len(sym.Type.Params) != 0 || sym.Type.Ret.Kind != types.KindVoid {
			return nil
		}
		if !g.funcExports[sym] {
			return nil
		}
		return sym
	}
	return nil
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
	case types.KindString, types.KindArray, types.KindTuple, types.KindObject:
		return "i32"
	default:
		return "i32"
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
		f.emitExpr(s.Init, f.g.checker.ExprTypes[s.Init])
		local := f.addLocal(s.Name, f.g.checker.ExprTypes[s.Init])
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
			f.emitExpr(s.Value, f.g.checker.ExprTypes[s.Value])
		}
		f.emit("return")
	case *ast.IfStmt:
		f.emitExpr(s.Cond, f.g.checker.ExprTypes[s.Cond])
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
	arrLocal := f.addLocalRaw("i32")
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
		
		// Get element from array/tuple
		f.emit(fmt.Sprintf("(local.get %s)", arrLocal))
		f.emit(fmt.Sprintf("(i32.const %d)", i))
		f.emit("(call $prelude.arr_get)")
		f.emitUnboxIfPrimitive(elemType)
		
		// Store in local variable
		local := f.addLocal(name, elemType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
		f.bindLocal(name, local)
	}
}

func (f *funcEmitter) emitObjectDestructure(s *ast.ObjectDestructureStmt) {
	initType := f.g.checker.ExprTypes[s.Init]
	
	// Store the object in a temporary local
	objLocal := f.addLocalRaw("i32")
	f.emitExpr(s.Init, initType)
	f.emit(fmt.Sprintf("(local.set %s)", objLocal))
	
	// Extract each property and bind to a local
	for _, key := range s.Keys {
		// Get property type
		propType := initType.PropType(key)
		
		// Get property from object
		f.emit(fmt.Sprintf("(local.get %s)", objLocal))
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(key)))
		f.emit("(call $prelude.obj_get)")
		f.emitUnboxIfPrimitive(propType)
		
		// Store in local variable
		local := f.addLocal(key, propType)
		f.emit(fmt.Sprintf("(local.set %s)", local))
		f.bindLocal(key, local)
	}
}

func (f *funcEmitter) emitForOf(s *ast.ForOfStmt) {
	iterType := f.g.checker.ExprTypes[s.Iter]
	elem := elemType(iterType)
	arrLocal := f.addLocalRaw("i32")
	lenLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	valLocal := f.addLocalRaw("i32")

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
	loopVarLocal := f.addLocal(s.VarName, elem)
	f.bindLocal(s.VarName, loopVarLocal)
	f.emit(fmt.Sprintf("(local.get %s)", valLocal))
	f.emitUnboxIfPrimitive(elem)
	f.emit(fmt.Sprintf("(local.set %s)", loopVarLocal))
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
	case *ast.StringLit:
		f.emit(fmt.Sprintf("(global.get %s)", f.g.stringGlobal(e.Value)))
	case *ast.IdentExpr:
		if local, ok := f.lookup(e.Name); ok {
			f.emit(fmt.Sprintf("(local.get %s)", local))
			return
		}
		if sym := f.g.checker.IdentSymbols[e]; sym != nil && sym.Kind == types.SymVar {
			f.emit(fmt.Sprintf("(global.get %s)", f.g.globalNames[sym]))
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
	case *ast.BinaryExpr:
		f.emitBinaryExpr(e, t)
	case *ast.TernaryExpr:
		f.emitTernaryExpr(e, t)
	case *ast.SwitchExpr:
		f.emitSwitchExpr(e, t)
	case *ast.BlockExpr:
		f.emitBlockExpr(e)
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
		f.emit("(call $prelude.arr_get)")
		f.emitUnboxIfPrimitive(t)
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

func (f *funcEmitter) emitTernaryExpr(e *ast.TernaryExpr, t *types.Type) {
	// 条件を評価
	f.emitExpr(e.Cond, f.g.checker.ExprTypes[e.Cond])
	// if-then-else構造を生成
	wt := wasmType(t)
	f.emit(fmt.Sprintf("(if (result %s)", wt))
	f.indent++
	f.emit("(then")
	f.indent++
	f.emitExpr(e.Then, t)
	f.indent--
	f.emit(")")
	f.emit("(else")
	f.indent++
	f.emitExpr(e.Else, t)
	f.indent--
	f.emit(")")
	f.indent--
	f.emit(")")
}

func (f *funcEmitter) emitBlockExpr(e *ast.BlockExpr) {
	// Execute all statements in the block (returns void)
	f.pushScope()
	for _, stmt := range e.Stmts {
		f.emitStmt(stmt)
	}
	f.popScope()
}

func (f *funcEmitter) emitSwitchExpr(e *ast.SwitchExpr, t *types.Type) {
	valueType := f.g.checker.ExprTypes[e.Value]

	// Store the switch value in a local variable
	valueLocal := f.addLocalRaw(wasmType(valueType))
	f.emitExpr(e.Value, valueType)
	f.emit(fmt.Sprintf("(local.set %s)", valueLocal))

	// Generate nested if-else chain
	f.emitSwitchCases(e.Cases, e.Default, valueLocal, valueType, t, 0)
}

func (f *funcEmitter) emitSwitchCases(cases []ast.SwitchCase, defaultExpr ast.Expr, valueLocal string, valueType, resultType *types.Type, idx int) {
	isVoid := resultType.Kind == types.KindVoid

	if idx >= len(cases) {
		// No more cases, emit default
		if defaultExpr != nil {
			f.emitExpr(defaultExpr, resultType)
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
			}
		}
		return
	}

	cas := cases[idx]
	patternType := f.g.checker.ExprTypes[cas.Pattern]

	// Emit comparison: value == pattern
	f.emit(fmt.Sprintf("(local.get %s)", valueLocal))
	f.emitExpr(cas.Pattern, patternType)

	// Emit equality check based on type
	if valueType.Kind == types.KindString || valueType.Kind == types.KindObject || valueType.Kind == types.KindArray || valueType.Kind == types.KindTuple {
		f.emit("(call $prelude.val_eq)")
	} else {
		f.emit(eqOp(valueType))
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
	f.emitExpr(cas.Body, resultType)
	f.indent--
	f.emit(")")
	f.emit("(else")
	f.indent++
	// Recursively emit remaining cases
	f.emitSwitchCases(cases, defaultExpr, valueLocal, valueType, resultType, idx+1)
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
		if leftType.Kind == types.KindString || leftType.Kind == types.KindObject || leftType.Kind == types.KindArray || leftType.Kind == types.KindTuple {
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
	if isPreludeName(name) {
		f.emitPreludeCall(name, call, t)
		return
	}
	for _, arg := range call.Args {
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
	}
	sym := f.g.checker.IdentSymbols[ident]
	if sym != nil {
		f.emit(fmt.Sprintf("(call %s)", f.g.funcImplName(sym)))
	}
}

// emitMethodCallExpr emits code for method-style calls: obj.func(args) => func(obj, args)
func (f *funcEmitter) emitMethodCallExpr(call *ast.CallExpr, member *ast.MemberExpr, t *types.Type) {
	funcName := member.Property
	
	if isPreludeName(funcName) {
		// Create a synthetic call with object as first argument
		allArgs := append([]ast.Expr{member.Object}, call.Args...)
		syntheticCall := &ast.CallExpr{
			Callee: &ast.IdentExpr{Name: funcName, Span: member.Span},
			Args:   allArgs,
			Span:   call.Span,
		}
		f.emitPreludeCall(funcName, syntheticCall, t)
		return
	}
	
	// Emit object (first argument)
	f.emitExpr(member.Object, f.g.checker.ExprTypes[member.Object])
	
	// Emit remaining arguments
	for _, arg := range call.Args {
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
	}
	
	// Look up the function symbol
	// We need to find the symbol for funcName
	// Since checkMethodCall validated this, we can assume it exists
	for _, mod := range f.g.modules {
		if sym, ok := mod.Top[funcName]; ok && sym.Kind == types.SymFunc {
			f.emit(fmt.Sprintf("(call %s)", f.g.funcImplName(sym)))
			return
		}
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

func (f *funcEmitter) emitPreludeCall(name string, call *ast.CallExpr, t *types.Type) {
	switch name {
	case "print":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[arg])
		f.emit("(call $prelude.print)")
	case "stringify":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[arg])
		f.emit("(call $prelude.stringify)")
	case "parse":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emit("(call $prelude.parse)")
		f.emitUnboxIfPrimitive(t)
	case "toString":
		arg := call.Args[0]
		f.emitExpr(arg, f.g.checker.ExprTypes[arg])
		f.emitBoxIfPrimitive(f.g.checker.ExprTypes[arg])
		f.emit("(call $prelude.toString)")
	case "range":
		start := call.Args[0]
		end := call.Args[1]
		f.emitExpr(start, f.g.checker.ExprTypes[start])
		f.emitExpr(end, f.g.checker.ExprTypes[end])
		f.emit("(call $prelude.range)")
	case "length":
		f.emitLength(call)
	case "map":
		f.emitMap(call, t)
	case "reduce":
		f.emitReduce(call, t)
	case "dbSave":
		f.emitDbSave(call)
	case "dbOpen":
		f.emitDbOpen(call)
	case "getArgs":
		f.emit("(call $prelude.get_args)")
	case "sqlQuery":
		f.emitSqlQuery(call)
	case "createServer":
		f.emit("(call $prelude.http_create_server)")
	case "listen":
		f.emitHttpListen(call)
	case "addRoute":
		f.emitHttpAddRoute(call)
	case "responseText":
		f.emitHttpResponseText(call)
	case "responseHtml":
		f.emitHttpResponseHtml(call)
	case "responseJson":
		f.emitHttpResponseJson(call)
	case "responseRedirect":
		f.emitHttpResponseRedirect(call)
	case "getPath":
		f.emitHttpGetPath(call)
	case "getMethod":
		f.emitHttpGetMethod(call)
	}
}

func (f *funcEmitter) emitDbSave(call *ast.CallExpr) {
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
				f.emit("(call $prelude.db_save)")
				return
			}
		}
		// For non-literal strings (already a handle), use directly
		f.emitExpr(arg, argType)
		f.emit("(call $prelude.db_save)")
	}
}

func (f *funcEmitter) emitDbOpen(call *ast.CallExpr) {
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
				f.emit("(call $prelude.db_open)")
				return
			}
		}
		// For non-literal strings (already a handle), use directly
		f.emitExpr(arg, argType)
		f.emit("(call $prelude.db_open)")
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

func (f *funcEmitter) emitHttpListen(call *ast.CallExpr) {
	serverArg := call.Args[0]
	portArg := call.Args[1]
	serverType := f.g.checker.ExprTypes[serverArg]

	// Emit server handle
	f.emitExpr(serverArg, serverType)

	// Handle port string
	if strLit, ok := portArg.(*ast.StringLit); ok {
		f.g.internString(strLit.Value)
		datum := f.g.stringDataByValue(strLit.Value)
		if datum != nil {
			f.emit(fmt.Sprintf("(i32.const %d)", datum.offset))
			f.emit(fmt.Sprintf("(i32.const %d)", datum.length))
		}
	}

	f.emit("(call $prelude.http_listen)")
}

func (f *funcEmitter) emitHttpAddRoute(call *ast.CallExpr) {
	serverArg := call.Args[0]
	pathArg := call.Args[1]
	handlerArg := call.Args[2]
	serverType := f.g.checker.ExprTypes[serverArg]
	handlerType := f.g.checker.ExprTypes[handlerArg]

	// Emit server handle
	f.emitExpr(serverArg, serverType)

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

	f.emit("(call $prelude.http_add_route)")
}

func (f *funcEmitter) emitHttpResponseText(call *ast.CallExpr) {
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
		f.emit("(call $prelude.http_response_text)")
	} else {
		// For non-literal strings (e.g., JSX), use the string handle version
		f.emitExpr(textArg, textType)
		f.emit("(call $prelude.http_response_text_str)")
	}
}

func (f *funcEmitter) emitHttpResponseHtml(call *ast.CallExpr) {
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
		f.emit("(call $prelude.http_response_html)")
	} else {
		// For non-literal strings (e.g., JSX), use the string handle version
		f.emitExpr(htmlArg, htmlType)
		f.emit("(call $prelude.http_response_html_str)")
	}
}

func (f *funcEmitter) emitHttpResponseJson(call *ast.CallExpr) {
	dataArg := call.Args[0]
	dataType := f.g.checker.ExprTypes[dataArg]
	f.emitExpr(dataArg, dataType)
	f.emit("(call $prelude.http_response_json)")
}

func (f *funcEmitter) emitHttpResponseRedirect(call *ast.CallExpr) {
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
		f.emit("(call $prelude.http_response_redirect)")
	} else {
		// For non-literal strings, use the string handle version
		f.emitExpr(urlArg, urlType)
		f.emit("(call $prelude.http_response_redirect_str)")
	}
}

func (f *funcEmitter) emitHttpGetPath(call *ast.CallExpr) {
	reqArg := call.Args[0]
	reqType := f.g.checker.ExprTypes[reqArg]
	f.emitExpr(reqArg, reqType)
	f.emit("(call $prelude.http_get_path)")
}

func (f *funcEmitter) emitHttpGetMethod(call *ast.CallExpr) {
	reqArg := call.Args[0]
	reqType := f.g.checker.ExprTypes[reqArg]
	f.emitExpr(reqArg, reqType)
	f.emit("(call $prelude.http_get_method)")
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
	arrLocal := f.addLocalRaw("i32")
	lenLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	resultLocal := f.addLocalRaw("i32")
	valueLocal := f.addLocalRaw("i32")

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

func (f *funcEmitter) emitReduce(call *ast.CallExpr, t *types.Type) {
	arrExpr := call.Args[0]
	fnExpr := call.Args[1]
	initExpr := call.Args[2]
	arrType := f.g.checker.ExprTypes[arrExpr]
	fnName, fnType := f.resolveFunctionExpr(fnExpr)
	accType := fnType.Ret
	arrLocal := f.addLocalRaw("i32")
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
		info := entryInfo{entry: entry, handleLocal: f.addLocalRaw("i32")}
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
	arrLocal := f.addLocalRaw("i32")
	idxLocal := f.addLocalRaw("i32")
	valueLocal := f.addLocalRaw("i32")
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
	paramsLocal := f.addLocalRaw("i32")
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
		return "i32"
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
	return "i32.eq"
}

func cmpOp(t *types.Type, op string) string {
	if t.Kind == types.KindI64 {
		return "i64." + op + "_s"
	}
	return "f64." + op
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

func isPreludeName(name string) bool {
	switch name {
	case "print", "stringify", "parse", "toString", "range", "length", "map", "reduce", "dbSave", "dbOpen", "getArgs", "sqlQuery",
		"createServer", "listen", "addRoute", "responseText", "responseHtml", "responseJson", "responseRedirect", "getPath", "getMethod":
		return true
	default:
		return false
	}
}

// emitJSXElement generates code for a JSX element (converts to string concatenation)
func (f *funcEmitter) emitJSXElement(e *ast.JSXElement) {
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
			tempLocal := f.addLocalRaw("i32")
			f.emit(fmt.Sprintf("(local.set %s)", tempLocal)) // save str
			f.emitExpr(attr.Value, attrType)
			f.emit("(if (result i32)")
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
