package types

import (
	"testing"

	"negitoro/internal/ast"
	"negitoro/internal/parser"
)

func TestMapInferenceFromAnyReceiver(t *testing.T) {
	const src = `
import { map, range } from "prelude";

const nums: any = range(1, 3);
const doubled: integer[] = nums.map(function (value: integer): integer {
  return value * 2;
});
const tripled: integer[] = map(nums, function (value: integer): integer {
  return value * 3;
});
`

	mod := mustParseModule(t, "map_any.ngtr", src)
	checker := runChecker(t, mod)

	doubled := findConstDecl(t, mod, "doubled")
	assertArrayElemKind(t, checker.ExprTypes[doubled.Init], KindI64, "doubled")

	tripled := findConstDecl(t, mod, "tripled")
	assertArrayElemKind(t, checker.ExprTypes[tripled.Init], KindI64, "tripled")
}

func TestMapInferenceHandlesObjectAndStringResults(t *testing.T) {
	const src = `
import { map, toString } from "prelude";

const raw: any = [
  { "value": 1 },
  { "value": 2 }
];

const wrapped: { "value": integer }[] = map(raw, function (item: { "value": integer }): { "value": integer } {
  return { "value": item.value };
});

const labels: string[] = raw.map(function (item: { "value": integer }): string {
  return toString(item.value);
});
`

	mod := mustParseModule(t, "map_object.ngtr", src)
	checker := runChecker(t, mod)

	wrapped := findConstDecl(t, mod, "wrapped")
	assertArrayElemKind(t, checker.ExprTypes[wrapped.Init], KindObject, "wrapped")

	labels := findConstDecl(t, mod, "labels")
	assertArrayElemKind(t, checker.ExprTypes[labels.Init], KindString, "labels")
}

func mustParseModule(t *testing.T, path, src string) *ast.Module {
	t.Helper()
	p := parser.New(path, src)
	mod, err := p.ParseModule()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return mod
}

func runChecker(t *testing.T, mod *ast.Module) *Checker {
	t.Helper()
	checker := NewChecker()
	checker.AddModule(mod)
	if !checker.Check() {
		t.Fatalf("type check failed: %v", checker.Errors)
	}
	return checker
}

func findConstDecl(t *testing.T, mod *ast.Module, name string) *ast.ConstDecl {
	t.Helper()
	for _, decl := range mod.Decls {
		if cd, ok := decl.(*ast.ConstDecl); ok && cd.Name == name {
			return cd
		}
	}
	t.Fatalf("const %s not found", name)
	return nil
}

func assertArrayElemKind(t *testing.T, typ *Type, elem Kind, label string) {
	t.Helper()
	if typ == nil {
		t.Fatalf("%s has no inferred type", label)
	}
	if typ.Kind != KindArray {
		t.Fatalf("%s expected array type, got %v", label, typ.Kind)
	}
	if typ.Elem == nil {
		t.Fatalf("%s array element type missing", label)
	}
	if typ.Elem.Kind != elem {
		t.Fatalf("%s expected element kind %v, got %v", label, elem, typ.Elem.Kind)
	}
}
