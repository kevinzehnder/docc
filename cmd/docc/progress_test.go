package main

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kevinzehnder/docc/internal/ingest"
)

// fakeClock drives the renderer's own timing. The rate and the estimate are
// arithmetic on wall-clock durations, and asserting them against a real clock
// would mean asserting nothing.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// newTestTTY builds a renderer with no redraw goroutine: the test calls
// redraw itself, so every assertion is against a known frame.
func newTestTTY() (*ttyProgress, *fakeClock, *bytes.Buffer) {
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	var buf bytes.Buffer
	p := newTTYProgress(&buf, clock.now)
	p.work = ingestWork
	return p, clock, &buf
}

func TestTTYProgressShowsPageRateAndTokens(t *testing.T) {
	p, clock, buf := newTestTTY()
	p.begin("kb-fragen.pdf")
	p.event(ingest.Event{Kind: ingest.EventRasterized, Total: 17, DPI: 200})
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 4, Seq: 4, Total: 17})

	clock.advance(12 * time.Second)
	p.event(ingest.Event{Kind: ingest.EventPageDelta, Page: 4, Seq: 4, Total: 17, Tokens: 1200})
	buf.Reset()
	p.redraw()

	got := buf.String()
	for _, want := range []string{"page 4/17", "1.2k tok", "100 tok/s", "12s"} {
		if !strings.Contains(got, want) {
			t.Errorf("status line %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "left") {
		t.Errorf("status line %q shows an estimate before any page has finished — there is no sample to estimate from", got)
	}
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("status line %q should be redrawn in place", got)
	}
}

func TestTTYProgressShowsEncodingBeforeFirstToken(t *testing.T) {
	p, clock, buf := newTestTTY()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 3})
	clock.advance(8 * time.Second)
	p.redraw()

	if got := buf.String(); !strings.Contains(got, "encoding…") || !strings.Contains(got, "8s") {
		t.Errorf("status line = %q, want it to show the wait before the first token rather than a frozen 0 tok", got)
	}
}

func TestTTYProgressEstimatesFromCompletedPages(t *testing.T) {
	p, clock, buf := newTestTTY()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 5})
	p.event(ingest.Event{Kind: ingest.EventPageDone, Page: 1, Seq: 1, Total: 5, Tokens: 100, Elapsed: 30 * time.Second})
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 2, Seq: 2, Total: 5})

	clock.advance(10 * time.Second)
	buf.Reset()
	p.redraw()

	// Four pages left at a mean of 30s, less the 10s page 2 has already spent.
	if got := buf.String(); !strings.Contains(got, "[~1m50s left]") {
		t.Errorf("status line = %q, want an estimate of 1m50s", got)
	}
}

func TestTTYProgressHidesATrivialEstimate(t *testing.T) {
	p, clock, buf := newTestTTY()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 2})
	p.event(ingest.Event{Kind: ingest.EventPageDone, Page: 1, Seq: 1, Total: 2, Tokens: 100, Elapsed: 4 * time.Second})
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 2, Seq: 2, Total: 2})

	clock.advance(time.Second)
	buf.Reset()
	p.redraw()

	if got := buf.String(); strings.Contains(got, "left") {
		t.Errorf("status line = %q, want no estimate when it is down to a few seconds", got)
	}
}

func TestTTYProgressErasesTheLongerPreviousLine(t *testing.T) {
	p, _, buf := newTestTTY()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 999})
	p.event(ingest.Event{Kind: ingest.EventPageDelta, Page: 1, Seq: 1, Total: 999, Tokens: 123456})
	p.redraw()
	long := p.lastWidth

	buf.Reset()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 2, Seq: 2, Total: 9})
	p.redraw()

	got := buf.String()
	// The padding is counted in runes: a braille spinner frame is three bytes,
	// and padding by byte count leaves the tail of the old line on screen.
	pad := long - p.lastWidth
	if pad <= 0 {
		t.Fatalf("test setup: the second line (%d runes) is not shorter than the first (%d)", p.lastWidth, long)
	}
	if !strings.Contains(got, strings.Repeat(" ", pad)) {
		t.Errorf("redraw %q does not pad the %d runes the previous line was longer by", got, pad)
	}
}

func TestTTYProgressPrintsWarningsAboveTheLine(t *testing.T) {
	p, _, buf := newTestTTY()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 2})
	p.redraw()
	buf.Reset()

	p.event(ingest.Event{Kind: ingest.EventWarning, Delta: "the server does not list a model named \"olmocr\""})
	got := buf.String()
	if !strings.Contains(got, "warning: the server does not list") {
		t.Errorf("output = %q, want the warning text", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output = %q, want the warning to stay on screen on its own line", got)
	}
	if p.lastWidth != 0 {
		t.Error("the status line should be cleared before a permanent line is written over it")
	}
}

func TestPlainProgressIsOneLinePerPage(t *testing.T) {
	var buf bytes.Buffer
	p := &plainProgress{w: &buf, work: ingestWork}
	p.begin("scan.pdf")
	p.event(ingest.Event{Kind: ingest.EventRasterized, Total: 2, DPI: 200})
	p.event(ingest.Event{Kind: ingest.EventPageDelta, Page: 1, Seq: 1, Total: 2, Tokens: 5})
	p.event(ingest.Event{Kind: ingest.EventPageDone, Page: 1, Seq: 1, Total: 2, Tokens: 1243, Elapsed: 31 * time.Second})
	p.event(ingest.Event{Kind: ingest.EventPageDone, Page: 2, Seq: 2, Total: 2, Tokens: 980, Elapsed: 24 * time.Second})
	p.finish(nil)

	got := buf.String()
	if strings.Contains(got, "\r") {
		t.Errorf("plain output must not redraw in place, got %q", got)
	}
	for _, want := range []string{"ingest scan.pdf", "rasterized 2 pages @200dpi", "page 1/2  1243 tok  31s", "page 2/2  980 tok  24s"} {
		if !strings.Contains(got, want) {
			t.Errorf("plain output missing %q\n---\n%s", want, got)
		}
	}
	// One line per milestone, and nothing per streamed chunk: a piped run
	// transcribing forty pages must not emit forty thousand lines.
	if n := strings.Count(got, "\n"); n != 5 {
		t.Errorf("plain output has %d lines, want 5 (header, rasterized, two pages, summary):\n%s", n, got)
	}
}

func TestProgressOffWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, progressOff, ingestWork)
	p.begin("scan.pdf")
	p.event(ingest.Event{Kind: ingest.EventPageDone, Seq: 1, Total: 1, Tokens: 10})
	p.finish(nil)

	if buf.Len() != 0 {
		t.Errorf("--json mode wrote %q, want nothing — stdout is the machine channel and stderr should stay quiet too", buf.String())
	}
}

func TestShortDurationAndCounts(t *testing.T) {
	if got := shortDuration(220 * time.Second); got != "3m40s" {
		t.Errorf("shortDuration(3m40s) = %q", got)
	}
	if got := shortDuration(400 * time.Millisecond); got != "0s" {
		t.Errorf("shortDuration(400ms) = %q, want a stable 0s rather than a jittering sub-second value", got)
	}
	if got := humanCount(999); got != "999" {
		t.Errorf("humanCount(999) = %q", got)
	}
	if got := humanCount(1234); got != "1.2k" {
		t.Errorf("humanCount(1234) = %q", got)
	}
	if got := shortName(strings.Repeat("a", 40)); len([]rune(got)) != 30 {
		t.Errorf("shortName capped to %d runes, want 30 — a longer name wraps the status line and \\r then leaves debris", len([]rune(got)))
	}
}

func TestTTYProgressFinishClearsAndSummarises(t *testing.T) {
	p, clock, buf := newTestTTY()
	p.begin("scan.pdf")
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 2})
	p.event(ingest.Event{Kind: ingest.EventPageDone, Page: 1, Seq: 1, Total: 2, Tokens: 10, Elapsed: time.Second})
	p.redraw()
	buf.Reset()
	clock.advance(7 * time.Second)

	p.finish(nil)
	got := buf.String()
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("finish must erase the status line first, got %q", got)
	}
	if !strings.Contains(got, "1 page in 7s") {
		t.Errorf("finish output = %q, want a singular-page summary", got)
	}
	if p.lastWidth != 0 {
		t.Error("finish left the renderer thinking a status line is still on screen")
	}
}

// A failed run's error is printed by the caller; the renderer must not add a
// summary claiming the run finished.
func TestTTYProgressFinishStaysQuietOnError(t *testing.T) {
	p, _, buf := newTestTTY()
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 2})
	p.event(ingest.Event{Kind: ingest.EventPageDone, Page: 1, Seq: 1, Total: 2, Tokens: 10, Elapsed: time.Second})
	p.redraw()
	buf.Reset()

	p.finish(errors.New("page 2: boom"))
	if strings.Contains(buf.String(), "in ") {
		t.Errorf("finish printed a summary for a failed run: %q", buf.String())
	}
}

// The ticker is what animates the spinner through the minute of silence
// before a page's first token, when no events arrive at all.
func TestTTYProgressRedrawsWithoutEvents(t *testing.T) {
	var buf lockedBuffer
	p := newProgress(&buf, progressTTY, ingestWork)
	p.begin("scan.pdf")
	p.event(ingest.Event{Kind: ingest.EventPageStart, Page: 1, Seq: 1, Total: 9})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "page 1/9") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	p.finish(nil)

	if !strings.Contains(buf.String(), "page 1/9") {
		t.Errorf("the redraw goroutine never drew a status line: %q", buf.String())
	}
}

// lockedBuffer is a bytes.Buffer safe to read while the redraw goroutine writes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestPlainProgressFlagsTruncatedPages(t *testing.T) {
	var buf bytes.Buffer
	p := &plainProgress{w: &buf, work: ingestWork}
	p.event(ingest.Event{Kind: ingest.EventPageDone, Seq: 1, Total: 1, Tokens: 4096, Elapsed: time.Minute, Truncated: true})
	p.event(ingest.Event{Kind: ingest.EventPageFailed, Seq: 1, Total: 1, Elapsed: 3 * time.Second})

	got := buf.String()
	if !strings.Contains(got, "cut off at max_tokens") {
		t.Errorf("a page that hit the token cap must say so: %q", got)
	}
	if !strings.Contains(got, "failed after 3s") {
		t.Errorf("a failed page should be reported with its elapsed time: %q", got)
	}
}

// The structuring pass is a handful of model calls with nothing between them,
// which for as long as it runs looks exactly like a server that has stopped
// answering. One renderer serves both commands; only the verb and the unit
// differ.
func TestPlainProgressReportsStructuredBlocks(t *testing.T) {
	var buf bytes.Buffer
	p := &plainProgress{w: &buf, work: structureWork}

	p.begin("replik.md")
	p.event(ingest.Event{Kind: ingest.EventBlocksFound, Total: 2})
	p.event(ingest.Event{Kind: ingest.EventBlockStart, Seq: 1, Total: 2})
	p.event(ingest.Event{Kind: ingest.EventBlockDone, Seq: 1, Total: 2, Items: 4, Elapsed: 3 * time.Second})
	p.event(ingest.Event{Kind: ingest.EventBlockDone, Seq: 2, Total: 2, Items: 1, Elapsed: 2 * time.Second})
	p.finish(nil)

	for _, want := range []string{
		"structure replik.md",
		"2 blocks to structure",
		"block 1/2  4 items  3s",
		"block 2/2  1 item  2s",
		"2 blocks in",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "page") {
		t.Errorf("structure output counts pages:\n%s", buf.String())
	}
}

// A block the pass declined to rewrite is not an error — the document still
// says what the page said — but it is the one thing in the run a reviewer has
// to go and look at, so the line says so rather than reporting "0 items".
func TestPlainProgressMarksAnUnconvertedBlock(t *testing.T) {
	var buf bytes.Buffer
	p := &plainProgress{w: &buf, work: structureWork}

	p.event(ingest.Event{Kind: ingest.EventBlockDone, Seq: 1, Total: 1, Items: 0, Elapsed: time.Second})
	if !strings.Contains(buf.String(), "left as transcribed") {
		t.Errorf("an unconverted block is not marked:\n%s", buf.String())
	}
}

// The status line during a block: no token count, because this pass asks for a
// short list and does not stream, so there is nothing to report between
// sending and receiving.
func TestTTYProgressRendersABlockLine(t *testing.T) {
	p, clock, buf := newTestTTY()
	p.work = structureWork

	p.begin("replik.md")
	p.event(ingest.Event{Kind: ingest.EventBlockStart, Seq: 2, Total: 9})
	clock.advance(4 * time.Second)
	p.redraw()

	if !strings.Contains(buf.String(), "block 2/9") {
		t.Errorf("status line missing the block position:\n%q", buf.String())
	}
	if strings.Contains(buf.String(), "tok") {
		t.Errorf("status line reports tokens for a pass that does not stream:\n%q", buf.String())
	}
}
