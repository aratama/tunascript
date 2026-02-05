package tests

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"testing"

	"tuna/internal/compiler"
	"tuna/internal/runtime"
)

func compileWasm(t *testing.T, src string) []byte {
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
	return res.Wasm
}

func forceHeapAlloc() uint64 {
	goruntime.GC()
	debug.FreeOSMemory()
	var ms goruntime.MemStats
	goruntime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func TestExplicitGCStressDoesNotGrowHeap(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	src := `
import { gc } from "prelude"
import { range } from "array"

function allocateChunk(seed: integer): void {
  for (const i of range(0, 1499)) {
    const item = {
      "id": seed + i,
      "tag": "abcdefghijklmnopqrstuvwxyz012345",
      "payload": [i, i + 1, i + 2, i + 3],
      "meta": { "even": i % 2 == 0, "text": "chunk" }
    }
    if (item.id == -1) {
      gc()
    }
  }
}

export function main(): void {
  for (const batch of range(0, 119)) {
    allocateChunk(batch * 1500)
    gc()
  }
  gc()
}
`
	wasm := compileWasm(t, src)
	runner := runtime.NewRunner()

	// Warm-up to absorb one-time allocations.
	if _, err := runner.Run(wasm); err != nil {
		t.Fatalf("warm-up runtime error: %v", err)
	}

	before := forceHeapAlloc()

	const runs = 5
	for i := 0; i < runs; i++ {
		if _, err := runner.Run(wasm); err != nil {
			t.Fatalf("runtime error on run %d: %v", i+1, err)
		}
	}

	after := forceHeapAlloc()
	const maxGrowth uint64 = 64 << 20
	if after > before+maxGrowth {
		t.Fatalf("heap grew too much after stress runs: before=%d after=%d growth=%d limit=%d", before, after, after-before, maxGrowth)
	}
}
