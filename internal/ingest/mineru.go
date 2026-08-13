package ingest

import (
	"context"
	"fmt"
)

// MinerU's own prompts, from mineru_vl_utils' DEFAULT_PROMPTS. They are the
// whole task specification — one image, one plain-text suffix, no system
// prompt and no examples — and the leading newline is part of them.
const (
	layoutPrompt   = "\nLayout Detection:"
	textPrompt     = "\nText Recognition:"
	tablePrompt    = "\nTable Recognition:"
	formulaPrompt  = "\nFormula Recognition:"
	layoutMaxToken = 4096
)

// recognition maps a block type onto the prompt and sampling parameters MinerU
// uses for it. The penalties are not decoration: a crop of a table or of a
// column of figures is exactly the input on which an unpenalized decoder falls
// into repeating a row until it hits the token limit.
var recognition = map[string]struct {
	prompt string
	opts   CallOptions
}{
	"table":    {tablePrompt, CallOptions{PresencePenalty: 1.0, FrequencyPenalty: 0.005}},
	"equation": {formulaPrompt, CallOptions{PresencePenalty: 1.0, FrequencyPenalty: 0.05}},
}

var defaultRecognition = struct {
	prompt string
	opts   CallOptions
}{textPrompt, CallOptions{PresencePenalty: 1.0, FrequencyPenalty: 0.05}}

// minerU implements Backend with MinerU2.5's two-pass protocol: one call to
// find the blocks, then one call per block to read it.
//
// The trade is round trips for structure. It costs a dozen or more calls per
// page where the chat backend costs one, and in exchange a heading is a
// classification rather than a stylistic choice, a running header is a type
// rather than a line the prompt asked the model to please leave out, and every
// block is read from a crop at the page's own resolution instead of from a
// thumbnail of the whole page.
type minerU struct {
	client *Client
	anchor bool
}

func newMinerU(cfg Config) Backend {
	return &minerU{client: NewClient(cfg), anchor: cfg.Anchor}
}

// UsesAnchors is false: the protocol's prompt is the task name, and there is
// nowhere in it to put a text layer.
func (m *minerU) UsesAnchors() bool { return false }

func (m *minerU) Ping(ctx context.Context, warn func(string)) error {
	if err := m.client.Ping(ctx, warn); err != nil {
		return err
	}
	if m.anchor && warn != nil {
		// Said once, from Ping, rather than per page. The protocol's prompt is
		// the task name and nothing else — there is nowhere to put a text
		// layer without changing the string the model was trained to answer.
		warn("the mineru backend ignores anchor: the layout and recognition prompts take no additional text. " +
			"The text layer is still what `docc ingest --backend chat` uses.")
	}
	return nil
}

func (m *minerU) Page(ctx context.Context, page Page, _ string, onDelta func(string)) (PageOutput, error) {
	img, err := loadImage(page.PNGPath)
	if err != nil {
		return PageOutput{}, err
	}

	layoutURL, err := pngDataURL(layoutImage(img))
	if err != nil {
		return PageOutput{}, err
	}

	var out PageOutput
	raw, err := m.client.CompleteImage(ctx, layoutURL, layoutPrompt, CallOptions{MaxTokens: layoutMaxToken}, nil)
	if err != nil {
		return PageOutput{}, fmt.Errorf("layout pass: %w", err)
	}
	out.Tokens += raw.Tokens
	if raw.Truncated {
		// Not recoverable by reading on: the truncation lands mid-coordinate,
		// and a page whose block list is cut short is a page missing its
		// bottom half with nothing to show that it is.
		return PageOutput{}, fmt.Errorf("layout pass was cut off at max_tokens — the page has more blocks than the limit allows")
	}

	blocks, err := ParseLayout(raw.Content)
	if err != nil {
		return PageOutput{}, err
	}

	for i := range blocks {
		b := &blocks[i]
		if visual[b.Type] || furniture[b.Type] {
			// Neither has text worth a round trip: one is a picture, the other
			// is about to be dropped.
			continue
		}
		crop := cropBlock(img, b.Box)
		if crop == nil {
			continue
		}
		cropURL, err := pngDataURL(crop)
		if err != nil {
			return PageOutput{}, err
		}

		task, ok := recognition[b.Type]
		if !ok {
			task = defaultRecognition
		}
		res, err := m.client.CompleteImage(ctx, cropURL, task.prompt, task.opts, onDelta)
		if err != nil {
			return PageOutput{}, fmt.Errorf("recognizing %s block %d of %d: %w", b.Type, i+1, len(blocks), err)
		}
		b.Text = res.Content
		out.Tokens += res.Tokens
		out.Truncated = out.Truncated || res.Truncated
	}

	out.Markdown = AssembleBlocks(blocks)
	return out, nil
}
