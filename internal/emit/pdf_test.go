package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// fakeConverter writes an executable script standing in for wordcraft-cli.
func fakeConverter(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wordcraft-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil { //nolint:gosec // test helper
		t.Fatal(err)
	}
	return path
}

func TestConvertWordcraftOnce(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	docx := filepath.Join(t.TempDir(), "doc.docx")

	t.Run("writes the PDF it was asked for", func(t *testing.T) {
		// $3 is the output path in `convert <in> <out> --strict`.
		bin := fakeConverter(t, `printf '%%PDF-1.7\n%%%%EOF\n' > "$3"`)
		pdf := filepath.Join(t.TempDir(), "out.pdf")
		if err := convertWordcraftOnce(bin, docx, pdf, 5*time.Second, false); err != nil {
			t.Fatal(err)
		}
		if err := verifyPDF(pdf); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a glyph it cannot draw is an error", func(t *testing.T) {
		bin := fakeConverter(t, `echo '{"missing":"U+1F600"}' >&2; exit 3`)
		pdf := filepath.Join(t.TempDir(), "out.pdf")
		err := convertWordcraftOnce(bin, docx, pdf, 5*time.Second, false)
		if err == nil || !strings.Contains(err.Error(), "cannot be drawn") || !strings.Contains(err.Error(), "U+1F600") {
			t.Fatalf("err = %v, want the strict report", err)
		}
		if _, statErr := os.Stat(pdf); !os.IsNotExist(statErr) {
			t.Fatal("output exists after a failed conversion")
		}
	})
	t.Run("exit 0 without a file is an error", func(t *testing.T) {
		bin := fakeConverter(t, `exit 0`)
		if err := convertWordcraftOnce(bin, docx, filepath.Join(t.TempDir(), "out.pdf"), 5*time.Second, false); err == nil {
			t.Fatal("a converter that produced nothing succeeded")
		}
	})
}

func TestFindConverter(t *testing.T) {
	if _, err := findConverter("pdfkit"); err == nil {
		t.Error("an unknown converter was accepted")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := findConverter("wordcraft"); err == nil {
		t.Error("a forced but missing wordcraft-cli was accepted")
	}
	if _, err := findConverter(""); err == nil || !strings.Contains(err.Error(), "neither wordcraft-cli nor soffice") {
		t.Errorf("err = %v, want both named", err)
	}
	bin := fakeConverter(t, `exit 0`)
	t.Setenv("PATH", filepath.Dir(bin))
	if c, err := findConverter(""); err != nil || c.name != "wordcraft" {
		t.Errorf("findConverter(\"\") = %+v, %v; want wordcraft first", c, err)
	}
}
