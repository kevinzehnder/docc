package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffComparesRenderedContent(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)
	source := filepath.Join(dir, "memo.md")
	built := filepath.Join(dir, "memo.docx")
	write(t, source, "---\ndocc: 1\ndocument_type: memo\ntitle: Example\n---\n\n# Summary\n\nOriginal wording.\n")

	common := []string{"--schema-dir", schemaDir, "--theme-dir", themeDir}
	if code := run(append([]string{"build", source, "--output", built}, common...)); code != 0 {
		t.Fatalf("build = %d", code)
	}
	if code := run(append([]string{"diff", source, built}, common...)); code != 0 {
		t.Fatalf("unchanged diff = %d, want 0", code)
	}

	edited := filepath.Join(dir, "edited.docx")
	rewriteDocx(t, built, edited, func(name, text string) string {
		if name == "word/document.xml" {
			return strings.Replace(text, "Original wording.", "Edited wording.", 1)
		}
		return text
	})
	var code int
	out := captureStdout(t, func() {
		code = run(append([]string{"diff", source, edited, "--json"}, common...))
	})
	if code != exitDiag {
		t.Fatalf("edited diff = %d, want %d\n%s", code, exitDiag, out)
	}
	var result diffResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("diff JSON: %v\n%s", err, out)
	}
	if result.Equal || result.Changes != 1 || len(result.Stories) != 1 {
		t.Fatalf("result = %+v", result)
	}
	h := result.Stories[0].Hunks[0]
	if len(h.Old) != 1 || h.Old[0] != "Original wording." || len(h.New) != 1 || h.New[0] != "Edited wording." {
		t.Fatalf("hunk = %+v", h)
	}
}

func rewriteDocx(t *testing.T, src, dst string, change func(name, text string) string) {
	t.Helper()
	data, err := os.ReadFile(src) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		part, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(f.Name, ".xml") {
			part = []byte(change(f.Name, string(part)))
		}
		if _, err := w.Write(part); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
