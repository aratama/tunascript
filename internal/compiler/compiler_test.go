package compiler_test

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"tuna/internal/compiler"
	tunaruntime "tuna/internal/runtime"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func compileAndRun(t *testing.T, files map[string]string, entry string) string {
	t.Helper()
	return compileAndRunWithBackend(t, files, entry, compiler.BackendGC)
}

func compileAndRunWithBackend(t *testing.T, files map[string]string, entry string, backend compiler.Backend) string {
	t.Helper()
	if !runtimeAvailable {
		t.Skip("CGO が無効なためテストをスキップします")
	}
	ensureLibDirEnv(t)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	entryPath := filepath.Join(dir, entry)
	comp := compiler.New()
	if err := comp.SetBackend(backend); err != nil {
		t.Fatal(err)
	}
	res, err := comp.Compile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := tunaruntime.NewRunner()
	out, err := runner.Run(res.Wasm)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ensureLibDirEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("TUNASCRIPT_LIB_DIR") != "" {
		return
	}
	libDir, err := findLibDirForTests()
	if err != nil {
		t.Fatalf("failed to locate lib dir: %v", err)
	}
	t.Setenv("TUNASCRIPT_LIB_DIR", libDir)
}

func findLibDirForTests() (string, error) {
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
	if _, file, _, ok := goruntime.Caller(0); ok {
		if path, ok := searchUp(filepath.Dir(file)); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("lib directory not found")
}

func TestBackendGCBasic(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  log(to_string(42))
}
`,
	}, "main.ts", compiler.BackendGC)
	if out != "42\n" {
		t.Fatalf("output mismatch: got %q, want %q", out, "42\n")
	}
}

func TestBackendGCArrayAndObject(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const base: { x: i64 } = { "x": 41 }
  const obj: { x: i64, y: string } = { ...base, "y": "ok" }
  const xs: i64[] = [1, 2, 3]
  log(to_string(obj.x + 1))
  log(obj.y)
  for (const x: i64 of xs) {
    log(to_string(x))
  }
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "42\nok\n1\n2\n3\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendGCStringOps(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log, to_string, string_length } from "prelude"
export function main(): void {
  const s: string = "こん" + "にちは"
  log(to_string(string_length(s)))
  if (s == "こんにちは") {
    log("eq")
  } else {
    log("neq")
  }
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "5\neq\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendGCHigherOrderCall(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"

function apply(v: i64, fn: (i64) => i64): i64 {
  return fn(v)
}

function add2(v: i64): i64 {
  return v + 2
}

export function main(): void {
  log(to_string(apply(40, add2)))
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "42\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendGCArrayIndexError(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log } from "prelude"

export function main(): void {
  const xs: i64[] = [1, 2]
  const v: i64 | error = xs[9]
  switch (v) {
    case n as i64: log("ok")
    case e as error: log(e.message)
  }
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "index out of range\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendGCEscapeHTMLAttr(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log } from "prelude"

export function main(): void {
  log(<div title={"A&B<\"'"}></div>)
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "<div title=\"A&amp;B&lt;&quot;&#39;\"></div>\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendGCMapReduceLength(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
import { map, reduce, length } from "array"

function double(n: i64): i64 {
  return n * 2
}

function sumValues(acc: i64, v: i64): i64 {
  return acc + v
}

export function main(): void {
  const xs: i64[] = [1, 2, 3]
  const doubled: i64[] = map(xs, double)
  const total: i64 = reduce(doubled, sumValues, 0)
  const size: i64 = length(doubled)
  log(to_string(total))
  log(to_string(size))
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "12\n3\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendGCJSONBridgeDoesNotImportHostJSONFuncs(t *testing.T) {
	ensureLibDirEnv(t)
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "main.tuna")
	src := `import { toJSON, decode } from "json"
import { log } from "prelude"

type User = { name: string }

export function main(): void {
  const p = toJSON("{\"name\":\"ok\"}")
  switch (p) {
    case e as error: {
      log(e.message)
    }
    case j as json: {
      const d = decode<User>(j)
      switch (d) {
        case de as error: {
          log(de.message)
        }
        case u as User: {
          log(u.name)
        }
      }
    }
  }
}
`
	if err := os.WriteFile(entryPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	comp := compiler.New()
	res, err := comp.Compile(entryPath)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	if strings.Contains(res.Wat, `(import "host" "json_stringify"`) ||
		strings.Contains(res.Wat, `(import "host" "json_parse"`) ||
		strings.Contains(res.Wat, `(import "host" "json_toJSON"`) ||
		strings.Contains(res.Wat, `(import "host" "json_decode"`) {
		t.Fatalf("json bridge should not import host::json_* directly")
	}
	if strings.Contains(res.Wat, `(import "json" "stringify"`) ||
		strings.Contains(res.Wat, `(import "json" "parse"`) ||
		strings.Contains(res.Wat, `(import "json" "toJSON"`) ||
		strings.Contains(res.Wat, `(import "json" "decode"`) {
		t.Fatalf("json module should be inlined WAT without json:: imports")
	}
}

func TestBackendGCTupleIndex(t *testing.T) {
	out := compileAndRunWithBackend(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"

export function main(): void {
  const t: [i64, string] = [1, "a"]
  const v0 = switch (t[0]) {
    case n as i64: to_string(n)
    case e as error: e.message
  }
  const v1 = switch (t[1]) {
    case s as string: s
    case e as error: e.message
  }
  log(v0)
  log(v1)
}
`,
	}, "main.ts", compiler.BackendGC)
	want := "1\na\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func compileExpectError(t *testing.T, src string) {
	t.Helper()
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(entryPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	comp := compiler.New()
	_, err := comp.Compile(entryPath)
	if err == nil {
		t.Fatalf("error expected")
	}
}

func compileExpectErrorContains(t *testing.T, src, want string) {
	t.Helper()
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(entryPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	comp := compiler.New()
	_, err := comp.Compile(entryPath)
	if err == nil {
		t.Fatalf("error expected")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got %q", want, err.Error())
	}
}

func TestArithmeticAndString(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const a: i64 = 40 + 2
  log(to_string(a))
  const s: string = "ab" + "cd"
  log(s)
}
`,
	}, "main.ts")
	want := "42\nabcd\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestPreludeExternStringLength(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string, string_length } from "prelude"
export function main(): void {
  const asciiLen: i64 = string_length("hello")
  const utfLen: i64 = string_length("こんにちは")
  log(to_string(asciiLen))
  log(to_string(utfLen))
}
`,
	}, "main.ts")
	want := "5\n5\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestPreludeAndThen(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string, then } from "prelude"

function parseValue(text: string): i64 | error {
  if (text == "bad") {
    return error("boom")
  }
  return 20
}

function plusOne(v: i64): i64 | error {
  return v + 1
}

export function main(): void {
  const ok: i64 | error = then(parseValue("ok"), plusOne)
  const ng: i64 | error = parseValue("bad").then(plusOne)

  const okText: string = switch (ok) {
    case v as i64: to_string(v)
    case e as error: e.message
  }
  const ngText: string = switch (ng) {
    case v as i64: to_string(v)
    case e as error: e.message
  }
  log(okText)
  log(ngText)
}
`,
	}, "main.ts")
	want := "21\nboom\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestHigherOrderFunctionVariableCall(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"

function apply(v: i64, fn: (i64) => i64): i64 {
  return fn(v)
}

function inc(v: i64): i64 {
  return v + 1
}

function add2(v: i64): i64 {
  return v + 2
}

export function main(): void {
  log(to_string(apply(40, inc)))
  log(to_string(apply(40, add2)))
}
`,
	}, "main.ts")
	want := "41\n42\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestMapWithFunctionVariable(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
import { map } from "array"

function applyMap(xs: i64[], fn: (i64) => i64): i64[] {
  return map(xs, fn)
}

function add3(x: i64): i64 {
  return x + 3
}

export function main(): void {
  const ys: i64[] = applyMap([1, 2, 3], add3)
  for (const y: i64 of ys) {
    log(to_string(y))
  }
}
`,
	}, "main.ts")
	want := "4\n5\n6\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestObjectSpreadAndStringify(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log } from "prelude"
import { stringify } from "json"
export function main(): void {
  const a: { x: i64, y: i64 } = { "x": 1, "y": 2 }
  const b: { x: i64, y: i64 } = { ...a, "x": 1 }
  log(stringify(b))
}
`,
	}, "main.ts")
	want := "{\"x\":1,\"y\":2}\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestArrayForOf(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const xs: i64[] = [1, 2, 3]
  for (const x: i64 of xs) {
    log(to_string(x))
  }
}
`,
	}, "main.ts")
	want := "1\n2\n3\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestPreludeRange(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
import { range } from "array"
export function main(): void {
  const xs: i64[] = range(0, 4)
  for (const x: i64 of xs) {
    log(to_string(x))
  }
}
`,
	}, "main.ts")
	want := "0\n1\n2\n3\n4\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestFunctionDeclaration(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
function add(a: i64, b: i64): i64 {
  return a + b
}

export function main(): void {
  const v: i64 = add(1, 2)
  log(to_string(v))
}
`,
	}, "main.ts")
	want := "3\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestArraySpread(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const base: i64[] = [2, 3]
  const xs: i64[] = [1, ...base, 4]
  for (const x: i64 of xs) {
    log(to_string(x))
  }
}
`,
	}, "main.ts")
	want := "1\n2\n3\n4\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestPreludeMapReduceLength(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
import { map, reduce, length } from "array"
function double(n: i64): i64 {
  return n * 2
}

function sumValues(acc: i64, v: i64): i64 {
  return acc + v
}

export function main(): void {
  const xs: i64[] = [1, 2, 3]
  const doubled: i64[] = map(xs, double)
  const total: i64 = reduce(doubled, sumValues, 0)
  const size: i64 = length(doubled)
  log(to_string(total))
  log(to_string(size))
}
`,
	}, "main.ts")
	want := "12\n3\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestTupleIndex(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const t: [i64, string] = [1, "a"]
  const v0 = switch (t[0]) {
    case n as i64: to_string(n)
    case e as error: e.message
  }
  const v1 = switch (t[1]) {
    case s as string: s
    case e as error: e.message
  }
  log(v0)
  log(v1)
}
`,
	}, "main.ts")
	want := "1\na\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestParseStringify(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log } from "prelude"
import { toJSON, stringify } from "json"
export function main(): void {
  const parsed = toJSON("{\"a\":1,\"b\":\"x\"}")
  switch (parsed) {
    case err as error:
      log(err.message)
    case v as json:
      log(stringify(v))
  }
}
`,
	}, "main.ts")
	want := "{\"a\":1,\"b\":\"x\"}\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestTernaryOperatorIsSyntaxError(t *testing.T) {
	compileExpectError(t, `import { log, to_string } from "prelude"
export function main(): void {
  const x: i64 = true ? 1 : 2
  log(to_string(x))
}
`)
}

func TestJSXStyleAndScriptAllowStringExpressions(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log } from "prelude"
export function main(): void {
  const style: string = "body { color: red; }"
  const script: string = "const x = 1;"
  const html: string = <html><head><style>{style}</style><script>{script}</script></head><body>Hello</body></html>
  log(html)
}
`,
	}, "main.ts")
	want := "<html><head><style>body { color: red; }</style><script>const x = 1;</script></head><body>Hello</body></html>\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestModuleImport(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"lib.ts": `export function add(a: i64, b: i64): i64 { return a + b }`,
		"main.ts": `import { add } from "./lib"
import { log, to_string } from "prelude"
export function main(): void {
  const v: i64 = add(20, 22)
  log(to_string(v))
}
`,
	}, "main.ts")
	want := "42\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestTextFileDefaultImport(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"style.css": `body { color: red; }`,
		"main.ts": `import style from "./style.css"
import { log } from "prelude"
export function main(): void {
  const s: string = style
  log(s)
}
`,
	}, "main.ts")
	want := "body { color: red; }\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestSemicolonIsSyntaxError(t *testing.T) {
	compileExpectError(t, `import { log } from "prelude";
export function main(): void {
  log("x")
}
`)
}

func TestLineHeadParenIsNotCall(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const n: i64 = 1
  (2)
  log(to_string(n))
}
`,
	}, "main.ts")
	want := "1\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestLineHeadBracketIsNotIndexAccess(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, to_string } from "prelude"
export function main(): void {
  const n: i64 = 1
  [2]
  log(to_string(n))
}
`,
	}, "main.ts")
	want := "1\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestTypeErrors(t *testing.T) {
	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const a: i64 = 1
  const b: f64 = 1.0
  if (a == b) { log("x"); }
}
`)

	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const a: i64 = 1
  const s: string = "a" + a
  log(s)
}
`)

	compileExpectError(t, `import { log } from "prelude"
import { toJSON } from "json"
	export function main(): void {
	  const v: json = toJSON("{\"a\":1}")
	  switch (v) {
	    case v as i64: {} 
	  }
	  log("x")
	}
	`)

	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const t: [i64, i64] = [1, 2]
  for (const x: i64 of t) {} 
  log("x")
}
`)

	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const a: { x: i64 } = { "x": "invalid" }
  log("x")
}
`)
}

func TestArgumentTypeMismatchShowsExpectedAndFound(t *testing.T) {
	compileExpectErrorContains(t, `export function takeInt(x: i64): void {}
export function main(): void {
  const s: string = "x"
  takeInt(s)
}
`, "argument type mismatch: expect i64, found string")
}

func TestSQLCreateAndSelect(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log } from "prelude"
import { stringify } from "json"
export function main(): void {
  execute {
    CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)
  }
  execute {
    INSERT INTO users (id, name) VALUES (1, 'Alice')
  }
  execute {
    INSERT INTO users (id, name) VALUES (2, 'Bob')
  }
  const fetched = fetch_all {
    SELECT id, name FROM users ORDER BY id
  }
  switch (fetched) {
    case err as error:
      log(err.message)
    case rows as { id: string, name: string }[]:
      log(stringify(rows))
  }
}
`,
	}, "main.ts")
	want := "[{\"id\":\"1\",\"name\":\"Alice\"},{\"id\":\"2\",\"name\":\"Bob\"}]\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestSQLWithUpdate(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log } from "prelude"
import { stringify } from "json"
export function main(): void {
  execute {
    CREATE TABLE items (id INTEGER, value TEXT)
  }
  execute {
    INSERT INTO items VALUES (1, 'old')
  }
  execute {
    UPDATE items SET value = 'new' WHERE id = 1
  }
  const fetched = fetch_all {
    SELECT value FROM items WHERE id = 1
  }
  switch (fetched) {
    case err as error:
      log(err.message)
    case rows as { value: string }[]:
      log(stringify(rows))
  }
}
`,
	}, "main.ts")
	want := "[{\"value\":\"new\"}]\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}
