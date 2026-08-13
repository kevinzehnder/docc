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

// outlineNormalizer rewrites a page's headings to the outline its document type
// declares.
//
// It exists for the same reason rzNormalizer does. A transcribing model decides
// heading markup by eye, and decides it differently on each page: over one
// four-page brief the chat backend marked four of eight headings and the
// layout-first backend marked thirteen. Neither is a judgement call — a Swiss
// brief's outline is BEGRÜNDUNG:, then I., then A., and a document type that
// can say so should not have to ask.
//
// Both directions matter, which is why this is not simply a promoter. Promoting
// alone leaves the layout backend's invented headings in place; demoting alone
// leaves the chat backend's missed ones as prose. Together they turn 4 of 8 and
// 13 of 8 into the same 8.
type outlineNormalizer struct {
	rules []OutlineRule
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
		text, marked := line, false
		if m := headingLine.FindStringSubmatch(line); m != nil {
			text, marked = m[2], true
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}

		if level, ok := o.level(trimmed); ok {
			lines[i] = strings.Repeat("#", level) + " " + trimmed
			continue
		}
		if marked {
			// A heading matching no declared pattern is not one this document
			// type has. The text survives as prose — a title the outline does
			// not cover loses its marker and stays readable, which is a
			// mistake a reviewer can see and fix.
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
