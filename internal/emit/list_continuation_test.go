package emit

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/kevinzehnder/docc/internal/docx"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/theme"
)

func buildListContinuationFixture(t *testing.T, source string) *docx.Document {
	t.Helper()
	var th theme.Theme
	if err := yaml.Unmarshal([]byte(`name: list-test
styles:
  Standard: { default: true }
  Item:
    based_on: Standard
    indent: { left: 10mm, hanging: 10mm }
  Bullet: { based_on: Standard }
numbering:
  Ordered:
    format: decimal
    text: "%1."
    style: Item
    indent: 10mm
    hanging: 10mm
    levels:
      - { format: lowerLetter, text: "%2.", style: Item, indent: 20mm, hanging: 10mm }
  Bullets:
    format: bullet
    text: "–"
    style: Bullet
    indent: 7mm
    hanging: 7mm
    levels:
      - { format: bullet, text: "–", style: Bullet, indent: 14mm, hanging: 7mm }
`), &th); err != nil {
		t.Fatal(err)
	}
	sc := testSchema()
	sc.Styles = map[string]string{"paragraph": "Standard", "ordered_list": "Ordered", "bullet_list": "Bullets"}
	f, ds := parse.Parse("list.md", []byte("---\ndocc: 1\n---\n\n"+source))
	if ds.HasErrors() {
		t.Fatalf("parse: %+v", ds)
	}
	built, err := Build(ir.Build(f, "test", map[string]any{}), sc, &th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func listParagraphs(doc *docx.Document) map[string]docx.Paragraph {
	out := map[string]docx.Paragraph{}
	for _, block := range doc.Body {
		if p, ok := block.(docx.Paragraph); ok {
			out[text(p)] = p
		}
	}
	return out
}

func assertListContinuation(t *testing.T, p docx.Paragraph, style string, indent docx.Twips) {
	t.Helper()
	if p.Props.Numbering == nil || p.Props.Numbering.ID != 0 {
		t.Errorf("continuation must explicitly disable numbering: %+v", p.Props.Numbering)
	}
	if p.Props.Style != style {
		t.Errorf("style = %q, want %q", p.Props.Style, style)
	}
	if p.Props.Indent.Left != indent || p.Props.Indent.Hanging != 0 || p.Props.Indent.FirstLine != 0 {
		t.Errorf("indent = %+v, want text aligned at %v with no hanging first line", p.Props.Indent, indent)
	}
	if !p.Props.Indent.ExplicitLeft || !p.Props.Indent.ExplicitHanging {
		t.Error("continuation must override inherited indentation, including zero")
	}
}

func TestListItemContinuationParagraphs(t *testing.T) {
	doc := buildListContinuationFixture(t, `1. First item.
2. Parent item.

   First continuation.

   Second continuation.

3. Following item.

Outside the list.
`)
	ps := listParagraphs(doc)
	for _, text := range []string{"First continuation.", "Second continuation."} {
		assertListContinuation(t, ps[text], "Item", docx.Mm(10))
	}
	id := ps["Parent item."].Props.Numbering.ID
	if id == 0 || ps["Following item."].Props.Numbering.ID != id {
		t.Error("continuations must not split or restart the parent list")
	}
	outside := ps["Outside the list."]
	if outside.Props.Style != "Standard" || outside.Props.Numbering != nil || outside.Props.Indent != (docx.Indent{}) {
		t.Errorf("list formatting leaked into outside prose: %+v", outside.Props)
	}
	body := xml(t, doc)
	if !strings.Contains(body, `<w:ind w:left="567" w:hanging="0"/>`) {
		t.Error("continuation does not cancel the inherited hanging indent in XML")
	}
}

func TestNestedListContinuationParagraphs(t *testing.T) {
	doc := buildListContinuationFixture(t, `1. Parent item.

   1. Nested item.

      Nested continuation.

   Parent continuation.

2. Following item.
`)
	ps := listParagraphs(doc)
	assertListContinuation(t, ps["Nested continuation."], "Item", docx.Mm(20))
	assertListContinuation(t, ps["Parent continuation."], "Item", docx.Mm(10))
	parent := ps["Parent item."].Props.Numbering
	nested := ps["Nested item."].Props.Numbering
	if nested == nil || parent == nil || nested.ID != parent.ID || nested.Level != 1 {
		t.Errorf("same-kind nested list must retain the parent instance at level 1: %+v / %+v", parent, nested)
	}
}

func TestBulletsInsideOrderedListUseBulletDefinition(t *testing.T) {
	doc := buildListContinuationFixture(t, `1. Parent item.

   - First bullet.

     Bullet continuation.

   - Second bullet.

   Parent continuation.

2. Following item.
`)
	ps := listParagraphs(doc)
	parent := ps["Parent item."].Props.Numbering
	bullet := ps["First bullet."].Props.Numbering
	if parent == nil || bullet == nil || parent.ID == bullet.ID {
		t.Fatal("nested bullets must not use the parent's decimal numbering instance")
	}
	var abstractID int
	for _, instance := range doc.Numbering.Instances {
		if instance.ID == bullet.ID {
			abstractID = instance.AbstractID
		}
	}
	found := false
	for _, def := range doc.Numbering.Abstract {
		if def.ID == abstractID {
			for _, level := range def.Levels {
				if level.Level == bullet.Level && level.Format == docx.NumBullet {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("nested bullet references no actual bullet level")
	}
	assertListContinuation(t, ps["Bullet continuation."], "Bullet", docx.Mm(14))
	assertListContinuation(t, ps["Parent continuation."], "Item", docx.Mm(10))
	if ps["Following item."].Props.Numbering.ID != parent.ID {
		t.Error("nested bullets changed the outer list's numbering instance")
	}
}
