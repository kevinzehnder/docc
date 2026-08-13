package ingest

import (
	"fmt"
	"regexp"
	"strings"
)

// headingLine matches a markdown ATX heading, capturing its level and its text
// so that a heading the model marked can be re-levelled or unmarked.
var headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

// OutlineRule is one level of a document type's section titles: a pattern that
// recognizes them, and the markdown level they become.
type OutlineRule struct {
	Pattern *regexp.Regexp
	Level   int
}

// OutlinePattern is one declared rule, as it appears in a schema. It is the
// caller's job to read it out of the schema: ingest takes plain data, the way
// it takes the Randziffer policy, so that transcribing does not depend on the
// schema package.
type OutlinePattern struct {
	Pattern string
	Level   int
}

// CompileOutline turns a document type's declared outline into matchers.
func CompileOutline(in []OutlinePattern) ([]OutlineRule, error) {
	rules := make([]OutlineRule, 0, len(in))
	for _, p := range in {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, fmt.Errorf("outline pattern %q: %w", p.Pattern, err)
		}
		rules = append(rules, OutlineRule{Pattern: re, Level: p.Level})
	}
	return rules, nil
}

// outlineNormalizer marks the headings a document type's usual section-title
// scheme recognizes.
//
// It exists for the same reason rzNormalizer does. A transcribing model decides
// heading markup by eye, and decides it differently on each page: over one
// brief the chat backend marked four of seven titles and put a fifth at the
// wrong depth, while the layout-first backend marked thirteen on a document
// with eight. A scheme the document type can name turns that into a lookup.
//
// By default it only promotes. The other half — unmarking a heading that
// matches no rule — is wrong as a default, and the reason is whose document
// this is. A transcription is of somebody else's brief, written to their
// conventions, and a firm that outlines its filings some way nobody anticipated
// is unusual rather than mistaken. Demoting on that basis would silently strip
// the real structure out of exactly the documents whose structure could not be
// predicted.
//
// strict turns that judgement around, because the person setting it has made a
// different one. A scheme the schema offers is a guess about a document nobody
// has read; a scheme a caller confirms with --outline-strict is a statement
// about a document they have. Under it a heading matching no rule is not
// evidence the scheme is wrong, it is a heading the model invented, and
// unmarking it is the correct answer. The text is kept either way.
type outlineNormalizer struct {
	rules []OutlineRule
	// strict unmarks headings that match no rule. Off unless the caller asks.
	strict bool
}

// Apply rewrites one page's markdown. With no rules it returns the input
// unchanged, which is what a document type that has not declared an outline —
// and every run without --type — gets.
func (o *outlineNormalizer) Apply(md string) string {
	if len(o.rules) == 0 {
		return md
	}

	lines := strings.Split(md, "\n")
	for i, line := range lines {
		// Fenced-code and table content is not scanned: this walks lines, and
		// a heading inside a fence is not a heading. Blocks arrive from the
		// backends one per element, so a "# " inside one is rare enough that
		// getting it wrong costs a marker, not text.
		// Any marker the model already wrote is stripped before matching, so
		// that a title it marked at the wrong depth is re-levelled rather than
		// missed.
		text, wasMarked := line, false
		if m := headingLine.FindStringSubmatch(line); m != nil {
			text, wasMarked = m[2], true
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}

		if level, ok := o.level(trimmed); ok {
			lines[i] = strings.Repeat("#", level) + " " + trimmed
			continue
		}
		// No match. Left exactly as it arrived unless the caller has said the
		// scheme is the document's — see the type comment for why that is a
		// decision only they can make.
		if o.strict && wasMarked {
			lines[i] = trimmed
		}
	}
	return strings.Join(lines, "\n")
}

// level reports the outline level a line belongs at. First match wins, so the
// order rules are declared in is the order they are tried — a pattern for a
// deeper level that would also match a shallower one goes second.
func (o *outlineNormalizer) level(text string) (int, bool) {
	for _, r := range o.rules {
		if r.Pattern.MatchString(text) {
			return r.Level, true
		}
	}
	return 0, false
}
