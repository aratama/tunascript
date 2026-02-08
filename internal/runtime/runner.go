//go:build cgo
// +build cgo

package runtime

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go/v41"
)

type Runner struct {
	engine *wasmtime.Engine
}

func NewRunner() *Runner {
	config := wasmtime.NewConfig()
	// Enable Wasm GC-related proposals so GC-enabled modules can run.
	config.SetWasmFunctionReferences(true)
	config.SetWasmGC(true)
	return &Runner{engine: wasmtime.NewEngineWithConfig(config)}
}

func (r *Runner) Run(wasm []byte) (string, error) {
	return r.RunWithArgs(wasm, nil)
}

func (r *Runner) RunWithArgs(wasm []byte, args []string) (string, error) {
	rt, err := r.runWithArgs(wasm, args)
	if err != nil {
		if rt == nil {
			return "", err
		}
		return rt.Output(), err
	}
	return rt.Output(), nil
}

func (r *Runner) runWithArgs(wasm []byte, args []string) (rt *Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recoveredErr, ok := recovered.(error); ok {
				err = recoveredErr
			} else {
				err = fmt.Errorf("%v", recovered)
			}
		}
	}()

	store := wasmtime.NewStore(r.engine)
	linker := wasmtime.NewLinker(r.engine)

	rt = NewRuntime()
	rt.SetArgs(args)
	if err := defineWASIFDWrite(linker, store, rt); err != nil {
		return rt, err
	}
	if err := rt.Define(linker, store); err != nil {
		return rt, err
	}
	module, err := wasmtime.NewModule(r.engine, wasm)
	if err != nil {
		return rt, err
	}
	instance, err := linker.Instantiate(store, module)
	if err != nil {
		return rt, err
	}
	// Set WASM context for optional host callbacks.
	rt.SetWasmContext(store, instance)
	if err := rt.ensureDefaultDB(); err != nil {
		return rt, err
	}
	start := instance.GetFunc(store, "_start")
	if start == nil {
		return rt, errors.New("error")
	}
	if _, err := start.Call(store); err != nil {
		return rt, err
	}

	// _start終了時は1回強制GCして短命な参照を回収する。
	rt.maybeStoreGC(true)
	if err := rt.StartPendingServer(); err != nil {
		return rt, err
	}
	return rt, nil
}

func defineWASIFDWrite(linker *wasmtime.Linker, store *wasmtime.Store, rt *Runtime) error {
	return linker.DefineFunc(store, "wasi_snapshot_preview1", "fd_write", func(caller *wasmtime.Caller, fd int32, iovs int32, iovsLen int32, nwritten int32) int32 {
		ext := caller.GetExport("memory")
		if ext == nil {
			return 21
		}
		memory := ext.Memory()
		if memory == nil {
			return 21
		}
		data := memory.UnsafeData(caller)
		total := 0

		for i := int32(0); i < iovsLen; i++ {
			base := int(iovs + i*8)
			if base < 0 || base+8 > len(data) {
				return 21
			}
			ptr := int(binary.LittleEndian.Uint32(data[base : base+4]))
			length := int(binary.LittleEndian.Uint32(data[base+4 : base+8]))
			if ptr < 0 || ptr+length > len(data) {
				return 21
			}
			chunk := string(data[ptr : ptr+length])
			switch fd {
			case 1:
				if err := rt.appendOutputChunk(chunk); err != nil {
					return 1
				}
			case 3:
				if err := rt.appendHTMLChunk(chunk); err != nil {
					return 1
				}
			}
			total += length
		}

		nw := int(nwritten)
		if nw < 0 || nw+4 > len(data) {
			return 21
		}
		binary.LittleEndian.PutUint32(data[nw:nw+4], uint32(total))
		return 0
	})
}
