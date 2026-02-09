package formatter

import (
	"strings"
	"testing"

	"tuna/internal/parser"
)

func TestFormatPreservesComments(t *testing.T) {
	src := `// top
import { ok } from "mod"
// before decl
function main(): void {
  // before stmt
  const value = 1 // trailing
  /* after stmt */
}
`

	out, err := New().Format("sample.tuna", src)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	wantOrder := []string{
		"// top",
		"// before decl",
		"// before stmt",
		"// trailing",
		"/* after stmt */",
	}
	last := -1
	for _, want := range wantOrder {
		idx := strings.Index(out, want)
		if idx < 0 {
			t.Fatalf("formatted output is missing comment %q\n%s", want, out)
		}
		if idx <= last {
			t.Fatalf("comment order is not preserved for %q\n%s", want, out)
		}
		last = idx
	}
}

func TestFormatModuleWithCommentsAfterAnnotate(t *testing.T) {
	src := `function main(): void {
  // keep
  const value = 1
}
`

	p := parser.New("sample.tuna", src)
	mod, err := p.ParseModule()
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	f := New()
	if err := f.AnnotateModuleTypes(mod); err != nil {
		t.Fatalf("AnnotateModuleTypes failed: %v", err)
	}

	out := f.FormatModuleWithComments(mod, p.Comments())
	if !strings.Contains(out, "// keep") {
		t.Fatalf("formatted output is missing comment\n%s", out)
	}
	if !strings.Contains(out, "const value: i64 = 1") {
		t.Fatalf("type annotation is missing after annotate format\n%s", out)
	}
}
