package emit

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
		err := convertOnce(binary, docxPath, pdfPath, timeout, opts.Verbose)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < attempts-1 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	return fmt.Errorf("PDF conversion failed after %d attempt(s): %w", attempts, lastErr)
}

func convertOnce(binary, docxPath, pdfPath string, timeout time.Duration, verbose bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Keep both LibreOffice's profile and its fixed-name output private. Writing
	// into the destination directory used to let a stale or concurrent doc.pdf
	// satisfy the existence check and then get moved over the requested output.
	workDir, err := os.MkdirTemp(filepath.Dir(pdfPath), ".docc-pdf-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	profile := filepath.Join(workDir, "profile")

	// The binary comes from exec.LookPath and the paths are the caller's own
	// arguments; there is no shell involved, so no interpolation to escape.
	cmd := exec.CommandContext(ctx, binary, //nolint:gosec // fixed argv, no shell
		"-env:UserInstallation=file://"+profile,
		"--headless",
		"--norestore",
		"--convert-to", "pdf",
		"--outdir", workDir,
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
	produced := filepath.Join(workDir, replaceExt(filepath.Base(docxPath), ".pdf"))
	if _, statErr := os.Stat(produced); statErr != nil {
		return fmt.Errorf("soffice exited 0 but produced no PDF\n%s", strings.TrimSpace(string(out)))
	}
	if err := verifyPDF(produced); err != nil {
		return err
	}
	if err := os.Rename(produced, pdfPath); err != nil {
		return fmt.Errorf("move converted PDF: %w", err)
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

	info, err := f.Stat()
	if err != nil || info.Size() < 5 {
		return fmt.Errorf("%s is not a valid PDF", path)
	}
	head := make([]byte, 5)
	if _, err := io.ReadFull(f, head); err != nil || string(head) != "%PDF-" {
		return fmt.Errorf("%s is not a valid PDF", path)
	}
	tailSize := min(info.Size(), int64(1024))
	if _, err := f.Seek(-tailSize, io.SeekEnd); err != nil {
		return err
	}
	tail := make([]byte, tailSize)
	if _, err := io.ReadFull(f, tail); err != nil || !bytes.Contains(tail, []byte("%%EOF")) {
		return fmt.Errorf("%s is not a complete PDF", path)
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
