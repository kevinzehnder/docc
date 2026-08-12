package ingest

import "strings"

// PageResult is one page's transcription.
type PageResult struct {
	// Index is the 1-based page number.
	Index int
	// Markdown is the page's transcribed body text.
	Markdown string
	// HadAnchor reports whether a born-digital text layer was available for
	// this page.
	HadAnchor bool
	// LowConfidence flags a page with no text layer to cross-check against,
	// so the reviewer knows to look at it more closely.
	LowConfidence bool
	// Note explains why LowConfidence is set, or is empty.
	Note string
}

// ParsePageResponse wraps the VLM's raw response as a PageResult: the
// response is the page's markdown, verbatim.
func ParsePageResponse(index int, raw string) PageResult {
	return PageResult{Index: index, Markdown: strings.TrimSpace(raw)}
}
