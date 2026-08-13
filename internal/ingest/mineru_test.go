package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// minerUServer stands in for a llama-server hosting MinerU2.5: it answers the
// layout prompt with one canned block list and every recognition prompt from a
// queue, so a test drives both passes without a model.
type minerUServer struct {
	*httptest.Server
	layout string

	mu       sync.Mutex
	recog    []string
	prompts  []string
	recogged int
}

func newMinerUServer(t *testing.T, layout string, recognitions ...string) *minerUServer {
	t.Helper()
	s := &minerUServer{layout: layout, recog: recognitions}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		prompt := ""
		for _, m := range req.Messages {
			for _, c := range m.Content {
				if c.Type == "text" {
					prompt = c.Text
				}
			}
		}

		s.mu.Lock()
		s.prompts = append(s.prompts, prompt)
		body := ""
		switch prompt {
		case layoutPrompt:
			body = s.layout
		default:
			if s.recogged < len(s.recog) {
				body = s.recog[s.recogged]
			}
			s.recogged++
		}
		s.mu.Unlock()

		if body == "" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("no scripted response"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(dataFrame(body, "") + "\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(dataFrame("", "stop") + "\n\ndata: [DONE]\n\n"))
		flusher.Flush()
	})

	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *minerUServer) config() Config {
	cfg := Defaults()
	cfg.Backend = BackendMinerU
	cfg.Endpoint = s.URL + "/v1/chat/completions"
	cfg.Model = "MinerU2.5"
	cfg.Anchor = false
	return cfg
}

func (s *minerUServer) sentPrompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.prompts...)
}

// writePageImage puts a plain white page on disk for the backend to crop.
func writePageImage(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "page.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The whole pipeline in one test: layout, one recognition call per non-visual
// block, furniture dropped, heading marked, gutter number attached.
func TestMinerUPageRunsBothPasses(t *testing.T) {
	layout := strings.Join([]string{
		"887 027 936 060header",
		"131 175 218 189title",
		"085 193 102 203text",
		"131 193 870 532text",
		"871 959 895 972page_number",
	}, "\n")
	srv := newMinerUServer(t, layout, "Zu Rz. 80", "55", "Richtig ist, dass ...")

	backend, err := NewBackend(srv.config())
	if err != nil {
		t.Fatal(err)
	}
	out, err := backend.Page(context.Background(), Page{Index: 1, PNGPath: writePageImage(t, 1240, 1754)}, "", nil)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}

	want := "## Zu Rz. 80\n\n[Rz 55] Richtig ist, dass ..."
	if out.Markdown != want {
		t.Errorf("markdown =\n%q\nwant\n%q", out.Markdown, want)
	}

	// The header and page number never cost a round trip: they are dropped
	// before recognition, not after.
	prompts := srv.sentPrompts()
	if len(prompts) != 4 {
		t.Fatalf("sent %d calls (%v), want 4: one layout and three blocks", len(prompts), prompts)
	}
	if prompts[0] != layoutPrompt {
		t.Errorf("first call was %q, want the layout prompt", prompts[0])
	}
}

// A table gets its own prompt, because it comes back as HTML and needs the
// sampling parameters that keep a decoder from repeating a row forever.
func TestMinerUUsesTheTablePrompt(t *testing.T) {
	srv := newMinerUServer(t, "131 175 870 400table", "<table><tr><td>x</td></tr></table>")

	backend, err := NewBackend(srv.config())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Page(context.Background(), Page{Index: 1, PNGPath: writePageImage(t, 800, 1000)}, "", nil); err != nil {
		t.Fatalf("Page: %v", err)
	}

	prompts := srv.sentPrompts()
	if len(prompts) != 2 || prompts[1] != tablePrompt {
		t.Errorf("prompts = %q, want the layout prompt then %q", prompts, tablePrompt)
	}
}

// A layout response that does not parse fails the page by name rather than
// producing a page with a hole in it.
func TestMinerUFailsOnUnparseableLayout(t *testing.T) {
	srv := newMinerUServer(t, "I could not find any blocks on this page.")

	backend, err := NewBackend(srv.config())
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Page(context.Background(), Page{Index: 1, PNGPath: writePageImage(t, 800, 1000)}, "", nil)
	if err == nil {
		t.Fatal("Page succeeded on an unparseable layout response")
	}
	if !strings.Contains(err.Error(), "layout line 1") {
		t.Errorf("error %q does not name the offending line", err)
	}
}

// Anchoring has nowhere to go in this protocol, and saying so once at preflight
// is better than a run that quietly ignores a configured setting.
func TestMinerUWarnsThatAnchoringIsIgnored(t *testing.T) {
	srv := newMinerUServer(t, "131 175 870 400text", "text")
	cfg := srv.config()
	cfg.Anchor = true

	backend, err := NewBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var warnings []string
	if err := backend.Ping(context.Background(), func(msg string) { warnings = append(warnings, msg) }); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !slicesContainsSubstring(warnings, "ignores anchor") {
		t.Errorf("warnings = %q, want one about anchoring", warnings)
	}
}

func TestNewBackendRejectsAnUnknownName(t *testing.T) {
	cfg := Defaults()
	cfg.Backend = "paddle"
	_, err := NewBackend(cfg)
	if err == nil {
		t.Fatal("NewBackend accepted an unknown backend")
	}
	for _, want := range []string{"paddle", BackendChat, BackendMinerU} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// An empty backend name is what every configuration written before the setting
// existed has, and it has to keep meaning the chat backend.
func TestNewBackendDefaultsToChat(t *testing.T) {
	cfg := Defaults()
	cfg.Backend = ""
	b, err := NewBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*chatBackend); !ok {
		t.Errorf("got %T, want *chatBackend", b)
	}
}

func slicesContainsSubstring(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
