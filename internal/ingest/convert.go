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
//
// firstPage and lastPage restrict conversion to a 1-based, inclusive page
// range; zero on either end means from the first / through the last page.
// They have no effect on image input, which is always a single page.
func Convert(ctx context.Context, inputPath string, cfg Config, firstPage, lastPage int, opts AssembleOptions) (string, []PageResult, error) {
	client := NewClient(cfg)
	isPDF := IsPDF(inputPath)

	var pages []Page
	if isPDF {
		workDir, err := os.MkdirTemp("", "docc-ingest-")
		if err != nil {
			return "", nil, err
		}
		defer func() { _ = os.RemoveAll(workDir) }()

		rendered, err := RenderPages(inputPath, workDir, RasterOptions{DPI: cfg.DPI, First: firstPage, Last: lastPage})
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

		raw, truncated, err := client.CompletePage(ctx, page.PNGPath, prompt)
		if err != nil {
			return "", nil, fmt.Errorf("page %d: %w", page.Index, err)
		}

		res := ParsePageResponse(page.Index, raw)
		res.HadAnchor = hadAnchor
		switch {
		case truncated:
			res.LowConfidence = true
			res.Note = "response was cut off at max_tokens — page is likely incomplete"
		case isPDF && !hadAnchor:
			res.LowConfidence = true
			res.Note = "no text layer found on this page — verify carefully"
		}
		results = append(results, res)
	}

	opts.SourceFile = filepath.Base(inputPath)
	md := Assemble(results, opts)
	return md, results, nil
}
