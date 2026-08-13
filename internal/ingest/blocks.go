package ingest

import (
	"fmt"
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

// AssembleBlocks turns recognized blocks into one page's markdown. It is the
// mineru backend's half of what Assemble does for a document: this builds a
// page body, Assemble joins the pages and writes the frontmatter.
//
// Three things happen here that the chat backend has to ask a model for, and
// gets about two thirds of the time: running headers and page numbers are
// dropped, headings are marked because the layout pass called them headings,
// and a number alone in the gutter becomes a [Rz N] marker on the paragraph it
// sits beside. Only the last of those is a guess, and the Randziffer sequence
// check in rzNormalizer is what catches it when it is wrong.
func AssembleBlocks(blocks []Block) string {
	body, margins := splitGutter(blocks)

	var out []string
	for _, b := range body {
		text := strings.TrimSpace(b.Text)

		switch {
		case visual[b.Type]:
			out = append(out, fmt.Sprintf("<!-- %s on the page here, not transcribed -->", b.Type))
			continue
		case text == "":
			continue
		}

		if rz, ok := margins[b.index]; ok {
			// Prefixed rather than put on its own line: rzNormalizer reads a
			// paragraph's opening, and a lone [Rz 55] above a paragraph is not
			// something docc's frontend has any meaning for either.
			text = fmt.Sprintf("[Rz %s] %s", rz, text)
		}
		out = append(out, render(b.Type, text))
	}
	return strings.Join(out, "\n\n")
}

// render wraps one block's recognized text in the markup its type calls for.
func render(blockType, text string) string {
	switch blockType {
	case "doc_title":
		return "# " + oneLine(text)
	case "title", "paragraph_title":
		// One level for every heading: MinerU's layout pass reports that a
		// block is a heading, not how deep it is, and inferring depth from
		// bbox height guesses wrong on the first document that uses a larger
		// font for emphasis. The reviewer sets the levels.
		return "## " + oneLine(text)
	case "list_item":
		return "- " + oneLine(text)
	case "equation":
		return "$$\n" + text + "\n$$"
	case "table":
		// Kept as the HTML the model produced. Converting to a pipe table
		// would lose exactly the merged cells that make a table worth
		// transcribing carefully.
		return text
	case "code", "algorithm":
		return "```\n" + text + "\n```"
	default:
		return text
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

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
		case furniture[b.Type]:
			continue
		case b.Box.X1 <= left-marginGap && isGutterText(b.Text):
			gutter = append(gutter, b)
		default:
			body = append(body, positioned{Block: b, index: len(body)})
		}
	}

	margins := map[int]string{}
	for _, g := range gutter {
		for _, n := range gutterNumbers(g.Text) {
			if i, ok := nearestBelow(body, g.Box.Y0); ok {
				if _, taken := margins[i]; !taken {
					margins[i] = n
				}
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
// whose top is not above it. Randziffern are set against the paragraph they
// number, and a number that lands between two paragraphs belongs to the one
// starting after it.
func nearestBelow(body []positioned, y float64) (int, bool) {
	const tolerance = 0.01
	for _, b := range body {
		if b.Box.Y1 >= y+tolerance {
			return b.index, true
		}
	}
	return 0, false
}
