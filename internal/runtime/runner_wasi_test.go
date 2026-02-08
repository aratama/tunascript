//go:build cgo
// +build cgo

package runtime

import (
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v41"
)

func TestRunnerCapturesWASIStdout(t *testing.T) {
	wasm, err := wasmtime.Wat2Wasm(`
	(module
	  (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))
	  (memory 1)
	  (export "memory" (memory 0))
	  (data (i32.const 16) "hello wasi\0a")
	  (func (export "_start")
	    (i32.store (i32.const 0) (i32.const 16))
	    (i32.store (i32.const 4) (i32.const 11))
	    (call $fd_write
	      (i32.const 1)
	      (i32.const 0)
	      (i32.const 1)
	      (i32.const 8))
	    drop))
	`)
	if err != nil {
		t.Fatalf("wat2wasm failed: %v", err)
	}

	runner := NewRunner()
	out, err := runner.Run(wasm)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out != "hello wasi\n" {
		t.Fatalf("unexpected stdout: got %q, want %q", out, "hello wasi\n")
	}
}

func TestRunnerCapturesWASIFD3AsHTML(t *testing.T) {
	wasm, err := wasmtime.Wat2Wasm(`
	(module
	  (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))
	  (memory 1)
	  (export "memory" (memory 0))
	  (data (i32.const 16) "<h1>ok</h1>")
	  (func (export "_start")
	    (i32.store (i32.const 0) (i32.const 16))
	    (i32.store (i32.const 4) (i32.const 11))
	    (call $fd_write
	      (i32.const 3)
	      (i32.const 0)
	      (i32.const 1)
	      (i32.const 8))
	    drop))
	`)
	if err != nil {
		t.Fatalf("wat2wasm failed: %v", err)
	}

	runner := NewRunner()
	rt, err := runner.runWithArgs(wasm, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if rt.Output() != "" {
		t.Fatalf("unexpected stdout: %q", rt.Output())
	}
	if rt.htmlOutput.String() != "<h1>ok</h1>" {
		t.Fatalf("unexpected html output: got %q, want %q", rt.htmlOutput.String(), "<h1>ok</h1>")
	}
}
