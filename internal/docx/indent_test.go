package docx

import (
	"strings"
	"testing"
)

func TestExplicitZeroIndentOverridesStyle(t *testing.T) {
	d := &Document{
		Styles: []Style{{ID: "Item", Type: StyleParagraph, Para: ParaProps{Indent: Indent{Left: Mm(10), Hanging: Mm(10)}}}},
		Body: []Block{Paragraph{
			Props: ParaProps{Style: "Item", Indent: Indent{ExplicitLeft: true, ExplicitHanging: true}},
			Runs:  []Run{R("Continuation")},
		}},
	}
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	body := partOf(t, data, "word/document.xml")
	if !strings.Contains(body, `<w:ind w:left="0" w:hanging="0"/>`) {
		t.Errorf("explicit zero indentation omitted: %s", body)
	}
}
