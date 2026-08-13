package ingest

import "time"

// EventKind identifies which milestone of the pipeline an Event reports.
type EventKind int

const (
	// EventRasterizing reports that pdftoppm has started. The page count is
	// not known yet, and on a long document this step is itself silent for
	// ten seconds or more.
	EventRasterizing EventKind = iota + 1
	// EventRasterized reports Total and DPI. It never fires for image input,
	// which is already one page.
	EventRasterized
	// EventPageStart reports that anchor extraction and the VLM call for a
	// page are about to begin.
	EventPageStart
	// EventPageDelta reports one streamed chunk. Tokens is the running count.
	EventPageDelta
	// EventPageDone reports a finished page: Tokens, Elapsed and Truncated
	// are final.
	EventPageDone
	// EventPageFailed reports a page that errored. Conversion stops after it.
	EventPageFailed
	// EventWarning reports something the user should see but that does not
	// stop the run, in Delta.
	EventWarning
)

// Event is one progress notification from Convert.
type Event struct {
	Kind EventKind
	// Page is the 1-based document page number and Seq its position within
	// this run's page set: under --pages 8-17, page 8 is Seq 1 of Total 10.
	// The distinction matters — Seq drives "4/10", Page drives the resume
	// hint.
	Page, Seq, Total int
	// DPI is set on EventRasterized.
	DPI int
	// Tokens is the running count on EventPageDelta and the final count on
	// EventPageDone.
	Tokens int
	// Delta carries the text just received on EventPageDelta, and the message
	// on EventWarning. It is informational: a page must not be reassembled
	// from it.
	Delta string
	// Elapsed is measured from the start of the page, or of the run for the
	// rasterization events.
	Elapsed time.Duration
	// Truncated is set on EventPageDone when the response hit max_tokens.
	Truncated bool
	// Err is set on EventPageFailed.
	Err error
}
