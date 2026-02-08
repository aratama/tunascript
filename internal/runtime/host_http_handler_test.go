//go:build cgo
// +build cgo

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"tuna/internal/compiler"
)

func TestHostBackendHTTPHandlerInvocation(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "main.tuna")
	src := `
import { create_server, add_route, response_html, type Request, type Response } from "http"

function handle_root(req: Request): Response {
  return response_html(<p>ok</p>)
}

export function main(): void {
  const server = create_server()
  add_route(server, "get", "/", handle_root)
}
`
	if err := os.WriteFile(entry, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	comp := compiler.New()
	if err := comp.SetBackend(compiler.BackendHost); err != nil {
		t.Fatalf("set backend failed: %v", err)
	}
	res, err := comp.Compile(entry)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	runner := NewRunner()
	rt, err := runner.runWithArgs(res.Wasm, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(rt.httpServers) != 1 {
		t.Fatalf("expected exactly one server, got %d", len(rt.httpServers))
	}

	var server *HTTPServer
	for _, s := range rt.httpServers {
		server = s
		break
	}
	if server == nil {
		t.Fatal("server not found")
	}

	resp, err := rt.invokeRouteHandler(server, "/", "GET", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("invokeRouteHandler failed: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Body != "<p>ok</p>" {
		t.Fatalf("unexpected body: %q", resp.Body)
	}
}
