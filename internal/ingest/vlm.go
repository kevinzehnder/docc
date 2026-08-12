package ingest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client calls an OpenAI-compatible chat completions endpoint — the API a
// llama.cpp llama-server instance exposes for a locally hosted VLM. It is
// plain net/http, not a vendor SDK: docc keeps its dependency list to what
// the standard library cannot do.
type Client struct {
	Endpoint    string
	Model       string
	Temperature float64
	HTTPClient  *http.Client
}

// NewClient builds a Client from a Config, applying a request timeout
// generous enough for a large local model on modest hardware.
func NewClient(cfg Config) *Client {
	return &Client{
		Endpoint:    cfg.Endpoint,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		HTTPClient:  &http.Client{Timeout: 5 * time.Minute},
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CompletePage sends one page image, and optionally an anchoring text prompt,
// to the VLM and returns its raw text response. imagePath may be empty for a
// text-only retry; prompt is never empty.
func (c *Client) CompletePage(ctx context.Context, imagePath, prompt string) (string, error) {
	parts := []contentPart{{Type: "text", Text: prompt}}
	if imagePath != "" {
		dataURL, err := encodeImage(imagePath)
		if err != nil {
			return "", err
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: dataURL}})
	}

	req := chatRequest{
		Model:       c.Model,
		Temperature: c.Temperature,
		Messages: []chatMessage{
			{Role: "user", Content: parts},
		},
	}
	return c.complete(ctx, req)
}

func (c *Client) complete(ctx context.Context, req chatRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encode VLM request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call VLM at %s: %w", c.Endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read VLM response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("VLM at %s returned %s: %s", c.Endpoint, resp.Status, bytes.TrimSpace(respBody))
	}

	var out chatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode VLM response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("VLM at %s: %s", c.Endpoint, out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("VLM at %s returned no choices", c.Endpoint)
	}
	return out.Choices[0].Message.Content, nil
}

func encodeImage(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is the caller's own rasterized page output
	if err != nil {
		return "", fmt.Errorf("read page image: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b), nil
}
