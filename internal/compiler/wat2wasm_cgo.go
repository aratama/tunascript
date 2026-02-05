//go:build cgo
// +build cgo

package compiler

import "github.com/bytecodealliance/wasmtime-go/v41"

func (g *Generator) WatToWasm(wat string) ([]byte, error) {
	return wasmtime.Wat2Wasm(wat)
}
