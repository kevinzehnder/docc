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
	// Timeout bounds the whole pdftoppm run. Zero means 120 seconds.
	Timeout time.Duration
}

// IsPDF reports whether path's extension marks it as a PDF, as opposed to an
// already-rasterized image that can skip RenderPages entirely.
func IsPDF(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pdf")
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
	// binary comes from exec.LookPath and the paths are the caller's own
	// arguments; there is no shell involved, so no interpolation to escape.
	cmd := exec.CommandContext(ctx, binary, //nolint:gosec // fixed argv, no shell
		"-png",
		"-r", strconv.Itoa(dpi),
		pdfPath,
		prefix,
	)
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
