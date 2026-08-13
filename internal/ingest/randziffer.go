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

// minRZRun is how many consecutive numbers it takes before a run is believed.
//
// One is not enough, and that is the whole lesson here. Any four-digit number
// opening a paragraph looks exactly like a Randziffer: a postal code does, and
// on our own rendered fixture "5400 Baden" in the letterhead was accepted as the
// document's first paragraph number, after which 1, 2, 3 and 4 were all rejected
// for not continuing from 5401. Every model and both backends scored 1 of 4 on
// that fixture for the whole of a day's measurement, and the one that was found
// was the postal code.
const minRZRun = 2

// rzLoneMax bounds the one case where a single number is believed without a
// successor to corroborate it: a document that offers exactly one candidate,
// which is what converting one page of a brief looks like.
//
// The bound is what separates that from the postal code. Randziffern count
// paragraphs, and a brief with a thousand of them does not exist; Swiss postal
// codes are four digits starting at 1000, and a year is four digits too. So a
// lone candidate is believed below 1000 and never above it.
const rzLoneMax = 1000

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
// The sequence is the safeguard, and it only works if the sequence is chosen
// after seeing all of it. Randziffern count up by one across the whole
// document, so the numbers to mark are the longest chain that does — found by
// looking at every candidate in the document and picking the chain, rather than
// by trusting whichever number happened to come first. That is what keeps
// "2010 wurde der Vertrag geschlossen" from becoming [Rz 2010], and it is why
// this runs over the finished pages instead of each page as it arrives.
type rzNormalizer struct {
	// strip removes the number instead of marking it. A document being
	// transcribed to become one of our own carries no paragraph numbers in
	// source: docc generates those at render time and they renumber
	// themselves, so a transcribed one would print twice and go stale the
	// first time a section moved.
	strip bool
}

// rzCandidate is one line that could be a paragraph number.
type rzCandidate struct {
	page, line int
	n          int
	// rest is the paragraph text after the number, for the rewrite.
	rest string
	// marked reports that the model wrote the [Rz N] itself.
	marked bool
}

// ApplyNodes marks or strips the paragraph numbers of a document's elements.
//
// The chain is still what decides, and for the same reason — a lone number is
// indistinguishable from a postal code — but the candidates are better. A
// backend that found a number in the gutter has already put it on the element
// it belongs to, so that half needs no guessing at all: the pass reads
// SourceNumber instead of re-parsing a marker it wrote itself a moment ago.
// What is left to guess is a number the backend merged into the body text,
// which is the same regular expression as before, now applied to a paragraph
// rather than to a line that might be anything.
//
// A page of KindRaw is the chat backend's, and has no elements to consult. Its
// text goes through Apply. A document is one backend's throughout, so the two
// paths do not interleave — there is one sequence either way.
func (r *rzNormalizer) ApplyNodes(pages [][]Node) [][]Node {
	// Where a candidate lives, so the decision can be written back to it. Kept
	// alongside rather than derived: the raw bodies are not one per page in
	// general, and indexing the result of Apply by page number would misalign
	// the moment a page had none.
	type ref struct{ page, node int }

	var (
		rawPages []string
		rawRefs  []ref
	)
	for p, nodes := range pages {
		for i, n := range nodes {
			if n.Kind == KindRaw {
				rawRefs = append(rawRefs, ref{p, i})
				rawPages = append(rawPages, n.Text)
			}
		}
	}
	if len(rawPages) > 0 {
		for k, text := range r.Apply(rawPages) {
			at := rawRefs[k]
			pages[at.page][at.node].Text = text
		}
		return pages
	}

	var (
		cands []rzCandidate
		refs  []ref
	)
	for p, nodes := range pages {
		for i, n := range nodes {
			switch {
			case n.SourceNumber != nil:
				// Already located by the backend, and already separated from
				// the text. It joins the chain as a link that needs no proof.
				cands = append(cands, rzCandidate{n: *n.SourceNumber, marked: true, rest: n.Text})
				refs = append(refs, ref{p, i})
			case n.Kind == KindPara:
				if m := randziffer.FindStringSubmatch(n.Text); m != nil {
					v, err := strconv.Atoi(m[1])
					if err != nil || v <= 0 {
						continue
					}
					cands = append(cands, rzCandidate{n: v, rest: m[2]})
					refs = append(refs, ref{p, i})
				}
			}
		}
	}

	// Everything not in the chain loses the number it was carrying: a block the
	// backend read as a gutter number, that no sequence corroborates, is a
	// figure in the margin rather than a paragraph number.
	chosen := map[int]bool{}
	for _, i := range longestChain(cands) {
		chosen[i] = true
	}
	for i, c := range cands {
		at := &pages[refs[i].page][refs[i].node]
		switch {
		case !chosen[i]:
			at.SourceNumber = nil
		case r.strip:
			// A document destined to become one of our own carries no
			// paragraph numbers in source: docc generates those at render
			// time, and a transcribed one would print twice.
			at.SourceNumber, at.Text = nil, c.rest
		default:
			v := c.n
			at.SourceNumber, at.Text = &v, c.rest
		}
	}
	return pages
}

// Apply rewrites the pages of one document in place, marking or stripping the
// paragraph numbers of the longest chain of consecutive values it can find.
//
// Pages are taken together because a chain spans them: a document numbered 1 to
// 90 puts one or two on every page, and no page on its own carries enough of
// the sequence to tell it from a postal code.
func (r *rzNormalizer) Apply(pages []string) []string {
	split := make([][]string, len(pages))
	var cands []rzCandidate
	for p, page := range pages {
		split[p] = strings.Split(page, "\n")
		for l, line := range split[p] {
			if c, ok := candidate(p, l, line); ok {
				cands = append(cands, c)
			}
		}
	}

	for _, i := range longestChain(cands) {
		c := cands[i]
		if r.strip {
			split[c.page][c.line] = c.rest
		} else {
			split[c.page][c.line] = fmt.Sprintf("[Rz %d] %s", c.n, c.rest)
		}
	}

	out := make([]string, len(pages))
	for p := range split {
		out[p] = strings.Join(split[p], "\n")
	}
	return out
}

// candidate reads one line as a possible paragraph number.
func candidate(page, line int, s string) (rzCandidate, bool) {
	if m := marked.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return rzCandidate{}, false
		}
		return rzCandidate{
			page: page, line: line, n: n, marked: true,
			rest: strings.TrimSpace(strings.TrimPrefix(s, m[0])),
		}, true
	}
	if m := randziffer.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return rzCandidate{}, false
		}
		return rzCandidate{page: page, line: line, n: n, rest: m[2]}, true
	}
	return rzCandidate{}, false
}

// longestChain returns the indices of the longest run of candidates whose
// values increase by exactly one, in document order.
//
// Skipping is allowed between links: a stray number between two real ones — a
// postal code, a year, a figure the model put on its own line — breaks nothing,
// because the chain steps over it rather than ending there.
//
// A chain shorter than minRZRun is not returned at all. A document with one
// candidate and no way to corroborate it is better left with a bare number a
// reader can see than with a marker asserting something nobody checked.
func longestChain(cands []rzCandidate) []int {
	if len(cands) == 0 {
		return nil
	}
	// One candidate and nothing to check it against: converting a single page
	// of a brief. Believed only below rzLoneMax, which is what keeps a lone
	// letterhead postcode or a year from becoming paragraph 5400.
	if len(cands) == 1 {
		if cands[0].n < rzLoneMax {
			return []int{0}
		}
		return nil
	}

	length := make([]int, len(cands))
	pred := make([]int, len(cands))
	// endOf maps a value to the candidate index where the best chain ending on
	// that value stops, so the next link can find its predecessor in one look.
	endOf := map[int]int{}

	best := 0
	for i, c := range cands {
		length[i], pred[i] = 1, -1
		if j, ok := endOf[c.n-1]; ok {
			length[i], pred[i] = length[j]+1, j
		}
		// Ties go to the earlier candidate, which keeps a document's first
		// numbering the one that wins when a later block repeats its values.
		if j, ok := endOf[c.n]; !ok || length[i] > length[j] {
			endOf[c.n] = i
		}
		if length[i] > length[best] {
			best = i
		}
	}
	if length[best] < minRZRun {
		return nil
	}

	chain := make([]int, 0, length[best])
	for i := best; i >= 0; i = pred[i] {
		chain = append(chain, i)
	}
	// Reconstructed backwards; the caller reads them in document order.
	for l, r := 0, len(chain)-1; l < r; l, r = l+1, r-1 {
		chain[l], chain[r] = chain[r], chain[l]
	}
	return chain
}
