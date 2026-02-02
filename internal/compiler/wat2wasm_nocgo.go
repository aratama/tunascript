//go:build !cgo
// +build !cgo

package compiler

import "fmt"

func (g *Generator) WatToWasm(wat string) ([]byte, error) {
	return nil, fmt.Errorf("CGO が無効です（wasmtime-go が必要です）")
}
