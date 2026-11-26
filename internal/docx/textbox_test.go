package docx

import (
	"strings"
	"testing"
)

func framed(text string, f FramePr) Paragraph {
	p := P("Standard", text)
	p.Props.Frame = &f
	return p
}

// Consecutive identical frames share one box; a different frame starts the
// next; each box hangs on the following flow paragraph, a trailing one on the
// last; nothing is written as w:framePr.
func TestFramesBecomeTextBoxes(t *testing.T) {
	addr := FramePr{X: Mm(25), Y: Mm(60), Width: Mm(95)}
	col := FramePr{X: Mm(150), Y: Mm(40), Width: Mm(45), Wrap: "behind"}
	tail := FramePr{X: Mm(25), Y: Mm(280), HAnchor: "margin", VAnchor: "text", Wrap: "top_and_bottom"}
	d := &Document{
		Styles: []Style{{ID: "Standard", Name: "Standard", Type: StyleParagraph, Default: true}},
		Body: []Block{
			framed("Frau Muster", addr),
			framed("Bahnhofstrasse 1", addr),
			framed("Kanzlei", col),
			P("Standard", "Date"),
			P("Standard", "Body"),
			framed("Footnote box", tail),
		},
		Headers: []HeaderFooter{{Type: HFFirst, Blocks: []Block{framed("Only a frame", col)}}},
	}
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, data, "word/document.xml")

	if strings.Contains(doc, "w:framePr") {
		t.Error("a frame was written as w:framePr")
	}
	if n := strings.Count(doc, "<wps:wsp>"); n != 3 {
		t.Fatalf("got %d text boxes, want 3 (address, column, tail)", n)
	}
	// Both address lines in the first box, as two paragraphs.
	first := doc[strings.Index(doc, "<w:txbxContent>"):strings.Index(doc, "</w:txbxContent>")]
	if !strings.Contains(first, "Frau Muster") || !strings.Contains(first, "Bahnhofstrasse 1") || strings.Count(first, "<w:p>") != 2 {
		t.Errorf("address box does not hold both lines:\n%s", first)
	}
	// The address and column hang on "Date"; the tail box on "Body".
	date := doc[:strings.Index(doc, ">Date<")]
	if strings.Count(date, "<wps:wsp>") != 2 {
		t.Error("the two leading boxes are not anchored to the paragraph after them")
	}
	if body := doc[strings.Index(doc, ">Date<"):strings.Index(doc, ">Body<")]; !strings.Contains(body, "Footnote box") {
		t.Error("a trailing box is not anchored to the last flow paragraph")
	}
	for _, want := range []string{
		`<wp:positionH relativeFrom="page"><wp:posOffset>` + itoa(int(Mm(25).EMU())),
		`<wp:positionV relativeFrom="page"><wp:posOffset>` + itoa(int(Mm(60).EMU())),
		`<wp:positionH relativeFrom="margin">`,
		`<wp:positionV relativeFrom="paragraph">`,
		`<wp:wrapSquare wrapText="bothSides"/>`,
		`<wp:wrapTopAndBottom/>`,
		`behindDoc="1"`,
		`<a:spAutoFit/>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document.xml lacks %s", want)
		}
	}

	// A header holding only a frame still has a paragraph for it to hang on,
	// and its drawing id does not repeat the body's.
	hdr := partOf(t, data, "word/header1.xml")
	if !strings.Contains(hdr, "Only a frame") || strings.Count(hdr, "<w:p>") != 2 {
		t.Errorf("header box not hosted on an empty paragraph:\n%s", hdr)
	}
	if strings.Contains(hdr, `<wp:docPr id="1"`) {
		t.Error("header text box reuses a body drawing id")
	}
}
