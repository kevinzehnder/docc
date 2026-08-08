package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommand(t *testing.T) {
	root := t.TempDir()
	if got := run([]string{"init", root}); got != 0 {
		t.Fatalf("run(init) = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".docc", "schemas", "letter.yaml")); err != nil {
		t.Fatalf("starter letter schema: %v", err)
	}
}
