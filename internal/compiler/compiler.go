package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"tuna/internal/ast"
	"tuna/internal/parser"
	"tuna/internal/types"
)

type Result struct {
	Wat  string
	Wasm []byte
}

type Backend string

const (
	BackendGC   Backend = "gc"
	BackendHost Backend = "host"
)

type Compiler struct {
	Modules    map[string]*ast.Module
	backend    Backend
	libDir     string
	libModules map[string]string
	moduleWAT  map[string]string
}

func New() *Compiler {
	return &Compiler{
		Modules: map[string]*ast.Module{},
		backend: BackendGC,
	}
}

func (c *Compiler) SetBackend(backend Backend) error {
	switch backend {
	case BackendGC, BackendHost:
		c.backend = backend
		return nil
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

func (c *Compiler) Compile(entry string) (*Result, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
		return nil, err
	}
	if err := c.ensureLibIndex(abs); err != nil {
		return nil, err
	}
	if err := c.loadBuiltinModule("prelude"); err != nil {
		return nil, err
	}
	if err := c.loadRecursive(abs); err != nil {
		return nil, err
	}
	if c.needsSqliteModule() {
		if err := c.loadBuiltinModule("sqlite"); err != nil {
			return nil, err
		}
	}
	checker := types.NewChecker()
	for _, mod := range c.Modules {
		checker.AddModule(mod)
	}
	if !checker.Check() {
		return nil, checker.Errors[0]
	}
	gen := NewGenerator(checker)
	gen.SetModuleWATs(c.moduleWAT)
	gen.SetBackend(c.backend)
	wat, err := gen.Generate(abs)
	if err != nil {
		return nil, err
	}
	wasm, err := gen.WatToWasm(wat)
	if err != nil {
		return nil, err
	}
	return &Result{Wat: wat, Wasm: wasm}, nil
}

func (c *Compiler) ensureLibIndex(entryAbs string) error {
	if c.libModules != nil {
		return nil
	}
	libDir, ok := findLibDir(filepath.Dir(entryAbs))
	if !ok {
		c.libModules = map[string]string{}
		return nil
	}
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return err
	}
	libModules := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".tuna" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		libModules[name] = filepath.Join(libDir, entry.Name())
	}
	c.libDir = libDir
	c.libModules = libModules
	return nil
}

func (c *Compiler) loadBuiltinModule(name string) error {
	if _, ok := c.Modules[name]; ok {
		return nil
	}
	if c.moduleNeedsHostBridge(name) {
		if err := c.loadBuiltinModule("interop"); err != nil {
			return err
		}
	}
	if c.libModules == nil {
		c.libModules = map[string]string{}
	}
	path, ok := c.libModules[name]
	if !ok {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if c.libDir != "" {
		if c.moduleWAT == nil {
			c.moduleWAT = map[string]string{}
		}
		if _, ok := c.moduleWAT[name]; !ok {
			watSrc, watErr := c.loadBuiltinModuleWAT(name)
			if watErr != nil {
				return watErr
			}
			if watSrc != "" {
				c.moduleWAT[name] = watSrc
			}
		}
	}
	p := parser.New(path, string(src))
	mod, err := p.ParseModule()
	if err != nil {
		return err
	}
	if watSrc := c.moduleWAT[name]; watSrc != "" {
		filterDeclsForWAT(mod, moduleDefinedInWAT(name, watSrc))
	}
	mod.Path = name
	c.Modules[name] = mod

	dir := filepath.Dir(path)
	for i := range mod.Imports {
		imp := &mod.Imports[i]
		if c.isBuiltinModuleName(imp.From) {
			if err := c.loadBuiltinModule(imp.From); err != nil {
				return err
			}
			continue
		}
		resolved, err := c.resolveImport(dir, imp.From)
		if err != nil {
			return err
		}
		imp.From = resolved
		if c.isBuiltinModuleName(resolved) {
			continue
		}
		if err := c.loadRecursive(resolved); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) isBuiltinModuleName(name string) bool {
	if name == "" || c.libModules == nil {
		return false
	}
	_, ok := c.libModules[name]
	return ok
}

func (c *Compiler) loadRecursive(path string) error {
	if c.isBuiltinModuleName(path) {
		return c.loadBuiltinModule(path)
	}
	if _, ok := c.Modules[path]; ok {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" && ext != ".tuna" && ext != ".ts" {
		return c.loadTextModule(path)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p := parser.New(path, string(src))
	mod, err := p.ParseModule()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	for i := range mod.Imports {
		imp := &mod.Imports[i]
		resolved, err := c.resolveImport(dir, imp.From)
		if err != nil {
			return err
		}
		imp.From = resolved
		if c.isBuiltinModuleName(resolved) {
			continue
		}
		if err := c.loadRecursive(resolved); err != nil {
			return err
		}
	}
	c.Modules[path] = mod
	return nil
}

func findLibDir(startDir string) (string, bool) {
	isLibDir := func(dir string) bool {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return false
		}
		preludePath := filepath.Join(dir, "prelude.tuna")
		if _, err := os.Stat(preludePath); err != nil {
			return false
		}
		return true
	}
	searchUp := func(dir string) (string, bool) {
		cur := dir
		for {
			candidate := filepath.Join(cur, "lib")
			if isLibDir(candidate) {
				return candidate, true
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
		return "", false
	}
	if env := os.Getenv("TUNASCRIPT_LIB_DIR"); env != "" {
		if isLibDir(env) {
			return env, true
		}
	}
	if exe, err := os.Executable(); err == nil {
		if path, ok := searchUp(filepath.Dir(exe)); ok {
			return path, true
		}
	}
	if pwd := os.Getenv("PWD"); pwd != "" {
		if path, ok := searchUp(pwd); ok {
			return path, true
		}
	}
	if path, ok := searchUp(startDir); ok {
		return path, true
	}
	if cwd, err := os.Getwd(); err == nil {
		if path, ok := searchUp(cwd); ok {
			return path, true
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		if path, found := searchUp(filepath.Dir(file)); found {
			return path, true
		}
	}
	return "", false
}

func (c *Compiler) needsSqliteModule() bool {
	for _, mod := range c.Modules {
		if mod == nil || mod.Path == "sqlite" || mod.Path == "server" || mod.Path == "host" {
			continue
		}
		if moduleNeedsSqlite(mod) {
			return true
		}
	}
	return false
}

func moduleNeedsSqlite(mod *ast.Module) bool {
	if mod == nil {
		return false
	}
	for _, decl := range mod.Decls {
		if _, ok := decl.(*ast.TableDecl); ok {
			return true
		}
		if declNeedsSqlite(decl) {
			return true
		}
	}
	return false
}

func declNeedsSqlite(decl ast.Decl) bool {
	switch d := decl.(type) {
	case *ast.ConstDecl:
		return exprNeedsSqlite(d.Init)
	case *ast.FuncDecl:
		return blockNeedsSqlite(d.Body)
	default:
		return false
	}
}

func blockNeedsSqlite(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Stmts {
		if stmtNeedsSqlite(stmt) {
			return true
		}
	}
	return false
}

func stmtNeedsSqlite(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		return exprNeedsSqlite(s.Init)
	case *ast.DestructureStmt:
		return exprNeedsSqlite(s.Init)
	case *ast.ObjectDestructureStmt:
		return exprNeedsSqlite(s.Init)
	case *ast.ExprStmt:
		return exprNeedsSqlite(s.Expr)
	case *ast.IfStmt:
		return exprNeedsSqlite(s.Cond) || blockNeedsSqlite(s.Then) || blockNeedsSqlite(s.Else)
	case *ast.ForOfStmt:
		return exprNeedsSqlite(s.Iter) || blockNeedsSqlite(s.Body)
	case *ast.ReturnStmt:
		return exprNeedsSqlite(s.Value)
	case *ast.BlockStmt:
		return blockNeedsSqlite(s)
	default:
		return false
	}
}

func exprNeedsSqlite(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SQLExpr:
		return true
	case *ast.TemplateLit:
		for _, ex := range e.Exprs {
			if exprNeedsSqlite(ex) {
				return true
			}
		}
	case *ast.ArrayLit:
		for _, entry := range e.Entries {
			if exprNeedsSqlite(entry.Value) {
				return true
			}
		}
	case *ast.ObjectLit:
		for _, entry := range e.Entries {
			if exprNeedsSqlite(entry.Value) {
				return true
			}
		}
	case *ast.CallExpr:
		if exprNeedsSqlite(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if exprNeedsSqlite(arg) {
				return true
			}
		}
	case *ast.MemberExpr:
		return exprNeedsSqlite(e.Object)
	case *ast.IndexExpr:
		return exprNeedsSqlite(e.Array) || exprNeedsSqlite(e.Index)
	case *ast.TryExpr:
		return exprNeedsSqlite(e.Expr)
	case *ast.UnaryExpr:
		return exprNeedsSqlite(e.Expr)
	case *ast.AsExpr:
		return exprNeedsSqlite(e.Expr)
	case *ast.BinaryExpr:
		return exprNeedsSqlite(e.Left) || exprNeedsSqlite(e.Right)
	case *ast.IfExpr:
		return exprNeedsSqlite(e.Cond) || exprNeedsSqlite(e.Then) || exprNeedsSqlite(e.Else)
	case *ast.SwitchExpr:
		if exprNeedsSqlite(e.Value) {
			return true
		}
		for _, c := range e.Cases {
			if exprNeedsSqlite(c.Pattern) || exprNeedsSqlite(c.Body) {
				return true
			}
		}
		return exprNeedsSqlite(e.Default)
	case *ast.BlockExpr:
		for _, stmt := range e.Stmts {
			if stmtNeedsSqlite(stmt) {
				return true
			}
		}
	case *ast.ArrowFunc:
		if e.Expr != nil {
			return exprNeedsSqlite(e.Expr)
		}
		return blockNeedsSqlite(e.Body)
	case *ast.JSXElement:
		return jsxElementNeedsSqlite(e)
	case *ast.JSXFragment:
		return jsxFragmentNeedsSqlite(e)
	default:
		return false
	}
	return false
}

func jsxElementNeedsSqlite(elem *ast.JSXElement) bool {
	if elem == nil {
		return false
	}
	for _, attr := range elem.Attributes {
		if attr.Value != nil && exprNeedsSqlite(attr.Value) {
			return true
		}
	}
	for _, child := range elem.Children {
		switch child.Kind {
		case ast.JSXChildElement:
			if jsxElementNeedsSqlite(child.Element) {
				return true
			}
		case ast.JSXChildExpr:
			if exprNeedsSqlite(child.Expr) {
				return true
			}
		}
	}
	return false
}

func jsxFragmentNeedsSqlite(frag *ast.JSXFragment) bool {
	if frag == nil {
		return false
	}
	for _, child := range frag.Children {
		switch child.Kind {
		case ast.JSXChildElement:
			if jsxElementNeedsSqlite(child.Element) {
				return true
			}
		case ast.JSXChildExpr:
			if exprNeedsSqlite(child.Expr) {
				return true
			}
		}
	}
	return false
}

func (c *Compiler) loadTextModule(path string) error {
	if _, ok := c.Modules[path]; ok {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mod := &ast.Module{
		Path: path,
		Decls: []ast.Decl{
			&ast.ConstDecl{
				Name:   "default",
				Export: true,
				Type:   &ast.NamedType{Name: "string"},
				Init:   &ast.StringLit{Value: string(src)},
			},
		},
	}
	c.Modules[path] = mod
	return nil
}

func (c *Compiler) resolveImport(baseDir, spec string) (string, error) {
	if c.isBuiltinModuleName(spec) {
		if err := c.loadBuiltinModule(spec); err != nil {
			return "", err
		}
		return spec, nil
	}
	if len(spec) >= 2 && (spec[:2] == "./" || spec[:3] == "../") {
		path := filepath.Join(baseDir, spec)
		if filepath.Ext(path) == "" {
			path += ".ts"
		}
		return filepath.Clean(path), nil
	}
	return "", fmt.Errorf("unsupported import: %s", spec)
}

func (c *Compiler) moduleNeedsHostBridge(name string) bool {
	switch name {
	case "server", "runtime", "sqlite":
		return true
	case "http", "file":
		return c.backend == BackendHost
	default:
		return false
	}
}

func (c *Compiler) loadBuiltinModuleWAT(name string) (string, error) {
	if c.libDir == "" {
		return "", nil
	}
	candidates := make([]string, 0, 2)
	if c.backend == BackendHost {
		candidates = append(candidates, filepath.Join(c.libDir, name+".host.wat"))
	}
	candidates = append(candidates, filepath.Join(c.libDir, name+".wat"))
	for _, candidate := range candidates {
		src, err := os.ReadFile(candidate)
		if err == nil {
			return string(src), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", nil
}

func moduleDefinedInWAT(moduleName, src string) map[string]bool {
	defined := map[string]bool{}
	if strings.TrimSpace(src) == "" {
		return defined
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`\(\s*func\s+\$%s\.([A-Za-z0-9_]+)`, regexp.QuoteMeta(moduleName)))
	for _, match := range pattern.FindAllStringSubmatch(src, -1) {
		if len(match) > 1 {
			defined[match[1]] = true
		}
	}
	return defined
}

func filterDeclsForWAT(mod *ast.Module, defined map[string]bool) {
	if mod == nil {
		return
	}
	allowedExterns := map[string]bool{}
	disallowedExterns := map[string]bool{}
	for _, decl := range mod.Decls {
		ext, ok := decl.(*ast.ExternFuncDecl)
		if !ok {
			continue
		}
		if defined[ext.Name] || externDeclNumericOnly(ext) {
			allowedExterns[ext.Name] = true
		} else {
			disallowedExterns[ext.Name] = true
		}
	}

	filtered := make([]ast.Decl, 0, len(mod.Decls))
	for _, decl := range mod.Decls {
		switch d := decl.(type) {
		case *ast.ExternFuncDecl:
			if allowedExterns[d.Name] {
				filtered = append(filtered, decl)
			}
		case *ast.FuncDecl:
			if funcDeclCallsDisallowedExtern(d, disallowedExterns) {
				continue
			}
			filtered = append(filtered, decl)
		default:
			filtered = append(filtered, decl)
		}
	}
	mod.Decls = filtered
}

func externDeclNumericOnly(d *ast.ExternFuncDecl) bool {
	if d == nil {
		return false
	}
	for _, p := range d.Params {
		if !typeExprNumericOnly(p.Type) {
			return false
		}
	}
	if d.Ret == nil {
		return true
	}
	return typeExprNumericOnly(d.Ret)
}

func typeExprNumericOnly(t ast.TypeExpr) bool {
	if t == nil {
		return true
	}
	if named, ok := t.(*ast.NamedType); ok {
		switch named.Name {
		case "integer", "number", "boolean", "short":
			return true
		default:
			return false
		}
	}
	return false
}

func funcDeclCallsDisallowedExtern(fn *ast.FuncDecl, disallowed map[string]bool) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	return blockCallsDisallowed(fn.Body, disallowed)
}

func blockCallsDisallowed(block *ast.BlockStmt, disallowed map[string]bool) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Stmts {
		if stmtCallsDisallowed(stmt, disallowed) {
			return true
		}
	}
	return false
}

func stmtCallsDisallowed(stmt ast.Stmt, disallowed map[string]bool) bool {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		return exprCallsDisallowed(s.Init, disallowed)
	case *ast.DestructureStmt:
		return exprCallsDisallowed(s.Init, disallowed)
	case *ast.ObjectDestructureStmt:
		return exprCallsDisallowed(s.Init, disallowed)
	case *ast.ExprStmt:
		return exprCallsDisallowed(s.Expr, disallowed)
	case *ast.ReturnStmt:
		return exprCallsDisallowed(s.Value, disallowed)
	case *ast.IfStmt:
		if exprCallsDisallowed(s.Cond, disallowed) {
			return true
		}
		if blockCallsDisallowed(s.Then, disallowed) {
			return true
		}
		if s.Else != nil && blockCallsDisallowed(s.Else, disallowed) {
			return true
		}
	case *ast.ForOfStmt:
		if exprCallsDisallowed(s.Iter, disallowed) {
			return true
		}
		if blockCallsDisallowed(s.Body, disallowed) {
			return true
		}
	case *ast.BlockStmt:
		return blockCallsDisallowed(s, disallowed)
	}
	return false
}

func exprCallsDisallowed(expr ast.Expr, disallowed map[string]bool) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		if ident, ok := e.Callee.(*ast.IdentExpr); ok {
			if disallowed[ident.Name] {
				return true
			}
		}
		if exprCallsDisallowed(e.Callee, disallowed) {
			return true
		}
		for _, arg := range e.Args {
			if exprCallsDisallowed(arg, disallowed) {
				return true
			}
		}
		return false
	case *ast.MemberExpr:
		return exprCallsDisallowed(e.Object, disallowed)
	case *ast.IndexExpr:
		return exprCallsDisallowed(e.Array, disallowed) || exprCallsDisallowed(e.Index, disallowed)
	case *ast.AsExpr:
		return exprCallsDisallowed(e.Expr, disallowed)
	case *ast.TryExpr:
		return exprCallsDisallowed(e.Expr, disallowed)
	case *ast.UnaryExpr:
		return exprCallsDisallowed(e.Expr, disallowed)
	case *ast.BinaryExpr:
		return exprCallsDisallowed(e.Left, disallowed) || exprCallsDisallowed(e.Right, disallowed)
	case *ast.IfExpr:
		if exprCallsDisallowed(e.Cond, disallowed) {
			return true
		}
		if exprCallsDisallowed(e.Then, disallowed) {
			return true
		}
		if e.Else != nil && exprCallsDisallowed(e.Else, disallowed) {
			return true
		}
	case *ast.SwitchExpr:
		if exprCallsDisallowed(e.Value, disallowed) {
			return true
		}
		for _, cas := range e.Cases {
			if exprCallsDisallowed(cas.Pattern, disallowed) {
				return true
			}
			if exprCallsDisallowed(cas.Body, disallowed) {
				return true
			}
		}
		if e.Default != nil && exprCallsDisallowed(e.Default, disallowed) {
			return true
		}
	case *ast.ArrayLit:
		for _, entry := range e.Entries {
			if exprCallsDisallowed(entry.Value, disallowed) {
				return true
			}
		}
	case *ast.ObjectLit:
		for _, entry := range e.Entries {
			if exprCallsDisallowed(entry.Value, disallowed) {
				return true
			}
		}
	case *ast.ArrowFunc:
		if e.Body != nil && blockCallsDisallowed(e.Body, disallowed) {
			return true
		}
		if e.Expr != nil && exprCallsDisallowed(e.Expr, disallowed) {
			return true
		}
	case *ast.TemplateLit:
		for _, part := range e.Exprs {
			if exprCallsDisallowed(part, disallowed) {
				return true
			}
		}
	}
	return false
}
