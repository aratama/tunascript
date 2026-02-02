package formatter

import (
	"fmt"
	"strings"

	"negitoro/internal/ast"
	"negitoro/internal/parser"
)

// Formatter formats Negitoro source code
type Formatter struct {
	indent int
	buf    strings.Builder
}

// New creates a new Formatter
func New() *Formatter {
	return &Formatter{}
}

// Format formats a source file and returns the formatted code
func (f *Formatter) Format(path, src string) (string, error) {
	p := parser.New(path, src)
	mod, err := p.ParseModule()
	if err != nil {
		return "", err
	}
	return f.FormatModule(mod), nil
}

// FormatModule formats an AST module
func (f *Formatter) FormatModule(mod *ast.Module) string {
	f.buf.Reset()
	f.indent = 0

	// Format imports
	for i, imp := range mod.Imports {
		f.formatImport(imp)
		if i < len(mod.Imports)-1 {
			f.buf.WriteString("\n")
		}
	}

	if len(mod.Imports) > 0 && len(mod.Decls) > 0 {
		f.buf.WriteString("\n")
	}

	// Format declarations
	for i, decl := range mod.Decls {
		f.formatDecl(decl)
		if i < len(mod.Decls)-1 {
			f.buf.WriteString("\n")
		}
	}

	return f.buf.String()
}

func (f *Formatter) writeIndent() {
	for i := 0; i < f.indent; i++ {
		f.buf.WriteString("  ")
	}
}

func (f *Formatter) formatImport(imp ast.ImportDecl) {
	f.buf.WriteString("import { ")
	for i, item := range imp.Items {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		if item.IsType {
			f.buf.WriteString("type ")
		}
		f.buf.WriteString(item.Name)
	}
	f.buf.WriteString(" } from \"")
	f.buf.WriteString(imp.From)
	f.buf.WriteString("\";\n")
}

func (f *Formatter) formatDecl(decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.ConstDecl:
		f.formatConstDecl(d)
	case *ast.FuncDecl:
		f.formatFuncDecl(d)
	case *ast.TypeAliasDecl:
		f.formatTypeAliasDecl(d)
	case *ast.TableDecl:
		f.formatTableDecl(d)
	}
}

func (f *Formatter) formatConstDecl(d *ast.ConstDecl) {
	f.writeIndent()
	if d.Export {
		f.buf.WriteString("export ")
	}
	f.buf.WriteString("const ")
	f.buf.WriteString(d.Name)
	f.buf.WriteString(": ")
	f.formatType(d.Type)
	f.buf.WriteString(" = ")
	f.formatExpr(d.Init)
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatFuncDecl(d *ast.FuncDecl) {
	f.writeIndent()
	if d.Export {
		f.buf.WriteString("export ")
	}
	f.buf.WriteString("function ")
	f.buf.WriteString(d.Name)
	f.buf.WriteString("(")
	for i, param := range d.Params {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(param.Name)
		f.buf.WriteString(": ")
		f.formatType(param.Type)
	}
	f.buf.WriteString("): ")
	f.formatType(d.Ret)
	f.buf.WriteString(" ")
	f.formatBlockStmt(d.Body)
	f.buf.WriteString("\n")
}

func (f *Formatter) formatTableDecl(d *ast.TableDecl) {
	f.writeIndent()
	f.buf.WriteString("create_table ")
	f.buf.WriteString(d.Name)
	f.buf.WriteString(" {\n")
	f.indent++
	for i, col := range d.Columns {
		f.writeIndent()
		f.buf.WriteString(col.Name)
		f.buf.WriteString(" ")
		f.buf.WriteString(col.Type)
		if col.Constraints != "" {
			f.buf.WriteString(" ")
			f.buf.WriteString(col.Constraints)
		}
		if i < len(d.Columns)-1 {
			f.buf.WriteString(",")
		}
		f.buf.WriteString("\n")
	}
	f.indent--
	f.writeIndent()
	f.buf.WriteString("}\n")
}

func (f *Formatter) formatTypeAliasDecl(d *ast.TypeAliasDecl) {
	f.writeIndent()
	if d.Export {
		f.buf.WriteString("export ")
	}
	f.buf.WriteString("type ")
	f.buf.WriteString(d.Name)
	f.buf.WriteString(" = ")
	f.formatType(d.Type)
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatBlockStmt(block *ast.BlockStmt) {
	f.buf.WriteString("{\n")
	f.indent++
	for _, stmt := range block.Stmts {
		f.formatStmt(stmt)
	}
	f.indent--
	f.writeIndent()
	f.buf.WriteString("}")
}

func (f *Formatter) formatStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		f.formatConstStmt(s)
	case *ast.DestructureStmt:
		f.formatDestructureStmt(s)
	case *ast.ObjectDestructureStmt:
		f.formatObjectDestructureStmt(s)
	case *ast.ExprStmt:
		f.formatExprStmt(s)
	case *ast.IfStmt:
		f.formatIfStmt(s)
	case *ast.ForOfStmt:
		f.formatForOfStmt(s)
	case *ast.ReturnStmt:
		f.formatReturnStmt(s)
	case *ast.BlockStmt:
		f.writeIndent()
		f.formatBlockStmt(s)
		f.buf.WriteString("\n")
	}
}

func (f *Formatter) formatConstStmt(s *ast.ConstStmt) {
	f.writeIndent()
	f.buf.WriteString("const ")
	f.buf.WriteString(s.Name)
	if s.Type != nil {
		f.buf.WriteString(": ")
		f.formatType(s.Type)
	}
	f.buf.WriteString(" = ")
	f.formatExpr(s.Init)
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatDestructureStmt(s *ast.DestructureStmt) {
	f.writeIndent()
	f.buf.WriteString("const [")
	for i, name := range s.Names {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(name)
		if i < len(s.Types) && s.Types[i] != nil {
			f.buf.WriteString(": ")
			f.formatType(s.Types[i])
		}
	}
	f.buf.WriteString("] = ")
	f.formatExpr(s.Init)
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatObjectDestructureStmt(s *ast.ObjectDestructureStmt) {
	f.writeIndent()
	f.buf.WriteString("const { ")
	for i, key := range s.Keys {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(key)
		if i < len(s.Types) && s.Types[i] != nil {
			f.buf.WriteString(": ")
			f.formatType(s.Types[i])
		}
	}
	f.buf.WriteString(" } = ")
	f.formatExpr(s.Init)
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatExprStmt(s *ast.ExprStmt) {
	f.writeIndent()
	f.formatExpr(s.Expr)
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatIfStmt(s *ast.IfStmt) {
	f.writeIndent()
	f.buf.WriteString("if (")
	f.formatExpr(s.Cond)
	f.buf.WriteString(") ")
	f.formatBlockStmt(s.Then)
	if s.Else != nil {
		f.buf.WriteString(" else ")
		f.formatBlockStmt(s.Else)
	}
	f.buf.WriteString("\n")
}

func (f *Formatter) formatForOfStmt(s *ast.ForOfStmt) {
	f.writeIndent()
	f.buf.WriteString("for (const ")
	f.buf.WriteString(s.VarName)
	if s.VarType != nil {
		f.buf.WriteString(": ")
		f.formatType(s.VarType)
	}
	f.buf.WriteString(" of ")
	f.formatExpr(s.Iter)
	f.buf.WriteString(") ")
	f.formatBlockStmt(s.Body)
	f.buf.WriteString("\n")
}

func (f *Formatter) formatReturnStmt(s *ast.ReturnStmt) {
	f.writeIndent()
	f.buf.WriteString("return")
	if s.Value != nil {
		f.buf.WriteString(" ")
		f.formatExpr(s.Value)
	}
	f.buf.WriteString(";\n")
}

func (f *Formatter) formatExpr(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		f.buf.WriteString(e.Name)
	case *ast.IntLit:
		f.buf.WriteString(fmt.Sprintf("%d", e.Value))
	case *ast.FloatLit:
		f.buf.WriteString(fmt.Sprintf("%g", e.Value))
	case *ast.BoolLit:
		if e.Value {
			f.buf.WriteString("true")
		} else {
			f.buf.WriteString("false")
		}
	case *ast.StringLit:
		f.buf.WriteString("\"")
		f.buf.WriteString(escapeString(e.Value))
		f.buf.WriteString("\"")
	case *ast.ArrayLit:
		f.formatArrayLit(e)
	case *ast.ObjectLit:
		f.formatObjectLit(e)
	case *ast.CallExpr:
		f.formatCallExpr(e)
	case *ast.MemberExpr:
		f.formatExpr(e.Object)
		f.buf.WriteString(".")
		f.buf.WriteString(e.Property)
	case *ast.IndexExpr:
		f.formatExpr(e.Array)
		f.buf.WriteString("[")
		f.formatExpr(e.Index)
		f.buf.WriteString("]")
	case *ast.UnaryExpr:
		f.buf.WriteString(e.Op)
		f.formatExpr(e.Expr)
	case *ast.BinaryExpr:
		f.formatBinaryExpr(e)
	case *ast.TernaryExpr:
		f.formatExpr(e.Cond)
		f.buf.WriteString(" ? ")
		f.formatExpr(e.Then)
		f.buf.WriteString(" : ")
		f.formatExpr(e.Else)
	case *ast.SwitchExpr:
		f.formatSwitchExpr(e)
	case *ast.BlockExpr:
		f.formatBlockExpr(e)
	case *ast.ArrowFunc:
		f.formatArrowFunc(e)
	case *ast.SQLExpr:
		f.formatSQLExpr(e)
	case *ast.JSXElement:
		f.formatJSXElement(e)
	case *ast.JSXFragment:
		f.formatJSXFragment(e)
	}
}

func (f *Formatter) formatJSXElement(e *ast.JSXElement) {
	f.buf.WriteString("<")
	f.buf.WriteString(e.Tag)
	for _, attr := range e.Attributes {
		f.buf.WriteString(" ")
		f.buf.WriteString(attr.Name)
		if attr.Value != nil {
			f.buf.WriteString("=")
			if strLit, ok := attr.Value.(*ast.StringLit); ok {
				f.buf.WriteString("\"")
				f.buf.WriteString(escapeString(strLit.Value))
				f.buf.WriteString("\"")
			} else {
				f.buf.WriteString("{")
				f.formatExpr(attr.Value)
				f.buf.WriteString("}")
			}
		}
	}
	if e.SelfClose {
		f.buf.WriteString(" />")
		return
	}
	f.buf.WriteString(">")

	// Special handling for <style> tag - format CSS content
	if e.Tag == "style" && len(e.Children) == 1 && e.Children[0].Kind == ast.JSXChildText {
		cssText := e.Children[0].Text
		formattedCSS := f.formatCSS(cssText)
		f.indent++
		f.buf.WriteString("\n")
		f.buf.WriteString(formattedCSS)
		f.indent--
		f.buf.WriteString("\n")
		f.writeIndent()
		f.buf.WriteString("</")
		f.buf.WriteString(e.Tag)
		f.buf.WriteString(">")
		return
	}

	// Check if we have element children (not just text/expr)
	hasElementChildren := false
	for _, child := range e.Children {
		if child.Kind == ast.JSXChildElement {
			hasElementChildren = true
			break
		}
	}

	if hasElementChildren {
		f.indent++
		for _, child := range e.Children {
			f.buf.WriteString("\n")
			f.writeIndent()
			f.formatJSXChild(child)
		}
		f.indent--
		f.buf.WriteString("\n")
		f.writeIndent()
	} else {
		for _, child := range e.Children {
			f.formatJSXChild(child)
		}
	}

	f.buf.WriteString("</")
	f.buf.WriteString(e.Tag)
	f.buf.WriteString(">")
}

// formatCSS formats CSS content with proper indentation
func (f *Formatter) formatCSS(css string) string {
	var result strings.Builder
	css = strings.TrimSpace(css)

	// Base indent for CSS content (one level deeper than <style> tag)
	baseIndent := f.indentStr() + "  "

	i := 0
	for i < len(css) {
		// Find selector (everything before {)
		selectorStart := i
		for i < len(css) && css[i] != '{' {
			i++
		}
		if i >= len(css) {
			break
		}

		selector := strings.TrimSpace(css[selectorStart:i])
		i++ // skip '{'

		// Find properties (everything before })
		propsStart := i
		braceCount := 1
		for i < len(css) && braceCount > 0 {
			if css[i] == '{' {
				braceCount++
			} else if css[i] == '}' {
				braceCount--
			}
			i++
		}
		propsEnd := i - 1
		propsStr := strings.TrimSpace(css[propsStart:propsEnd])

		// Write selector
		result.WriteString(baseIndent)
		result.WriteString(selector)
		result.WriteString(" {\n")

		// Parse and write properties
		props := strings.Split(propsStr, ";")
		for _, prop := range props {
			prop = strings.TrimSpace(prop)
			if prop == "" {
				continue
			}
			result.WriteString(baseIndent)
			result.WriteString("  ")
			result.WriteString(prop)
			result.WriteString(";\n")
		}

		// Write closing brace
		result.WriteString(baseIndent)
		result.WriteString("}")
		if i < len(css) {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// indentStr returns the current indentation as a string
func (f *Formatter) indentStr() string {
	return strings.Repeat("  ", f.indent)
}

func (f *Formatter) formatJSXFragment(e *ast.JSXFragment) {
	f.buf.WriteString("<>")

	// Check if we have element children
	hasElementChildren := false
	for _, child := range e.Children {
		if child.Kind == ast.JSXChildElement {
			hasElementChildren = true
			break
		}
	}

	if hasElementChildren {
		f.indent++
		for _, child := range e.Children {
			f.buf.WriteString("\n")
			f.writeIndent()
			f.formatJSXChild(child)
		}
		f.indent--
		f.buf.WriteString("\n")
		f.writeIndent()
	} else {
		for _, child := range e.Children {
			f.formatJSXChild(child)
		}
	}

	f.buf.WriteString("</>")
}

func (f *Formatter) formatJSXChild(child ast.JSXChild) {
	switch child.Kind {
	case ast.JSXChildText:
		f.buf.WriteString(child.Text)
	case ast.JSXChildExpr:
		f.buf.WriteString("{")
		f.formatExpr(child.Expr)
		f.buf.WriteString("}")
	case ast.JSXChildElement:
		f.formatJSXElement(child.Element)
	}
}

func (f *Formatter) formatArrayLit(e *ast.ArrayLit) {
	f.buf.WriteString("[")
	for i, entry := range e.Entries {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		if entry.Kind == ast.ArraySpread {
			f.buf.WriteString("...")
		}
		f.formatExpr(entry.Value)
	}
	f.buf.WriteString("]")
}

func (f *Formatter) formatObjectLit(e *ast.ObjectLit) {
	if len(e.Entries) == 0 {
		f.buf.WriteString("{}")
		return
	}
	f.buf.WriteString("{ ")
	for i, entry := range e.Entries {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		if entry.Kind == ast.ObjectSpread {
			f.buf.WriteString("...")
			f.formatExpr(entry.Value)
		} else {
			f.buf.WriteString("\"")
			f.buf.WriteString(entry.Key)
			f.buf.WriteString("\": ")
			f.formatExpr(entry.Value)
		}
	}
	f.buf.WriteString(" }")
}

func (f *Formatter) formatCallExpr(e *ast.CallExpr) {
	f.formatExpr(e.Callee)
	f.buf.WriteString("(")

	// Check if any argument is a JSX element or fragment
	hasJSXArg := false
	for _, arg := range e.Args {
		if _, ok := arg.(*ast.JSXElement); ok {
			hasJSXArg = true
			break
		}
		if _, ok := arg.(*ast.JSXFragment); ok {
			hasJSXArg = true
			break
		}
	}

	if hasJSXArg {
		f.buf.WriteString("\n")
		f.indent++
		for i, arg := range e.Args {
			if i > 0 {
				f.buf.WriteString(",\n")
			}
			f.writeIndent()
			f.formatExpr(arg)
		}
		f.buf.WriteString("\n")
		f.indent--
		f.writeIndent()
	} else {
		for i, arg := range e.Args {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.formatExpr(arg)
		}
	}

	f.buf.WriteString(")")
}

func (f *Formatter) formatBinaryExpr(e *ast.BinaryExpr) {
	needParens := false
	if _, ok := e.Left.(*ast.BinaryExpr); ok {
		needParens = true
	}
	if needParens {
		f.buf.WriteString("(")
	}
	f.formatExpr(e.Left)
	if needParens {
		f.buf.WriteString(")")
	}
	f.buf.WriteString(" ")
	f.buf.WriteString(e.Op)
	f.buf.WriteString(" ")
	needParens = false
	if _, ok := e.Right.(*ast.BinaryExpr); ok {
		needParens = true
	}
	if needParens {
		f.buf.WriteString("(")
	}
	f.formatExpr(e.Right)
	if needParens {
		f.buf.WriteString(")")
	}
}

func (f *Formatter) formatSwitchExpr(e *ast.SwitchExpr) {
	f.buf.WriteString("switch (")
	f.formatExpr(e.Value)
	f.buf.WriteString(") {\n")
	f.indent++
	for _, c := range e.Cases {
		f.writeIndent()
		f.buf.WriteString("case ")
		f.formatExpr(c.Pattern)
		f.buf.WriteString(":")
		if _, isBlock := c.Body.(*ast.BlockExpr); isBlock {
			f.buf.WriteString(" ")
			f.formatExpr(c.Body)
		} else {
			f.buf.WriteString("\n")
			f.indent++
			f.writeIndent()
			f.formatExpr(c.Body)
			f.indent--
		}
		f.buf.WriteString("\n")
	}
	if e.Default != nil {
		f.writeIndent()
		f.buf.WriteString("default:")
		if _, isBlock := e.Default.(*ast.BlockExpr); isBlock {
			f.buf.WriteString(" ")
			f.formatExpr(e.Default)
		} else {
			f.buf.WriteString("\n")
			f.indent++
			f.writeIndent()
			f.formatExpr(e.Default)
			f.indent--
		}
		f.buf.WriteString("\n")
	}
	f.indent--
	f.writeIndent()
	f.buf.WriteString("}")
}

func (f *Formatter) formatBlockExpr(e *ast.BlockExpr) {
	f.buf.WriteString("{\n")
	f.indent++
	for _, stmt := range e.Stmts {
		f.formatStmt(stmt)
	}
	f.indent--
	f.writeIndent()
	f.buf.WriteString("}")
}

func (f *Formatter) formatArrowFunc(e *ast.ArrowFunc) {
	f.buf.WriteString("function (")
	for i, param := range e.Params {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(param.Name)
		f.buf.WriteString(": ")
		f.formatType(param.Type)
	}
	f.buf.WriteString("): ")
	f.formatType(e.Ret)
	if e.Body != nil {
		f.buf.WriteString(" ")
		f.formatBlockStmt(e.Body)
	} else if e.Expr != nil {
		f.buf.WriteString(" => ")
		f.formatExpr(e.Expr)
	}
}

func (f *Formatter) formatSQLExpr(e *ast.SQLExpr) {
	switch e.Kind {
	case ast.SQLQueryExecute:
		f.buf.WriteString("execute")
	case ast.SQLQueryFetchOne:
		f.buf.WriteString("fetch_one")
	case ast.SQLQueryFetchOptional:
		f.buf.WriteString("fetch_optional")
	case ast.SQLQueryFetch:
		f.buf.WriteString("fetch")
	case ast.SQLQueryFetchAll:
		f.buf.WriteString("fetch_all")
	}
	f.buf.WriteString(" {\n")
	f.indent++
	f.writeIndent()
	// Format SQL with parameter placeholders replaced back
	query := e.Query
	for i, param := range e.Params {
		query = strings.Replace(query, "?", "{"+f.exprToString(param)+"}", 1)
		_ = i
	}
	f.buf.WriteString(strings.TrimSpace(query))
	f.buf.WriteString("\n")
	f.indent--
	f.writeIndent()
	f.buf.WriteString("}")
}

func (f *Formatter) exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Name
	default:
		// For complex expressions, return a placeholder
		var tmp strings.Builder
		tmpFmt := &Formatter{buf: tmp}
		tmpFmt.formatExpr(expr)
		return tmpFmt.buf.String()
	}
}

func (f *Formatter) formatType(t ast.TypeExpr) {
	if t == nil {
		return
	}
	switch ty := t.(type) {
	case *ast.NamedType:
		f.buf.WriteString(ty.Name)
	case *ast.ArrayType:
		f.formatType(ty.Elem)
		f.buf.WriteString("[]")
	case *ast.TupleType:
		f.buf.WriteString("[")
		for i, elem := range ty.Elems {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.formatType(elem)
		}
		f.buf.WriteString("]")
	case *ast.FuncType:
		f.buf.WriteString("(")
		for i, param := range ty.Params {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.buf.WriteString(param.Name)
			f.buf.WriteString(": ")
			f.formatType(param.Type)
		}
		f.buf.WriteString(") => ")
		f.formatType(ty.Ret)
	case *ast.ObjectType:
		f.buf.WriteString("{ ")
		for i, prop := range ty.Props {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.buf.WriteString("\"")
			f.buf.WriteString(prop.Key)
			f.buf.WriteString("\": ")
			f.formatType(prop.Type)
		}
		f.buf.WriteString(" }")
	}
}

func escapeString(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\t':
			result.WriteString("\\t")
		case '\r':
			result.WriteString("\\r")
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
}
