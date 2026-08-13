package ingest

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is one element of a transcribed document.
//
// It is the intermediate every pass after a backend works on. Ingest used to
// carry a page as markdown between its stages, which meant that each pass
// rediscovered by regular expression what the pass before it had just
// serialized: the outline pass looked for `^#{1,6}` to find headings the layout
// model had already classified, the Randziffer pass re-parsed `[Rz N]` markers
// that block assembly had written a moment earlier, and the structuring pass
// tracked `:::` fences with a boolean because it had no other way to know
// whether it was inside one. Every one of those is a parser for a language we
// generate ourselves, which is a parser that can only lose.
//
// So markdown stops being the thing that moves between stages and becomes what
// the last stage prints. A pass takes []Node and returns []Node; Render is the
// only function in the package that knows what a "#" means.
//
// Node deliberately does not reuse ir.Block. That type is what the emitter
// consumes, and it carries an inline formatting tree ingest does not produce
// and no provenance, which is the one thing ingest must not lose.
type Node struct {
	Kind Kind
	// Text is the element's text, already trimmed. For KindTable it is the
	// HTML the model produced, and for KindVisual it is empty.
	Text string
	// Level is the heading depth, 1 to 6. Zero on everything else.
	Level int
	// SourceNumber is the marginal paragraph number the source document
	// printed beside this element, or nil.
	//
	// It is a number the document *has*, not one docc generates: a transcribed
	// Randziffer is a fact about somebody else's brief and renumbering it would
	// make every citation of it wrong. Our own documents carry no such number
	// in source — the schema generates those at render time — which is the
	// distinction --strip-randziffern exists to make.
	SourceNumber *int

	// Box is where on the page this came from, and RawType is the backend's
	// own label for it ("title", "aside_text", "chart"). Both are carried
	// rather than consumed: Box is what will let a reviewer be shown the crop
	// a paragraph was read from, and RawType keeps a backend's finer
	// vocabulary available to a pass that wants it without forcing that
	// vocabulary on the passes that do not.
	Box     BBox
	RawType string
}

// Kind is what a Node is, in the vocabulary every pass shares. It is coarser
// than a backend's own block types on purpose: "title" and "paragraph_title"
// are both a heading to everything downstream, and a pass that genuinely needs
// to tell them apart has RawType.
type Kind int

const (
	// KindPara is body prose, and the zero value is deliberately not it — an
	// unset Kind is a bug rather than a paragraph.
	KindPara Kind = iota + 1
	KindHeading
	KindListItem
	KindTable
	KindCode
	KindEquation
	// KindVisual is a picture: announced in the output, never transcribed.
	KindVisual
	// KindRaw is markdown that has not been broken into elements, printed
	// back exactly as it arrived.
	//
	// It is how the chat backend's output crosses this seam for now: that
	// protocol returns a page of free-running markdown, and parsing it into
	// elements is a change to what the chat path emits, which is a change only
	// the round trip in internal/eval can sign off on. Until then one node
	// holds the page, Render gives it back byte for byte, and the passes that
	// still work on text are none the wiser.
	//
	// A pass that needs to see inside a page cannot use this, which is the
	// point at which the chat backend has to learn to parse.
	KindRaw
)

// kinds maps a backend's block type onto a Kind and, for a heading, its depth.
//
// Two levels and no more: MinerU's layout pass reports that a block is a
// heading, not how deep it is, and a document type's outline scheme is what
// decides depth properly a few passes later. Guessing here would only give that
// pass something wrong to correct.
var kinds = map[string]struct {
	kind  Kind
	level int
}{
	"doc_title":       {KindHeading, 1},
	"title":           {KindHeading, 2},
	"paragraph_title": {KindHeading, 2},
	"list_item":       {KindListItem, 0},
	"table":           {KindTable, 0},
	"equation":        {KindEquation, 0},
	"code":            {KindCode, 0},
	"algorithm":       {KindCode, 0},
	"image":           {KindVisual, 0},
	"chart":           {KindVisual, 0},
}

// kindOf classifies one backend block type.
func kindOf(blockType string) (Kind, int) {
	if k, ok := kinds[blockType]; ok {
		return k.kind, k.level
	}
	return KindPara, 0
}

// Nodes turns one page's recognized blocks into document elements: page
// furniture and container blocks are dropped, gutter numbers are attached to
// the paragraphs they sit beside, and what is left is classified.
func Nodes(blocks []Block) []Node {
	body, margins := splitGutter(blocks)

	out := make([]Node, 0, len(body))
	for _, b := range body {
		kind, level := kindOf(b.Type)
		n := Node{
			Kind:    kind,
			Text:    strings.TrimSpace(b.Text),
			Level:   level,
			Box:     b.Box,
			RawType: b.Type,
		}
		// A picture is announced even though it has no text; anything else
		// with nothing in it is not an element of the document.
		//
		// The empty check comes after the gutter split rather than before it,
		// which is what keeps this equivalent to the assembly it replaces: an
		// empty block still occupies a position, so a marginal number beside
		// it still resolves to it and is still dropped along with it. Skipping
		// empties earlier would silently move that number onto the next
		// paragraph, which is a paragraph it does not belong to.
		if n.Kind != KindVisual && n.Text == "" {
			continue
		}
		if rz, ok := margins[b.index]; ok {
			// Atoi cannot fail on what gutterNumbers admits, which is one to
			// four digits. Parsing rather than carrying the string is what
			// makes the number comparable to its neighbours, and it drops a
			// leading zero a scan occasionally produces.
			if v, err := strconv.Atoi(rz); err == nil {
				n.SourceNumber = &v
			}
		}
		out = append(out, n)
	}
	return out
}

// Render prints document elements as markdown. It is the only place in ingest
// that writes markdown syntax.
func Render(nodes []Node) string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, renderNode(n))
	}
	return strings.Join(out, "\n\n")
}

// renderNode prints one element.
func renderNode(n Node) string {
	if n.Kind == KindVisual {
		// Announced rather than dropped silently: a reviewer comparing the
		// draft against the source needs to know something was on the page
		// here.
		return fmt.Sprintf("<!-- %s on the page here, not transcribed -->", n.RawType)
	}

	text := n.Text
	if n.SourceNumber != nil {
		// Prefixed rather than put on its own line: a lone [Rz 55] above a
		// paragraph is not something docc's frontend has any meaning for.
		text = fmt.Sprintf("[Rz %d] %s", *n.SourceNumber, text)
	}

	switch n.Kind {
	case KindHeading:
		return strings.Repeat("#", n.Level) + " " + oneLine(text)
	case KindListItem:
		return "- " + oneLine(text)
	case KindEquation:
		return "$$\n" + text + "\n$$"
	case KindCode:
		return "```\n" + text + "\n```"
	case KindTable:
		// Kept as the HTML the model produced. Converting to a pipe table
		// would lose exactly the merged cells that make a table worth
		// transcribing carefully.
		return text
	case KindRaw:
		return text
	default:
		return text
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
