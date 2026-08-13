package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPDF(t *testing.T) {
	cases := map[string]bool{
		"doc.pdf":  true,
		"doc.PDF":  true,
		"doc.png":  false,
		"doc.jpeg": false,
		"doc":      false,
	}
	for path, want := range cases {
		if got := IsPDF(path); got != want {
			t.Errorf("IsPDF(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestRenderPagesRealBinary exercises the actual pdftoppm call against a
// hand-built PDF — unit-testing collectPages alone would miss a flag or
// naming-convention mismatch with the real tool.
func TestRenderPagesRealBinary(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not on PATH")
	}
	pdfPath := writeMinimalPDF(t, t.TempDir(), "Render Test")
	outDir := t.TempDir()

	pages, err := RenderPages(pdfPath, outDir, RasterOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderPages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if pages[0].Index != 1 {
		t.Errorf("Index = %d, want 1", pages[0].Index)
	}
	if pages[0].PNGPath == "" {
		t.Error("PNGPath is empty")
	}
}

func TestRenderPagesMissingFile(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not on PATH")
	}
	_, err := RenderPages("/nonexistent/file.pdf", t.TempDir(), RasterOptions{})
	if err == nil {
		t.Fatal("expected an error rendering a nonexistent PDF")
	}
}

func TestCheckInput(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	png := filepath.Join(dir, "scan.PNG")
	md := filepath.Join(dir, "notes.md")
	for _, p := range []string{pdf, png, md} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, ok := range []string{pdf, png} {
		if err := CheckInput(ok); err != nil {
			t.Errorf("CheckInput(%q) = %v, want nil", filepath.Base(ok), err)
		}
	}

	// The exact shape of `docc ingest scan.pdf out.md`: the second argument is
	// read as another input, so the message has to point at --output.
	err := CheckInput(md)
	if err == nil {
		t.Fatal("CheckInput on a markdown file: want an error")
	}
	if !strings.Contains(err.Error(), "--output") {
		t.Errorf("error = %v, want it to name the flag that writes the output file", err)
	}

	if err := CheckInput(filepath.Join(dir, "gone.pdf")); err == nil ||
		!strings.Contains(err.Error(), "no such file") {
		t.Errorf("CheckInput on a missing file = %v, want a plain no-such-file error", err)
	}
	if err := CheckInput(dir); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("CheckInput on a directory = %v, want it to say so", err)
	}
}
