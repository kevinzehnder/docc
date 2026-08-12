package ingest

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed prompt.tmpl
var promptSource string

//go:embed prompt_plain.tmpl
var promptPlainSource string

var (
	promptTemplate      = template.Must(template.New("ingest-prompt").Parse(promptSource))
	promptPlainTemplate = template.Must(template.New("ingest-prompt-plain").Parse(promptPlainSource))
)

// BuildPrompt renders the per-page prompt sent to the VLM. anchorText is the
// page's born-digital text layer, already reconstructed into reading-order
// lines by PromptText; an empty string means no text layer was available
// (a scan, or a plain image input) and the model works from the image alone.
//
// It asks the model to omit Randziffern (marginal paragraph numbers) from
// the markdown body and report them separately, since docc computes and
// renders them itself — see Verify. BuildPlainPrompt is the alternative for
// callers that just want a literal transcription with no docc-specific
// behavior.
func BuildPrompt(anchorText string) (string, error) {
	return renderPrompt(promptTemplate, anchorText)
}

// BuildPlainPrompt renders a prompt for literal transcription: everything on
// the page, including any marginal numbers, exactly as it appears. Use it
// with ParsePlainResponse — the response has no RZ section to parse, so
// Verify has nothing to check against and should not be run against its
// output.
func BuildPlainPrompt(anchorText string) (string, error) {
	return renderPrompt(promptPlainTemplate, anchorText)
}

func renderPrompt(tmpl *template.Template, anchorText string) (string, error) {
	var b strings.Builder
	data := struct{ AnchorText string }{AnchorText: strings.TrimSpace(anchorText)}
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("build ingest prompt: %w", err)
	}
	return b.String(), nil
}
