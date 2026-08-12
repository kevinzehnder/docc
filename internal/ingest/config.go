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

	"github.com/goccy/go-yaml"
)

// Config holds the settings for talking to a local VLM. Zero value is not
// usable directly — call Defaults() or Load, which applies it.
type Config struct {
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
}

// Defaults returns the configuration used when no .docc/ingest.yaml exists.
func Defaults() Config {
	return Config{
		Endpoint:    "http://localhost:8080/v1/chat/completions",
		Temperature: 0.1,
		DPI:         200,
		Anchor:      true,
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
