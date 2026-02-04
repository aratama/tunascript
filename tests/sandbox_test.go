package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuna/internal/compiler"
	"tuna/internal/runtime"
)

func TestSandboxBuffersLogAndHTML(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { log } from "prelude";
import { createServer, addRoute, listen, responseHtml, type Request, type Response } from "http";

function handleRoot(req: Request): Response {
  log("from handler");
  return responseHtml(<div>Hello Sandbox</div>);
}

export function main(): void {
  const server = createServer();
  log("before listen");
  addRoute(server, "/", handleRoot);
  listen(server, ":8080");
}
`
	result := compileAndRunSandbox(t, src, nil)
	if result.ExitCode != 0 {
		t.Fatalf("sandbox failed: %s", result.Error)
	}
	if result.Stdout != "before listen\nfrom handler\n" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if !strings.Contains(result.HTML, "Hello Sandbox") {
		t.Fatalf("unexpected html: %q", result.HTML)
	}
}

func TestSandboxEscapesJSXAttributeValue(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { createServer, addRoute, listen, responseHtml, type Request, type Response } from "http";

function handleRoot(req: Request): Response {
  const inner = "<div class=\"x\">Hello</div>";
  return responseHtml(<iframe srcdoc={inner}></iframe>);
}

export function main(): void {
  const server = createServer();
  addRoute(server, "/", handleRoot);
  listen(server, ":8080");
}
`
	result := compileAndRunSandbox(t, src, nil)
	if result.ExitCode != 0 {
		t.Fatalf("sandbox failed: %s", result.Error)
	}
	if !strings.Contains(result.HTML, `srcdoc="&lt;div class=&#34;x&#34;&gt;Hello&lt;/div&gt;"`) {
		t.Fatalf("attribute escaping failed: %q", result.HTML)
	}
}

func TestSandboxRejectsDuplicateRootRoute(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { createServer, addRoute, listen, responseHtml, type Request, type Response } from "http";

function h1(req: Request): Response { return responseHtml(<p>a</p>); }
function h2(req: Request): Response { return responseHtml(<p>b</p>); }

export function main(): void {
  const server = createServer();
  addRoute(server, "/", h1);
  addRoute(server, "/", h2);
  listen(server, ":8080");
}
`
	result := compileAndRunSandbox(t, src, nil)
	if result.ExitCode != 1 {
		t.Fatalf("expected exitCode=1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, `addRoute(server, "/", handler)`) {
		t.Fatalf("unexpected error: %q", result.Error)
	}
}

func TestSandboxDbOpenNoop(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sandbox.sqlite3")
	src := fmt.Sprintf(`
import { dbOpen } from "sqlite";

export function main(): void {
  dbOpen("%s");
}
`, dbPath)
	result := compileAndRunSandbox(t, src, nil)
	if result.ExitCode != 0 {
		t.Fatalf("sandbox failed: %s", result.Error)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("db file should not be created in sandbox mode: %s", dbPath)
	}
}

func TestRunSandboxBuiltin(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { runSandbox, parse, decode, toString, log, type Result, type Error } from "prelude";

type RunResult = {
  stdout: string,
  html: string,
  exitCode: integer,
  error: string
};

export function main(): void {
  const child = "import { log } from \"prelude\";\nexport function main(): void { log(\"child-ok\"); }\n";
  const raw = runSandbox(child);
  const decoded: Result<RunResult> = decode<RunResult>(parse(raw));
  switch (decoded) {
    case ok as RunResult: {
      log(toString(ok.exitCode));
      log(ok.stdout);
    }
    case err as Error: {
      log("decode error: " + err.message);
    }
  }
}
`

	out := compileAndRunNormal(t, src, nil)
	if !strings.Contains(out, "0\n") {
		t.Fatalf("expected exitCode output, got: %q", out)
	}
	if !strings.Contains(out, "child-ok\n") {
		t.Fatalf("expected sandbox stdout output, got: %q", out)
	}
}

func compileAndRunSandbox(t *testing.T, src string, args []string) runtime.SandboxResult {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.tuna")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	comp := compiler.New()
	res, err := comp.Compile(path)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	runner := runtime.NewRunner()
	return runner.RunSandboxWithArgs(res.Wasm, args)
}

func compileAndRunNormal(t *testing.T, src string, args []string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.tuna")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	comp := compiler.New()
	res, err := comp.Compile(path)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	runner := runtime.NewRunner()
	out, err := runner.RunWithArgs(res.Wasm, args)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	return out
}
