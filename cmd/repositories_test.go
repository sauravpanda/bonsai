package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeScanRootsResolvesAndDeduplicatesAliases(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}

	roots, err := normalizeScanRoots([]string{root, alias})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("normalized roots = %v, want one physical directory", roots)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if roots[0] != resolved {
		t.Fatalf("normalized root = %q, want %q", roots[0], resolved)
	}
}
