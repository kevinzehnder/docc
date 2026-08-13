package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDraftCreatesAndReplacesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := WriteDraft(path, "first"); err != nil {
		t.Fatalf("WriteDraft new file: %v", err)
	}
	if err := WriteDraft(path, "second"); err != nil {
		t.Fatalf("WriteDraft replacement: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("draft = %q, want replacement content", got)
	}
}

func TestWriteDraftDoesNotReplaceDestinationWhenTemporaryFileCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	missingDir := filepath.Join(filepath.Dir(dir), "moved")
	if err := os.Rename(dir, missingDir); err != nil {
		t.Fatal(err)
	}
	err := WriteDraft(path, "replacement")
	if err == nil || !strings.Contains(err.Error(), "create temporary draft") {
		t.Fatalf("WriteDraft error = %v, want temporary-file creation failure", err)
	}

	got, err := os.ReadFile(filepath.Join(missingDir, "draft.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("draft = %q, want original content retained", got)
	}
}
