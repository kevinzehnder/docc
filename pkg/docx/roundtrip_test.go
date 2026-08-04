//go:build roundtrip

// Package-level round-trip test: generate a .docx and have LibreOffice convert
// it to PDF. This is the only check that catches structural mistakes Word and
// LibreOffice reject but a string assertion cannot see — a missing required
// child, a dangling relationship, a malformed drawing.
//
// Build-tagged because it needs `soffice` on PATH and takes seconds, not
// milliseconds. Run with: task test:roundtrip

package docx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLibreOfficeAcceptsOutput(t *testing.T) {
	if _, err := exec.LookPath("soffice"); err != nil {
		t.Skip("soffice not on PATH")
	}

	dir := t.TempDir()
	docxPath := filepath.Join(dir, "sample.docx")
	if err := sample().Write(docxPath); err != nil {
		t.Fatalf("write docx: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// A private profile directory avoids the lock contention that makes
	// concurrent headless conversions fail intermittently.
	profile := filepath.Join(dir, "loprofile")
	cmd := exec.CommandContext(ctx, "soffice",
		"-env:UserInstallation=file://"+profile,
		"--headless", "--convert-to", "pdf",
		"--outdir", dir,
		docxPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("soffice failed: %v\n%s", err, out)
	}

	pdfPath := filepath.Join(dir, "sample.pdf")
	info, err := os.Stat(pdfPath)
	if err != nil {
		// soffice exits 0 even when it produces nothing, so the output file is
		// the only trustworthy signal.
		t.Fatalf("no PDF produced (soffice exited 0 regardless):\n%s", out)
	}
	if info.Size() == 0 {
		t.Fatal("PDF is empty")
	}

	head, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(head) < 5 || string(head[:5]) != "%PDF-" {
		t.Errorf("output is not a PDF: % x", head[:min(16, len(head))])
	}
	t.Logf("converted to %d byte PDF", info.Size())
}
