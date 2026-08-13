package ingest

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Backend turns one rasterized page into markdown. It is the seam between the
// parts of ingest that do not depend on the model — rasterizing, anchoring,
// Randziffer normalization, assembly, resume — and the protocol a particular
// model speaks.
//
// Two protocols exist. The chat backend sends the whole page image with a
// prompt describing the document and takes markdown back, which is what a
// general-purpose VLM does well. The mineru backend runs a layout pass first
// and recognizes each detected block separately, which is what a document
// parsing model does well. They differ in the number of round trips per page
// and in whether structure is a classification or a stylistic choice, and
// nothing above this interface needs to know which is which.
type Backend interface {
	// Ping checks that the backend is reachable and ready, before a run
	// commits to rasterizing a document. warn reports something the user
	// should see that does not stop the run.
	Ping(ctx context.Context, warn func(string)) error

	// Page transcribes one page. anchorText is the page's born-digital text
	// layer, empty when there is none or when the caller disabled anchoring.
	// onDelta, if non-nil, is called with each chunk of text as it arrives,
	// on this goroutine.
	Page(ctx context.Context, page Page, anchorText string, onDelta func(string)) (PageOutput, error)

	// UsesAnchors reports whether this backend's protocol can take the page's
	// text layer at all.
	//
	// It decides two things, and the second is why it is on the interface
	// rather than inferred from Config. A backend that does not use anchors
	// must not have pdftotext run for it, once per page, for nothing. And it
	// must not have its pages marked "no text layer found — verify carefully":
	// that note tells a reviewer the transcription had no cross-check it
	// normally has, which is a statement about a signal this backend never
	// reads. Printed anyway it is worse than noise, because it invites a
	// careful re-read of pages that are no less trustworthy than the rest.
	UsesAnchors() bool
}

// PageOutput is one page as a backend produced it, before the pipeline's own
// post-processing.
type PageOutput struct {
	// Nodes is the page body as document elements. A backend that knows what
	// it found returns it broken up; one that returns free-running markdown
	// returns a single KindRaw node holding the page, which Render prints back
	// unchanged.
	//
	// It replaced a markdown string here, because a string is the form in
	// which a backend's own knowledge of the page — this region is a heading,
	// this one is a running header — has already been thrown away.
	Nodes []Node
	// Truncated reports that the model was cut off at max_tokens rather than
	// choosing to stop. A backend making several calls per page sets it if any
	// one of them was truncated.
	Truncated bool
	// Tokens is the total completion tokens the page cost.
	Tokens int
}

// Backend names, as they appear in .docc/ingest.yaml and after --backend.
const (
	// BackendChat sends one whole-page image per page and takes markdown
	// back. It is the default because it is what every model here has been
	// scored against.
	BackendChat = "chat"
	// BackendMinerU runs MinerU2.5's two-pass protocol: detect the layout,
	// then recognize each block from a crop at native resolution.
	BackendMinerU = "mineru"
)

var backends = map[string]func(Config) Backend{
	BackendChat:   func(cfg Config) Backend { return &chatBackend{client: NewClient(cfg)} },
	BackendMinerU: func(cfg Config) Backend { return newMinerU(cfg) },
}

// NewBackend builds the backend named by cfg. An empty name means BackendChat,
// so a configuration written before this setting existed keeps working.
func NewBackend(cfg Config) (Backend, error) {
	name := cfg.Backend
	if name == "" {
		name = BackendChat
	}
	build, ok := backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown ingest backend %q — known backends are %s", cfg.Backend, strings.Join(backendNames(), ", "))
	}
	return build(cfg), nil
}

func backendNames() []string {
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// chatBackend is the original protocol: one call per page, the whole page
// image, and a prompt that describes the document conventions in prose.
type chatBackend struct {
	client *Client
}

func (b *chatBackend) Ping(ctx context.Context, warn func(string)) error {
	return b.client.Ping(ctx, warn)
}

func (b *chatBackend) UsesAnchors() bool { return true }

func (b *chatBackend) Page(ctx context.Context, page Page, anchorText string, onDelta func(string)) (PageOutput, error) {
	prompt, err := BuildPrompt(anchorText)
	if err != nil {
		return PageOutput{}, err
	}
	out, err := b.client.CompletePageStream(ctx, page.PNGPath, prompt, onDelta)
	if err != nil {
		return PageOutput{}, err
	}
	// One node for the page: this protocol's answer is markdown the model laid
	// out itself, and it crosses the seam whole. See KindRaw.
	body := strings.TrimSpace(out.Content)
	var nodes []Node
	if body != "" {
		nodes = []Node{{Kind: KindRaw, Text: body}}
	}
	return PageOutput{Nodes: nodes, Truncated: out.Truncated, Tokens: out.Tokens}, nil
}
