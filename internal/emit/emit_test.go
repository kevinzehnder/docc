package emit

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/docx"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/theme"
)

func testTheme() *theme.Theme {
	return &theme.Theme{
		Name: "test",
		Styles: map[string]theme.Style{
			"Standard":      {Name: "Standard", Default: true},
			"Ueberschrift1": {Name: "heading 1", BasedOn: "Standard", Bold: ptrTo(true)},
			"Listenabsatz":  {Name: "List Paragraph", BasedOn: "Standard"},
			"Evidence":      {Name: "Evidence", BasedOn: "Standard", Italic: ptrTo(true)},
			"EvidenceLabel": {Name: "Evidence Label", Type: "character"},
			"Formularzeile": {Name: "Formularzeile", BasedOn: "Standard"},
			"Feldname":      {Name: "Feldname", Type: "character"},
			"Firma":         {Name: "Firma", Type: "character", Bold: ptrTo(true)},
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
			"h1":                 "Ueberschrift1",
			"paragraph":          "Standard",
			"ordered_list":       "Nummerierung",
			"div.evidence":       "Evidence",
			"div.evidence.label": "EvidenceLabel",
			"div.feld":           "Formularzeile",
			"div.feld.field":     "Feldname",
			"span.firma":         "Firma",
		},
		Spans: map[string]schema.SpanSpec{"firma": {}},
		// The furniture tests below interpolate these. Validate rejects a theme
		// that names a field the schema does not declare, so the test schema has
		// to declare them for the same reason a real one does.
		Frontmatter: schema.Fields{
			"recipient":   {Type: "recipient"},
			"vertreter":   {Type: "string"},
			"attachments": {Type: "list<string>"},
		},
		Types: map[string]schema.Fields{
			"recipient": {
				"name":         {Type: "string"},
				"organization": {Type: "string"},
			},
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

// Two pattern keys on one block is a contradiction, not a combination: emit.div
// takes the first it finds and the other renders nothing. It validated, it
// rendered, and it did nothing — so the author blamed the theme.
func TestValidateRejectsTwoBlockPatterns(t *testing.T) {
	sc := testSchema()
	sc.Styles["div.evidence.field"] = "Feldname" // already selects .label

	err := Validate(sc, testTheme())
	if err == nil {
		t.Fatal("expected an error for a block selecting two rendering patterns")
	}
	for _, want := range []string{"div.evidence.label", "div.evidence.field"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %s, got: %v", want, err)
		}
	}
}

// One pattern key plus its dependent keys is the normal case and must stay
// valid: `.total` and `.words` decorate amount rendering, they do not select
// a second pattern.
func TestValidateAcceptsAmountBlockWithTotalAndWords(t *testing.T) {
	sc := testSchema()
	sc.Styles["div.betraege"] = "Standard"
	sc.Styles["div.betraege.amount"] = "Firma"
	sc.Styles["div.betraege.total"] = "Standard"
	sc.Styles["div.betraege.total.amount"] = "Firma"
	sc.Styles["div.betraege.words"] = "Standard"

	if err := Validate(sc, testTheme()); err != nil {
		t.Fatalf("an amount block with its total and words keys should be valid: %v", err)
	}
}

func TestValidateAcceptsMappingToListDefinition(t *testing.T) {
	if err := Validate(testSchema(), testTheme()); err != nil {
		t.Fatalf("ordered_list -> Nummerierung should be valid: %v", err)
	}
}

// A placeholder naming a field the schema does not declare expands to nothing,
// and a furniture line whose fields are all empty is dropped — so a typo in an
// address block silently posts a letter with no city on it. This is the check
// that turns that into a build failure.
func TestValidateCatchesUnknownField(t *testing.T) {
	th := testTheme()
	th.Prologue = []theme.Line{
		{Style: "Standard", Text: "{{ recipient.name }}"},
		{Style: "Standard", Text: "{{ recipent.city }}"}, // typo
	}

	err := Validate(testSchema(), th)
	if err == nil {
		t.Fatal("expected an error for a field the schema does not declare")
	}
	if !strings.Contains(err.Error(), "recipent.city") {
		t.Errorf("error should name the bad path, got: %v", err)
	}
	if strings.Contains(err.Error(), "{{ recipient.name }}") {
		t.Errorf("the correct path should not be reported, got: %v", err)
	}
}

// A path may only descend into a field whose type is a declared object. Walking
// into a string is a theme bug, not an empty value.
func TestValidateCatchesDescentIntoScalar(t *testing.T) {
	th := testTheme()
	th.Prologue = []theme.Line{{Style: "Standard", Text: "{{ vertreter.name }}"}}

	err := Validate(testSchema(), th)
	if err == nil {
		t.Fatal("expected an error for a member of a scalar field")
	}
	if !strings.Contains(err.Error(), `field "vertreter" is a string`) {
		t.Errorf("error should say why the descent failed, got: %v", err)
	}
}

// The name a repeat binds exists only for one iteration and has no declaration;
// what must be declared is the list the repeat names.
func TestValidateChecksRepeatFieldNotItem(t *testing.T) {
	th := testTheme()
	th.Epilogue = []theme.Line{{Style: "Standard", Text: "– {{ item }}", Repeat: "attachments"}}
	if err := Validate(testSchema(), th); err != nil {
		t.Fatalf("`item` inside a repeat should not need declaring: %v", err)
	}

	th.Epilogue = []theme.Line{{Style: "Standard", Text: "– {{ item }}", Repeat: "enclosures"}}
	err := Validate(testSchema(), th)
	if err == nil {
		t.Fatal("expected an error for a repeat over an undeclared list")
	}
	if !strings.Contains(err.Error(), "enclosures") {
		t.Errorf("error should name the repeated field, got: %v", err)
	}
}

// A `date` field arrives from YAML as a string, so without the schema-driven
// conversion a theme's `formats.date` is dead configuration and the letterhead
// prints an ISO date under a theme that asked for a written-out one.
func TestDateFieldsRenderThroughThemeFormat(t *testing.T) {
	sc := testSchema()
	sc.Frontmatter["sent"] = schema.Field{Type: "date"}
	sc.Frontmatter["deadlines"] = schema.Field{Type: "list<date>"}
	sc.Types["recipient"]["founded"] = schema.Field{Type: "date"}

	th := testTheme()
	th.Formats = theme.Formats{Date: "2 January 2006", ListSeparator: "; "}
	th.Prologue = []theme.Line{
		{Style: "Standard", Text: "{{ sent }}"},
		{Style: "Standard", Text: "{{ recipient.founded }}"},
		{Style: "Standard", Text: "{{ deadlines }}"},
	}

	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{
		"sent":      "2026-08-04",
		"recipient": map[string]any{"founded": "1998-03-11"},
		"deadlines": []any{"2026-09-01", "2026-10-15"},
	})

	built, err := Build(doc, sc, th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := xml(t, built)

	for _, want := range []string{
		"4 August 2026",                     // top level
		"11 March 1998",                     // inside a declared object type
		"1 September 2026; 15 October 2026", // inside a list<date>
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "2026-08-04") {
		t.Error("the raw ISO string reached the document")
	}
}

// A date that does not parse is left as written: sema has already reported it,
// and --force still has to produce a file.
func TestUnparsableDatePassesThrough(t *testing.T) {
	sc := testSchema()
	sc.Frontmatter["sent"] = schema.Field{Type: "date"}

	th := testTheme()
	th.Formats = theme.Formats{Date: "2 January 2006"}
	th.Prologue = []theme.Line{{Style: "Standard", Text: "{{ sent }}"}}

	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{"sent": "04.08.2026"})

	built, err := Build(doc, sc, th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out := xml(t, built); !strings.Contains(out, "04.08.2026") {
		t.Errorf("a malformed date should survive verbatim:\n%s", out)
	}
}

// Runs carry their own placeholders and Text is ignored when they are present,
// so a validator that only reads Text misses every party line in a brief.
func TestValidateReadsRunPlaceholders(t *testing.T) {
	th := testTheme()
	th.Prologue = []theme.Line{{
		Style: "Standard",
		Runs: []theme.LineRun{
			{Text: "vertreten durch "},
			{Text: "{{ vertretr }}"}, // typo
		},
	}}

	err := Validate(testSchema(), th)
	if err == nil {
		t.Fatal("expected an error for a bad path inside a run")
	}
	if !strings.Contains(err.Error(), "vertretr") {
		t.Errorf("error should name the bad path, got: %v", err)
	}
}

// numberedParagraphs pairs each paragraph's numbering reference with its text,
// in document order, so a test can assert on what carries which instance.
func numberedParagraphs(t *testing.T, d *docx.Document) []struct {
	NumID, Level int
	Text         string
} {
	t.Helper()
	var out []struct {
		NumID, Level int
		Text         string
	}
	for _, blk := range d.Body {
		p, isPara := blk.(docx.Paragraph)
		if !isPara {
			continue
		}
		var text strings.Builder
		for _, r := range p.Runs {
			for _, item := range r.Items {
				if s, isText := item.(docx.Text); isText {
					text.WriteString(string(s))
				}
			}
		}
		entry := struct {
			NumID, Level int
			Text         string
		}{Text: text.String()}
		if p.Props.Numbering != nil {
			entry.NumID = p.Props.Numbering.ID
			entry.Level = p.Props.Numbering.Level
		}
		out = append(out, entry)
	}
	return out
}

// renderSchema is testSchema with an outline over the headings and a marginal
// number on prose, both starting at a marker heading.
func renderSchema() *schema.Schema {
	sc := testSchema()
	sc.Styles["h2"] = "Ueberschrift2"
	sc.Styles["h3"] = "Ueberschrift3"
	sc.Render = schema.Render{
		HeadingNumbering: &schema.NumberingRule{
			Definition: "Outline", StartAtHeading: "START",
		},
		ParagraphNumbering: &schema.NumberingRule{
			Definition: "Margin", StartAfterHeading: "START",
		},
	}
	return sc
}

func renderTheme() *theme.Theme {
	th := testTheme()
	th.Styles["Ueberschrift2"] = theme.Style{Name: "heading 2", BasedOn: "Standard"}
	th.Styles["Ueberschrift3"] = theme.Style{Name: "heading 3", BasedOn: "Standard"}
	th.Numbering["Outline"] = theme.NumFormat{
		Format: "upperRoman", Text: "%1.", Style: "Ueberschrift1",
		Levels: []theme.NumFormat{
			{Format: "upperLetter", Text: "%2.", Style: "Ueberschrift2"},
			{Format: "decimal", Text: "%3.", Style: "Ueberschrift3"},
		},
	}
	th.Numbering["Margin"] = theme.NumFormat{Format: "decimal", Text: "%1.", Style: "Standard"}
	return th
}

func buildRendered(t *testing.T, source string) *docx.Document {
	t.Helper()
	f, ds := parse.Parse("t.md", []byte(source))
	if ds.HasErrors() {
		t.Fatalf("parse: %+v", ds)
	}
	built, err := Build(ir.Build(f, "test", map[string]any{}), renderSchema(), renderTheme(), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return built
}

// Every heading in the outline shares one numbering instance and takes its
// level from the markdown depth. A fresh instance per heading would restart the
// count, so every top-level section would be I.
func TestHeadingNumberingSharesOneInstance(t *testing.T) {
	got := numberedParagraphs(t, buildRendered(t,
		"---\nx: 1\n---\n\n# START\n\n# Second\n\n## Sub\n\n### Deep\n\n# Third\n"))

	var numID int
	for _, p := range got {
		if p.NumID == 0 {
			t.Fatalf("heading %q got no numbering", p.Text)
		}
		if numID == 0 {
			numID = p.NumID
		}
		if p.NumID != numID {
			t.Errorf("heading %q uses numId %d, want the shared %d", p.Text, p.NumID, numID)
		}
	}
	wantLevels := map[string]int{"START": 0, "Second": 0, "Sub": 1, "Deep": 2, "Third": 0}
	for _, p := range got {
		if want, known := wantLevels[p.Text]; known && p.Level != want {
			t.Errorf("heading %q at level %d, want %d", p.Text, p.Level, want)
		}
	}
}

// The marker bounds the outline. `start_at_heading` numbers the marker itself;
// anything above it is introductory content that must stay unnumbered.
func TestHeadingNumberingStartsAtMarker(t *testing.T) {
	got := numberedParagraphs(t, buildRendered(t,
		"---\nx: 1\n---\n\n# Vorbemerkung\n\n# START\n\n# After\n"))

	byText := map[string]int{}
	for _, p := range got {
		byText[p.Text] = p.NumID
	}
	if byText["Vorbemerkung"] != 0 {
		t.Errorf("a heading before the marker was numbered (numId %d)", byText["Vorbemerkung"])
	}
	if byText["START"] == 0 {
		t.Error("start_at_heading must number the marker heading itself")
	}
	if byText["After"] != byText["START"] {
		t.Error("headings after the marker must share the marker's instance")
	}
}

// `start_after_heading` is the other half: the marker heading is not itself
// numbered, and the count begins with the prose below it.
// An annex may retain a heading style after a deed outline ends, but it must
// not become the next Roman-numbered section.
func TestHeadingNumberingEndsBeforeMarker(t *testing.T) {
	sc := renderSchema()
	sc.Render.HeadingNumbering.EndBeforeHeading = "Anmeldung"
	f, ds := parse.Parse("t.md", []byte("---\nx: 1\n---\n\n# START\n\n# Before annex\n\n# Anmeldung\n\n# Annex tail\n"))
	if ds.HasErrors() {
		t.Fatalf("parse: %+v", ds)
	}
	built, err := Build(ir.Build(f, "test", map[string]any{}), sc, renderTheme(), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	byText := map[string]int{}
	for _, p := range numberedParagraphs(t, built) {
		byText[p.Text] = p.NumID
	}
	if byText["START"] == 0 || byText["Before annex"] == 0 {
		t.Fatal("headings before the end marker must be numbered")
	}
	if byText["Anmeldung"] != 0 || byText["Annex tail"] != 0 {
		t.Errorf("headings after end_before_heading were numbered: %#v", byText)
	}
}

func TestParagraphNumberingStartsAfterMarker(t *testing.T) {
	got := numberedParagraphs(t, buildRendered(t,
		"---\nx: 1\n---\n\nBefore the marker.\n\n# START\n\nFirst.\n\n# Section\n\nSecond.\n"))

	byText := map[string]struct{ id, lvl int }{}
	for _, p := range got {
		byText[p.Text] = struct{ id, lvl int }{p.NumID, p.Level}
	}
	if byText["Before the marker."].id != 0 {
		t.Error("prose before the marker was numbered")
	}
	first, second := byText["First."], byText["Second."]
	if first.id == 0 || second.id == 0 {
		t.Fatalf("prose after the marker was not numbered: %+v", byText)
	}
	// One instance, so the count continues across the heading between them.
	if first.id != second.id {
		t.Errorf("prose paragraphs got instances %d and %d; the count would restart", first.id, second.id)
	}
	if first.lvl != 0 || second.lvl != 0 {
		t.Error("marginal numbers are a single level")
	}
	// The heading has its own instance, which must not be the prose one.
	if byText["Section"].id == first.id {
		t.Error("headings and prose share an instance; each would advance the other")
	}
}

// Structures that already carry their own labels must not acquire a second one.
// A Rechtsbegehren item and a Beweismittel entry are both paragraphs the
// emitter reaches, and neither is body prose.
func TestParagraphNumberingSkipsNestedContent(t *testing.T) {
	got := numberedParagraphs(t, buildRendered(t, "---\nx: 1\n---\n"+`
# START

Prose.

1. list item
2. another item

::: evidence
- evidence entry
:::

> quoted prose

`+"```\ncode line\n```\n"))

	var proseID int
	for _, p := range got {
		if p.Text == "Prose." {
			proseID = p.NumID
		}
	}
	if proseID == 0 {
		t.Fatal("body prose was not numbered")
	}
	for _, p := range got {
		if p.Text == "Prose." {
			continue
		}
		if p.NumID == proseID {
			t.Errorf("%q acquired a marginal number", p.Text)
		}
	}
}

// A schema that configures no render numbering must produce exactly what it did
// before the feature existed.
func TestNoRenderNumberingLeavesDocumentAlone(t *testing.T) {
	d := build(t, "---\nx: 1\n---\n\n# Heading\n\nProse.\n")
	for _, p := range numberedParagraphs(t, d) {
		if p.NumID != 0 {
			t.Errorf("%q was numbered without a render rule", p.Text)
		}
	}
}

func TestValidateCatchesUnknownRenderDefinition(t *testing.T) {
	sc := renderSchema()
	sc.Render.HeadingNumbering.Definition = "Nonexistent"

	err := Validate(sc, renderTheme())
	if err == nil {
		t.Fatal("expected an error for a definition the theme does not define")
	}
	if !strings.Contains(err.Error(), "Nonexistent") {
		t.Errorf("error should name the definition, got: %v", err)
	}
}

// Setting both start keys has no obvious precedence, so it is rejected rather
// than resolved.
func TestValidateRejectsTwoStartMarkers(t *testing.T) {
	sc := renderSchema()
	sc.Render.ParagraphNumbering.StartAtHeading = "START"

	err := Validate(sc, renderTheme())
	if err == nil || !strings.Contains(err.Error(), "start_at_heading") {
		t.Fatalf("expected a both-markers error, got: %v", err)
	}
}

func TestValidateCatchesBadNumberingSuffix(t *testing.T) {
	th := renderTheme()
	def := th.Numbering["Margin"]
	def.Suffix = "comma"
	th.Numbering["Margin"] = def

	err := Validate(renderSchema(), th)
	if err == nil || !strings.Contains(err.Error(), `unknown suffix "comma"`) {
		t.Fatalf("expected a suffix error, got: %v", err)
	}
}

// A misspelt `restart:` would otherwise be a level that quietly keeps Word's
// default — the same silence the value exists to escape.
func TestValidateCatchesBadNumberingRestart(t *testing.T) {
	th := renderTheme()
	def := th.Numbering["Margin"]
	def.Restart = "sometimes"
	th.Numbering["Margin"] = def

	err := Validate(renderSchema(), th)
	if err == nil || !strings.Contains(err.Error(), `unknown restart "sometimes"`) {
		t.Fatalf("expected a restart error, got: %v", err)
	}
}

// A nested `levels:` is an author expecting a tree. Silently flattening it
// would renumber their outline; saying so costs one line to fix.
func TestValidateCatchesNestedLevels(t *testing.T) {
	th := renderTheme()
	def := th.Numbering["Outline"]
	def.Levels[0].Levels = []theme.NumFormat{{Format: "decimal"}}
	th.Numbering["Outline"] = def

	err := Validate(renderSchema(), th)
	if err == nil || !strings.Contains(err.Error(), "flat list") {
		t.Fatalf("expected a nested-levels error, got: %v", err)
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

func TestLabelledDivMovesEvidenceLabelToRightTab(t *testing.T) {
	doc := build(t, "---\nx: 1\n---\n\n::: evidence\n- [Exhibit 1] Contract\n:::\n")
	if len(doc.Body) != 1 {
		t.Fatalf("got %d body blocks, want 1", len(doc.Body))
	}
	p, ok := doc.Body[0].(docx.Paragraph)
	if !ok {
		t.Fatalf("body block is %T, want paragraph", doc.Body[0])
	}
	if p.Props.Style != "Evidence" {
		t.Errorf("paragraph style = %q, want Evidence", p.Props.Style)
	}
	if p.Props.Numbering == nil {
		t.Fatal("labelled evidence lost its bullet")
	}
	if got := runText(p.Runs[0]); got != "Contract" {
		t.Errorf("description = %q, want Contract", got)
	}
	if len(p.Runs) != 3 {
		t.Fatalf("got %d runs, want description + tab + label", len(p.Runs))
	}
	if _, ok := p.Runs[1].Items[0].(docx.Tab); !ok {
		t.Errorf("middle run = %T, want tab", p.Runs[1].Items[0])
	}
	if p.Runs[2].Props.Style != "EvidenceLabel" {
		t.Errorf("label style = %q, want EvidenceLabel", p.Runs[2].Props.Style)
	}
	if got := runText(p.Runs[2]); got != "Exhibit 1" {
		t.Errorf("label = %q, want Exhibit 1", got)
	}
}

// The form pattern: label left, value right, and no list marker. It is the
// labelled pattern with the emission order reversed, which is what a registry
// form needs and what furniture could only fake.
func TestFieldDivPutsLabelBeforeValue(t *testing.T) {
	doc := build(t, "---\nx: 1\n---\n\n::: feld\n- [Firma:] [Fake AI]{.firma} GmbH\n:::\n")
	if len(doc.Body) != 1 {
		t.Fatalf("got %d body blocks, want 1", len(doc.Body))
	}
	p, ok := doc.Body[0].(docx.Paragraph)
	if !ok {
		t.Fatalf("body block is %T, want paragraph", doc.Body[0])
	}
	if p.Props.Style != "Formularzeile" {
		t.Errorf("paragraph style = %q, want Formularzeile", p.Props.Style)
	}
	// A form row is not a bulleted list; its columns are tab stops.
	if p.Props.Numbering != nil {
		t.Errorf("form row got list numbering: %+v", p.Props.Numbering)
	}
	if len(p.Runs) < 4 {
		t.Fatalf("got %d runs, want tab + label + tab + value", len(p.Runs))
	}
	if _, ok := p.Runs[0].Items[0].(docx.Tab); !ok {
		t.Errorf("first run = %T, want the tab into the label column", p.Runs[0].Items[0])
	}
	if p.Runs[1].Props.Style != "Feldname" {
		t.Errorf("label style = %q, want Feldname", p.Runs[1].Props.Style)
	}
	if got := runText(p.Runs[1]); got != "Firma:" {
		t.Errorf("label = %q, want Firma:", got)
	}
	if _, ok := p.Runs[2].Items[0].(docx.Tab); !ok {
		t.Errorf("third run = %T, want the tab into the value column", p.Runs[2].Items[0])
	}

	// The point of reaching this from the body rather than from furniture: the
	// spans the checks anchor on survive into the output.
	var value, styled string
	for _, r := range p.Runs[3:] {
		value += runText(r)
		if r.Props.Style == "Firma" {
			styled += runText(r)
		}
	}
	if strings.TrimSpace(value) != "Fake AI GmbH" {
		t.Errorf("value = %q, want Fake AI GmbH", value)
	}
	if styled != "Fake AI" {
		t.Errorf("span run = %q, want the .firma span styled as Firma", styled)
	}
}

func runText(r docx.Run) string {
	var out strings.Builder
	for _, item := range r.Items {
		if text, ok := item.(docx.Text); ok {
			out.WriteString(string(text))
		}
	}
	return out.String()
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
	th.Epilogue = []theme.Line{{Style: "Standard", Text: "– {{ item }}", Repeat: "attachments"}}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{
		"attachments": []any{"Vertrag", "Protokoll", "Rechnung"},
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

func TestFurnitureIfNonemptySkipsEmptyEnclosureSection(t *testing.T) {
	th := testTheme()
	th.Epilogue = []theme.Line{
		{Style: "Standard", Text: "Beilagen", IfNonempty: "attachments", PageBreak: true},
		{Style: "Standard", Text: "{{ item }}", Repeat: "attachments"},
	}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))

	empty := ir.Build(f, "test", map[string]any{"attachments": []any{}})
	built, err := Build(empty, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out := xml(t, built); strings.Contains(out, "Beilagen") {
		t.Errorf("empty enclosure list emitted its heading:\n%s", out)
	}

	filled := ir.Build(f, "test", map[string]any{"attachments": []any{"Vertrag"}})
	built, err = Build(filled, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := xml(t, built)
	for _, want := range []string{"Beilagen", "Vertrag"} {
		if !strings.Contains(out, want) {
			t.Errorf("populated enclosure list omitted %q:\n%s", want, out)
		}
	}
}

// An enclosures index is a repeat that has to come out numbered 1., 2., 3. —
// one shared instance across the repeated lines, so adding an entry renumbers
// the rest instead of restarting at 1.
func TestFurnitureNumberingSharesOneInstance(t *testing.T) {
	th := testTheme()
	th.Numbering["Index"] = theme.NumFormat{Format: "decimal", Text: "%1.", Style: "Standard"}
	th.Epilogue = []theme.Line{
		{Style: "Standard", Text: "{{ item }}", Repeat: "attachments", Numbering: "Index"},
	}
	f, _ := parse.Parse("t.md", []byte("---\nx: 1\n---\n\nBody.\n"))
	doc := ir.Build(f, "test", map[string]any{
		"attachments": []any{"Vertrag", "Protokoll", "Rechnung"},
	})

	built, err := Build(doc, testSchema(), th, Options{})
	if err != nil {
		t.Fatal(err)
	}

	var ids []int
	for _, p := range numberedParagraphs(t, built) {
		if p.NumID != 0 {
			ids = append(ids, p.NumID)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("got %d numbered entries, want 3", len(ids))
	}
	for _, id := range ids[1:] {
		if id != ids[0] {
			t.Errorf("index entries got instances %v; the count would restart", ids)
		}
	}
}

// A furniture line naming a definition the theme does not have would render
// without its number and say nothing about it.
func TestValidateCatchesUnknownFurnitureNumbering(t *testing.T) {
	th := testTheme()
	th.Epilogue = []theme.Line{
		{Style: "Standard", Text: "{{ item }}", Repeat: "attachments", Numbering: "Nonexistent"},
	}
	err := Validate(testSchema(), th)
	if err == nil || !strings.Contains(err.Error(), "Nonexistent") {
		t.Fatalf("expected an unknown-numbering error, got: %v", err)
	}
}

func TestRepeatOverMissingListEmitsNothing(t *testing.T) {
	th := testTheme()
	th.Epilogue = []theme.Line{{Style: "Standard", Text: "– {{ item }}", Repeat: "attachments"}}
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

// A block's style reaches every paragraph inside it, including a heading. That
// is the documented behaviour of the plain rendering pattern, and it has a
// consequence worth pinning: a heading written inside a block stops being a
// heading. It loses its heading style, and the render pass that numbers headings
// does not see it.
//
// Neither is an error, and nothing in the source looks wrong, so the reference
// documentation tells authors to keep headings between blocks rather than in
// them. This test is what keeps that advice true.
func TestHeadingInsideDivIsNotAHeading(t *testing.T) {
	sc := testSchema()
	sc.Render.HeadingNumbering = &schema.NumberingRule{Definition: "Nummerierung"}

	f, ds := parse.Parse("t.md", []byte("---\nx: 1\n---\n\n# Outside\n\n::: evidence\n## Inside\n:::\n\n# After\n"))
	if ds.HasErrors() {
		t.Fatalf("parse: %+v", ds)
	}
	built, err := Build(ir.Build(f, "test", map[string]any{}), sc, testTheme(), Options{})
	if err != nil {
		t.Fatal(err)
	}

	var inside docx.Paragraph
	var found bool
	for _, blk := range built.Body {
		p, isPara := blk.(docx.Paragraph)
		if !isPara {
			continue
		}
		if text(p) == "Inside" {
			inside, found = p, true
		}
	}
	if !found {
		t.Fatal("the heading inside the block produced no paragraph")
	}
	if inside.Props.Style != "Evidence" {
		t.Errorf("style = %q, want Evidence — the block styles every paragraph in it", inside.Props.Style)
	}
	if inside.Props.Numbering != nil {
		t.Error("a heading inside a block should not pick up heading numbering")
	}
}

// text joins a paragraph's runs, for assertions.
func text(p docx.Paragraph) string {
	var b strings.Builder
	for _, r := range p.Runs {
		for _, item := range r.Items {
			if t, ok := item.(docx.Text); ok {
				b.WriteString(string(t))
			}
		}
	}
	return b.String()
}

// ptrTo is for the theme's tri-state toggles, where absent, on and explicitly
// off are three different things.
func ptrTo[T any](v T) *T { return &v }
