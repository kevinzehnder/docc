package emit

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/theme"
	"github.com/kevinzehnder/docc/pkg/docx"
)

func testTheme() *theme.Theme {
	return &theme.Theme{
		Name: "test",
		Styles: map[string]theme.Style{
			"Standard":      {Name: "Standard", Default: true},
			"Ueberschrift1": {Name: "heading 1", BasedOn: "Standard", Bold: true},
			"Listenabsatz":  {Name: "List Paragraph", BasedOn: "Standard"},
			"Beweismittel":  {Name: "Beweismittel", BasedOn: "Standard", Italic: true},
		},
		Numbering: map[string]theme.NumFormat{
			"Nummerierung": {Format: "decimal", Text: "%1.", Style: "Listenabsatz"},
		},
	}
}

func testSchema() *schema.Schema {
	return &schema.Schema{
		Type: "test",
		Styles: map[string]string{
			"h1":           "Ueberschrift1",
			"paragraph":    "Standard",
			"ordered_list": "Nummerierung",
			"div.beweis":   "Beweismittel",
		},
	}
}

func build(t *testing.T, source string) *docx.Document {
	t.Helper()
	f, ds := parse.Parse("t.md", []byte(source))
	if ds.HasErrors() {
		t.Fatalf("parse: %+v", ds)
	}
	doc := ir.Build(f, "test", map[string]any{})
	built, err := Build(doc, testSchema(), testTheme(), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return built
}

func xml(t *testing.T, d *docx.Document) string {
	t.Helper()
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return partOf(t, data, "word/document.xml")
}

// Validate is the check that removes a whole class of silent failure: Word
// renders a paragraph naming an unknown style as body text without complaint.
func TestValidateCatchesUnknownStyle(t *testing.T) {
	sc := testSchema()
	sc.Styles["h2"] = "Ueberschrift2" // not defined by the theme

	err := Validate(sc, testTheme())
	if err == nil {
		t.Fatal("expected an error for a style the theme does not define")
	}
	if !strings.Contains(err.Error(), "Ueberschrift2") {
		t.Errorf("error should name the missing style, got: %v", err)
	}
	// The message must also say what is available, or the author is left guessing.
	if !strings.Contains(err.Error(), "Ueberschrift1") {
		t.Errorf("error should list the defined styles, got: %v", err)
	}
}

func TestValidateAcceptsMappingToListDefinition(t *testing.T) {
	if err := Validate(testSchema(), testTheme()); err != nil {
		t.Fatalf("ordered_list -> Nummerierung should be valid: %v", err)
	}
}

// Two consecutive lists must not share a numId, or the second continues the
// first's numbering: a Klageschrift's second Rechtsbegehren list would start
// at 3.
func TestConsecutiveListsRestart(t *testing.T) {
	d := build(t, "---\nx: 1\n---\n\n1. one\n2. two\n\nBetween.\n\n1. three\n4. four\n")

	if len(d.Numbering.Instances) < 2 {
		t.Fatalf("got %d numbering instances, want at least 2", len(d.Numbering.Instances))
	}
	seen := map[int]bool{}
	for _, inst := range d.Numbering.Instances {
		if seen[inst.ID] {
			t.Errorf("numId %d issued twice", inst.ID)
		}
		seen[inst.ID] = true
	}

	// Both lists should share one abstract definition; only the instance differs.
	if len(d.Numbering.Abstract) != 1 {
		t.Errorf("got %d abstract definitions, want 1 shared", len(d.Numbering.Abstract))
	}
}

func TestHeadingUsesMappedStyle(t *testing.T) {
	doc := xml(t, build(t, "---\nx: 1\n---\n\n# Rechtsbegehren\n"))
	if !strings.Contains(doc, `<w:pStyle w:val="Ueberschrift1"/>`) {
		t.Errorf("heading did not get the mapped style:\n%s", doc)
	}
}

func TestDivUsesMappedStyle(t *testing.T) {
	doc := xml(t, build(t, "---\nx: 1\n---\n\n::: beweis\n- Vertrag // Beilage 1\n:::\n"))
	if !strings.Contains(doc, `<w:pStyle w:val="Beweismittel"/>`) {
		t.Errorf("div content did not get the mapped style:\n%s", doc)
	}
}

// Inline emphasis must survive as run properties rather than as literal
// asterisks.
func TestInlineEmphasisBecomesRunProps(t *testing.T) {
	doc := xml(t, build(t, "---\nx: 1\n---\n\nNormal **fett** und *kursiv*.\n"))
	if !strings.Contains(doc, "<w:b/>") {
		t.Error("bold text produced no w:b")
	}
	if !strings.Contains(doc, "<w:i/>") {
		t.Error("italic text produced no w:i")
	}
	if strings.Contains(doc, "**") {
		t.Error("markdown syntax leaked into the output")
	}
}

// Furniture interpolates metadata; a line whose fields are all empty is
// dropped rather than left blank.
func TestFurnitureDropsEmptyLines(t *testing.T) {
	th := testTheme()
	th.Prologue = []theme.Line{
		{Style: "Standard", Text: "{{ recipient.name }}"},
		{Style: "Standard", Text: "{{ recipient.organization }}"},
	}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{
		"recipient": map[string]any{"name": "Hans Beispiel", "organization": ""},
	})

	built, err := Build(doc, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := xml(t, built)

	if !strings.Contains(out, "Hans Beispiel") {
		t.Error("a filled furniture line was dropped")
	}
	// Prologue plus one body paragraph: the empty organisation line is gone.
	if got := countParagraphs(out); got != 2 {
		t.Errorf("got %d paragraphs, want 2 (the empty line should be dropped)", got)
	}
}

// A run-based line loses the whole paragraph when its only field is empty: a
// dangling "vertreten durch" with no name is worse than nothing.
func TestRunLineDropsWhenOnlyLiteralsRemain(t *testing.T) {
	th := testTheme()
	th.Prologue = []theme.Line{{
		Style: "Standard",
		Runs: []theme.LineRun{
			{Text: "vertreten durch "},
			{Text: "{{ vertreter }}"},
		},
	}}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{"vertreter": ""})

	built, err := Build(doc, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out := xml(t, built); strings.Contains(out, "vertreten durch") {
		t.Errorf("a label survived without the value it introduces:\n%s", out)
	}
}

func TestRepeatEmitsOnePerItem(t *testing.T) {
	th := testTheme()
	th.Epilogue = []theme.Line{{Style: "Standard", Text: "– {{ item }}", Repeat: "beilagen"}}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{
		"beilagen": []any{"Vertrag", "Protokoll", "Rechnung"},
	})

	built, err := Build(doc, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := xml(t, built)
	for _, want := range []string{"Vertrag", "Protokoll", "Rechnung"} {
		if !strings.Contains(out, want) {
			t.Errorf("repeat dropped %q", want)
		}
	}
}

func TestRepeatOverMissingListEmitsNothing(t *testing.T) {
	th := testTheme()
	th.Epilogue = []theme.Line{{Style: "Standard", Text: "– {{ item }}", Repeat: "beilagen"}}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{})

	built, err := Build(doc, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := countParagraphs(xml(t, built)); got != 1 {
		t.Errorf("got %d paragraphs, want 1 (the body only)", got)
	}
}

func countParagraphs(doc string) int {
	return strings.Count(doc, "<w:p>") + strings.Count(doc, "<w:p/>")
}
