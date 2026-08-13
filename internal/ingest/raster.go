package ingest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Page is one rasterized page, ready to send to the VLM.
type Page struct {
	// Index is the 1-based page number.
	Index int
	// PNGPath is the rendered page image.
	PNGPath string
}

// RasterOptions configures page rasterization.
type RasterOptions struct {
	// DPI controls resolution. Zero means Defaults().DPI.
	DPI int
	// First and Last restrict rasterization to a 1-based, inclusive page
	// range. Zero means from the first page / through the last page — useful
	// to try a prompt against a handful of pages before spending a VLM call
	// on every page of a long document.
	First, Last int
	// Timeout bounds the whole pdftoppm run. Zero means 120 seconds.
	Timeout time.Duration
}

// IsPDF reports whether path's extension marks it as a PDF, as opposed to an
// already-rasterized image that can skip RenderPages entirely.
func IsPDF(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}

// imageMIME is the set of image inputs ingest accepts, mapped to the media
// type its data URL has to declare. It is a map rather than a list because
// the type has to be named correctly on the wire — a JPEG announced as
// image/png is at the mercy of whether the server sniffs the bytes.
var imageMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// CheckInput reports whether path is something ingest can convert. It exists
// so that a mistyped command fails immediately, naming the offending file,
// rather than after a page has already been sent to the VLM — every page is
// minutes of work, and discovering the mistake afterwards means paying for
// the pages that did convert twice.
func CheckInput(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file", path)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s: is a directory — ingest converts one document at a time", path)
	}
	if IsPDF(path) {
		return nil
	}
	if _, ok := imageMIME[strings.ToLower(filepath.Ext(path))]; ok {
		return nil
	}
	return fmt.Errorf("%s: not a PDF or image — ingest converts %s; to name the output file use --output",
		path, strings.Join(supportedExtensions(), ", "))
}

func supportedExtensions() []string {
	exts := make([]string, 0, len(imageMIME)+1)
	exts = append(exts, ".pdf")
	for ext := range imageMIME {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

// RenderPages rasterizes every page of a PDF to PNG in outDir using pdftoppm,
// following the same discipline as internal/emit/pdf.go's soffice wrapper:
// bounded timeout, fixed argv, and verification by output rather than exit
// code, since a tool that reports success having produced nothing is worse
// than one that fails loudly.
func RenderPages(pdfPath, outDir string, opts RasterOptions) ([]Page, error) {
	binary, err := findBinary("pdftoppm")
	if err != nil {
		return nil, err
	}
	dpi := opts.DPI
	if dpi == 0 {
		dpi = Defaults().DPI
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	prefix := filepath.Join(outDir, "page")
	args := []string{"-png", "-r", strconv.Itoa(dpi)}
	if opts.First > 0 {
		args = append(args, "-f", strconv.Itoa(opts.First))
	}
	if opts.Last > 0 {
		args = append(args, "-l", strconv.Itoa(opts.Last))
	}
	args = append(args, pdfPath, prefix)

	// binary comes from exec.LookPath and the paths are the caller's own
	// arguments; there is no shell involved, so no interpolation to escape.
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // fixed argv, no shell
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("pdftoppm timed out after %s", timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("pdftoppm: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	pages, err := collectPages(outDir)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no pages for %s", pdfPath)
	}
	return pages, nil
}

var pageFileRe = regexp.MustCompile(`^page-(\d+)\.png$`)

// collectPages globs pdftoppm's output. pdftoppm pads page numbers to the
// width of the highest page number in the document, so the names cannot be
// predicted in advance — they have to be listed back out.
func collectPages(dir string) ([]Page, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var pages []Page
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := pageFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() == 0 {
			return nil, fmt.Errorf("pdftoppm produced an empty page image %s", e.Name())
		}
		pages = append(pages, Page{Index: n, PNGPath: filepath.Join(dir, e.Name())})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Index < pages[j].Index })
	return pages, nil
}
