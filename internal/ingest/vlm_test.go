package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientCompletePageSendsExpectedRequest(t *testing.T) {
	var gotReq chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "# hello\n\nsome transcribed text"}}},
		})
	}))
	defer srv.Close()

	imgPath := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(imgPath, []byte{0x89, 'P', 'N', 'G'}, 0o644); err != nil {
		t.Fatal(err)
	}

	c := &Client{Endpoint: srv.URL, Model: "qwen3-vl", Temperature: 0.1, HTTPClient: srv.Client()}
	got, err := c.CompletePage(context.Background(), imgPath, "transcribe this page")
	if err != nil {
		t.Fatalf("CompletePage: %v", err)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("response = %q, want it to contain the model's markdown", got)
	}

	if gotReq.Model != "qwen3-vl" {
		t.Errorf("request model = %q, want qwen3-vl", gotReq.Model)
	}
	if gotReq.Temperature != 0.1 {
		t.Errorf("request temperature = %v, want 0.1", gotReq.Temperature)
	}
	if len(gotReq.Messages) != 1 || len(gotReq.Messages[0].Content) != 2 {
		t.Fatalf("request content parts = %+v, want one text part and one image part", gotReq.Messages)
	}
	if gotReq.Messages[0].Content[1].ImageURL == nil || !strings.HasPrefix(gotReq.Messages[0].Content[1].ImageURL.URL, "data:image/png;base64,") {
		t.Errorf("image part = %+v, want a base64 data URL", gotReq.Messages[0].Content[1])
	}
}

func TestClientCompletePageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("model not loaded"))
	}))
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	_, err := c.CompletePage(context.Background(), "", "prompt")
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("error = %v, want it to include the server's message", err)
	}
}

func TestClientCompletePageNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatResponse{})
	}))
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	_, err := c.CompletePage(context.Background(), "", "prompt")
	if err == nil {
		t.Fatal("expected an error when the server returns no choices")
	}
}
