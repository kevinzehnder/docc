package sema

import (
	"regexp"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/yuin/goldmark/ast"
)

// leadingZeroMarker matches a line opening an ordered list with a number that
// has a leading zero: "01. April 2027", "05) …".
var leadingZeroMarker = regexp.MustCompile(`^[ \t>]*0\d*[.)][ \t]`)

// checkLeadingZeroLists warns about an ordered list whose first number is
// written with a leading zero.
//
// Nobody numbers a list "01.", but a date at the start of a paragraph reads
// exactly like that: "01. April 2027" is a one-item list starting at 1 with
// the text "April 2027". The number is then set by the list — or, under
// `unnumbered_single_item_lists`, not at all — and the day is gone without a
// word. Found in an executed deed whose Antrittstermin lost its day.
func checkLeadingZeroLists(f *parse.File, ds *diag.List) {
	_ = ast.Walk(f.Body, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		list, ok := n.(*ast.List)
		if !entering || !ok || !list.IsOrdered() || list.FirstChild() == nil {
			return ast.WalkContinue, nil
		}
		for c := list.FirstChild().FirstChild(); c != nil; c = c.NextSibling() {
			lines := f.TextLines(c)
			if len(lines) == 0 {
				continue
			}
			line := f.LineText(lines[0].Pos.Line)
			if leadingZeroMarker.MatchString(line) {
				ds.Warnf(f.Path, diag.Position{Line: lines[0].Pos.Line, Col: 1}, "DOC044",
					`if this is a date, escape the full stop: "01\. April 2027"`,
					"%q starts a numbered list, not a date: the number is set by the list and the text loses it", line)
			}
			break
		}
		return ast.WalkContinue, nil
	})
}
