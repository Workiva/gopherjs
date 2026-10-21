package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParallelCompileMatchesSerial(t *testing.T) {
	// Generic types shared across packages exercise the instance sets that all
	// packages read during compilation.
	importPaths := []string{
		"github.com/gopherjs/gopherjs/tests/testdata/gencircle/catbox",
		"github.com/gopherjs/gopherjs/tests/testdata/gencircle/trammel",
	}
	for _, importPath := range importPaths {
		t.Run(filepath.Base(importPath), func(t *testing.T) {
			parallel := buildCommandJS(t, importPath)
			t.Setenv("GOPHERJS_SERIAL_COMPILE", "1")
			serial := buildCommandJS(t, importPath)
			if parallel != serial {
				t.Errorf("parallel compile of %q produced different output than serial compile", importPath)
			}
		})
	}
}

func buildCommandJS(t *testing.T, importPath string) string {
	t.Helper()
	s, err := NewSession(&Options{Minify: true})
	if err != nil {
		t.Fatalf("NewSession() returned error: %v", err)
	}
	pkg, err := s.XContext().Import(importPath, "", 0)
	if err != nil {
		t.Fatalf("Import(%q) returned error: %v", importPath, err)
	}
	archive, err := s.BuildProject(pkg)
	if err != nil {
		t.Fatalf("BuildProject(%q) returned error: %v", importPath, err)
	}
	out := filepath.Join(t.TempDir(), "out.js")
	if err := s.WriteCommandPackage(archive, out); err != nil {
		t.Fatalf("WriteCommandPackage() returned error: %v", err)
	}
	js, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(js)
}
