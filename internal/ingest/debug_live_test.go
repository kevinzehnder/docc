package ingest

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
)

// TestDebugPage is a manual debugging aid, not part of the suite. It runs the
// layout and recognition passes against a live model and dumps every block —
// including furniture and gutter — with its geometry and the gutter split's
// decisions. Gated on DOCC_DEBUG_PDF so `go test ./...` never touches it.
//
//	DOCC_DEBUG_PDF=assets/x.pdf DOCC_DEBUG_PAGE=6 go test -run TestDebugPage -v ./internal/ingest
func TestDebugPage(t *testing.T) {
	pdf := os.Getenv("DOCC_DEBUG_PDF")
	if pdf == "" {
		t.Skip("set DOCC_DEBUG_PDF to run")
	}
	pageNo := 1
	if s := os.Getenv("DOCC_DEBUG_PAGE"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatal(err)
		}
		pageNo = n
	}
	endpoint := os.Getenv("DOCC_DEBUG_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8080/v1/chat/completions"
	}
	model := os.Getenv("DOCC_DEBUG_MODEL")
	if model == "" {
		model = "mineru-pro-2605"
	}

	cfg := Defaults()
	cfg.Endpoint = endpoint
	cfg.Model = model
	cfg.Backend = BackendMinerU
	if s := os.Getenv("DOCC_DEBUG_DPI"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatal(err)
		}
		cfg.DPI = n
	}

	ctx := context.Background()
	dir := t.TempDir()
	pages, err := RenderPages(ctx, pdf, dir, RasterOptions{DPI: cfg.DPI, First: pageNo, Last: pageNo})
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient(cfg)
	img, err := loadImage(pages[0].PNGPath)
	if err != nil {
		t.Fatal(err)
	}
	layoutURL, err := pngDataURL(layoutImage(img))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.CompleteImage(ctx, layoutURL, layoutPrompt, CallOptions{MaxTokens: layoutMaxToken}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("=== raw layout answer ===\n%s\n=== end ===\n", raw.Content)

	blocks, err := ParseLayout(raw.Content)
	if err != nil {
		t.Fatal(err)
	}

	for i := range blocks {
		b := &blocks[i]
		if visual[b.Type] || container[b.Type] {
			continue
		}
		crop := cropBlock(img, b.Box)
		if crop == nil {
			continue
		}
		cropURL, err := pngDataURL(crop)
		if err != nil {
			t.Fatal(err)
		}
		task, ok := recognition[b.Type]
		if !ok {
			task = defaultRecognition
		}
		res, err := client.CompleteImage(ctx, cropURL, task.prompt, task.opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		b.Text = res.Content
	}

	full := os.Getenv("DOCC_DEBUG_FULL") != ""
	fmt.Printf("\n=== %d blocks (page %d) ===\n", len(blocks), pageNo)
	for i, b := range blocks {
		text := b.Text
		if !full && len(text) > 80 {
			text = text[:80] + "…"
		}
		fmt.Printf("[%2d] %-16s x %.3f-%.3f  y %.3f-%.3f  %q\n", i, b.Type, b.Box.X0, b.Box.X1, b.Box.Y0, b.Box.Y1, text)
	}

	left := bodyLeft(blocks)
	fmt.Printf("\nbodyLeft = %.3f (gutter cut at %.3f)\n", left, left-marginGap)
	body, margins := splitGutter(blocks)
	for _, b := range body {
		mark := ""
		if n, ok := margins[b.index]; ok {
			mark = "  <- Rz " + n
		}
		text := b.Text
		if len(text) > 60 {
			text = text[:60] + "…"
		}
		fmt.Printf("body[%2d] %-16s %q%s\n", b.index, b.Type, text, mark)
	}
}
