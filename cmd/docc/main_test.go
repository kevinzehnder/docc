package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// write creates a file and the directories above it, for building throwaway
// projects in the tests below.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

const memoSchema = `type: memo
description: A memo.
theme: t
frontmatter:
  title:
    type: string
    required: true
body:
  - heading: Summary
    level: 1
    required: false
`

const minimalTheme = `name: t
description: A theme.
`

// TestCheckLoadsSchemaPerFile guards against resolving every file's schema from
// the first argument: two documents from different projects must each validate
// against their own project's contract.
func TestCheckLoadsSchemaPerFile(t *testing.T) {
	projA := t.TempDir()
	write(t, filepath.Join(projA, ".docc", "schemas", "memo.yaml"), "type: memo\ndescription: A memo.\n")
	docA := filepath.Join(projA, "a.md")
	write(t, docA, "---\ndocc: 1\ndocument_type: memo\n---\n\n# Hi\n")

	projB := t.TempDir()
	write(t, filepath.Join(projB, ".docc", "schemas", "note.yaml"), "type: note\ndescription: A note.\n")
	docB := filepath.Join(projB, "b.md")
	write(t, docB, "---\ndocc: 1\ndocument_type: note\n---\n\n# Hi\n")

	// Both are valid against their own project. Before the per-file fix, docB's
	// `note` type was checked against projA's schemas and failed as unknown.
	if got := run([]string{"check", docA, docB}); got != 0 {
		t.Errorf("run(check a b) = %d, want 0 — each file should use its own project's schema", got)
	}
}

// TestBuildStrictBlocksOnWarning verifies --strict promotes warnings to errors
// at build time, so a document that only warns produces no output.
func TestBuildStrictBlocksOnWarning(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	// Valid but for the missing optional "Summary" section, which warns (DOC021).
	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\ntitle: Q3\n---\n\n# Body\n")
	out := filepath.Join(dir, "memo.docx")

	if got := run([]string{"build", "--strict", "--schema-dir", schemaDir, "--theme-dir", themeDir, "--output", out, doc}); got != 1 {
		t.Fatalf("run(build --strict) = %d, want 1", got)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("--strict build should write no output, but %s exists", out)
	}
}

// TestBuildJSONErrorEmitsJSON verifies --json stays machine-readable on a
// validation failure instead of falling back to the human caret rendering.
func TestBuildJSONErrorEmitsJSON(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	// Missing the required `title`, so validation fails with an error.
	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\n---\n\n# Summary\n")
	out := filepath.Join(dir, "memo.docx")

	var code int
	stdout := captureStdout(t, func() {
		code = run([]string{"build", "--json", "--schema-dir", schemaDir, "--theme-dir", themeDir, "--output", out, doc})
	})
	if code != 1 {
		t.Fatalf("run(build --json, invalid) = %d, want 1", code)
	}
	stdout = strings.TrimSpace(stdout)
	if !strings.HasPrefix(stdout, "{") || !strings.Contains(stdout, `"ok":false`) {
		t.Errorf("stdout is not the JSON failure object:\n%s", stdout)
	}
}

func TestCatalogJSONIsOneObject(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	t.Run("types", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"types", "--json", "--schema-dir", schemaDir})
		})
		if code != 0 {
			t.Fatalf("run(types --json) = %d, want 0", code)
		}
		var got struct {
			Types []struct {
				Type string `json:"type"`
			} `json:"types"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("types JSON: %v\n%s", err, out)
		}
		if len(got.Types) != 1 || got.Types[0].Type != "memo" {
			t.Errorf("types = %+v, want memo", got.Types)
		}
	})

	t.Run("themes", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"themes", "--json", "--theme-dir", themeDir})
		})
		if code != 0 {
			t.Fatalf("run(themes --json) = %d, want 0", code)
		}
		var got struct {
			Themes []struct {
				Name string `json:"name"`
			} `json:"themes"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("themes JSON: %v\n%s", err, out)
		}
		if len(got.Themes) != 1 || got.Themes[0].Name != "t" {
			t.Errorf("themes = %+v, want t", got.Themes)
		}
	})
}

func TestWriteAtomic(t *testing.T) {
	t.Run("success writes destination", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out.txt")
		err := writeAtomic(dest, func(p string) error {
			return os.WriteFile(p, []byte("new"), 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(dest); string(b) != "new" {
			t.Errorf("dest = %q, want %q", b, "new")
		}
		assertNoTemp(t, dest)
	})

	t.Run("failure leaves an existing destination intact", func(t *testing.T) {
		dir := t.TempDir()
		dest := filepath.Join(dir, "out.txt")
		if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := writeAtomic(dest, func(p string) error {
			_ = os.WriteFile(p, []byte("partial"), 0o644)
			return io.ErrUnexpectedEOF
		})
		if err == nil {
			t.Fatal("expected the write error to propagate")
		}
		if b, _ := os.ReadFile(dest); string(b) != "old" {
			t.Errorf("dest = %q, want it untouched (%q)", b, "old")
		}
		assertNoTemp(t, dest)
	})
}

// assertNoTemp checks writeAtomic left no `.<name>.tmp-*` scratch file behind.
func assertNoTemp(t *testing.T, dest string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+filepath.Base(dest)+".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
