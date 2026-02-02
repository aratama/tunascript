//go:build !cgo
// +build !cgo

package runtime

import "fmt"

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(wasm []byte) (string, error) {
	return "", fmt.Errorf("CGO が無効です（wasmtime-go が必要です）")
}
