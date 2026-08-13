package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ConvertOptions configures one Convert run.
type ConvertOptions struct {
	// First and Last restrict conversion to a 1-based, inclusive page range;
	// zero on either end means from the first / through the last page. They
	// have no effect on image input, which is always a single page.
	First, Last int
	// DocType, if non-empty, is written into the output frontmatter.
	DocType string
	// StripRandziffern removes the source document's marginal paragraph
	// numbers instead of marking them as [Rz N]. Set it when the draft is
	// destined to become one of our own documents, whose schema generates
	// those numbers at render time; leave it false to transcribe faithfully,
	// which is what a reference document and a schema-less run both want.
	StripRandziffern bool
	// Progress, if non-nil, receives one Event per pipeline milestone. It is
	// called synchronously from the goroutine driving the conversion,
	// including once per streamed chunk, so it must not block: a renderer
	// records state and redraws on its own schedule.
	Progress func(Event)
}

// Convert runs the full pipeline for one PDF or image file: check that the VLM
// server is up, rasterize pages (skipped for image inputs, which are already
// one page), extract born-digital text-layer anchors where available, call the
// VLM once per page, and assemble the results into one markdown document.
//
// On error after at least one page has been transcribed, Convert returns the
// pages it completed and a markdown document assembled from them and marked
// incomplete — a run that dies on page 39 of 40 has thirty-eight usable pages,
// and discarding them means paying for them again. A caller that wants the
// partial draft writes it whenever the returned markdown is non-empty.
func Convert(ctx context.Context, inputPath string, cfg Config, opts ConvertOptions) (string, []PageResult, error) {
	emit := opts.Progress
	if emit == nil {
		emit = func(Event) {}
	}
	if err := CheckInput(inputPath); err != nil {
		return "", nil, err
	}

	client := NewClient(cfg)
	if err := client.Ping(ctx, func(msg string) { emit(Event{Kind: EventWarning, Delta: msg}) }); err != nil {
		return "", nil, err
	}

	isPDF := IsPDF(inputPath)
	var pages []Page
	if isPDF {
		workDir, err := os.MkdirTemp("", "docc-ingest-")
		if err != nil {
			return "", nil, err
		}
		defer func() { _ = os.RemoveAll(workDir) }()

		start := time.Now()
		emit(Event{Kind: EventRasterizing})
		rendered, err := RenderPages(ctx, inputPath, workDir, RasterOptions{DPI: cfg.DPI, First: opts.First, Last: opts.Last})
		if err != nil {
			return "", nil, err
		}
		pages = rendered
		emit(Event{Kind: EventRasterized, Total: len(pages), DPI: cfg.DPI, Elapsed: time.Since(start)})
	} else {
		pages = []Page{{Index: 1, PNGPath: inputPath}}
	}

	results := make([]PageResult, 0, len(pages))
	// One normalizer for the whole document: Randziffern count up across
	// pages, and the sequence is what tells a paragraph number from a year.
	rz := rzNormalizer{strip: opts.StripRandziffern}

	// stop hands back whatever was transcribed before failedAt, marked so that
	// neither a reader nor a later docc run mistakes it for the whole document.
	stop := func(failedAt int, err error) (string, []PageResult, error) {
		if len(results) == 0 {
			return "", nil, err
		}
		reason := err.Error()
		if errors.Is(err, context.Canceled) {
			reason = "interrupted"
		}
		return Assemble(results, AssembleOptions{
			SourceFile: filepath.Base(inputPath),
			DocType:    opts.DocType,
			Incomplete: &Incomplete{
				Completed: len(results),
				Total:     len(pages),
				NextPage:  failedAt,
				LastPage:  pages[len(pages)-1].Index,
				Reason:    reason,
			},
		}), results, err
	}

	for i, page := range pages {
		seq := i + 1
		pageStart := time.Now()
		emit(Event{Kind: EventPageStart, Page: page.Index, Seq: seq, Total: len(pages)})

		var anchorText string
		hadAnchor := false
		if cfg.Anchor && isPDF {
			anchors, err := ExtractAnchors(ctx, inputPath, page.Index, 0)
			if err != nil {
				return stop(page.Index, fmt.Errorf("page %d: %w", page.Index, err))
			}
			if len(anchors) > 0 {
				anchorText = PromptText(anchors)
				hadAnchor = true
			}
		}

		prompt, err := BuildPrompt(anchorText)
		if err != nil {
			return stop(page.Index, err)
		}

		tokens := 0
		out, err := client.CompletePageStream(ctx, page.PNGPath, prompt, func(delta string) {
			tokens++
			emit(Event{
				Kind: EventPageDelta, Page: page.Index, Seq: seq, Total: len(pages),
				Tokens: tokens, Delta: delta, Elapsed: time.Since(pageStart),
			})
		})
		if err != nil {
			err = fmt.Errorf("page %d: %w", page.Index, err)
			emit(Event{Kind: EventPageFailed, Page: page.Index, Seq: seq, Total: len(pages), Err: err, Elapsed: time.Since(pageStart)})
			// The partial text of the failing page is deliberately dropped: a
			// transcription cut off mid-sentence and merged in silently is the
			// exact failure the truncation note below exists to prevent, and
			// dropping it makes this page the clean start of a resumed run.
			return stop(page.Index, err)
		}

		res := ParsePageResponse(page.Index, out.Content)
		res.Markdown = rz.Apply(res.Markdown)
		res.HadAnchor = hadAnchor
		switch {
		case out.Truncated:
			res.LowConfidence = true
			res.Note = "response was cut off at max_tokens — page is likely incomplete"
		case isPDF && !hadAnchor:
			res.LowConfidence = true
			res.Note = "no text layer found on this page — verify carefully"
		}
		results = append(results, res)

		emit(Event{
			Kind: EventPageDone, Page: page.Index, Seq: seq, Total: len(pages),
			Tokens: out.Tokens, Truncated: out.Truncated, Elapsed: time.Since(pageStart),
		})
	}

	md := Assemble(results, AssembleOptions{
		SourceFile: filepath.Base(inputPath),
		DocType:    opts.DocType,
	})
	return md, results, nil
}
