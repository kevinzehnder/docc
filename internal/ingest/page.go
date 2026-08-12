package ingest

import (
	"encoding/json"
	"strings"
)

// PageResult is one page's transcription, parsed out of the VLM's raw
// response.
type PageResult struct {
	// Index is the 1-based page number.
	Index int
	// Markdown is the page's transcribed body text.
	Markdown string
	// RzSeq is the Randziffer sequence the VLM reported seeing on this page,
	// in order. It is a verification signal, never markdown content — see
	// Verify.
	RzSeq []int
	// HadAnchor reports whether a born-digital text layer was available for
	// this page.
	HadAnchor bool
	// LowConfidence flags a page whose response could not be parsed as
	// expected, so the reviewer knows to look at it more closely.
	LowConfidence bool
	// Note explains why LowConfidence is set, or is empty.
	Note string
}

const (
	markdownMarker = "===MARKDOWN==="
	rzMarker       = "===RZ==="
)

// ParsePageResponse splits the VLM's raw response into the markdown body and
// the reported Randziffer sequence. A response that does not follow the
// requested format is not an error — it is passed through as the page's
// markdown, flagged low-confidence, so a malformed reply from a
// less-cooperative model still produces a draft rather than nothing.
func ParsePageResponse(index int, raw string) PageResult {
	res := PageResult{Index: index}

	mdStart := strings.Index(raw, markdownMarker)
	rzStart := strings.Index(raw, rzMarker)
	if mdStart < 0 || rzStart < 0 || rzStart < mdStart {
		res.Markdown = strings.TrimSpace(raw)
		res.LowConfidence = true
		res.Note = "response did not follow the requested ===MARKDOWN===/===RZ=== format"
		return res
	}

	res.Markdown = strings.TrimSpace(raw[mdStart+len(markdownMarker) : rzStart])

	rzSection := raw[rzStart+len(rzMarker):]
	seq, ok := parseRzJSON(rzSection)
	if !ok {
		res.LowConfidence = true
		res.Note = "could not parse the RZ section as JSON"
		return res
	}
	res.RzSeq = seq
	return res
}

func parseRzJSON(section string) ([]int, bool) {
	body := section
	if start := strings.Index(section, "```"); start >= 0 {
		rest := section[start+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "\n")
		if end := strings.Index(rest, "```"); end >= 0 {
			body = rest[:end]
		} else {
			body = rest
		}
	}

	var parsed struct {
		Randziffern []int `json:"randziffern"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &parsed); err != nil {
		return nil, false
	}
	return parsed.Randziffern, true
}
