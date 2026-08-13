package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	// MaxTokens caps the response length. A dense page — a long list, a big
	// table — needs more than a typical chat reply, and an unset limit
	// leaves the page silently truncated at whatever the server defaults to.
	MaxTokens int
	// Seed fixes the sampler so the same page transcribes the same way twice.
	Seed int
	// StallTimeout bounds the silence between two streamed chunks. Zero means
	// defaultStallTimeout.
	StallTimeout time.Duration
	HTTPClient   *http.Client
}

// defaultStallTimeout has to be generous: evaluating a 200 dpi page image
// against a projector running partly on the CPU produces no output at all for
// a minute or more before the first token appears. It bounds mid-generation
// death, not startup — a server that is not running at all is caught by Ping
// in three seconds.
const defaultStallTimeout = 3 * time.Minute

// NewClient builds a Client from a Config.
func NewClient(cfg Config) *Client {
	return &Client{
		Endpoint:     cfg.Endpoint,
		Model:        cfg.Model,
		Temperature:  cfg.Temperature,
		MaxTokens:    cfg.MaxTokens,
		Seed:         cfg.Seed,
		StallTimeout: cfg.StallTimeout,
		// No whole-request deadline: a page that legitimately takes six
		// minutes is indistinguishable from a hung server under one, and the
		// stall watchdog in stream draws that distinction properly.
		HTTPClient: &http.Client{},
	}
}

func (c *Client) stallTimeout() time.Duration {
	if c.StallTimeout > 0 {
		return c.StallTimeout
	}
	return defaultStallTimeout
}

type chatRequest struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	// Seed carries no omitempty: 0 is a valid seed, and leaving the field out
	// is what makes a server pick a random one.
	Seed int `json:"seed"`
	// PresencePenalty and FrequencyPenalty are omitted when zero, which is the
	// OpenAI default and what every chat-backend request wants. MinerU's
	// recognition pass sets them: a crop of a table or a column of figures is
	// exactly the input on which an unpenalized decoder falls into repeating a
	// row forever.
	PresencePenalty  float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty float64        `json:"frequency_penalty,omitempty"`
	Messages         []chatMessage  `json:"messages"`
	Stream           bool           `json:"stream,omitempty"`
	StreamOptions    *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
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

// chatStreamChunk is one SSE data frame. The final frame carries usage and an
// empty Choices slice, so nothing here may assume Choices has an element.
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Completion is one page's response.
type Completion struct {
	Content string
	// Truncated reports whether the server cut the response off at max_tokens
	// (finish_reason "length") rather than the model choosing to stop — a
	// dense page's transcription ending mid-sentence would otherwise look
	// exactly like a normal, complete response.
	Truncated bool
	// Tokens is the server's reported completion_tokens, or the number of
	// non-empty chunks received if the server reports no usage.
	Tokens int
}

// CompletePageStream sends one page image, and optionally an anchoring text
// prompt, to the VLM and streams the response back. onDelta, if non-nil, is
// called with each chunk of text as it arrives, on this goroutine: a callback
// that blocks throttles generation.
//
// imagePath may be empty for a text-only call; prompt is never empty.
func (c *Client) CompletePageStream(ctx context.Context, imagePath, prompt string, onDelta func(string)) (Completion, error) {
	var dataURL string
	if imagePath != "" {
		var err error
		if dataURL, err = encodeImage(imagePath); err != nil {
			return Completion{}, err
		}
	}
	return c.CompleteImage(ctx, dataURL, prompt, CallOptions{}, onDelta)
}

// CallOptions carries the per-call sampling settings that differ between
// tasks. The zero value is what every chat-backend call wants; MinerU's
// recognition pass varies them by block type.
type CallOptions struct {
	PresencePenalty  float64
	FrequencyPenalty float64
	// MaxTokens overrides the client's own cap when non-zero. A single
	// cropped block does not need a whole page's budget, and a smaller cap
	// bounds how long a decoder that starts repeating can run.
	MaxTokens int
}

// CompleteImage is CompletePageStream for an image already in memory: dataURL
// is a "data:" URL, or empty for a text-only call. It exists because a backend
// that crops and rescales pages itself has no file to name — writing every crop
// to disk to read it straight back would be the only reason a temp directory
// had to reach the backend at all.
func (c *Client) CompleteImage(ctx context.Context, dataURL, prompt string, opts CallOptions, onDelta func(string)) (Completion, error) {
	parts := []contentPart{{Type: "text", Text: prompt}}
	if dataURL != "" {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: dataURL}})
	}

	maxTokens := c.MaxTokens
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}

	return c.stream(ctx, chatRequest{
		Model:            c.Model,
		Temperature:      c.Temperature,
		MaxTokens:        maxTokens,
		Seed:             c.Seed,
		PresencePenalty:  opts.PresencePenalty,
		FrequencyPenalty: opts.FrequencyPenalty,
		Messages: []chatMessage{
			{Role: "user", Content: parts},
		},
	}, onDelta)
}

// CompletePage is CompletePageStream without progress reporting.
func (c *Client) CompletePage(ctx context.Context, imagePath, prompt string) (content string, truncated bool, err error) {
	out, err := c.CompletePageStream(ctx, imagePath, prompt, nil)
	return out.Content, out.Truncated, err
}

// errStalled is the cancellation cause the watchdog uses. It exists so that a
// stalled server and a user pressing Ctrl-C, which both surface as
// context.Canceled from the HTTP layer, can be told apart.
var errStalled = errors.New("stalled")

func (c *Client) stream(ctx context.Context, req chatRequest, onDelta func(string)) (Completion, error) {
	req.Stream = true
	req.StreamOptions = &streamOptions{IncludeUsage: true}

	body, err := json.Marshal(req)
	if err != nil {
		return Completion{}, fmt.Errorf("encode VLM request: %w", err)
	}

	stall := c.stallTimeout()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// Armed before the request goes out, so the watchdog also covers connect
	// and the wait for response headers — the part of a dead-server hang the
	// user sees first.
	watchdog := time.AfterFunc(stall, func() { cancel(errStalled) })
	defer watchdog.Stop()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Completion{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return Completion{}, c.streamErr(ctx, fmt.Errorf("call VLM at %s: %w", c.Endpoint, err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return Completion{}, fmt.Errorf("VLM at %s returned %s: %s", c.Endpoint, resp.Status, bytes.TrimSpace(msg))
	}

	var (
		out    Completion
		text   strings.Builder
		chunks int
		sawAny bool
	)
	sc := bufio.NewScanner(resp.Body)
	// A token-sized frame fits the 64 KiB default many times over; a server
	// returning a large error object on one line does not.
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)

	for sc.Scan() {
		// Per line, not per token: a server sending keep-alive comments is
		// alive, and that is what the watchdog is asking about.
		watchdog.Reset(stall)

		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, ":") {
			continue // frame separator, or a keep-alive comment
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // event:, id:, retry: — unused by this API
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return out, fmt.Errorf("decode VLM stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return out, fmt.Errorf("VLM at %s: %s", c.Endpoint, chunk.Error.Message)
		}
		if chunk.Usage != nil {
			out.Tokens = chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			sawAny = true
			if choice.Delta.Content != "" {
				chunks++
				text.WriteString(choice.Delta.Content)
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
			if choice.FinishReason == "length" {
				out.Truncated = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return out, c.streamErr(ctx, fmt.Errorf("read VLM stream: %w", err))
	}
	if !sawAny {
		return out, fmt.Errorf("VLM at %s returned no choices", c.Endpoint)
	}

	out.Content = text.String()
	if out.Tokens == 0 {
		out.Tokens = chunks
	}
	return out, nil
}

// streamErr replaces a bare transport error with the reason the context died,
// where there is one. Without this a stalled server, a cancelled run and a
// genuine network failure all report the same unhelpful "context canceled".
func (c *Client) streamErr(ctx context.Context, err error) error {
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, errStalled):
		return fmt.Errorf("VLM at %s sent nothing for %s — the server may have died mid-generation, or the page may be too large for it",
			c.Endpoint, c.stallTimeout())
	case cause != nil && errors.Is(cause, context.Canceled):
		return cause
	default:
		return err
	}
}

func encodeImage(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is the caller's own rasterized page output
	if err != nil {
		return "", fmt.Errorf("read page image: %w", err)
	}
	// Rasterized pages are always PNG; a direct image input is whatever the
	// user handed us, and CheckInput has already restricted that to a type
	// named here.
	mime, ok := imageMIME[strings.ToLower(filepath.Ext(path))]
	if !ok {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}
