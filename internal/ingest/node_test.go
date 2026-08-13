package ingest

import (
	"strings"
	"testing"
)

// A backend's block type decides what an element is; Render decides how it is
// written down. This is the table that used to test the two as one function.
func TestRenderWrapsByKind(t *testing.T) {
	cases := []struct{ blockType, text, want string }{
		{"doc_title", "Replik", "# Replik"},
		{"title", "Zu Rz. 80", "## Zu Rz. 80"},
		{"paragraph_title", "Zu Rz. 80", "## Zu Rz. 80"},
		{"list_item", "Parteibefragung", "- Parteibefragung"},
		{"equation", "a = b", "$$\na = b\n$$"},
		{"code", "go build ./...", "```\ngo build ./...\n```"},
		{"algorithm", "go build ./...", "```\ngo build ./...\n```"},
		{"table", "<table><tr><td>x</td></tr></table>", "<table><tr><td>x</td></tr></table>"},
		{"text", "Der Kläger.", "Der Kläger."},
		{"image", "", "<!-- image on the page here, not transcribed -->"},
		{"chart", "", "<!-- chart on the page here, not transcribed -->"},
	}
	for _, c := range cases {
		kind, level := kindOf(c.blockType)
		n := Node{Kind: kind, Level: level, Text: c.text, RawType: c.blockType}
		if got := renderNode(n); got != c.want {
			t.Errorf("renderNode(%q, %q) = %q, want %q", c.blockType, c.text, got, c.want)
		}
	}
}

// An unrecognized block type is prose. The layout vocabulary grows with the
// model, and a type nobody has mapped yet still holds text the document needs.
func TestKindOfDefaultsToProse(t *testing.T) {
	if kind, level := kindOf("ref_text"); kind != KindPara || level != 0 {
		t.Errorf("kindOf(ref_text) = %v/%d, want KindPara/0", kind, level)
	}
}

// The marginal number is a field, not a prefix somebody has to find again with
// a regular expression. This is the whole reason Node exists.
func TestNodesCarryTheGutterNumberAsAValue(t *testing.T) {
	nodes := Nodes([]Block{
		block("text", 0.085, 0.193, 0.102, 0.203, "55"),
		block("text", 0.131, 0.193, 0.870, 0.532, "Richtig ist, dass ..."),
	})

	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (the gutter number is not an element)", len(nodes))
	}
	if nodes[0].SourceNumber == nil {
		t.Fatal("SourceNumber not set")
	}
	if got := *nodes[0].SourceNumber; got != 55 {
		t.Errorf("SourceNumber = %d, want 55", got)
	}
	if strings.Contains(nodes[0].Text, "55") {
		t.Errorf("the number leaked into the text: %q", nodes[0].Text)
	}
}

// A gutter number is parsed, not copied. This is the one place the refactor
// changes what gets written: a scan that recognizes the margin as "055" used to
// render "[Rz 055]", and now renders "[Rz 55]". Carrying the digits as a string
// was what made that possible, and a number that is not comparable to its
// neighbours cannot take part in the sequence check that catches a postal code
// pretending to be paragraph one.
func TestNodesNormalizeALeadingZero(t *testing.T) {
	nodes := Nodes([]Block{
		block("text", 0.085, 0.193, 0.102, 0.203, "055"),
		block("text", 0.131, 0.193, 0.870, 0.532, "Richtig ist, dass ..."),
	})
	if len(nodes) != 1 || nodes[0].SourceNumber == nil {
		t.Fatalf("got %d nodes, want 1 numbered", len(nodes))
	}
	if got := *nodes[0].SourceNumber; got != 55 {
		t.Errorf("SourceNumber = %d, want 55", got)
	}
	if got := renderNode(nodes[0]); !strings.HasPrefix(got, "[Rz 55] ") {
		t.Errorf("rendered %q, want a [Rz 55] prefix", got)
	}
}

// Provenance survives classification. Nothing reads Box yet; the passes that
// will — showing a reviewer the crop a paragraph came from — cannot recover it
// once it has been thrown away, which is what serializing to markdown did.
func TestNodesKeepProvenance(t *testing.T) {
	nodes := Nodes([]Block{
		block("title", 0.131, 0.175, 0.218, 0.189, "I. FORMELLES"),
	})
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	if nodes[0].RawType != "title" {
		t.Errorf("RawType = %q, want %q", nodes[0].RawType, "title")
	}
	if nodes[0].Box.X0 != 0.131 || nodes[0].Box.Y1 != 0.189 {
		t.Errorf("Box = %+v, want the block's own", nodes[0].Box)
	}
}

// An empty block is not an element, but it still holds its position while the
// gutter numbers are assigned — so a number beside it is dropped with it rather
// than sliding onto the next paragraph, which is a paragraph it never numbered.
func TestNodesDropEmptyBlocksWithTheirNumber(t *testing.T) {
	nodes := Nodes([]Block{
		block("text", 0.085, 0.193, 0.102, 0.203, "55"),
		block("text", 0.131, 0.193, 0.870, 0.300, ""),
		block("text", 0.131, 0.310, 0.870, 0.400, "Die Ausführungen ..."),
	})

	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	if nodes[0].SourceNumber != nil {
		t.Errorf("number %d slid onto the following paragraph", *nodes[0].SourceNumber)
	}
}
