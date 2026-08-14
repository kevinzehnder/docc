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

// bareNumber matches a block that is nothing but a number, which is what a
// marginal paragraph number in the gutter looks like once it is recognized on
// its own.
var bareNumber = regexp.MustCompile(`^\d{1,4}$`)

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
	var gutter []Block

	left := bodyLeft(blocks)
	for _, b := range blocks {
		switch {
		case furniture[b.Type], container[b.Type]:
			continue
		case b.Box.X1 <= left-marginGap && isGutterText(b.Text):
			gutter = append(gutter, b)
		default:
			body = append(body, positioned{Block: b, index: len(body)})
		}
	}

	// A gutter column often comes back as one block holding every number on
	// the page — "25\n26\n27\n28" spanning half the page height. The block has
	// one Y0 and the numbers do not, so each line's position is interpolated
	// across the block's span: the first number sits at the top, the last at
	// the bottom, and paragraph starts are close enough to evenly spaced for
	// the nearest-paragraph search below to land. Attachment then advances
	// monotonically — a number never binds above the one before it, which is
	// what stops two numbers landing on the same paragraph when the
	// interpolation is off by a line.
	//
	// An interpolated position is fuzzy where an exact one is not, so only the
	// interpolated path skips the blocks a Randziffer never numbers — an offer
	// of proof, or an indented continuation of one. A single number's position
	// is the number's own, and beside an indented block it means the indented
	// block.
	margins := map[int]string{}
	for _, g := range gutter {
		nums := gutterNumbers(g.Text)
		var skip func(positioned) bool
		if len(nums) > 1 {
			skip = func(b positioned) bool {
				return b.Box.X0 > left+marginGap || evidenceLead.MatchString(b.Text)
			}
		}
		prev := -1
		for k, n := range nums {
			y := g.Box.Y0
			if len(nums) > 1 {
				y += float64(k) * (g.Box.Y1 - g.Box.Y0) / float64(len(nums)-1)
			}
			if i, ok := nearestBelow(body, y, prev, skip); ok {
				if _, taken := margins[i]; !taken {
					margins[i] = n
				}
				prev = i
			}
		}
	}
	return body, margins
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

// isGutterText reports whether a block's recognized text is only numbers — one
// per line, as a merged gutter column comes back. A narrow block to the left of
// the body that says something else is a marginal note, and belongs in the
// document.
func isGutterText(text string) bool {
	lines := gutterNumbers(text)
	return len(lines) > 0
}

func gutterNumbers(text string) []string {
	var nums []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !bareNumber.MatchString(line) {
			return nil // one non-number and the whole block is prose
		}
		nums = append(nums, line)
	}
	return nums
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
