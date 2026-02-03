package compiler

import (
	"fmt"
	"os"
	"path/filepath"

	"tuna/internal/ast"
	"tuna/internal/parser"
	"tuna/internal/types"
)

type Result struct {
	Wat  string
	Wasm []byte
}

type Compiler struct {
	Modules map[string]*ast.Module
}

func New() *Compiler {
	return &Compiler{Modules: map[string]*ast.Module{}}
}

func (c *Compiler) Compile(entry string) (*Result, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
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

func (c *Compiler) loadRecursive(path string) error {
	if _, ok := c.Modules[path]; ok {
		return nil
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
		if imp.From == "prelude" || imp.From == "http" || imp.From == "sqlite" {
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

func resolveImport(baseDir, spec string) (string, error) {
	if spec == "prelude" {
		return spec, nil
	}
	if spec == "http" {
		return spec, nil
	}
	if spec == "sqlite" {
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
