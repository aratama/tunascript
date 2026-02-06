package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuna/internal/compiler"
	"tuna/internal/runtime"
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
	if !runtimeAvailable {
		t.Skip("CGO が無効なためテストをスキップします")
	}
	dir := t.TempDir()
	writeFiles(t, dir, files)
	entryPath := filepath.Join(dir, entry)
	comp := compiler.New()
	res, err := comp.Compile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := runtime.NewRunner()
	out, err := runner.Run(res.Wasm)
	if err != nil {
		t.Fatal(err)
	}
	return out
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
		"main.ts": `import { log, toString } from "prelude"
export function main(): void {
  const a: integer = 40 + 2
  log(toString(a))
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
		"main.ts": `import { log, toString, stringLength } from "prelude"
export function main(): void {
  const asciiLen: integer = stringLength("hello")
  const utfLen: integer = stringLength("こんにちは")
  log(toString(asciiLen))
  log(toString(utfLen))
}
`,
	}, "main.ts")
	want := "5\n5\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestObjectSpreadAndStringify(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log } from "prelude"
import { stringify } from "json"
export function main(): void {
  const a: { x: integer, y: integer } = { "x": 1, "y": 2 }
  const b: { x: integer, y: integer } = { ...a, "x": 1 }
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
		"main.ts": `import { log, toString } from "prelude"
export function main(): void {
  const xs: integer[] = [1, 2, 3]
  for (const x: integer of xs) {
    log(toString(x))
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
		"main.ts": `import { log, toString } from "prelude"
import { range } from "array"
export function main(): void {
  const xs: integer[] = range(0, 4)
  for (const x: integer of xs) {
    log(toString(x))
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
		"main.ts": `import { log, toString } from "prelude"
function add(a: integer, b: integer): integer {
  return a + b
}

export function main(): void {
  const v: integer = add(1, 2)
  log(toString(v))
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
		"main.ts": `import { log, toString } from "prelude"
export function main(): void {
  const base: integer[] = [2, 3]
  const xs: integer[] = [1, ...base, 4]
  for (const x: integer of xs) {
    log(toString(x))
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
		"main.ts": `import { log, toString } from "prelude"
import { map, reduce, length } from "array"
function double(n: integer): integer {
  return n * 2
}

function sumValues(acc: integer, v: integer): integer {
  return acc + v
}

export function main(): void {
  const xs: integer[] = [1, 2, 3]
  const doubled: integer[] = map(xs, double)
  const total: integer = reduce(doubled, sumValues, 0)
  const size: integer = length(doubled)
  log(toString(total))
  log(toString(size))
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
		"main.ts": `import { log, toString } from "prelude"
export function main(): void {
  const t: [integer, string] = [1, "a"]
  const v0 = switch (t[0]) {
    case n as integer: toString(n)
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
import { parse, stringify } from "json"
export function main(): void {
  const parsed = parse("{\"a\":1,\"b\":\"x\"}")
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
	compileExpectError(t, `import { log, toString } from "prelude"
export function main(): void {
  const x: integer = true ? 1 : 2
  log(toString(x))
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
		"lib.ts": `export function add(a: integer, b: integer): integer { return a + b }`,
		"main.ts": `import { add } from "./lib"
import { log, toString } from "prelude"
export function main(): void {
  const v: integer = add(20, 22)
  log(toString(v))
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
		"main.ts": `import { log, toString } from "prelude"
export function main(): void {
  const n: integer = 1
  (2)
  log(toString(n))
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
		"main.ts": `import { log, toString } from "prelude"
export function main(): void {
  const n: integer = 1
  [2]
  log(toString(n))
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
  const a: integer = 1
  const b: number = 1.0
  if (a == b) { log("x"); }
}
`)

	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const a: integer = 1
  const s: string = "a" + a
  log(s)
}
`)

	compileExpectError(t, `import { log } from "prelude"
import { parse } from "json"
	export function main(): void {
	  const v: json = parse("{\"a\":1}")
	  switch (v) {
	    case v as integer: {} 
	  }
	  log("x")
	}
	`)

	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const t: [integer, integer] = [1, 2]
  for (const x: integer of t) {} 
  log("x")
}
`)

	compileExpectError(t, `import { log } from "prelude"
export function main(): void {
  const a: { x: integer } = { "x": "invalid" }
  log("x")
}
`)
}

func TestArgumentTypeMismatchShowsExpectedAndFound(t *testing.T) {
	compileExpectErrorContains(t, `export function takeInt(x: integer): void {}
export function main(): void {
  const s: string = "x"
  takeInt(s)
}
`, "argument type mismatch: expect integer, found string")
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
