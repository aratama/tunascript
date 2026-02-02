//go:build !cgo
// +build !cgo

package runtime

type Runtime struct{}

func NewRuntime() *Runtime {
	return &Runtime{}
}

func (r *Runtime) Output() string {
	return ""
}
