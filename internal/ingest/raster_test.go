package ingest

import (
	"os/exec"
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
