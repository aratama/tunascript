//go:build !cgo
// +build !cgo

package runtime

import "fmt"

type Runner struct{}

type SandboxResult struct {
	Stdout   string `json:"stdout"`
	HTML     string `json:"html"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error"`
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(wasm []byte) (string, error) {
	return "", fmt.Errorf("CGO が無効です（wasmtime-go が必要です）")
}

func (r *Runner) RunWithArgs(wasm []byte, args []string) (string, error) {
	return "", fmt.Errorf("CGO が無効です（wasmtime-go が必要です）")
}

func (r *Runner) RunSandboxWithArgs(wasm []byte, args []string) SandboxResult {
	return SandboxResult{
		ExitCode: 1,
		Error:    "CGO が無効です（wasmtime-go が必要です）",
	}
}
