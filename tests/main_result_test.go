package tests

import (
	"os"
	"path/filepath"
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
import { log } from "prelude"

export function main(): void | error {
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
