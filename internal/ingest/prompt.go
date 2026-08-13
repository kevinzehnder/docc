package ingest

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed prompt.tmpl
var promptSource string

var promptTemplate = template.Must(template.New("ingest-prompt").Parse(promptSource))

// BuildPrompt renders the per-page prompt sent to the VLM: transcribe this
// page as faithfully as possible into plain markdown. anchorText is the
// page's born-digital text layer, already reconstructed into reading-order
// lines by PromptText; an empty string means no text layer was available
// (a scan, or a plain image input) and the model works from the image alone.
func BuildPrompt(anchorText string) (string, error) {
	var b strings.Builder
	data := struct{ AnchorText string }{AnchorText: strings.TrimSpace(anchorText)}
	if err := promptTemplate.Execute(&b, data); err != nil {
		return "", fmt.Errorf("build ingest prompt: %w", err)
	}
	return b.String(), nil
}

//go:embed structure.tmpl
var structureSource string

var structureTemplate = template.Must(template.New("ingest-structure").Parse(structureSource))

// BuildStructurePrompt renders the prompt for the structuring pass: split one
// transcribed offer of proof into labelled items. Unlike BuildPrompt this one
// carries no image — the input is text ingest already produced, which is what
// makes the pass cheap enough to re-run.
func BuildStructurePrompt(block string) (string, error) {
	var b strings.Builder
	data := struct{ Block string }{Block: strings.TrimSpace(block)}
	if err := structureTemplate.Execute(&b, data); err != nil {
		return "", fmt.Errorf("build structure prompt: %w", err)
	}
	return b.String(), nil
}
