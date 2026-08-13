package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeVLM serves preflight and streams one short response per page, counting
// completion requests so a test can assert that none was made.
func fakeVLM(t *testing.T, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"test-vlm"}]}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"# transcribed"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fixturePDF copies the package's own test PDF somewhere with no .docc above
// it, so the run uses flags and defaults rather than the repository's config.
func fixturePDF(t *testing.T, dir string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "assets", "test_document.pdf"))
	if err != nil {
		t.Skip("assets/test_document.pdf not available")
	}
	path := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ingestArgs(endpoint, input string, extra ...string) []string {
	args := []string{"ingest", "--model", "test-vlm", "--endpoint", endpoint + "/v1/chat/completions"}
	args = append(args, extra...)
	return append(args, input)
}

// The guard has to run before the conversion does. Refusing after the fact
// would still have spent the VLM time the refusal exists to protect.
func TestIngestRefusesEditedOutputBeforeCallingTheVLM(t *testing.T) {
	dir := t.TempDir()
	input := fixturePDF(t, dir)
	var calls atomic.Int64
	srv := fakeVLM(t, &calls)

	// A draft somebody has adapted by hand: no generated-by banner.
	edited := filepath.Join(dir, "scan.md")
	writeTestFile(t, edited, "---\ndocc: 1\ndocument_type: legal\n---\n\n# Klage\n\nHand-written.\n")

	if got := run(ingestArgs(srv.URL, input)); got != 2 {
		t.Errorf("run(ingest) = %d, want 2 for a refused destination", got)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("%d VLM requests were made; want 0 — the refusal must come before the work", n)
	}

	body, err := os.ReadFile(edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Hand-written") {
		t.Error("the edited draft was overwritten despite the refusal")
	}
}

func TestIngestForceOverwritesEditedOutput(t *testing.T) {
	dir := t.TempDir()
	input := fixturePDF(t, dir)
	var calls atomic.Int64
	srv := fakeVLM(t, &calls)

	edited := filepath.Join(dir, "scan.md")
	writeTestFile(t, edited, "# Hand-written\n")

	if got := run(ingestArgs(srv.URL, input, "--force")); got != 0 {
		t.Fatalf("run(ingest --force) = %d, want 0", got)
	}
	body, err := os.ReadFile(edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# transcribed") {
		t.Errorf("--force should have replaced the file, got:\n%s", body)
	}
}

func TestIngestWritesDraftAndAcceptsRerun(t *testing.T) {
	dir := t.TempDir()
	input := fixturePDF(t, dir)
	var calls atomic.Int64
	srv := fakeVLM(t, &calls)

	if got := run(ingestArgs(srv.URL, input)); got != 0 {
		t.Fatalf("run(ingest) = %d, want 0", got)
	}
	out := filepath.Join(dir, "scan.md")
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# transcribed") {
		t.Errorf("draft missing the transcription:\n%s", body)
	}

	// Re-running over ingest's own finished output is the ordinary iteration
	// loop and must not need --force.
	if got := run(ingestArgs(srv.URL, input)); got != 0 {
		t.Errorf("re-running over a generated draft = %d, want 0", got)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("VLM requests = %d, want 2 (one per run)", n)
	}
}

func TestIngestRejectsBadInputBeforeCallingTheVLM(t *testing.T) {
	dir := t.TempDir()
	input := fixturePDF(t, dir)
	var calls atomic.Int64
	srv := fakeVLM(t, &calls)

	notes := filepath.Join(dir, "notes.md")
	writeTestFile(t, notes, "not a document ingest can read\n")

	// The shape of `docc ingest scan.pdf out.md`: the second argument is read
	// as another input, and the whole command must fail before the first is
	// converted.
	args := append(ingestArgs(srv.URL, input), notes)
	if got := run(args); got != 2 {
		t.Errorf("run(ingest pdf md) = %d, want 2", got)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("%d VLM requests were made; want 0 — a usage error must not cost GPU time", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "scan.md")); !os.IsNotExist(err) {
		t.Error("the valid input was converted anyway; nothing should run when an argument is bad")
	}
}
