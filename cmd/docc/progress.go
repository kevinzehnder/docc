package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kevinzehnder/docc/internal/ingest"
)

// progress renders the events of one ingest run. A conversion is minutes of
// otherwise silent work, and a run that has hung is indistinguishable from one
// that is working unless something says so.
//
// Implementations are called from the goroutine driving the conversion, once
// per streamed chunk, so event must be cheap.
type progress interface {
	// begin announces the input file.
	begin(input string)
	event(ev ingest.Event)
	// finish clears any transient output and reports the run, so that
	// whatever the caller prints next starts on a clean line.
	finish(err error)
}

type progressMode int

const (
	// progressOff writes nothing: --json owns stdout, and a machine reading
	// it has no use for a spinner on stderr either.
	progressOff progressMode = iota
	// progressPlain writes one line per finished page — what a log wants.
	progressPlain
	// progressTTY redraws a single status line in place.
	progressTTY
)

// progressModeFor mirrors commonFlags.color, but against stderr: progress is
// diagnostic output, so stdout stays the machine channel and a script can keep
// reading the output path from it.
func progressModeFor(jsonOut bool) progressMode {
	if jsonOut {
		return progressOff
	}
	info, err := os.Stderr.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 || os.Getenv("TERM") == "dumb" {
		return progressPlain
	}
	return progressTTY
}

func newProgress(w io.Writer, mode progressMode) progress {
	switch mode {
	case progressTTY:
		p := newTTYProgress(w, time.Now)
		p.stop, p.done = make(chan struct{}), make(chan struct{})
		go p.run()
		return p
	case progressPlain:
		return &plainProgress{w: w}
	default:
		return offProgress{}
	}
}

// writef writes progress output, dropping any error. A status line that
// cannot be printed is not a reason to abandon a conversion that is otherwise
// working — the draft, not the spinner, is what the run is for.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

type offProgress struct{}

func (offProgress) begin(string)       {}
func (offProgress) event(ingest.Event) {}
func (offProgress) finish(error)       {}

// plainProgress writes one newline-terminated line per milestone: no carriage
// returns, nothing that assumes a cursor.
type plainProgress struct {
	w         io.Writer
	total     int
	completed int
	start     time.Time
}

func (p *plainProgress) begin(input string) {
	p.start = time.Now()
	writef(p.w, "ingest %s\n", input)
}

func (p *plainProgress) event(ev ingest.Event) {
	if ev.Total > 0 {
		p.total = ev.Total
	}
	switch ev.Kind {
	case ingest.EventRasterized:
		writef(p.w, "  rasterized %s @%ddpi\n", pageCount(ev.Total), ev.DPI)
	case ingest.EventPageDone:
		p.completed++
		writef(p.w, "  page %d/%d  %d tok  %s%s\n",
			ev.Seq, ev.Total, ev.Tokens, shortDuration(ev.Elapsed), truncatedSuffix(ev.Truncated))
	case ingest.EventPageFailed:
		writef(p.w, "  page %d/%d  failed after %s\n", ev.Seq, ev.Total, shortDuration(ev.Elapsed))
	case ingest.EventWarning:
		writef(p.w, "  warning: %s\n", ev.Delta)
	}
}

func (p *plainProgress) finish(err error) {
	if err == nil && p.completed > 0 {
		writef(p.w, "  %s in %s\n", pageCount(p.completed), shortDuration(time.Since(p.start)))
	}
}

// spinnerFrames animate during the minute or more a page can spend in prompt
// evaluation, before the first token arrives and there is anything to count.
var spinnerFrames = [...]rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// redrawInterval is fast enough to look continuous and slow enough that the
// write cost is irrelevant next to a page that takes half a minute.
const redrawInterval = 100 * time.Millisecond

type phase int

const (
	phaseIdle phase = iota
	phaseRaster
	phasePage
)

// ttyProgress redraws one status line in place.
//
// The line is fixed-format and stays under 60 runes by construction — the only
// unbounded string in it is the input filename, capped by shortName. That
// budget matters: on a line long enough to wrap, a carriage return moves the
// cursor to the start of the last physical row, and every earlier row is left
// on screen as debris. Terminal width is not detected, because the standard
// library cannot ask for it and a wrong guess is worse than no guess.
type ttyProgress struct {
	w   io.Writer
	now func() time.Time

	// mu guards everything below, and is held across the write in redraw so
	// that a warning cannot interleave with a status line. event never
	// writes, so the conversion goroutine only ever waits on one short write.
	mu    sync.Mutex
	frame int
	// lastWidth is in runes, not bytes: a braille spinner frame is three
	// bytes, and padding by byte count leaves debris on screen.
	lastWidth int
	st        lineState

	// stop and done are nil unless the redraw goroutine is running, which
	// lets a test drive redraw itself and still call finish.
	stop chan struct{}
	done chan struct{}
}

func newTTYProgress(w io.Writer, now func() time.Time) *ttyProgress {
	return &ttyProgress{w: w, now: now}
}

type lineState struct {
	phase            phase
	input            string
	page, seq, total int
	tokens           int
	runStart         time.Time
	pageStart        time.Time
	// completed and completedFor are the sample the ETA is estimated from.
	completed    int
	completedFor time.Duration
}

func (p *ttyProgress) begin(input string) {
	p.mu.Lock()
	p.st.input = input
	p.st.runStart = p.now()
	p.mu.Unlock()
	writef(p.w, "ingest %s\n", input)
}

func (p *ttyProgress) event(ev ingest.Event) {
	if ev.Kind == ingest.EventWarning {
		p.printAbove("  warning: " + ev.Delta)
		return
	}
	if ev.Kind == ingest.EventRasterized {
		p.mu.Lock()
		p.st.phase = phaseIdle
		p.mu.Unlock()
		p.printAbove(fmt.Sprintf("  rasterized %s @%ddpi", pageCount(ev.Total), ev.DPI))
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	switch ev.Kind {
	case ingest.EventRasterizing:
		p.st.phase = phaseRaster
	case ingest.EventPageStart:
		p.st.phase = phasePage
		p.st.page, p.st.seq, p.st.total = ev.Page, ev.Seq, ev.Total
		p.st.tokens = 0
		p.st.pageStart = p.now()
	case ingest.EventPageDelta:
		p.st.tokens = ev.Tokens
	case ingest.EventPageDone:
		p.st.tokens = ev.Tokens
		p.st.completed++
		p.st.completedFor += ev.Elapsed
	case ingest.EventPageFailed:
		p.st.phase = phaseIdle
	}
}

func (p *ttyProgress) finish(err error) {
	if p.stop != nil {
		close(p.stop)
		<-p.done
	}

	p.mu.Lock()
	completed, since := p.st.completed, p.now().Sub(p.st.runStart)
	p.clearLocked()
	p.mu.Unlock()

	if err == nil && completed > 0 {
		writef(p.w, "  %s in %s\n", pageCount(completed), shortDuration(since))
	}
}

func (p *ttyProgress) run() {
	t := time.NewTicker(redrawInterval)
	defer t.Stop()
	defer close(p.done)
	for {
		select {
		case <-t.C:
			p.redraw()
		case <-p.stop:
			return
		}
	}
}

func (p *ttyProgress) redraw() {
	p.mu.Lock()
	defer p.mu.Unlock()

	line := p.st.render(spinnerFrames[p.frame], p.now())
	p.frame = (p.frame + 1) % len(spinnerFrames)
	if line == "" {
		p.clearLocked()
		return
	}

	width := utf8.RuneCountInString(line)
	pad := max(p.lastWidth-width, 0)
	p.lastWidth = width
	writef(p.w, "%s", "\r"+line+strings.Repeat(" ", pad)+"\r")
}

// printAbove writes a line that stays on screen, without leaving remnants of
// the status line on it.
func (p *ttyProgress) printAbove(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearLocked()
	writef(p.w, "%s\n", s)
}

func (p *ttyProgress) clearLocked() {
	if p.lastWidth == 0 {
		return
	}
	writef(p.w, "%s", "\r"+strings.Repeat(" ", p.lastWidth)+"\r")
	p.lastWidth = 0
}

// render builds the status line, or returns "" when there is nothing running.
func (s lineState) render(spinner rune, now time.Time) string {
	switch s.phase {
	case phaseRaster:
		return fmt.Sprintf("  %c rasterizing %s…  %s", spinner, shortName(s.input), shortDuration(now.Sub(s.runStart)))
	case phasePage:
		elapsed := now.Sub(s.pageStart)
		var b strings.Builder
		fmt.Fprintf(&b, "  %c page %d/%d  ", spinner, s.seq, s.total)
		if s.tokens == 0 {
			// Prompt evaluation of a page image: the server has the request
			// and has not produced a token yet.
			b.WriteString("encoding…  ")
		} else {
			fmt.Fprintf(&b, "%s tok  ", humanCount(s.tokens))
			// Below half a second the rate is dominated by the first chunk
			// and reads as nonsense.
			if elapsed > 500*time.Millisecond {
				fmt.Fprintf(&b, "%.0f tok/s  ", float64(s.tokens)/elapsed.Seconds())
			}
		}
		b.WriteString(shortDuration(elapsed))
		if eta, ok := s.eta(now); ok {
			fmt.Fprintf(&b, "   [~%s left]", shortDuration(eta))
		}
		return b.String()
	default:
		return ""
	}
}

// eta estimates the time left from the mean duration of the pages already
// done, discounting what the page in flight has already spent. It stays hidden
// until there is a sample: a guess made from nothing is worse than no number.
func (s lineState) eta(now time.Time) (time.Duration, bool) {
	if s.completed == 0 || s.total == 0 || s.completed >= s.total {
		return 0, false
	}
	// etaFloor hides the estimate once it is down to noise: "[~0s left]" on the
	// last page of a run is a number that tells the reader nothing.
	const etaFloor = 5 * time.Second

	mean := s.completedFor / time.Duration(s.completed)
	left := mean*time.Duration(s.total-s.completed) - now.Sub(s.pageStart)
	if left < etaFloor {
		return 0, false
	}
	return left, true
}

// shortName caps a filename so the status line cannot wrap. See ttyProgress.
func shortName(s string) string {
	const maxName = 30
	if utf8.RuneCountInString(s) <= maxName {
		return s
	}
	return string([]rune(s)[:maxName-1]) + "…"
}

func humanCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func shortDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	return d.Round(time.Second).String()
}

func pageCount(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}

func truncatedSuffix(truncated bool) string {
	if truncated {
		return "  (cut off at max_tokens)"
	}
	return ""
}
