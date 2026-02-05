//go:build cgo
// +build cgo

package runtime

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytecodealliance/wasmtime-go/v41"
)

const (
	sandboxTimeout        = 2 * time.Second
	sandboxMaxOutputBytes = 1 << 20
)

type SandboxResult struct {
	Stdout   string `json:"stdout"`
	HTML     string `json:"html"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error"`
}

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
	rt, err := r.runWithArgs(wasm, args, false)
	if err != nil {
		if rt == nil {
			return "", err
		}
		return rt.Output(), err
	}
	return rt.Output(), nil
}

func (r *Runner) RunSandboxWithArgs(wasm []byte, args []string) SandboxResult {
	done := make(chan SandboxResult, 1)
	go func() {
		done <- r.runSandboxWithArgsNoTimeout(wasm, args)
	}()
	select {
	case result := <-done:
		return result
	case <-time.After(sandboxTimeout):
		return SandboxResult{
			ExitCode: 1,
			Error:    fmt.Sprintf("sandbox execution timed out after %dms", sandboxTimeout.Milliseconds()),
		}
	}
}

func (r *Runner) runSandboxWithArgsNoTimeout(wasm []byte, args []string) SandboxResult {
	rt, err := r.runWithArgs(wasm, args, true)
	if err != nil {
		return sandboxFailure(rt, err)
	}
	stdout, html := rt.SandboxOutput()
	return SandboxResult{
		Stdout:   stdout,
		HTML:     html,
		ExitCode: 0,
		Error:    "",
	}
}

func sandboxFailure(rt *Runtime, err error) SandboxResult {
	result := SandboxResult{
		ExitCode: 1,
		Error:    err.Error(),
	}
	if rt != nil {
		result.Stdout, result.HTML = rt.SandboxOutput()
	}
	return result
}

func (r *Runner) runWithArgs(wasm []byte, args []string, sandbox bool) (rt *Runtime, err error) {
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
	if sandbox {
		rt.ConfigureSandbox(sandboxMaxOutputBytes)
	}
	rt.SetArgs(args)
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
	// Set WASM context for HTTP server callbacks
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
	if mainResult := instance.GetFunc(store, "__main_result"); mainResult != nil {
		result, err := mainResult.Call(store)
		if err != nil {
			return rt, err
		}
		if result != nil {
			handle, ok := result.(*Value)
			if !ok {
				return rt, fmt.Errorf("__main_result is not externref: %T", result)
			}
			if msg, isErr, err := rt.resultErrorMessage(handle); err != nil {
				return rt, err
			} else if isErr {
				return rt, errors.New(msg)
			}
		}
	}
	// _start終了時は1回強制GCして短命なexternrefを回収する。
	rt.maybeStoreGC(true)
	if sandbox {
		return rt, nil
	}
	// Start pending HTTP server if one was registered
	//
	// 重要: この呼び出しはWASM実行(_start.Call)が完了した後でなければならない。
	// WASM実行中にHTTPサーバーを起動すると、HTTPハンドラーからWASM関数を
	// 呼び出す際にスタックの再入(reentrant)が発生し、
	// "wasm trap: call stack exhausted"エラーとなる。
	//
	// 詳細はruntime.goのpendingHTTPServer構造体のコメントを参照。
	if err := rt.StartPendingServer(); err != nil {
		return rt, err
	}
	return rt, nil
}
