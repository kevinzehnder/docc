package ingest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// randziffer matches a paragraph that opens with a bare number, which is how
// a marginal paragraph number is transcribed when the model does not mark it
// itself. The number must be followed by a word starting the sentence, so a
// markdown list item ("1. Erstens") and a heading are not candidates.
var randziffer = regexp.MustCompile(`^(\d{1,4})\s+([^\s\d].*)$`)

// marked matches a paragraph number the model did mark, so the two paths keep
// one shared sequence.
var marked = regexp.MustCompile(`^\[Rz (\d{1,4})\]`)

// rzNormalizer rewrites the bare paragraph numbers of a Randziffer document
// into explicit [Rz N] markers.
//
// It exists because asking the model to do it is unreliable in a way that
// matters: measured over eight pages of a scanned brief, two different
// promptings both transcribed the number faithfully but only marked 63% and
// 73% of them, leaving the rest as bare digits indistinguishable from a
// paragraph that happens to begin with a year. Transcription is what a VLM is
// good at; a mechanical rewrite is what code is good at, and code does it the
// same way every time.
//
// The sequence is the safeguard. Randziffern count up by one across the whole
// document, so a leading number is only accepted when it continues the count.
// That is what keeps "2010 wurde der Vertrag geschlossen" from becoming
// [Rz 2010], and it is why the normalizer spans pages rather than resetting.
type rzNormalizer struct {
	// strip removes the number instead of marking it. A document being
	// transcribed to become one of our own carries no paragraph numbers in
	// source: docc generates those at render time and they renumber
	// themselves, so a transcribed one would print twice and go stale the
	// first time a section moved.
	strip bool

	last  int
	found bool
}

// Apply rewrites one page's markdown: each paragraph number is either marked
// as [Rz N] or removed, according to strip. Paragraphs the model already marked
// advance the sequence either way.
func (r *rzNormalizer) Apply(md string) string {
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		if m := marked.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil || !r.accepts(n) {
				continue
			}
			if r.strip {
				lines[i] = strings.TrimSpace(strings.TrimPrefix(line, m[0]))
			}
			r.last, r.found = n, true
			continue
		}
		m := randziffer.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || !r.accepts(n) {
			continue
		}
		if r.strip {
			lines[i] = m[2]
		} else {
			lines[i] = fmt.Sprintf("[Rz %d] %s", n, m[2])
		}
		r.last, r.found = n, true
	}
	return strings.Join(lines, "\n")
}

// accepts reports whether n continues the document's Randziffer sequence. The
// first one seen sets the count — a run started with --pages 30 legitimately
// opens at 55 — and every later one has to be its successor.
func (r *rzNormalizer) accepts(n int) bool {
	if !r.found {
		return n > 0
	}
	return n == r.last+1
}
