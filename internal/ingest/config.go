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
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Config holds the resolved settings for one run — one model, one protocol,
// one set of timeouts. It is what every consumer in this package takes; the
// file that produced it is File, and the choosing happens in File.Resolve.
//
// Zero value is not usable directly — call Defaults() or LoadConfig, which
// applies it.
type Config struct {
	// Backend selects the protocol used to transcribe a page: "chat" sends the
	// whole page image with a prompt and takes markdown back, "mineru" detects
	// the layout first and recognizes each block from its own crop. Empty
	// means "chat".
	//
	// It is not independent of Model — see Profile, which is where the two are
	// chosen together.
	Backend string
	// Endpoint is the OpenAI-compatible chat completions URL, e.g. a llama.cpp
	// llama-server instance.
	Endpoint string
	// Model is the model name sent in each request.
	Model string
	// Temperature is sent with every request. Low values trade the model's
	// range for determinism, which is what a small, precision-critical corpus
	// wants.
	Temperature float64
	// DPI controls page rasterization resolution.
	DPI int
	// Anchor enables injecting the PDF's own born-digital text layer into the
	// prompt alongside the page image. Off only makes sense for scanned PDFs
	// or plain images, where there is no text layer to extract — and for a
	// backend whose protocol has nowhere to put one.
	Anchor bool
	// MaxTokens caps each page's response length. A dense page — a long
	// list, a big table — can need more than a typical chat reply; too low
	// a value silently truncates the transcription rather than erroring.
	MaxTokens int
	// Seed fixes the sampler, so that converting the same page twice produces
	// the same text. Everything else docc emits is reproducible — see
	// pkg/docx — and a transcription that differs run to run cannot be
	// diffed to see what a prompt change actually did.
	Seed int
	// StallTimeout bounds the silence between two streamed response chunks.
	// It replaces a whole-request deadline, which cannot tell a slow page
	// from a dead server. Raise it for hardware where the first token of a
	// page takes minutes to appear.
	StallTimeout time.Duration
}

// Profile is one model together with the protocol it speaks and the settings
// that follow from that choice.
//
// The model and the backend are one decision, not two. A checkpoint trained on
// MinerU's four task names cannot answer a prose transcription prompt, and a
// general-purpose VLM has never seen "\nLayout Detection:" — so of the pairings
// two free-standing settings can express, most produce a page of plausible
// garbage rather than an error. This file was committed for a while naming a
// MinerU model with the backend line commented out, which meant the default,
// which is chat. Nothing caught it, because there is nothing to catch: a model
// name is an arbitrary label on somebody's router and no amount of inspecting
// it reveals which protocol the weights answer.
//
// Declaring them together is the fix. It does not detect the invalid pairings;
// it makes them unwriteable.
//
// Anchor, DPI and MaxTokens live here too, because they are consequences of
// the same choice rather than independent preferences: anchoring is worth 0.116
// of precision to the chat protocol and is inert under mineru, whose prompt is
// a task name with nowhere to put a text layer.
//
// A field left unset inherits — from the file's top level, and then from
// Defaults.
type Profile struct {
	Model     string `yaml:"model"`
	Backend   string `yaml:"backend"`
	Anchor    *bool  `yaml:"anchor"`
	DPI       *int   `yaml:"dpi"`
	MaxTokens *int   `yaml:"max_tokens"`
}

// File is .docc/ingest.yaml as written.
//
// The top-level model settings are inline rather than in a block of their own
// so that a file naming one model and no profiles is still a whole
// configuration — which is the shape this file had before profiles existed,
// and the shape a project with one model never needs to outgrow. Where profiles
// are declared, the same fields become the defaults each one starts from.
type File struct {
	// Use names the profile this project transcribes with. Required once
	// profiles are declared: with several to choose from, leaving the choice
	// implicit is how the wrong one gets used for a week.
	Use string `yaml:"use"`
	// Profiles maps a name to one model and its protocol.
	Profiles map[string]Profile `yaml:"profiles"`

	// Endpoint, Temperature, Seed and StallTimeout are shared across profiles:
	// they describe the server and the sampler, not the model, and a project
	// that needs two of any of them needs two config files.
	Endpoint     string         `yaml:"endpoint"`
	Temperature  *float64       `yaml:"temperature"`
	Seed         *int           `yaml:"seed"`
	StallTimeout *time.Duration `yaml:"stall_timeout"`

	// Profile carries the top-level model settings, which are the defaults
	// every declared profile starts from.
	Profile `yaml:",inline"`
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

// LoadFile reads path as ingest configuration, without choosing a profile. A
// missing file is not an error — ingest works from flags and defaults alone.
func LoadFile(path string) (File, error) {
	var f File
	b, err := os.ReadFile(path) //nolint:gosec // path comes from project.IngestConfigPath or the caller's own flag
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return f, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.UnmarshalWithOptions(b, &f, yaml.Strict()); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Resolve layers the file's settings, and then the named profile's, over
// Defaults. An empty name means the file's own `use:`.
func (f File) Resolve(name string) (Config, error) {
	cfg := Defaults()
	if f.Endpoint != "" {
		cfg.Endpoint = f.Endpoint
	}
	if f.Temperature != nil {
		cfg.Temperature = *f.Temperature
	}
	if f.Seed != nil {
		cfg.Seed = *f.Seed
	}
	if f.StallTimeout != nil {
		cfg.StallTimeout = *f.StallTimeout
	}
	applyProfile(&cfg, f.Profile)

	if name == "" {
		name = f.Use
	}
	if name == "" {
		if len(f.Profiles) > 0 {
			return cfg, fmt.Errorf("no ingest profile selected — set `use:` in the config or pass --profile; declared profiles are %s",
				strings.Join(profileNames(f.Profiles), ", "))
		}
		return cfg, nil
	}

	p, ok := f.Profiles[name]
	if !ok {
		if len(f.Profiles) == 0 {
			return cfg, fmt.Errorf("no ingest profile %q — the config declares no profiles", name)
		}
		return cfg, fmt.Errorf("unknown ingest profile %q — declared profiles are %s",
			name, strings.Join(profileNames(f.Profiles), ", "))
	}
	applyProfile(&cfg, p)
	return cfg, nil
}

// applyProfile overlays the fields a profile sets, leaving the rest inherited.
func applyProfile(cfg *Config, p Profile) {
	if p.Model != "" {
		cfg.Model = p.Model
	}
	if p.Backend != "" {
		cfg.Backend = p.Backend
	}
	if p.Anchor != nil {
		cfg.Anchor = *p.Anchor
	}
	if p.DPI != nil {
		cfg.DPI = *p.DPI
	}
	if p.MaxTokens != nil {
		cfg.MaxTokens = *p.MaxTokens
	}
}

func profileNames(profiles map[string]Profile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LoadConfig reads path and resolves the profile it names, if any. It is
// LoadFile followed by Resolve("") — the whole configuration for a caller with
// no profile of its own to ask for.
func LoadConfig(path string) (Config, error) {
	f, err := LoadFile(path)
	if err != nil {
		return Defaults(), err
	}
	return f.Resolve("")
}
