package ingest

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// evidenceLead matches the line that opens an offer of proof. Swiss briefs
// abbreviate it "BO" (Beweisofferte); transcriptions also produce the spelled
// out forms, and the model sometimes emboldens the label.
var evidenceLead = regexp.MustCompile(`^\s*(?:\*\*|__)?\s*(?:BO|Beweis|Beweismittel|Beweisofferte)\s*:?\s*(?:\*\*|__)?\s*:?`)

// evidenceEnd matches the first line that cannot belong to an offer of proof:
// the next numbered paragraph, a heading, or a fence.
var evidenceEnd = regexp.MustCompile(`^\s*(?:\[Rz \d+\]|#{1,6}\s|:::|>)`)

// evidenceItem is the shape the legal schema requires of an item once the
// block is structured: a bracketed label, then a description.
var evidenceItem = regexp.MustCompile(`^-\s+\[[^\]\r\n]+\]\s+\S`)

// Region is a span of lines holding one offer of proof, as line indices into
// the document, end exclusive.
type Region struct {
	Start, End int
}

// EvidenceRegions finds the offers of proof in an ingested draft.
//
// Where a block starts is unambiguous — the source document labels it. Where it
// ends is not: an offer runs on across blank lines, listing a document on one
// line and two witnesses on the next two, and nothing marks the last of them.
// What does hold is that the paragraph after it resumes the brief, so the block
// runs until the next numbered paragraph, heading or fence.
//
// A block already inside a `::: beweis` fence is skipped: structuring is
// re-runnable, and doing it twice must not nest.
func EvidenceRegions(lines []string) []Region {
	var out []Region
	inFence := false
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), ":::") {
			inFence = !inFence
			continue
		}
		if inFence || !evidenceLead.MatchString(lines[i]) {
			continue
		}
		j := i + 1
		for j < len(lines) && !evidenceEnd.MatchString(lines[j]) {
			j++
		}
		// Trailing blank lines belong to the document, not the block.
		for j > i+1 && strings.TrimSpace(lines[j-1]) == "" {
			j--
		}
		out = append(out, Region{Start: i, End: j})
		i = j - 1
	}
	return out
}

// StructureNote reports one block the pass could not convert. A block is left
// exactly as it was rather than replaced with something unverified.
type StructureNote struct {
	Line   int // 1-based
	Reason string
}

// Structure rewrites a draft's offers of proof into `::: beweis` fenced divs,
// leaving every other byte of the document alone.
//
// It is a second pass over text rather than part of transcription because the
// two jobs have different failure modes. Transcription is one shot against an
// image and cannot be checked; this runs on text, costs no image encode, can be
// repeated after a prompt change, and its output is validated — the model's
// answer is accepted only if every line it returns is a labelled item of the
// shape the schema requires. Where it is not, the block stays as transcribed
// and the caller is told which one.
func Structure(ctx context.Context, c *Client, md string) (string, []StructureNote, error) {
	lines := strings.Split(md, "\n")
	regions := EvidenceRegions(lines)
	if len(regions) == 0 {
		return md, nil, nil
	}

	var (
		out   []string
		notes []StructureNote
		prev  int
	)
	for _, r := range regions {
		out = append(out, lines[prev:r.Start]...)
		prev = r.End

		// The lead label ("BO:") names the block, not the evidence, so it is
		// removed here rather than left for the model to notice — it leaked
		// into an item's description when it was not.
		// A page marker inside the block is not evidence. An offer of proof
		// runs on across a page break — a document on one line and two
		// witnesses on the next page — and leaving the comment in makes the
		// model answer with a line that is not a labelled item, which fails
		// validation and throws the whole block away.
		body := make([]string, 0, r.End-r.Start)
		for _, line := range lines[r.Start:r.End] {
			if !IsPageMarker(line) {
				body = append(body, line)
			}
		}
		body[0] = strings.TrimSpace(evidenceLead.ReplaceAllString(body[0], ""))
		block := strings.TrimSpace(strings.Join(body, "\n"))
		items, err := structureBlock(ctx, c, block)
		if err != nil {
			return "", nil, fmt.Errorf("line %d: %w", r.Start+1, err)
		}
		if len(items) == 0 {
			notes = append(notes, StructureNote{
				Line:   r.Start + 1,
				Reason: "no labelled items came back — left as transcribed",
			})
			out = append(out, lines[r.Start:r.End]...)
			continue
		}
		out = append(out, "::: beweis", "")
		out = append(out, items...)
		out = append(out, ":::")
	}
	out = append(out, lines[prev:]...)
	return strings.Join(out, "\n"), notes, nil
}

// structureBlock asks the model to split one block into labelled items, and
// keeps only the lines that came back in the required shape. A partial answer
// is treated as no answer: half an offer of proof is worse than the
// transcription, which at least still says what the page said.
func structureBlock(ctx context.Context, c *Client, block string) ([]string, error) {
	prompt, err := BuildStructurePrompt(block)
	if err != nil {
		return nil, err
	}
	out, err := c.CompletePageStream(ctx, "", prompt, nil)
	if err != nil {
		return nil, err
	}

	var items []string
	for _, line := range strings.Split(out.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		if !evidenceItem.MatchString(line) {
			return nil, nil
		}
		items = append(items, line)
	}
	return items, nil
}
