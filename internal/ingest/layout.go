package ingest

import (
	"fmt"
	"strconv"
	"strings"
)

// Block is one region the layout pass found, in reading order.
type Block struct {
	// Type is a MinerU block type: "text", "title", "table", "header",
	// "page_number" and the rest of blockTypes. It is the whole reason this
	// backend exists — a heading here is a classification the model made
	// looking at the page, not a "#" it chose to type while free-running
	// markdown and forgot on the next page.
	Type string
	// Box is the block's position, as fractions of the page.
	Box BBox
	// Angle is the block's rotation in degrees: 0, 90, 180 or 270.
	Angle int
	// Text is the recognition pass's output, empty until it has run.
	Text string
}

// blockTypes is MinerU's vocabulary, from mineru_vl_utils' structs.py. Parsing
// against a closed set is what makes a malformed line detectable: without it,
// a truncated coordinate followed by the next line's digits reads as a
// perfectly plausible box with a type nobody has ever heard of.
var blockTypes = map[string]bool{
	"text": true, "title": true, "doc_title": true, "paragraph_title": true,
	"table": true, "equation": true, "formula_number": true,
	"code": true, "algorithm": true, "aside_text": true, "ref_text": true,
	"index": true, "phonetic": true, "list_item": true,
	"table_caption": true, "image_caption": true, "code_caption": true, "caption": true,
	"table_footnote": true, "image_footnote": true, "footnote": true,
	"header": true, "footer": true, "page_number": true, "page_footnote": true,
	"image": true, "chart": true,
	"list": true, "image_block": true, "equation_block": true,
	"unknown": true,
}

// rotations maps the tokens the model emits for a rotated block onto degrees.
var rotations = map[string]int{
	"<|rotate_up|>":    0,
	"<|rotate_right|>": 90,
	"<|rotate_down|>":  180,
	"<|rotate_left|>":  270,
}

// ParseLayout reads the layout pass's response. One block per line:
//
//	887 027 936 060header
//	131 175 218 189title
//	085 193 102 203text
//
// Four zero-padded thousandths — x0 y0 x1 y1 — with the type appended to the
// last one without a separator, and a rotation token before the type when the
// block is not upright.
//
// A line it cannot read is an error, not a line to skip. The point of this
// backend is that structure is decided by the model rather than guessed at, and
// silently dropping the blocks that did not parse would hand back a page with a
// hole in it that reads exactly like a page the document does not have.
func ParseLayout(raw string) ([]Block, error) {
	var blocks []Block
	for i, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b, err := parseBlockLine(line)
		if err != nil {
			return nil, fmt.Errorf("layout line %d: %w", i+1, err)
		}
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("the layout pass returned no blocks")
	}
	return blocks, nil
}

// parseBlockLine reads one "x0 y0 x1 y1[rotation]type" line.
func parseBlockLine(line string) (Block, error) {
	fields := strings.Fields(line)
	if len(fields) != 4 {
		return Block{}, fmt.Errorf("expected 4 space-separated coordinates, got %d in %q", len(fields), line)
	}

	coords := make([]float64, 4)
	for i, f := range fields {
		// The fourth field carries the type, and possibly a rotation token,
		// glued to the digits. Split at the first non-digit.
		digits := f
		if i == 3 {
			digits = leadingDigits(f)
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return Block{}, fmt.Errorf("coordinate %q is not a number in %q", digits, line)
		}
		if n < 0 || n > 1000 {
			return Block{}, fmt.Errorf("coordinate %d is outside 0..1000 in %q", n, line)
		}
		coords[i] = float64(n) / 1000
	}

	rest := fields[3][len(leadingDigits(fields[3])):]
	angle := 0
	for token, deg := range rotations {
		if after, ok := strings.CutPrefix(rest, token); ok {
			angle, rest = deg, after
			break
		}
	}
	if !blockTypes[rest] {
		return Block{}, fmt.Errorf("unknown block type %q in %q", rest, line)
	}

	box := BBox{X0: coords[0], Y0: coords[1], X1: coords[2], Y1: coords[3]}
	if box.Empty() {
		return Block{}, fmt.Errorf("block %q has no area in %q", rest, line)
	}
	return Block{Type: rest, Box: box, Angle: angle}, nil
}

func leadingDigits(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}
