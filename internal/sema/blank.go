package sema

// Blanking is the inverse of filling in: it takes a complete document and
// empties every position the schema said had to be decided, leaving a skeleton
// whose blanks are the contract's open questions.
//
// It lives beside the field checks rather than in the CLI because it is the
// same notion of "a field" — `.docc-field`, possibly behind a semantic class —
// and the two must not drift. A skeleton whose blanks the checker does not
// recognise is worse than no skeleton.

import (
	"slices"
	"strings"

	"github.com/kevinzehnder/docc/internal/parse"
)

// BlankFieldWidth is the width of an emptied marker. Wide enough to read as a
// blank to be filled rather than as a typo, and the same width everywhere so a
// skeleton's blanks line up in a plain editor.
const BlankFieldWidth = 12

// BlankFields empties the text of every `.docc-field` span, leaving the
// attribute block untouched. The result is a document turned back into the
// form an author starts from: `docc check` still accepts it, because a blank is
// content, and `docc build` refuses it while naming every position left to
// decide.
//
// The spans are found by parsing, not by matching brackets: a field marker may
// carry a semantic class before it — `[Muster Bau]{.firma .docc-field key=firma}`
// keeps its `span.firma` styling — and the attribute block is the parser's to
// read, not a regexp's.
func BlankFields(src string) string {
	f, _ := parse.Parse("skeleton", []byte(src))

	// Rewrite back to front so earlier offsets stay valid as lengths change.
	type edit struct{ start, stop int }
	var edits []edit
	for _, span := range f.Spans() {
		if !span.HasClass(FieldSpanType) {
			continue
		}
		edits = append(edits, edit{
			start: f.BodyBase + span.Literal.Start,
			stop:  f.BodyBase + span.Literal.Stop,
		})
	}
	slices.SortFunc(edits, func(a, b edit) int { return b.start - a.start })

	out := []byte(src)
	blank := []byte(strings.Repeat("_", BlankFieldWidth))
	for _, e := range edits {
		if e.start < 0 || e.stop > len(out) || e.start > e.stop {
			continue
		}
		out = append(out[:e.start], append(blank, out[e.stop:]...)...)
	}
	return string(out)
}
