// Package ingest converts PDF and image documents into the markdown +
// frontmatter shape docc's frontend consumes, using a locally hosted vision
// language model reached over an OpenAI-compatible chat completions API.
//
// It is a draft producer, not a trusted source: nothing here bypasses
// docc check. A conversion writes a banner-marked .md file the author reviews
// and fixes by hand, the same way any other author's draft is reviewed.
package ingest

import (
	"fmt"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

// Config holds the settings for talking to a local VLM. Zero value is not
// usable directly — call Defaults() or Load, which applies it.
type Config struct {
	// Backend selects the protocol used to transcribe a page: "chat" sends the
	// whole page image with a prompt and takes markdown back, "mineru" detects
	// the layout first and recognizes each block from its own crop. Empty
	// means "chat", so a configuration written before this setting existed
	// keeps working under yaml.Strict.
	Backend string `yaml:"backend"`
	// Endpoint is the OpenAI-compatible chat completions URL, e.g. a llama.cpp
	// llama-server instance.
	Endpoint string `yaml:"endpoint"`
	// Model is the model name sent in each request.
	Model string `yaml:"model"`
	// Temperature is sent with every request. Low values trade the model's
	// range for determinism, which is what a small, precision-critical corpus
	// wants.
	Temperature float64 `yaml:"temperature"`
	// DPI controls page rasterization resolution.
	DPI int `yaml:"dpi"`
	// Anchor enables injecting the PDF's own born-digital text layer into the
	// prompt alongside the page image. Off only makes sense for scanned PDFs
	// or plain images, where there is no text layer to extract.
	Anchor bool `yaml:"anchor"`
	// MaxTokens caps each page's response length. A dense page — a long
	// list, a big table — can need more than a typical chat reply; too low
	// a value silently truncates the transcription rather than erroring.
	MaxTokens int `yaml:"max_tokens"`
	// Seed fixes the sampler, so that converting the same page twice produces
	// the same text. Everything else docc emits is reproducible — see
	// pkg/docx — and a transcription that differs run to run cannot be
	// diffed to see what a prompt change actually did.
	Seed int `yaml:"seed"`
	// StallTimeout bounds the silence between two streamed response chunks.
	// It replaces a whole-request deadline, which cannot tell a slow page
	// from a dead server. Raise it for hardware where the first token of a
	// page takes minutes to appear.
	StallTimeout time.Duration `yaml:"stall_timeout"`
}

// Defaults returns the configuration used when no .docc/ingest.yaml exists.
func Defaults() Config {
	return Config{
		Backend:     BackendChat,
		Endpoint:    "http://localhost:8080/v1/chat/completions",
		Temperature: 0.1,
		DPI:         200,
		Anchor:      true,
		MaxTokens:   4096,

		StallTimeout: defaultStallTimeout,
	}
}

// LoadConfig reads path as ingest configuration, layering it over Defaults. A
// missing file is not an error — ingest works from flags and defaults alone.
func LoadConfig(path string) (Config, error) {
	cfg := Defaults()
	b, err := os.ReadFile(path) //nolint:gosec // path comes from project.IngestConfigPath or the caller's own flag
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.UnmarshalWithOptions(b, &cfg, yaml.Strict()); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}
