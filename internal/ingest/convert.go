package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Convert runs the full pipeline for one PDF or image file: rasterize pages
// (skipped for image inputs, which are already one page), extract
// born-digital text-layer anchors where available, call the VLM once per
// page, and assemble the results into one markdown document.
func Convert(ctx context.Context, inputPath string, cfg Config, opts AssembleOptions) (string, []PageResult, error) {
	client := NewClient(cfg)
	isPDF := IsPDF(inputPath)

	var pages []Page
	if isPDF {
		workDir, err := os.MkdirTemp("", "docc-ingest-")
		if err != nil {
			return "", nil, err
		}
		defer func() { _ = os.RemoveAll(workDir) }()

		rendered, err := RenderPages(inputPath, workDir, RasterOptions{DPI: cfg.DPI})
		if err != nil {
			return "", nil, err
		}
		pages = rendered
	} else {
		pages = []Page{{Index: 1, PNGPath: inputPath}}
	}

	results := make([]PageResult, 0, len(pages))
	for _, page := range pages {
		var anchorText string
		hadAnchor := false
		if cfg.Anchor && isPDF {
			anchors, err := ExtractAnchors(inputPath, page.Index, 0)
			if err != nil {
				return "", nil, fmt.Errorf("page %d: %w", page.Index, err)
			}
			if len(anchors) > 0 {
				anchorText = PromptText(anchors)
				hadAnchor = true
			}
		}

		prompt, err := BuildPrompt(anchorText)
		if err != nil {
			return "", nil, err
		}

		raw, err := client.CompletePage(ctx, page.PNGPath, prompt)
		if err != nil {
			return "", nil, fmt.Errorf("page %d: %w", page.Index, err)
		}

		res := ParsePageResponse(page.Index, raw)
		res.HadAnchor = hadAnchor
		if isPDF && !hadAnchor {
			res.LowConfidence = true
			if res.Note == "" {
				res.Note = "no text layer found on this page — verify carefully"
			}
		}
		results = append(results, res)
	}

	opts.SourceFile = filepath.Base(inputPath)
	md := Assemble(results, opts)
	return md, results, nil
}
