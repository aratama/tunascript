package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("TUNASCRIPT_LIB_DIR") == "" {
		if libDir, err := findLibDirForTests(); err == nil {
			_ = os.Setenv("TUNASCRIPT_LIB_DIR", libDir)
		}
	}
	os.Exit(m.Run())
}

func findLibDirForTests() (string, error) {
	searchUp := func(dir string) (string, bool) {
		cur := dir
		for {
			candidate := filepath.Join(cur, "lib")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, true
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
		return "", false
	}
	if pwd := os.Getenv("PWD"); pwd != "" {
		if path, ok := searchUp(pwd); ok {
			return path, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if path, ok := searchUp(cwd); ok {
			return path, nil
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		if path, ok := searchUp(filepath.Dir(file)); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("lib directory not found")
}
