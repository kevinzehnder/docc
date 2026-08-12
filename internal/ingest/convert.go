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
//
// plain switches to a literal transcription with no docc-specific behavior:
// Randziffern are transcribed as-is rather than stripped and reported
// separately, and the returned PageResults carry no RzSeq — Verify has
// nothing to check in that mode and should not be called on the result.
func Convert(ctx context.Context, inputPath string, cfg Config, firstPage, lastPage int, plain bool, opts AssembleOptions) (string, []PageResult, error) {
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

		buildPrompt, parseResponse := BuildPrompt, ParsePageResponse
		if plain {
			buildPrompt, parseResponse = BuildPlainPrompt, ParsePlainResponse
		}

		prompt, err := buildPrompt(anchorText)
		if err != nil {
			return "", nil, err
		}

		raw, err := client.CompletePage(ctx, page.PNGPath, prompt)
		if err != nil {
			return "", nil, fmt.Errorf("page %d: %w", page.Index, err)
		}

		res := parseResponse(page.Index, raw)
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
