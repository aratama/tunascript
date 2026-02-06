package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tuna/internal/ast"
	"tuna/internal/parser"
	"tuna/internal/types"
)

type Result struct {
	Wat  string
	Wasm []byte
}

type Compiler struct {
	Modules    map[string]*ast.Module
	preludeWAT string
}

func New() *Compiler {
	return &Compiler{Modules: map[string]*ast.Module{}}
}

func (c *Compiler) Compile(entry string) (*Result, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
		return nil, err
	}
	if err := c.loadPrelude(abs); err != nil {
		return nil, err
	}
	if err := c.loadRecursive(abs); err != nil {
		return nil, err
	}
	checker := types.NewChecker()
	for _, mod := range c.Modules {
		checker.AddModule(mod)
	}
	if !checker.Check() {
		return nil, checker.Errors[0]
	}
	gen := NewGenerator(checker)
	gen.SetPreludeWAT(c.preludeWAT)
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

func (c *Compiler) loadPrelude(entryAbs string) error {
	if _, ok := c.Modules["prelude"]; ok {
		return nil
	}
	preludePath, ok := findPreludePath(filepath.Dir(entryAbs))
	if !ok {
		return nil
	}
	src, err := os.ReadFile(preludePath)
	if err != nil {
		return err
	}
	watPath := filepath.Join(filepath.Dir(preludePath), "prelude.wat")
	watSrc, watErr := os.ReadFile(watPath)
	if watErr != nil {
		if !os.IsNotExist(watErr) {
			return watErr
		}
		c.preludeWAT = ""
	} else {
		c.preludeWAT = string(watSrc)
	}
	p := parser.New(preludePath, string(src))
	mod, err := p.ParseModule()
	if err != nil {
		return err
	}
	mod.Path = "prelude"

	dir := filepath.Dir(preludePath)
	for i := range mod.Imports {
		imp := &mod.Imports[i]
		if imp.From == "prelude" || imp.From == "json" || imp.From == "array" || imp.From == "runtime" || imp.From == "http" || imp.From == "sqlite" || imp.From == "file" {
			continue
		}
		resolved, err := resolveImport(dir, imp.From)
		if err != nil {
			return err
		}
		imp.From = resolved
		if err := c.loadRecursive(resolved); err != nil {
			return err
		}
	}

	c.Modules["prelude"] = mod
	return nil
}

func (c *Compiler) loadRecursive(path string) error {
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
		if imp.From == "prelude" || imp.From == "json" || imp.From == "array" || imp.From == "runtime" || imp.From == "http" || imp.From == "sqlite" || imp.From == "file" {
			continue
		}
		resolved, err := resolveImport(dir, imp.From)
		if err != nil {
			return err
		}
		imp.From = resolved
		if err := c.loadRecursive(resolved); err != nil {
			return err
		}
	}
	c.Modules[path] = mod
	return nil
}

func findPreludePath(startDir string) (string, bool) {
	searchUp := func(dir string) (string, bool) {
		cur := dir
		for {
			candidate := filepath.Join(cur, "lib", "prelude.tuna")
			if _, err := os.Stat(candidate); err == nil {
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
	if path, ok := searchUp(startDir); ok {
		return path, true
	}
	if cwd, err := os.Getwd(); err == nil {
		if path, ok := searchUp(cwd); ok {
			return path, true
		}
	}
	return "", false
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

func resolveImport(baseDir, spec string) (string, error) {
	if spec == "prelude" {
		return spec, nil
	}
	if spec == "json" {
		return spec, nil
	}
	if spec == "array" {
		return spec, nil
	}
	if spec == "runtime" {
		return spec, nil
	}
	if spec == "http" {
		return spec, nil
	}
	if spec == "sqlite" {
		return spec, nil
	}
	if spec == "file" {
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
