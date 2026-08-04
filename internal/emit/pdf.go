package emit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PDFOptions configures the LibreOffice conversion.
type PDFOptions struct {
	// Timeout bounds one conversion attempt. Zero means 120 seconds.
	Timeout time.Duration
	// Retries is the number of additional attempts after a failure.
	Retries int
	// Verbose prints the converter's output.
	Verbose bool
}

// ToPDF converts a .docx to PDF with LibreOffice.
//
// soffice is hostile to automation in three specific ways, and each is handled
// here rather than left to bite intermittently:
//
//   - it exits 0 even when it produced nothing, so the output file is the only
//     trustworthy signal of success;
//   - concurrent runs contend on a shared user profile and fail with an
//     unhelpful lock error, so every run gets a private one;
//   - it can hang indefinitely, so every attempt is bounded.
func ToPDF(docxPath, pdfPath string, opts PDFOptions) error {
	binary, err := findSoffice()
	if err != nil {
		return err
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	attempts := opts.Retries + 1

	outDir := filepath.Dir(pdfPath)
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return err
	}

	var lastErr error
	for attempt := range attempts {
		err := convertOnce(binary, docxPath, outDir, timeout, opts.Verbose)
		if err == nil {
			// soffice names the output after the input, which is not
			// necessarily what the caller asked for.
			produced := filepath.Join(outDir, replaceExt(filepath.Base(docxPath), ".pdf"))
			if produced != pdfPath {
				if err := os.Rename(produced, pdfPath); err != nil {
					return fmt.Errorf("move converted PDF: %w", err)
				}
			}
			return verifyPDF(pdfPath)
		}
		lastErr = err
		if attempt < attempts-1 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	return fmt.Errorf("PDF conversion failed after %d attempt(s): %w", attempts, lastErr)
}

func convertOnce(binary, docxPath, outDir string, timeout time.Duration, verbose bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	profile, err := os.MkdirTemp("", "docc-soffice-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(profile) }()

	// The binary comes from exec.LookPath and the paths are the caller's own
	// arguments; there is no shell involved, so no interpolation to escape.
	cmd := exec.CommandContext(ctx, binary, //nolint:gosec // fixed argv, no shell
		"-env:UserInstallation=file://"+profile,
		"--headless",
		"--norestore",
		"--convert-to", "pdf",
		"--outdir", outDir,
		docxPath,
	)
	out, err := cmd.CombinedOutput()
	if verbose && len(out) > 0 {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(string(out)))
	}
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("soffice timed out after %s", timeout)
	}
	if err != nil {
		return fmt.Errorf("soffice: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	// An exit code of 0 proves nothing; check that a file appeared.
	produced := filepath.Join(outDir, replaceExt(filepath.Base(docxPath), ".pdf"))
	info, statErr := os.Stat(produced)
	if statErr != nil {
		return fmt.Errorf("soffice exited 0 but produced no PDF\n%s", strings.TrimSpace(string(out)))
	}
	if info.Size() == 0 {
		return fmt.Errorf("soffice produced an empty PDF")
	}
	return nil
}

// verifyPDF checks the output really is a PDF and not a truncated write.
func verifyPDF(path string) error {
	f, err := os.Open(path) //nolint:gosec // path is the caller's own output
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, 5)
	n, err := f.Read(head)
	if err != nil || n < 5 || string(head) != "%PDF-" {
		return fmt.Errorf("%s is not a valid PDF", path)
	}
	return nil
}

func findSoffice() (string, error) {
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("neither soffice nor libreoffice found on PATH — PDF output needs LibreOffice installed")
}

func replaceExt(name, ext string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + ext
}
