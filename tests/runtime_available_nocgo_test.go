//go:build !cgo
// +build !cgo

package tests

func runtimeAvailable() bool {
	return false
}
