package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestConvertOnceIgnoresExistingSiblingPDF(t *testing.T) {
	trueBinary, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true command is unavailable")
	}

	dir := t.TempDir()
	docxPath := filepath.Join(t.TempDir(), "doc.docx")
	pdfPath := filepath.Join(dir, "result.pdf")
	sibling := filepath.Join(dir, "doc.pdf")
	want := []byte("%PDF-existing\n%%EOF\n")
	if err := os.WriteFile(sibling, want, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := convertOnce(trueBinary, docxPath, pdfPath, time.Second, false); err == nil {
		t.Fatal("converter that produced nothing succeeded")
	}
	got, err := os.ReadFile(sibling)
	if err != nil {
		t.Fatalf("existing sibling PDF was removed: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("existing sibling PDF changed: %q", got)
	}
	if _, err := os.Stat(pdfPath); !os.IsNotExist(err) {
		t.Fatalf("output exists after failed conversion: %v", err)
	}
}

func TestVerifyPDFRejectsTruncatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truncated.pdf")
	if err := os.WriteFile(path, []byte("%PDF-incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyPDF(path); err == nil {
		t.Fatal("truncated PDF passed verification")
	}
}
