package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"negitoro/internal/compiler"
	"negitoro/internal/runtime"
)

// TestNgtrFiles runs all .ngtr files in the tests directory.
// Each .ngtr file can optionally specify expected output using a comment at the top:
//
//	// expect: line1
//	// expect: line2
//
// If no expect comments are found, the test only verifies that the file compiles and runs without error.
func TestNgtrFiles(t *testing.T) {
	if !runtimeAvailable() {
		t.Skip("CGO が無効なためテストをスキップします")
	}

	// Find all .ngtr files in the tests directory
	testsDir := "."
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		t.Fatalf("failed to read tests directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ngtr") {
			continue
		}

		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(testsDir, name)
			runNgtrTest(t, path)
		})
	}
}

func runNgtrTest(t *testing.T, path string) {
	t.Helper()

	// Read the source file
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Extract expected output from comments
	expected := extractExpectedOutput(string(src))

	// Compile
	comp := compiler.New()
	res, err := comp.Compile(path)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	// Run
	runner := runtime.NewRunner()
	out, err := runner.Run(res.Wasm)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}

	// Check output if expected output is specified
	if expected != "" {
		if out != expected {
			t.Errorf("output mismatch:\nexpected:\n%s\ngot:\n%s", expected, out)
		}
	}
}

// extractExpectedOutput extracts lines starting with "// expect: " from the source
func extractExpectedOutput(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "// expect: ") {
			lines = append(lines, strings.TrimPrefix(trimmed, "// expect: "))
		} else if strings.HasPrefix(trimmed, "// expect:") {
			// Handle empty expect line
			lines = append(lines, strings.TrimPrefix(trimmed, "// expect:"))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
