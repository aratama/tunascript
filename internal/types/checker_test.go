package types

import (
	"strings"
	"testing"

	"tuna/internal/ast"
	"tuna/internal/parser"
)

func TestMapInferenceFromArrayLiteral(t *testing.T) {
	const src = `
import { map } from "array"

function double(value: integer): integer {
  return value * 2
}

function triple(value: integer): integer {
  return value * 3
}

const nums: integer[] = [1, 2, 3]
const doubled: integer[] = nums.map(double)
const tripled: integer[] = map(nums, triple)
`

	mod := mustParseModule(t, "map_array_literal.tuna", src)
	checker := runChecker(t, mod)

	doubled := findConstDecl(t, mod, "doubled")
	assertArrayElemKind(t, checker.ExprTypes[doubled.Init], KindI64, "doubled")

	tripled := findConstDecl(t, mod, "tripled")
	assertArrayElemKind(t, checker.ExprTypes[tripled.Init], KindI64, "tripled")
}

func TestMapInferenceHandlesObjectAndStringResults(t *testing.T) {
	const src = `
import { toString } from "prelude"
import { map } from "array"

function wrap(item: { value: integer }): { value: integer } {
  return { "value": item.value }
}

function label(item: { value: integer }): string {
  return toString(item.value)
}

const raw: { value: integer }[] = [
  { "value": 1 },
  { "value": 2 }
]

const wrapped: { value: integer }[] = map(raw, wrap)
const labels: string[] = raw.map(label)
`

	mod := mustParseModule(t, "map_object.tuna", src)
	checker := runChecker(t, mod)

	wrapped := findConstDecl(t, mod, "wrapped")
	assertArrayElemKind(t, checker.ExprTypes[wrapped.Init], KindObject, "wrapped")

	labels := findConstDecl(t, mod, "labels")
	assertArrayElemKind(t, checker.ExprTypes[labels.Init], KindString, "labels")
}

func TestReduceInferenceFromRange(t *testing.T) {
	const src = `
import { reduce, range } from "array"

function sumValues(acc: integer, value: integer): integer {
  return acc + value
}

const nums: integer[] = range(1, 5)
const sum: integer = nums.reduce(sumValues, 0)
const sum2: integer = reduce(nums, sumValues, 0)
`

	mod := mustParseModule(t, "reduce_infer.tuna", src)
	checker := runChecker(t, mod)

	sum := findConstDecl(t, mod, "sum")
	assertTypeKind(t, checker.ExprTypes[sum.Init], KindI64, "sum")

	sum2 := findConstDecl(t, mod, "sum2")
	assertTypeKind(t, checker.ExprTypes[sum2.Init], KindI64, "sum2")
}

func TestFilterInferenceFromRange(t *testing.T) {
	const src = `
import { filter, range } from "array"

function isEven(value: integer): boolean {
  return value % 2 == 0
}

const nums: integer[] = range(1, 6)
const evens: integer[] = nums.filter(isEven)
const evens2: integer[] = filter(nums, isEven)
`

	mod := mustParseModule(t, "filter_infer.tuna", src)
	checker := runChecker(t, mod)

	evens := findConstDecl(t, mod, "evens")
	assertArrayElemKind(t, checker.ExprTypes[evens.Init], KindI64, "evens")

	evens2 := findConstDecl(t, mod, "evens2")
	assertArrayElemKind(t, checker.ExprTypes[evens2.Init], KindI64, "evens2")
}

func TestUnionSwitchAs(t *testing.T) {
	const src = `
const v: integer | string = 42
const msg: string = switch (v) {
  case v as integer: "int"
  case v as string: "str"
}
`

	mod := mustParseModule(t, "union_switch.tuna", src)
	checker := runChecker(t, mod)

	sym := checker.Modules[mod.Path].Top["v"]
	if sym == nil || sym.Type == nil {
		t.Fatalf("union symbol not found")
	}
	if sym.Type.Kind != KindUnion {
		t.Fatalf("expected union type, got %v", sym.Type.Kind)
	}
	if len(sym.Type.Union) != 2 {
		t.Fatalf("expected 2 union members, got %d", len(sym.Type.Union))
	}

	msg := findConstDecl(t, mod, "msg")
	assertTypeKind(t, checker.ExprTypes[msg.Init], KindString, "msg")
}

func TestIfExprTypeInference(t *testing.T) {
	const src = `
const a: integer | undefined = if (true) { 42 }
const b: integer | string = if (true) { 42 } else { "42" }
`

	mod := mustParseModule(t, "if_expr.tuna", src)
	checker := runChecker(t, mod)

	a := findConstDecl(t, mod, "a")
	assertUnionContainsBaseKind(t, checker.ExprTypes[a.Init], KindI64, "a")
	assertUnionContainsKind(t, checker.ExprTypes[a.Init], KindUndefined, "a")

	b := findConstDecl(t, mod, "b")
	assertUnionContainsBaseKind(t, checker.ExprTypes[b.Init], KindI64, "b")
	assertUnionContainsBaseKind(t, checker.ExprTypes[b.Init], KindString, "b")
}

func TestShadowingIsCompileError(t *testing.T) {
	const src = `
const x: integer = 1

function f(x: integer): void {
  const y: integer = 2
  if (true) {
    const y: integer = 3
  }
}
`
	mod := mustParseModule(t, "shadowing_error.tuna", src)
	checker := NewChecker()
	checker.AddModule(mod)
	if checker.Check() {
		t.Fatalf("expected shadowing error, but check succeeded")
	}
	if !hasErrorContaining(checker.Errors, "shadowing is not allowed: x") {
		t.Fatalf("expected shadowing error for x, got: %v", checker.Errors)
	}
	if !hasErrorContaining(checker.Errors, "shadowing is not allowed: y") {
		t.Fatalf("expected shadowing error for y, got: %v", checker.Errors)
	}
}

func TestSwitchAsSameNameAsSwitchTargetIsNotShadowing(t *testing.T) {
	const src = `
const x: integer | string = 1
const msg: string = switch (x) {
  case x as integer: "int"
  case x as string: x
}
`
	mod := mustParseModule(t, "switch_as_same_name.tuna", src)
	checker := runChecker(t, mod)

	msg := findConstDecl(t, mod, "msg")
	assertTypeKind(t, checker.ExprTypes[msg.Init], KindString, "msg")
}

func TestSwitchAsDifferentNameShadowingIsCompileError(t *testing.T) {
	const src = `
const x: integer | string = 1
const y: integer = 100
const msg: string = switch (x) {
  case y as integer: "int"
  case s as string: s
}
`
	mod := mustParseModule(t, "switch_as_shadowing_error.tuna", src)
	checker := NewChecker()
	checker.AddModule(mod)
	if checker.Check() {
		t.Fatalf("expected shadowing error, but check succeeded")
	}
	if !hasErrorContaining(checker.Errors, "shadowing is not allowed: y") {
		t.Fatalf("expected shadowing error for y, got: %v", checker.Errors)
	}
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

func assertTypeKind(t *testing.T, typ *Type, kind Kind, label string) {
	t.Helper()
	if typ == nil {
		t.Fatalf("%s has no inferred type", label)
	}
	if typ.Kind != kind {
		t.Fatalf("%s expected kind %v, got %v", label, kind, typ.Kind)
	}
}

func assertUnionContainsKind(t *testing.T, typ *Type, kind Kind, label string) {
	t.Helper()
	if typ == nil {
		t.Fatalf("%s has no inferred type", label)
	}
	if typ.Kind != KindUnion {
		t.Fatalf("%s expected union type, got %v", label, typ.Kind)
	}
	for _, member := range typ.Union {
		if member != nil && member.Kind == kind {
			return
		}
	}
	t.Fatalf("%s expected union to contain %v", label, kind)
}

func assertUnionContainsBaseKind(t *testing.T, typ *Type, kind Kind, label string) {
	t.Helper()
	if typ == nil {
		t.Fatalf("%s has no inferred type", label)
	}
	if typ.Kind != KindUnion {
		t.Fatalf("%s expected union type, got %v", label, typ.Kind)
	}
	for _, member := range typ.Union {
		if member != nil && member.Kind == kind && !member.Literal {
			return
		}
	}
	t.Fatalf("%s expected union to contain base %v", label, kind)
}

func hasErrorContaining(errs []error, needle string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), needle) {
			return true
		}
	}
	return false
}
