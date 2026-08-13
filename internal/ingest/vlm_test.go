package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sseFrames serves each frame as one SSE event, flushing after every one.
// Without the flush httptest buffers the whole response and every assertion
// about incremental delivery below becomes vacuous.
func sseFrames(t *testing.T, onRequest func(*http.Request), frames ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if onRequest != nil {
			onRequest(r)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test server response writer does not flush")
			return
		}
		for _, f := range frames {
			if _, err := w.Write([]byte(f + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}))
}

func dataFrame(content, finishReason string) string {
	return `data: {"choices":[{"delta":{"content":` + quote(content) + `},"finish_reason":` + quote(finishReason) + `}]}`
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestClientCompletePageSendsExpectedRequest(t *testing.T) {
	var gotReq chatRequest
	srv := sseFrames(t, func(r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request body: %v", err)
		}
	},
		dataFrame("# hello", ""),
		dataFrame("\n\nsome transcribed text", "stop"),
		"data: [DONE]",
	)
	defer srv.Close()

	imgPath := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(imgPath, []byte{0x89, 'P', 'N', 'G'}, 0o644); err != nil {
		t.Fatal(err)
	}

	c := &Client{Endpoint: srv.URL, Model: "qwen3-vl", Temperature: 0.1, MaxTokens: 2048, HTTPClient: srv.Client()}
	got, truncated, err := c.CompletePage(context.Background(), imgPath, "transcribe this page")
	if err != nil {
		t.Fatalf("CompletePage: %v", err)
	}
	if truncated {
		t.Error("truncated = true for finish_reason \"stop\"")
	}
	if want := "# hello\n\nsome transcribed text"; got != want {
		t.Errorf("response = %q, want the deltas concatenated in order: %q", got, want)
	}

	if !gotReq.Stream {
		t.Error("request stream = false, want true — progress reporting depends on the streamed response")
	}
	if gotReq.StreamOptions == nil || !gotReq.StreamOptions.IncludeUsage {
		t.Errorf("request stream_options = %+v, want include_usage true", gotReq.StreamOptions)
	}
	if gotReq.Model != "qwen3-vl" {
		t.Errorf("request model = %q, want qwen3-vl", gotReq.Model)
	}
	if gotReq.Temperature != 0.1 {
		t.Errorf("request temperature = %v, want 0.1", gotReq.Temperature)
	}
	if gotReq.MaxTokens != 2048 {
		t.Errorf("request max_tokens = %v, want 2048", gotReq.MaxTokens)
	}
	if len(gotReq.Messages) != 1 || len(gotReq.Messages[0].Content) != 2 {
		t.Fatalf("request content parts = %+v, want one text part and one image part", gotReq.Messages)
	}
	if gotReq.Messages[0].Content[1].ImageURL == nil || !strings.HasPrefix(gotReq.Messages[0].Content[1].ImageURL.URL, "data:image/png;base64,") {
		t.Errorf("image part = %+v, want a base64 data URL", gotReq.Messages[0].Content[1])
	}
}

func TestClientCompletePageStreamReportsDeltas(t *testing.T) {
	srv := sseFrames(t, nil,
		dataFrame("one ", ""),
		": keep-alive",
		"",
		dataFrame("two ", ""),
		"event: message",
		dataFrame("three", "stop"),
		`data: {"choices":[],"usage":{"completion_tokens":412}}`,
		"data: [DONE]",
	)
	defer srv.Close()

	var deltas []string
	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	out, err := c.CompletePageStream(context.Background(), "", "prompt", func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("CompletePageStream: %v", err)
	}
	if got, want := strings.Join(deltas, "|"), "one |two |three"; got != want {
		t.Errorf("deltas = %q, want %q", got, want)
	}
	if out.Content != "one two three" {
		t.Errorf("content = %q, want the deltas concatenated", out.Content)
	}
	if out.Tokens != 412 {
		t.Errorf("Tokens = %d, want 412 from the server's usage frame", out.Tokens)
	}
}

func TestClientCompletePageStreamCountsChunksWithoutUsage(t *testing.T) {
	srv := sseFrames(t, nil, dataFrame("a", ""), dataFrame("b", "stop"), "data: [DONE]")
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	out, err := c.CompletePageStream(context.Background(), "", "prompt", nil)
	if err != nil {
		t.Fatalf("CompletePageStream: %v", err)
	}
	if out.Tokens != 2 {
		t.Errorf("Tokens = %d, want the chunk count 2 when the server reports no usage", out.Tokens)
	}
}

func TestClientCompletePageTruncated(t *testing.T) {
	srv := sseFrames(t, nil, dataFrame("cut off mid-sente", "length"), "data: [DONE]")
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	_, truncated, err := c.CompletePage(context.Background(), "", "prompt")
	if err != nil {
		t.Fatalf("CompletePage: %v", err)
	}
	if !truncated {
		t.Error("expected truncated = true for finish_reason \"length\"")
	}
}

func TestClientCompletePageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("model not loaded"))
	}))
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	_, _, err := c.CompletePage(context.Background(), "", "prompt")
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("error = %v, want it to include the server's message", err)
	}
}

func TestClientCompletePageStreamError(t *testing.T) {
	srv := sseFrames(t, nil, `data: {"error":{"message":"context window exceeded"}}`)
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	_, _, err := c.CompletePage(context.Background(), "", "prompt")
	if err == nil {
		t.Fatal("expected an error for an error frame mid-stream")
	}
	if !strings.Contains(err.Error(), "context window exceeded") {
		t.Errorf("error = %v, want it to include the server's message", err)
	}
}

func TestClientCompletePageNoChoices(t *testing.T) {
	srv := sseFrames(t, nil, `data: {"choices":[]}`, "data: [DONE]")
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	_, _, err := c.CompletePage(context.Background(), "", "prompt")
	if err == nil {
		t.Fatal("expected an error when the server returns no choices")
	}
}

// stalledServer sends the given frames, then holds the connection open
// without sending anything more — a server that died mid-generation.
func stalledServer(t *testing.T, started chan<- struct{}, frames ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			_, _ = w.Write([]byte(f + "\n\n"))
			flusher.Flush()
		}
		if started != nil {
			close(started)
		}
		<-r.Context().Done()
	}))
}

func TestClientStallTimeout(t *testing.T) {
	srv := stalledServer(t, nil, dataFrame("first ", ""))
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client(), StallTimeout: 50 * time.Millisecond}
	start := time.Now()
	_, _, err := c.CompletePage(context.Background(), "", "prompt")
	if err == nil {
		t.Fatal("expected an error when the server stops sending")
	}
	if !strings.Contains(err.Error(), "sent nothing for") {
		t.Errorf("error = %v, want it to name the stall rather than a bare context error", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("stall took %s to surface, want it bounded by StallTimeout", elapsed)
	}
}

func TestClientCancellationIsNotReportedAsAStall(t *testing.T) {
	started := make(chan struct{})
	srv := stalledServer(t, started, dataFrame("first ", ""))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	// A stall timeout long enough that only the cancellation can end this.
	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client(), StallTimeout: time.Minute}
	_, _, err := c.CompletePage(ctx, "", "prompt")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap context.Canceled so a Ctrl-C is recognisable", err)
	}
	if strings.Contains(err.Error(), "sent nothing for") {
		t.Errorf("error = %v, want a cancelled run not to be reported as a stalled server", err)
	}
}
