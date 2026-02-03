package compiler_test

import (
	"os"
	"path/filepath"
	"testing"

	"negitoro/internal/compiler"
	"negitoro/internal/runtime"
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

func TestArithmeticAndString(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, toString } from "prelude";
export function main(): void {
  const a: integer = 40 + 2;
  log(toString(a));
  const s: string = "ab" + "cd";
  log(s);
}
`,
	}, "main.ts")
	want := "42\nabcd\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestObjectSpreadAndStringify(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, stringify } from "prelude";
export function main(): void {
  const a: { "x": integer, "y": integer } = { "x": 1, "y": 2 };
  const b: { "x": integer, "y": integer } = { ...a, "x": 1 };
  log(stringify(b));
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
		"main.ts": `import { log, toString } from "prelude";
export function main(): void {
  const xs: integer[] = [1, 2, 3];
  for (const x: integer of xs) {
    log(toString(x));
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
		"main.ts": `import { range, log, toString } from "prelude";
export function main(): void {
  const xs: integer[] = range(0, 4);
  for (const x: integer of xs) {
    log(toString(x));
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
		"main.ts": `import { log, toString } from "prelude";
function add(a: integer, b: integer): integer {
  return a + b;
}

export function main(): void {
  const v: integer = add(1, 2);
  log(toString(v));
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
		"main.ts": `import { log, toString } from "prelude";
export function main(): void {
  const base: integer[] = [2, 3];
  const xs: integer[] = [1, ...base, 4];
  for (const x: integer of xs) {
    log(toString(x));
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
		"main.ts": `import { map, reduce, length, log, toString } from "prelude";
function double(n: integer): integer {
  return n * 2;
}

function sumValues(acc: integer, v: integer): integer {
  return acc + v;
}

export function main(): void {
  const xs: integer[] = [1, 2, 3];
  const doubled: integer[] = map(xs, double);
  const total: integer = reduce(doubled, sumValues, 0);
  const size: integer = length(doubled);
  log(toString(total));
  log(toString(size));
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
		"main.ts": `import { log, toString } from "prelude";
export function main(): void {
  const t: [integer, string] = [1, "a"];
  log(toString(t[0]));
  log(t[1]);
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
		"main.ts": `import { parse, stringify, log } from "prelude";
export function main(): void {
  const v: { "a": integer, "b": string } = parse("{\"a\":1,\"b\":\"x\"}");
  log(stringify(v));
}
`,
	}, "main.ts")
	want := "{\"a\":1,\"b\":\"x\"}\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestModuleImport(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"lib.ts": `export function add(a: integer, b: integer): integer { return a + b; }`,
		"main.ts": `import { add } from "./lib";
import { log, toString } from "prelude";
export function main(): void {
  const v: integer = add(20, 22);
  log(toString(v));
}
`,
	}, "main.ts")
	want := "42\n"
	if out != want {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestTypeErrors(t *testing.T) {
	compileExpectError(t, `import { log } from "prelude";
export function main(): void {
  const a: integer = 1;
  const b: number = 1.0;
  if (a == b) { log("x"); }
}
`)

	compileExpectError(t, `import { log } from "prelude";
export function main(): void {
  const a: integer = 1;
  const s: string = "a" + a;
  log(s);
}
`)

	compileExpectError(t, `import { parse, log } from "prelude";
export function main(): void {
  log(parse("{\"a\":1}"));
}
`)

	compileExpectError(t, `import { log } from "prelude";
export function main(): void {
  const t: [integer, integer] = [1, 2];
  for (const x: integer of t) { }
  log("x");
}
`)

	compileExpectError(t, `import { log } from "prelude";
export function main(): void {
  const a: { "x": integer } = { "x": "invalid" };
  log("x");
}
`)
}

func TestSQLCreateAndSelect(t *testing.T) {
	out := compileAndRun(t, map[string]string{
		"main.ts": `import { log, stringify } from "prelude";
export function main(): void {
  execute {
    CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)
  };
  execute {
    INSERT INTO users (id, name) VALUES (1, 'Alice')
  };
  execute {
    INSERT INTO users (id, name) VALUES (2, 'Bob')
  };
  const rows = fetch_all {
    SELECT id, name FROM users ORDER BY id
  };
  log(stringify(rows));
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
		"main.ts": `import { log, stringify } from "prelude";
export function main(): void {
  execute {
    CREATE TABLE items (id INTEGER, value TEXT)
  };
  execute {
    INSERT INTO items VALUES (1, 'old')
  };
  execute {
    UPDATE items SET value = 'new' WHERE id = 1
  };
  const rows = fetch_all {
    SELECT value FROM items WHERE id = 1
  };
  log(stringify(rows));
}
`,
	}, "main.ts")
	want := "[{\"value\":\"new\"}]\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}
