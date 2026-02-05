package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuna/internal/compiler"
	"tuna/internal/runtime"
)

func compileAndRunWithRuntimeError(t *testing.T, src string, args []string) (string, error) {
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
	return runner.RunWithArgs(res.Wasm, args)
}

func TestMainCanReturnResultVoid(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { log, type Error } from "prelude"

export function main(): void | Error {
  log("ok")
  return undefined
}
`

	out, err := compileAndRunWithRuntimeError(t, src, nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	if out != "ok\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestMainResultErrorFailsProcess(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { Error, type Error } from "prelude"

export function main(): void | Error {
  return Error("boom-from-main")
}
`

	out, err := compileAndRunWithRuntimeError(t, src, nil)
	if out != "" {
		t.Fatalf("unexpected output: %q", out)
	}
	if err == nil {
		t.Fatalf("runtime error expected")
	}
	if !strings.Contains(err.Error(), "boom-from-main") {
		t.Fatalf("unexpected runtime error: %v", err)
	}
}

func TestMainResultErrorFailsSandbox(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { Error, type Error } from "prelude"

export function main(): void | Error {
  return Error("sandbox-main-error")
}
`

	result := compileAndRunSandbox(t, src, nil)
	if result.ExitCode != 1 {
		t.Fatalf("expected exitCode=1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "sandbox-main-error") {
		t.Fatalf("unexpected sandbox error: %q", result.Error)
	}
}
