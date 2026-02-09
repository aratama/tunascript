package compiler_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"tuna/internal/compiler"
)

func TestBackendHostFileExistsUsesRealFS(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "exists.txt")
	if err := os.WriteFile(targetPath, []byte("ok"), 0644); err != nil {
		t.Fatalf("failed to prepare file: %v", err)
	}

	src := fmt.Sprintf(`import { exists } from "file"
import { log, to_string } from "prelude"

export function main(): void {
  log(to_string(exists(%q)))
}
`, targetPath)

	out := compileAndRunWithBackend(t, map[string]string{
		"main.tuna": src,
	}, "main.tuna", compiler.BackendHost)

	if out != "true\n" {
		t.Fatalf("output mismatch: got %q, want %q", out, "true\n")
	}
}

func TestBackendHostSQLiteDbOpenOpensFile(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "host-backend.db")

	src := fmt.Sprintf(`import { db_open } from "sqlite"
import { log } from "prelude"

create_table todos {
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL
}

export function main(): void {
  const opened = db_open(%q)
  const inserted = execute {
    INSERT INTO todos (id, title) VALUES (1, "ok")
  }
  switch (opened) {
    case e as error: log("err:" + e.message)
    case openedOk as undefined: switch (inserted) {
      case ie as error: log("err:" + ie.message)
      case insertedOk as undefined: log("ok")
    }
  }
}
`, dbPath)

	out := compileAndRunWithBackend(t, map[string]string{
		"main.tuna": src,
	}, "main.tuna", compiler.BackendHost)

	if out != "ok\n" {
		t.Fatalf("output mismatch: got %q, want %q", out, "ok\n")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite file to exist: %v", err)
	}
}

func TestRunSandboxAlwaysUsesGCBackend(t *testing.T) {
	inner := `import { create_server, add_route, listen, response_html, type Request, type Response } from "http"

function handle_root(req: Request): Response {
  return response_html(<h1>sandbox</h1>)
}

export function main(): void {
  const server = create_server()
  add_route(server, "/", handle_root)
  listen(server, ":18080")
}
`

	outer := fmt.Sprintf(`import { run_sandbox, type SandboxResult } from "runtime"
import { log } from "prelude"

export function main(): void {
  const result = run_sandbox(%q)
  switch (result) {
    case e as error: log("err:" + e.message)
    case v as SandboxResult: log(v.stdout + "|" + v.html)
  }
}
`, inner)

	out := compileAndRunWithBackend(t, map[string]string{
		"main.tuna": outer,
	}, "main.tuna", compiler.BackendHost)

	want := "|<h1>sandbox</h1>\n"
	if out != want {
		t.Fatalf("output mismatch: got %q, want %q", out, want)
	}
}

func TestBackendHostArrayMapFilterReduce(t *testing.T) {
	src := `import { log, to_string } from "prelude"
import { map, filter, reduce, length } from "array"

function double(n: i64): i64 {
  return n * 2
}

function atLeastSix(n: i64): boolean {
  return n >= 6
}

function sumValues(acc: i64, v: i64): i64 {
  return acc + v
}

export function main(): void {
  const xs: i64[] = [1, 2, 3, 4]
  const doubled: i64[] = map(xs, double)
  const filtered: i64[] = filter(doubled, atLeastSix)
  const total: i64 = reduce(filtered, sumValues, 0)
  const size: i64 = length(filtered)
  log(to_string(total))
  log(to_string(size))
}
`

	out := compileAndRunWithBackend(t, map[string]string{
		"main.tuna": src,
	}, "main.tuna", compiler.BackendHost)

	if out != "14\n2\n" {
		t.Fatalf("output mismatch: got %q, want %q", out, "14\n2\n")
	}
}
