// Package eval scores a transcription against what the source document can be
// shown to contain.
//
// It exists because every choice behind ingest — which model, what DPI, whether
// a rule belongs in the prompt or in code — was settled by running commands by
// hand and reading the output. That is not repeatable, and nothing notices when
// a change makes the result worse.
//
// The scores here need no hand-labelled corpus. Ground truth comes from
// round-tripping: a document docc built is rendered to PDF, rasterized, and
// transcribed back, so what the page says is known exactly rather than
// approximately — we wrote it. A third party's PDF can be scored the same way
// against its own text layer, which is weaker evidence but real.
//
// Nothing in this package talks to a model, so all of it is testable without
// one.
package eval

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Score is one transcription's result.
type Score struct {
	// Words are precision and recall against the source's own text layer,
	// as fractions. They are meaningless for a scanned document, which has no
	// text layer to compare against; HasText reports whether they mean
	// anything.
	Precision, Recall, F1 float64
	HasText               bool

	// Missing and Spurious sample the disagreement, for reading rather than
	// counting: which words the transcription dropped, and which it invented.
	Missing, Spurious []string

	// RandzifferFound and RandzifferExpected count paragraph numbers. Expected
	// comes from the source's own margin, so it is zero for a scan.
	RandzifferFound, RandzifferExpected int
	// SequenceBreaks counts gaps, repeats and reversals in the numbering that
	// did come through — the signal that a page was lost.
	SequenceBreaks int

	// PageNumbers and Letterheads count running furniture that leaked into the
	// body. Both should be zero.
	PageNumbers, Letterheads int

	// HeadingsFound and HeadingsExpected count markdown headings. The word
	// scores cannot see these: the comparison splits on non-letters, so a "#"
	// is discarded and a model that renders every heading as plain text scores
	// exactly as well as one that marks them up. For a compiler whose next
	// stage validates document structure, that is the wrong thing to be blind
	// to — a draft with no headings fails every section check downstream.
	HeadingsFound, HeadingsExpected int
}

var (
	// pageNumberLine matches a page number standing alone, with or without the
	// dashes Swiss briefs frame them in.
	pageNumberLine = regexp.MustCompile(`^\s*-?\s*(?:Seite\s+)?\d{1,3}\s*-?\s*$`)
	// rzMarker matches ingest's own paragraph-number marker.
	rzMarker = regexp.MustCompile(`^\s*\[Rz (\d+)\]`)
	// headingLine matches an ATX heading, which is the only heading syntax
	// docc's own documents use.
	headingLine = regexp.MustCompile(`^\s{0,3}#{1,6}\s+\S`)
	// wordSplit keeps letters and digits together and drops everything else,
	// so punctuation the model normalises differently is not counted as an
	// OCR error. Umlauts and ß are letters and survive.
	wordSplit = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// Transcription is what a run produced, alongside what the source is known to
// contain.
type Transcription struct {
	// Markdown is the model's output.
	Markdown string
	// SourceText is what the page is known to say: the plain text of the
	// markdown a round trip started from, or a born-digital PDF's own text
	// layer. Empty means there is nothing to compare against and the word
	// scores are skipped.
	SourceText string
	// SourceRandziffern are the paragraph numbers found in the source's left
	// margin, empty for a scan.
	SourceRandziffern []int
	// Letterhead is a string that repeats on every page of this document — a
	// firm name — or empty to skip that check.
	Letterhead string
	// SourceHeadings is how many headings the source document has.
	SourceHeadings int
}

// Grade scores one transcription.
func Grade(t Transcription) Score {
	s := Score{HasText: strings.TrimSpace(t.SourceText) != ""}

	if s.HasText {
		// Both sides go through PlainText, because the comparison is between
		// what the page says and what the model read off it — and neither
		// frontmatter nor an HTML comment is on the page.
		//
		// Only the source was normalized for a while, so everything Assemble
		// prepends to a draft was scored as invention: `docc: 1`, and the
		// banner's "review before treating this as a source document" down to
		// the digits of its date. The first measured baseline has `08`, `13`
		// and `before` in its invented-words list for that reason, and the
		// floor this file attributes to theme boilerplate was partly docc
		// marking down its own output.
		s.Precision, s.Recall, s.F1, s.Missing, s.Spurious = compareWords(t.SourceText, PlainText(t.Markdown))
	}

	found := markedNumbers(t.Markdown)
	s.RandzifferFound = len(found)
	s.RandzifferExpected = len(t.SourceRandziffern)
	s.SequenceBreaks = sequenceBreaks(found)

	s.HeadingsExpected = t.SourceHeadings
	for _, line := range strings.Split(t.Markdown, "\n") {
		if headingLine.MatchString(line) {
			s.HeadingsFound++
		}
		if pageNumberLine.MatchString(line) {
			s.PageNumbers++
		}
		if t.Letterhead != "" && strings.Contains(strings.ToLower(line), strings.ToLower(t.Letterhead)) {
			s.Letterheads++
		}
	}
	return s
}

// compareWords scores the transcription against the source.
//
// Bags of words rather than an edit distance, because the two texts do not have
// to agree on order to be equally faithful: a marginal annotation can be placed
// before or after the paragraph it annotates, and a text layer's reading order
// is only approximate.
//
// The two halves count differently, because they answer different questions.
// Recall asks whether the words on the page came through, so it counts
// instances: dropping one of three occurrences is a third of a loss. Precision
// asks whether the model invented anything, so it asks only whether a word
// appears in the source at all — a date legitimately printed in the letterhead,
// the body and the signature block is not three inventions because the source
// names it once.
func compareWords(source, got string) (precision, recall, f1 float64, missing, spurious []string) {
	want := bagOf(source)
	have := bagOf(got)

	var covered, known int
	for w, n := range want {
		covered += min(have[w], n)
		if have[w] < n {
			missing = append(missing, w)
		}
	}
	for w, n := range have {
		if want[w] > 0 {
			known += n
			continue
		}
		spurious = append(spurious, w)
	}
	sort.Strings(missing)
	sort.Strings(spurious)

	wantN, haveN := total(want), total(have)
	if haveN > 0 {
		precision = float64(known) / float64(haveN)
	}
	if wantN > 0 {
		recall = float64(covered) / float64(wantN)
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1, sample(missing), sample(spurious)
}

func bagOf(s string) map[string]int {
	out := map[string]int{}
	for _, w := range wordSplit.Split(s, -1) {
		w = strings.ToLower(strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }))
		if w != "" {
			out[w]++
		}
	}
	return out
}

func total(bag map[string]int) int {
	n := 0
	for _, c := range bag {
		n += c
	}
	return n
}

// sample caps a diagnostic list. The whole list of every word that differs is
// not something anybody reads.
func sample(words []string) []string {
	const max = 12
	if len(words) <= max {
		return words
	}
	return words[:max]
}

// markedNumbers returns the paragraph numbers a transcription marked, in the
// order they appear.
func markedNumbers(md string) []int {
	var out []int
	for _, line := range strings.Split(md, "\n") {
		m := rzMarker.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// sequenceBreaks counts the places the numbering does not simply advance by
// one. Each is a page the transcription lost, two paragraphs it merged, or an
// order it got wrong.
func sequenceBreaks(nums []int) int {
	breaks := 0
	for i := 1; i < len(nums); i++ {
		if nums[i] != nums[i-1]+1 {
			breaks++
		}
	}
	return breaks
}

// frontmatter matches a leading YAML block, and syntax the markers and
// decoration a transcription is not expected to reproduce byte for byte.
var (
	frontmatterBlock = regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)
	htmlComment      = regexp.MustCompile(`(?s)<!--.*?-->`)
	fenceLine        = regexp.MustCompile(`(?m)^\s*(?::::*|` + "```" + `).*$`)
)

// PlainText reduces a docc source document to the words a transcription of it
// should contain: no frontmatter, no generated-by comments, no fence markers.
//
// It is deliberately crude. The comparison is a bag of words, so leaving a
// heading's "#" or a list item's "-" in would only add symbols that the word
// split discards anyway; what has to go is text the page never shows.
func PlainText(md string) string {
	md = frontmatterBlock.ReplaceAllString(md, "")
	md = htmlComment.ReplaceAllString(md, "")
	md = fenceLine.ReplaceAllString(md, "")
	return strings.TrimSpace(md)
}

// ExpectedRandziffern returns the paragraph numbers docc would generate for a
// source document, given where its schema starts numbering.
//
// A rendered brief numbers every top-level paragraph of prose after the named
// heading, continuously. Headings, list items and fenced blocks are not prose
// and are not numbered — the same rule internal/ir applies at build time, which
// is why a round trip can predict the sequence rather than guess it.
func ExpectedRandziffern(md, startAfterHeading string) int {
	body := PlainText(md)
	started := startAfterHeading == ""
	inFence := false
	n := 0
	for _, para := range strings.Split(body, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if strings.HasPrefix(para, ":::") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(para, "#") {
			if !started && strings.Contains(strings.ToUpper(para), strings.ToUpper(startAfterHeading)) {
				started = true
			}
			continue
		}
		if !started {
			continue
		}
		// A list is one block of items, none of which is body prose.
		if first, _, _ := strings.Cut(para, "\n"); isListItem(first) {
			continue
		}
		n++
	}
	return n
}

func isListItem(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i < len(line) && (line[i] == '.' || line[i] == ')')
}

// FlattenValues renders frontmatter values as the words they become on the
// page.
//
// A brief's court address, parties, subject line and signature are not body
// prose — they come from frontmatter, and the theme interpolates them into the
// letterhead and the signature block. They are still text a transcription is
// expected to reproduce, so scoring a round trip against the body alone counts
// every one of them as invented. Running headers and page numbers are the
// opposite case: genuinely on the page, and deliberately not transcribed, which
// is why they are counted separately rather than compared.
func FlattenValues(v any) string {
	var b strings.Builder
	if m, ok := v.(map[string]any); ok {
		// `docc` and `document_type` select and version the document; no theme
		// renders them, so demanding a transcription contain them asks the
		// model to read something that is not on the page.
		//
		// This hid behind a second error for a while. Assemble writes both back
		// into a draft's own frontmatter, so `document_type: legal` in the
		// output was covering the `legal` this function put in the ground
		// truth — docc satisfying its own requirement. Normalizing the
		// transcription exposed it as a dropped word, which is how it was
		// found.
		kept := make(map[string]any, len(m))
		for k, val := range m {
			if k == "docc" || k == "document_type" {
				continue
			}
			kept[k] = val
		}
		v = kept
	}
	flatten(&b, v)
	return b.String()
}

func flatten(b *strings.Builder, v any) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flatten(b, t[k])
		}
	case []any:
		for _, e := range t {
			flatten(b, e)
		}
	case nil:
		// A nullable field that is set to ~ renders as nothing.
	default:
		fmt.Fprintf(b, "%v ", t)
	}
}

// CountHeadings returns how many ATX headings a document has.
func CountHeadings(md string) int {
	n := 0
	for _, line := range strings.Split(md, "\n") {
		if headingLine.MatchString(line) {
			n++
		}
	}
	return n
}
