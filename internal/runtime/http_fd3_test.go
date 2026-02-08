//go:build cgo
// +build cgo

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuna/internal/compiler"
)

func TestHTTPListenWritesHTMLToFD3(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "main.tuna")
	src := `
import { create_server, add_route, listen, response_html, type Request, type Response } from "http"

function handle_root(req: Request): Response {
  return response_html(<div>Hello FD3</div>)
}

export function main(): void {
  const server = create_server()
  add_route(server, "/", handle_root)
  listen(server, ":8080")
}
`
	if err := os.WriteFile(entry, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	comp := compiler.New()
	res, err := comp.Compile(entry)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	runner := NewRunner()
	rt, err := runner.runWithArgs(res.Wasm, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(rt.htmlOutput.String(), "Hello FD3") {
		t.Fatalf("unexpected html output: %q", rt.htmlOutput.String())
	}
}
