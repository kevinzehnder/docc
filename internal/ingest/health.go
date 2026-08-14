package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// pingTimeout bounds the whole preflight. It is short on purpose: the point of
// asking is to fail before the run commits to rasterizing a document, so a
// server that cannot answer promptly is treated as one that is not there.
const pingTimeout = 3 * time.Second

// Ping checks that a VLM server is reachable and ready before a run commits to
// rasterizing pages. Without it, a stopped llama-server is discovered only
// after a minute of pdftoppm, as a connection error attributed to page 1.
//
// A model-name mismatch is reported through warn rather than returned. An
// OpenAI-compatible router in front of several models may accept names it does
// not list, and refusing to run against a working server is worse than
// transcribing with a name the router resolves itself.
func (c *Client) Ping(ctx context.Context, warn func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	base, err := serverBase(c.Endpoint)
	if err != nil {
		return err
	}

	code, body, err := c.get(ctx, base.JoinPath("health").String())
	switch {
	case err != nil:
		return fmt.Errorf("no VLM server answering at %s: %w\nstart one with `llama-server -hf <model>`, or point --endpoint at a running server", base, err)

	case code == http.StatusServiceUnavailable:
		// llama-server answers 503 for as long as it takes to load a
		// multi-gigabyte GGUF, which on a cold start is minutes.
		return fmt.Errorf("the VLM server at %s is still loading its model — wait for it to report ready and re-run: %s",
			base, strings.TrimSpace(string(body)))

	case code == http.StatusNotFound:
		// Not llama.cpp. /v1/models below is the portable check.

	case code != http.StatusOK:
		return fmt.Errorf("the VLM server at %s returned %d from /health", base, code)
	}

	if c.Model == "" || warn == nil {
		return nil
	}
	code, body, err = c.get(ctx, base.JoinPath("v1", "models").String())
	if err != nil || code != http.StatusOK {
		return nil // the server answered /health; not listing its models is not a failure
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &list) != nil || len(list.Data) == 0 {
		return nil
	}
	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if m.ID == c.Model {
			return nil
		}
		ids = append(ids, m.ID)
	}
	warn(fmt.Sprintf("the server at %s does not list a model named %q — it offers %s. Set model in .docc/ingest.yaml or pass --model; the run continues, in case the server routes the name anyway",
		base, c.Model, strings.Join(ids, ", ")))
	return nil
}

// serverBase strips the OpenAI path suffix from a chat completions endpoint so
// that sibling endpoints can be derived from it:
//
//	http://h:8080/v1/chat/completions -> http://h:8080
//	http://h/api/v1/chat/completions  -> http://h/api
//	http://h:8080/v1                  -> http://h:8080
//	http://h:8080                     -> http://h:8080
func serverBase(endpoint string) (*url.URL, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid endpoint %q — expected a URL like http://localhost:8080/v1/chat/completions", endpoint)
	}
	p := strings.TrimSuffix(u.Path, "/")
	p = strings.TrimSuffix(p, "/chat/completions")
	p = strings.TrimSuffix(p, "/v1")

	base := *u
	base.Path, base.RawQuery, base.Fragment = p, "", ""
	return &base, nil
}

// get fetches a preflight URL, returning the status code and a bounded body.
// A server's error page is diagnostic, not payload, so it is truncated rather
// than read whole.
func (c *Client) get(ctx context.Context, rawURL string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}
