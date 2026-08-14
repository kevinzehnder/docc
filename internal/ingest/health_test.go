package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerBase(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
		wantErr  bool
	}{
		{endpoint: "http://localhost:8080/v1/chat/completions", want: "http://localhost:8080"},
		{endpoint: "http://box/api/v1/chat/completions", want: "http://box/api"},
		{endpoint: "https://box:8443/v1/chat/completions", want: "https://box:8443"},
		{endpoint: "http://localhost:8080/v1", want: "http://localhost:8080"},
		{endpoint: "http://localhost:8080/", want: "http://localhost:8080"},
		{endpoint: "http://localhost:8080", want: "http://localhost:8080"},
		{endpoint: "http://localhost:8080/v1/chat/completions?key=x", want: "http://localhost:8080"},
		{endpoint: "localhost:8080", wantErr: true},
		{endpoint: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := serverBase(tt.endpoint)
		if tt.wantErr {
			if err == nil {
				t.Errorf("serverBase(%q) = %v, want an error", tt.endpoint, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("serverBase(%q): %v", tt.endpoint, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("serverBase(%q) = %q, want %q", tt.endpoint, got, tt.want)
		}
	}
}

// healthServer serves /health with the given status and /v1/models listing the
// given model ids, mimicking a llama-server.
func healthServer(t *testing.T, healthStatus int, modelIDs ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(healthStatus)
		if healthStatus == http.StatusServiceUnavailable {
			_, _ = w.Write([]byte(`{"error":{"message":"Loading model"}}`))
		}
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		data := make([]string, 0, len(modelIDs))
		for _, id := range modelIDs {
			data = append(data, `{"id":"`+id+`"}`)
		}
		_, _ = w.Write([]byte(`{"data":[` + strings.Join(data, ",") + `]}`))
	})
	return httptest.NewServer(mux)
}

func TestPingReady(t *testing.T) {
	srv := healthServer(t, http.StatusOK, "olmocr-2-7b")
	defer srv.Close()

	c := &Client{Endpoint: srv.URL + "/v1/chat/completions", Model: "olmocr-2-7b", HTTPClient: srv.Client()}
	warned := 0
	if err := c.Ping(context.Background(), func(string) { warned++ }); err != nil {
		t.Fatalf("Ping against a ready server: %v", err)
	}
	if warned != 0 {
		t.Errorf("warn called %d times for a model the server lists", warned)
	}
}

func TestPingLoadingModel(t *testing.T) {
	srv := healthServer(t, http.StatusServiceUnavailable)
	defer srv.Close()

	c := &Client{Endpoint: srv.URL + "/v1/chat/completions", HTTPClient: srv.Client()}
	err := c.Ping(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error while the server is still loading its model")
	}
	if !strings.Contains(err.Error(), "still loading") {
		t.Errorf("error = %v, want it to say the model is still loading", err)
	}
}

func TestPingUnreachable(t *testing.T) {
	srv := healthServer(t, http.StatusOK)
	url := srv.URL
	client := srv.Client()
	srv.Close() // nothing is listening on this address now

	c := &Client{Endpoint: url + "/v1/chat/completions", HTTPClient: client}
	err := c.Ping(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error when no server is listening")
	}
	if !strings.Contains(err.Error(), "no VLM server answering") || !strings.Contains(err.Error(), "llama-server") {
		t.Errorf("error = %v, want it to say nothing is answering and how to start a server", err)
	}
}

// A server that is not llama.cpp has no /health. That is not a failure: the
// only thing preflight needs to establish is that something is listening.
func TestPingWithoutHealthEndpoint(t *testing.T) {
	srv := healthServer(t, http.StatusNotFound, "some-model")
	defer srv.Close()

	c := &Client{Endpoint: srv.URL + "/v1/chat/completions", Model: "some-model", HTTPClient: srv.Client()}
	if err := c.Ping(context.Background(), func(string) {}); err != nil {
		t.Fatalf("Ping against a server with no /health: %v", err)
	}
}

func TestPingWarnsOnModelMismatch(t *testing.T) {
	srv := healthServer(t, http.StatusOK, "qwen3-vl", "gemma-4")
	defer srv.Close()

	c := &Client{Endpoint: srv.URL + "/v1/chat/completions", Model: "olmocr-2-7b", HTTPClient: srv.Client()}
	var msgs []string
	if err := c.Ping(context.Background(), func(m string) { msgs = append(msgs, m) }); err != nil {
		t.Fatalf("a model mismatch must not fail the run — a router may accept names it does not list: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("warn called %d times, want exactly one warning", len(msgs))
	}
	if !strings.Contains(msgs[0], "olmocr-2-7b") || !strings.Contains(msgs[0], "qwen3-vl") {
		t.Errorf("warning = %q, want it to name both the configured model and what the server offers", msgs[0])
	}
}
