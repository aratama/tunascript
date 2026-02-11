package types

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"tuna/internal/ast"
	"tuna/internal/parser"
)

func TestMapInferenceFromArrayLiteral(t *testing.T) {
	const src = `
import { map } from "array"

function double(value: i64): i64 {
  return value * 2
}

function triple(value: i64): i64 {
  return value * 3
}

const nums: i64[] = [1, 2, 3]
const doubled: i64[] = nums.map(double)
const tripled: i64[] = map(nums, triple)
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
import { to_string } from "prelude"
import { map } from "array"

function wrap(item: { value: i64 }): { value: i64 } {
  return { "value": item.value }
}

function label(item: { value: i64 }): string {
  return to_string(item.value)
}

const raw: { value: i64 }[] = [
  { "value": 1 },
  { "value": 2 }
]

const wrapped: { value: i64 }[] = map(raw, wrap)
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

function sumValues(acc: i64, value: i64): i64 {
  return acc + value
}

const nums: i64[] = range(1, 5)
const sum: i64 = nums.reduce(sumValues, 0)
const sum2: i64 = reduce(nums, sumValues, 0)
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

function isEven(value: i64): boolean {
  return value % 2 == 0
}

const nums: i64[] = range(1, 6)
const evens: i64[] = nums.filter(isEven)
const evens2: i64[] = filter(nums, isEven)
`

	mod := mustParseModule(t, "filter_infer.tuna", src)
	checker := runChecker(t, mod)

	evens := findConstDecl(t, mod, "evens")
	assertArrayElemKind(t, checker.ExprTypes[evens.Init], KindI64, "evens")

	evens2 := findConstDecl(t, mod, "evens2")
	assertArrayElemKind(t, checker.ExprTypes[evens2.Init], KindI64, "evens2")
}

func TestGenericFallbackMethodInference(t *testing.T) {
	const src = `
function fallback<T>(value: T | error, defaultValue: T): T {
  return defaultValue
}

const maybeOk: string | error = error("bad")
const resolved: string = maybeOk.fallback("")
const fromValue: string = fallback("hello", "")
const fromError: string = fallback(error("boom"), "")
`

	mod := mustParseModule(t, "fallback_method_infer.tuna", src)
	checker := runChecker(t, mod)

	resolved := findConstDecl(t, mod, "resolved")
	assertTypeKind(t, checker.ExprTypes[resolved.Init], KindString, "resolved")

	fromValue := findConstDecl(t, mod, "fromValue")
	assertTypeKind(t, checker.ExprTypes[fromValue.Init], KindString, "fromValue")

	fromError := findConstDecl(t, mod, "fromError")
	assertTypeKind(t, checker.ExprTypes[fromError.Init], KindString, "fromError")
}

func TestSwitchExpressionInferenceInGenericCall(t *testing.T) {
	const src = `
function passthrough<T>(value: T): T {
  return value
}

const data: i64 | string = 42
const msg: string = passthrough(switch (data) {
  case num as i64: "number"
  case text as string: "string"
})
`

	mod := mustParseModule(t, "switch_generic_call_infer.tuna", src)
	checker := runChecker(t, mod)

	msg := findConstDecl(t, mod, "msg")
	assertTypeKind(t, checker.ExprTypes[msg.Init], KindString, "msg")
}

func TestExternFunctionDeclaration(t *testing.T) {
	const src = `
extern function string_length(str: string): i64

const n: i64 = string_length("hello")
`

	mod := mustParseModule(t, "extern_decl.tuna", src)
	checker := NewChecker()
	checker.AddModule(mod)
	if checker.Check() {
		t.Fatalf("expected extern declaration outside prelude to fail")
	}
	if !hasErrorContaining(checker.Errors, "extern function is only supported in builtin modules") {
		t.Fatalf("expected extern restriction error, got: %v", checker.Errors)
	}
}

func TestExternFunctionDeclarationInPrelude(t *testing.T) {
	const src = `
export extern function string_length(str: string): i64

const n: i64 = string_length("hello")
`

	mod := mustParseModule(t, "prelude", src)
	checker := runChecker(t, mod)

	n := findConstDecl(t, mod, "n")
	assertTypeKind(t, checker.ExprTypes[n.Init], KindI64, "n")
}

func TestExternFunctionDeclarationShortTypeInPrelude(t *testing.T) {
	const src = `
export extern function rawLen(ptr: i32, len: i32): i32
`

	mod := mustParseModule(t, "prelude", src)
	checker := runChecker(t, mod)

	sym := checker.Modules[mod.Path].Top["rawLen"]
	if sym == nil || sym.Type == nil || sym.Type.Kind != KindFunc {
		t.Fatalf("rawLen symbol not found or not function")
	}
	if len(sym.Type.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(sym.Type.Params))
	}
	if sym.Type.Params[0].Kind != KindI32 || sym.Type.Params[1].Kind != KindI32 {
		t.Fatalf("expected i32 params, got %v and %v", sym.Type.Params[0].Kind, sym.Type.Params[1].Kind)
	}
	if sym.Type.Ret == nil || sym.Type.Ret.Kind != KindI32 {
		t.Fatalf("expected i32 return, got %v", sym.Type.Ret)
	}
}

func TestUnionSwitchAs(t *testing.T) {
	const src = `
const v: i64 | string = 42
const msg: string = switch (v) {
  case v as i64: "int"
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
const a: i64 | undefined = if (true) { 42 }
const b: i64 | string = if (true) { 42 } else { "42" }
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
const x: i64 = 1

function f(x: i64): void {
  const y: i64 = 2
  if (true) {
    const y: i64 = 3
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
const x: i64 | string = 1
const msg: string = switch (x) {
  case x as i64: "int"
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
const x: i64 | string = 1
const y: i64 = 100
const msg: string = switch (x) {
  case y as i64: "int"
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
	if err := addLibModules(checker); err != nil {
		t.Fatalf("failed to load lib modules: %v", err)
	}
	checker.AddModule(mod)
	if !checker.Check() {
		t.Fatalf("type check failed: %v", checker.Errors)
	}
	return checker
}

func addLibModules(checker *Checker) error {
	libDir, err := findLibDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".tuna" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		path := filepath.Join(libDir, entry.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p := parser.New(path, string(src))
		mod, err := p.ParseModule()
		if err != nil {
			return err
		}
		mod.Path = name
		checker.AddModule(mod)
	}
	return nil
}

func findLibDir() (string, error) {
	searchUp := func(dir string) (string, bool) {
		cur := dir
		for {
			candidate := filepath.Join(cur, "lib")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
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
	if pwd := os.Getenv("PWD"); pwd != "" {
		if path, ok := searchUp(pwd); ok {
			return path, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if path, ok := searchUp(cwd); ok {
			return path, nil
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		if path, ok := searchUp(filepath.Dir(file)); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("lib directory not found")
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
