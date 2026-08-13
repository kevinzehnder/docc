package ingest

// PageResult is one page's transcription.
type PageResult struct {
	// Index is the 1-based page number.
	Index int
	// Nodes is the page as document elements, as the backend found them.
	//
	// It is the representation the passes are moving onto. Markdown below is
	// still what several of them read, and is rendered from these — so the two
	// agree by construction rather than by anybody remembering to keep them in
	// step.
	Nodes []Node
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

// NewPageResult wraps a backend's elements as one page's result, rendering the
// markdown the passes that still work on text consume.
func NewPageResult(index int, nodes []Node) PageResult {
	return PageResult{Index: index, Nodes: nodes, Markdown: Render(nodes)}
}
