package ingest

import (
	"fmt"
	"regexp"
	"strconv"
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

// ApplyNodes marks the headings of one page's elements.
//
// This is the pass as it wants to be written. A backend that classified the
// page has already said which elements are headings; the scheme says how deep
// each one is. Neither question involves a "#", and the string form below only
// looks for one because markdown used to be the only thing crossing this seam.
//
// A KindRaw node is a page of free-running markdown from the chat backend,
// which has not been broken into elements. There is nothing to consult but the
// text, so it goes through Apply — the same code, on the only representation
// that path has. The branch disappears when that backend learns to parse.
func (o *outlineNormalizer) ApplyNodes(nodes []Node) []Node {
	if len(o.rules) == 0 {
		return nodes
	}

	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case KindRaw:
			n.Text = o.Apply(n.Text)

		case KindHeading:
			if level, ok := o.level(n.Text); ok {
				n.Level = level
			} else if o.strict {
				// The caller has said the scheme is this document's, so a
				// heading it does not recognize is one the model invented.
				// The text is kept either way — see the type comment.
				n.Kind, n.Level = KindPara, 0
			}

		case KindPara:
			// A title the backend read as body text. The string form promoted
			// these too, by matching a line whether or not it carried a
			// marker, and dropping that would lose every heading the layout
			// pass typed `text`.
			if level, ok := o.level(n.Text); ok {
				n.Kind, n.Level = KindHeading, level
			}
		}
		out = append(out, n)
	}
	return demoteNumberedRuns(out)
}

// FinalizeNodes runs once over the whole document, after every page has been
// through ApplyNodes. Per-page passes cannot see a heading's siblings on the
// pages around it, and one real ambiguity needs exactly that.
//
// I., V. and X. are a Roman numeral and a letter both, and the scheme's
// patterns settle the tie by count: Roman wins, because reaching letter I
// takes nine top-level sections. On a Klageantwort with sections A. through
// K., that put "I. Abbruchkosten" one level above the H. and J. it sits
// between. The siblings are the evidence the pattern cannot see: a single
// I., V. or X. whose neighbouring letter-led headings are its alphabetic
// predecessor or successor is the letter, at their level.
func (o *outlineNormalizer) FinalizeNodes(pages [][]Node) {
	if len(o.rules) == 0 {
		return
	}
	type ref struct{ p, i int }
	var heads []ref
	for p := range pages {
		for i, n := range pages[p] {
			if n.Kind == KindHeading {
				heads = append(heads, ref{p, i})
			}
		}
	}

	letterAt := func(h int) (byte, int, bool) {
		if h < 0 || h >= len(heads) {
			return 0, 0, false
		}
		n := pages[heads[h].p][heads[h].i]
		m := letterLead.FindStringSubmatch(n.Text)
		if m == nil {
			return 0, 0, false
		}
		return m[1][0], n.Level, true
	}

	for h, r := range heads {
		m := ambiguousRoman.FindStringSubmatch(pages[r.p][r.i].Text)
		if m == nil {
			continue
		}
		c := m[1][0]
		if ch, lv, ok := letterAt(h - 1); ok && ch+1 == c {
			pages[r.p][r.i].Level = lv
			continue
		}
		if ch, lv, ok := letterAt(h + 1); ok && ch == c+1 {
			pages[r.p][r.i].Level = lv
		}
	}
}

// ambiguousRoman matches a heading led by the one-character Roman numerals,
// which are letters too. II. and IV. cannot be letters and do not match.
var ambiguousRoman = regexp.MustCompile(`^([IVX])\.\s`)

// letterLead matches a heading led by a single capital letter.
var letterLead = regexp.MustCompile(`^([A-ZÄÖÜ])\.\s`)

// numberedLead reads the "N." a list item or a numbered title opens with.
var numberedLead = regexp.MustCompile(`^(\d{1,4})\.(?:\s|$)`)

// demoteNumberedRuns unmarks headings inside a run of consecutively numbered
// adjacent elements, which is what a list looks like — not a table of
// contents' worth of sections.
//
// The numbered-title patterns cannot see past one line: "1. Die Klage sei
// abzuweisen" is a prayer for relief and "1. Anwaltsvollmacht vom 4. August
// 2025" is an exhibit, and each on its own is exactly the shape of a numbered
// section title. What gives them away is their company. Nine consecutive
// "titles" with not a word of body between them are a Beilagenverzeichnis;
// a "title" whose immediate neighbour is a paragraph carrying the next number
// is the first entry of the list that paragraph continues. A real numbered
// section heading is followed by its section's content, so it never sits
// adjacent to the next number.
//
// A run of two all-heading elements is left alone: two adjacent numbered
// headings are also what a section ending exactly where the next begins looks
// like, and unmarking real structure costs more than leaving a two-entry list
// marked.
func demoteNumberedRuns(nodes []Node) []Node {
	leadOf := func(n Node) (int, bool) {
		if n.Kind != KindHeading && n.Kind != KindPara {
			return 0, false
		}
		m := numberedLead.FindStringSubmatch(n.Text)
		if m == nil {
			return 0, false
		}
		v, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, false
		}
		return v, true
	}

	for i := 0; i < len(nodes); {
		v, ok := leadOf(nodes[i])
		if !ok {
			i++
			continue
		}
		j, paras := i+1, 0
		if nodes[i].Kind == KindPara {
			paras++
		}
		for j < len(nodes) {
			w, ok := leadOf(nodes[j])
			if !ok || w != v+1 {
				break
			}
			if nodes[j].Kind == KindPara {
				paras++
			}
			v = w
			j++
		}
		if run := j - i; run >= 2 && (paras > 0 || run >= 3) {
			for k := i; k < j; k++ {
				if nodes[k].Kind == KindHeading {
					nodes[k].Kind, nodes[k].Level = KindPara, 0
				}
			}
		}
		i = j
	}
	return nodes
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
