//go:build cgo
// +build cgo

package runtime

import (
	"errors"

	"github.com/bytecodealliance/wasmtime-go"
)

type Runner struct {
	engine *wasmtime.Engine
}

func NewRunner() *Runner {
	return &Runner{engine: wasmtime.NewEngine()}
}

func (r *Runner) Run(wasm []byte) (string, error) {
	return r.RunWithArgs(wasm, nil)
}

func (r *Runner) RunWithArgs(wasm []byte, args []string) (string, error) {
	store := wasmtime.NewStore(r.engine)
	linker := wasmtime.NewLinker(r.engine)
	rt := NewRuntime()
	rt.SetArgs(args)
	if err := rt.Define(linker, store); err != nil {
		return "", err
	}
	module, err := wasmtime.NewModule(r.engine, wasm)
	if err != nil {
		return "", err
	}
	instance, err := linker.Instantiate(store, module)
	if err != nil {
		return "", err
	}
	// Set WASM context for HTTP server callbacks
	rt.SetWasmContext(store, instance)
	start := instance.GetFunc(store, "_start")
	if start == nil {
		return "", errors.New("error")
	}
	if _, err := start.Call(store); err != nil {
		return "", err
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
		return rt.Output(), err
	}
	return rt.Output(), nil
}
