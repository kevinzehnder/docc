package ingest

import (
	"regexp"
	"sort"
	"strings"
)

// furniture is the set of block types that are on the page but not in the
// document: the running header, the running footer and the page number.
//
// Dropping them here is the point of having geometry. Measured over eight pages
// of a scanned brief, in the best prompt condition, two page numbers and three
// letterheads survived a prompt that asked the model not to transcribe them —
// see docs/ingest-todo.md. A model asked to leave something out obeys most of
// the time; a type check leaves it out every time.
var furniture = map[string]bool{
	"header":      true,
	"footer":      true,
	"page_number": true,
}

// container is the set of block types that group other blocks rather than
// holding text of their own.
//
// The layout pass reports both a container and its children: a list of three
// items on a scanned brief came back as three `text` blocks at y 594-627,
// 631-665 and 670-685, and then a `list` block spanning 594-685. Recognizing
// both transcribes the same lines twice, which is what a rendered Rechtsbegehren
// with two prayers turned into four. Skipping containers also saves the round
// trips, since every word in one is already being read as a child.
var container = map[string]bool{
	"list":           true,
	"image_block":    true,
	"equation_block": true,
}

// visual is the set of block types with no text to recognize. They are
// announced rather than dropped silently: a reviewer comparing the draft
// against the source needs to know that something was on the page here.
var visual = map[string]bool{
	"image": true,
	"chart": true,
}

// bareNumber matches a line that is nothing but a number, which is what a
// marginal paragraph number in the gutter looks like once it is recognized on
// its own. dottedNumber is the margin's other resident: a section number set
// beside its heading, "5." against "Vertragswidriges Verhalten".
var (
	bareNumber   = regexp.MustCompile(`^\d{1,4}$`)
	dottedNumber = regexp.MustCompile(`^\d{1,4}\.$`)
	startsDigit  = regexp.MustCompile(`^\d`)
)

// marginGap is how far left of the body column a block has to start before it
// counts as gutter rather than an indented paragraph, as a fraction of the page
// width. On the measured page the body begins at 0.131 and the Randziffern at
// 0.085, so the two are separated by roughly a fifteenth of the page.
const marginGap = 0.02

// positioned pairs a block with its index in the body slice, so that a gutter
// number found later can be attached to it.
type positioned struct {
	Block
	index int
}

// splitGutter separates the document's own blocks from the page furniture and
// the marginal numbers, and works out which paragraph each number belongs to.
//
// The body column's left edge is taken from the blocks themselves rather than
// assumed: a letter, a brief and a scanned fax all put it somewhere different,
// and the only thing that holds across them is that most of the page's text
// starts at the same x.
func splitGutter(blocks []Block) ([]positioned, map[int]string) {
	var body []positioned
	var gutter []marginBlock

	left := bodyLeft(blocks)
	for _, b := range blocks {
		switch {
		case furniture[b.Type], container[b.Type]:
			continue
		case b.Box.X1 <= left-marginGap:
			if lines, ok := marginLines(b.Text); ok {
				gutter = append(gutter, marginBlock{box: b.Box, lines: lines})
				continue
			}
			// A narrow block that says something else is a marginal note,
			// and belongs in the document.
			body = append(body, positioned{Block: b, index: len(body)})
		default:
			body = append(body, positioned{Block: b, index: len(body)})
		}
	}

	// A gutter column often comes back as one block holding every number on
	// the page — "25\n26\n27\n28" spanning half the page height. The block has
	// one Y0 and the numbers do not, so each line's position is interpolated
	// across the block's span: the first line sits at the top, the last at
	// the bottom, and paragraph starts are close enough to evenly spaced for
	// the nearest-block search below to land. Attachment then advances
	// monotonically — a number never binds above the one before it, which is
	// what stops two numbers landing on the same paragraph when the
	// interpolation is off by a line.
	//
	// The lines are of three kinds. A bare number is a Randziffer and binds to
	// prose — never to a heading, an offer of proof, or an indented
	// continuation of one, which the interpolated path skips because its
	// position is fuzzy where a single number's is exact. A dotted number
	// ("5.") is the margin's copy of a section number and binds to the first
	// heading below that does not already carry one. Anything else is a
	// misread — tolerated for its slot in the interpolation, attaching
	// nothing.
	margins := map[int]string{}
	for _, g := range gutter {
		var skipProse func(positioned) bool
		if len(g.lines) > 1 {
			skipProse = func(b positioned) bool {
				return isHeadingType(b.Type) ||
					b.Box.X0 > left+marginGap ||
					evidenceLead.MatchString(b.Text)
			}
		}
		// An interpolated position can overshoot its block — line spacing in
		// the margin follows the paragraphs, not a grid — so an interior line
		// accepts a block reaching back up to half a step. The first and last
		// lines sit exactly at the block's edges, where the interpolation has
		// no error and reaching back would bind a block that ends above the
		// number. The cursor keeps a late estimate from re-binding something
		// already passed.
		step := 0.0
		if len(g.lines) > 1 {
			step = (g.box.Y1 - g.box.Y0) / float64(len(g.lines)-1)
		}
		prev := -1
		for k, line := range g.lines {
			y := g.box.Y0 + float64(k)*step
			slack := 0.0
			if k > 0 && k < len(g.lines)-1 {
				slack = step / 2
			}
			switch {
			case bareNumber.MatchString(line):
				if i, ok := nearestBelow(body, y-slack, prev, skipProse); ok {
					if _, taken := margins[i]; !taken {
						margins[i] = line
					}
					prev = i
				}
			case dottedNumber.MatchString(line):
				onlyHeadings := func(b positioned) bool { return !isHeadingType(b.Type) }
				if i, ok := nearestBelow(body, y-slack, prev, onlyHeadings); ok {
					if !startsDigit.MatchString(body[i].Text) {
						body[i].Text = line + " " + body[i].Text
					}
					prev = i
				}
			}
		}
	}
	return body, margins
}

// marginBlock is one gutter-column block, split into its recognized lines.
type marginBlock struct {
	box   BBox
	lines []string
}

// marginLines reports whether a narrow left block reads as the margin's
// number column, and returns its non-empty lines.
//
// Real margins are noisy. Alongside the Randziffern sit section numbers set
// beside their headings ("5.") and the odd misread — a page of a real
// Klageantwort came back as "8\nE\n9\n10\n1", and requiring every line to be
// a number leaked the whole block into the document as a garbage paragraph
// while losing all four numbers it carried. The column qualifies when its
// numeric lines outnumber the rest; the junk keeps its slot, so the
// interpolation above stays honest about where each line sat.
func marginLines(text string) ([]string, bool) {
	var lines []string
	numeric := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if bareNumber.MatchString(line) || dottedNumber.MatchString(line) {
			numeric++
		}
	}
	if numeric == 0 || numeric*2 <= len(lines) {
		return nil, false
	}
	return lines, true
}

// isHeadingType reports whether a backend block type is a heading.
func isHeadingType(blockType string) bool {
	k, _ := kindOf(blockType)
	return k == KindHeading
}

// bodyLeft is the left edge most of the page's text shares. The median rather
// than the minimum, so that one indented quotation does not move it.
func bodyLeft(blocks []Block) float64 {
	var xs []float64
	for _, b := range blocks {
		if furniture[b.Type] || visual[b.Type] {
			continue
		}
		xs = append(xs, b.Box.X0)
	}
	if len(xs) == 0 {
		return 0
	}
	sort.Float64s(xs)
	return xs[len(xs)/2]
}

// nearestBelow finds the body block a gutter number sits beside: the first one
// past index after, not skipped, whose bottom is not above y. Randziffern are
// set against the paragraph they number, and a number that lands between two
// paragraphs belongs to the one starting after it.
func nearestBelow(body []positioned, y float64, after int, skip func(positioned) bool) (int, bool) {
	const tolerance = 0.01
	for _, b := range body {
		if b.index <= after || (skip != nil && skip(b)) {
			continue
		}
		if b.Box.Y1 >= y+tolerance {
			return b.index, true
		}
	}
	return 0, false
}
