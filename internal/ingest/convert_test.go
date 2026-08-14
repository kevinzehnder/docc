package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// vlmServer stands in for a llama-server: it answers preflight, and streams
// one scripted response per page in order, so a test can make page 2 fail.
type vlmServer struct {
	*httptest.Server
	mu    sync.Mutex
	calls int
}

// newVLMServer serves the given per-page responses. A response is either
// markdown to stream back, or "" to answer with a 500.
func newVLMServer(t *testing.T, responses ...string) *vlmServer {
	t.Helper()
	s := &vlmServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		n := s.calls
		s.calls++
		s.mu.Unlock()

		body := ""
		if n < len(responses) {
			body = responses[n]
		}
		if body == "" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("out of memory"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, word := range strings.SplitAfter(body, " ") {
			_, _ = w.Write([]byte(dataFrame(word, "") + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte(dataFrame("", "stop") + "\n\ndata: [DONE]\n\n"))
		flusher.Flush()
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *vlmServer) config() Config {
	cfg := Defaults()
	cfg.Endpoint = s.URL + "/v1/chat/completions"
	cfg.Model = "test-vlm"
	cfg.DPI = 72
	cfg.Anchor = false // pdftotext is a separate binary; anchoring is tested in anchor_test.go
	return cfg
}

// collector records events from a conversion. Convert calls the hook from its
// own goroutine, so the slice needs a lock even though nothing here is
// concurrent by design.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) hook(ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *collector) kinds() []EventKind {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]EventKind, 0, len(c.events))
	for _, ev := range c.events {
		out = append(out, ev.Kind)
	}
	return out
}

func requirePDFTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not on PATH")
	}
}

func TestConvertReportsProgress(t *testing.T) {
	requirePDFTools(t)
	srv := newVLMServer(t, "# Klage gegen X")
	pdfPath := writeMinimalPDF(t, t.TempDir(), "Progress Test")

	var c collector
	md, pages, err := Convert(context.Background(), pdfPath, srv.config(), ConvertOptions{Progress: c.hook})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if !strings.Contains(md, "# Klage gegen X") {
		t.Errorf("assembled document missing the transcription:\n%s", md)
	}

	kinds := c.kinds()
	if len(kinds) < 5 {
		t.Fatalf("got %d events, want the rasterize pair, a page start, deltas and a page done: %v", len(kinds), kinds)
	}
	if kinds[0] != EventRasterizing || kinds[1] != EventRasterized {
		t.Errorf("first events = %v, want rasterizing then rasterized — pdftoppm is silent for seconds and needs its own event", kinds[:2])
	}
	if kinds[2] != EventPageStart {
		t.Errorf("third event = %v, want a page start", kinds[2])
	}
	if kinds[len(kinds)-1] != EventPageDone {
		t.Errorf("last event = %v, want a page done", kinds[len(kinds)-1])
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	sawDelta := false
	for _, ev := range c.events {
		switch ev.Kind {
		case EventRasterized:
			if ev.Total != 1 || ev.DPI != 72 {
				t.Errorf("rasterized event = %+v, want Total 1 at 72 dpi", ev)
			}
		case EventPageDelta:
			sawDelta = true
			if ev.Tokens < 1 || ev.Seq != 1 || ev.Total != 1 {
				t.Errorf("delta event = %+v, want a running token count and page 1/1", ev)
			}
		case EventPageDone:
			if ev.Tokens < 1 {
				t.Errorf("done event = %+v, want a final token count", ev)
			}
		}
	}
	if !sawDelta {
		t.Error("no EventPageDelta — without streamed deltas there is nothing to show while a page runs")
	}
}

func TestConvertKeepsCompletedPagesWhenOneFails(t *testing.T) {
	requirePDFTools(t)
	// Two pages transcribe, the third fails: the point of the whole exercise
	// is that the first two survive rather than being paid for twice.
	srv := newVLMServer(t, "page one text", "page two text", "")
	pdfPath := writeMultiPagePDF(t, t.TempDir(), 3)

	var c collector
	md, pages, err := Convert(context.Background(), pdfPath, srv.config(), ConvertOptions{Progress: c.hook})
	if err == nil {
		t.Fatal("expected an error when the server rejects a page")
	}
	if !strings.Contains(err.Error(), "out of memory") {
		t.Errorf("error = %v, want it to carry the server's message", err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d completed pages, want the 2 that succeeded before the failure", len(pages))
	}
	want := []string{
		"page one text",
		"page two text",
		"<!-- INCOMPLETE — docc ingest stopped after 2 of 3 pages:",
		"--pages 3 --output multipage.pages-3.md multipage.pdf",
		"transcription stops here",
	}
	for _, w := range want {
		if !strings.Contains(md, w) {
			t.Errorf("partial document missing %q\n---\n%s", w, md)
		}
	}

	kinds := c.kinds()
	if kinds[len(kinds)-1] != EventPageFailed {
		t.Errorf("last event = %v, want a page failed", kinds[len(kinds)-1])
	}
}

func TestConvertKeepsNothingWhenTheFirstPageFails(t *testing.T) {
	requirePDFTools(t)
	srv := newVLMServer(t, "")
	pdfPath := writeMinimalPDF(t, t.TempDir(), "Failure Test")

	md, pages, err := Convert(context.Background(), pdfPath, srv.config(), ConvertOptions{})
	if err == nil {
		t.Fatal("expected an error when the server rejects the page")
	}
	if md != "" || pages != nil {
		t.Errorf("a run that transcribed nothing must return no document, got %d pages:\n%s", len(pages), md)
	}
}

func TestConvertFailsFastWhenNoServerIsListening(t *testing.T) {
	requirePDFTools(t)
	srv := newVLMServer(t, "unused")
	cfg := srv.config()
	srv.Close()

	pdfPath := writeMinimalPDF(t, t.TempDir(), "Preflight Test")
	var c collector
	_, _, err := Convert(context.Background(), pdfPath, cfg, ConvertOptions{Progress: c.hook})
	if err == nil {
		t.Fatal("expected an error when no VLM server is listening")
	}
	if !strings.Contains(err.Error(), "no VLM server answering") {
		t.Errorf("error = %v, want the preflight message", err)
	}
	if kinds := c.kinds(); len(kinds) != 0 {
		t.Errorf("events = %v, want none — preflight runs before rasterization so the user does not wait for pdftoppm first", kinds)
	}
}

// A Ctrl-C is the most likely way a long run ends early, so it gets the same
// treatment as a failure: keep what was transcribed, and say why it stopped.
func TestConvertCancelledRunReportsInterrupted(t *testing.T) {
	requirePDFTools(t)

	calls := 0
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := calls
		calls++
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		if n > 0 {
			<-r.Context().Done() // the second page hangs until the run is cancelled
			return
		}
		_, _ = w.Write([]byte(dataFrame("page one text", "stop") + "\n\ndata: [DONE]\n\n"))
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Defaults()
	cfg.Endpoint = srv.URL + "/v1/chat/completions"
	cfg.Model, cfg.DPI, cfg.Anchor = "test-vlm", 72, false

	pdfPath := writeMultiPagePDF(t, t.TempDir(), 2)
	ctx, cancel := context.WithCancel(context.Background())
	md, pages, err := Convert(ctx, pdfPath, cfg, ConvertOptions{
		Progress: func(ev Event) {
			if ev.Kind == EventPageStart && ev.Seq == 2 {
				cancel()
			}
		},
	})
	defer cancel()

	if err == nil {
		t.Fatal("expected an error for a cancelled run")
	}
	if len(pages) != 1 {
		t.Fatalf("got %d completed pages, want the 1 finished before the interrupt", len(pages))
	}
	if !strings.Contains(md, "pages: interrupted -->") {
		t.Errorf("partial document should name the interrupt as the reason, not a raw context error:\n%s", md)
	}
	if !strings.Contains(md, "--pages 2") {
		t.Errorf("partial document missing the resume hint:\n%s", md)
	}
}

// The normalizer has to run inside the pipeline and keep its sequence across
// pages, or a document's numbering restarts at every page boundary.
func TestConvertMarksRandziffernAcrossPages(t *testing.T) {
	requirePDFTools(t)
	srv := newVLMServer(t,
		"1 Die vorliegende Eingabe erfolgt innert Frist.",
		"2 Daran ändert nichts, dass mehrere Streitgegenstände vorliegen.",
	)
	pdfPath := writeMultiPagePDF(t, t.TempDir(), 2)

	md, _, err := Convert(context.Background(), pdfPath, srv.config(), ConvertOptions{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	for _, want := range []string{"[Rz 1] Die vorliegende", "[Rz 2] Daran ändert"} {
		if !strings.Contains(md, want) {
			t.Errorf("assembled document missing %q\n---\n%s", want, md)
		}
	}
}
