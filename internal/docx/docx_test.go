package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

// sample builds a document exercising every construct the package supports, so
// one round-trip test covers styles, numbering, tables, frames and headers.
func sample() *Document {
	num := Numbering{}
	listID := num.AddList(DecimalList(3))
	bulletID := num.AddList(BulletList(2))

	return &Document{
		Properties: Properties{Title: "Test", Creator: "docc"},
		Section: Section{
			Page: A4,
			Margins: Margins{
				Top: Mm(20), Bottom: Mm(20), Left: Mm(26), Right: Mm(15),
				Header: Mm(10), Footer: Mm(10),
			},
			TitlePage: true,
		},
		Defaults: Defaults{
			Run: RunProps{Font: "Arial", Size: FontPt(11)},
		},
		Styles: []Style{
			{ID: "Standard", Name: "Standard", Type: StyleParagraph, Default: true},
			{
				ID: "Heading1", Name: "heading 1", Type: StyleParagraph,
				BasedOn: "Standard", Next: "Standard", QFormat: true,
				Run:  RunProps{Bold: ToggleOn, Size: FontPt(14)},
				Para: ParaProps{Spacing: Spacing{Before: Pt(12), After: Pt(6)}, KeepNext: true},
			},
		},
		Numbering: num,
		Headers: []HeaderFooter{
			{Type: HFFirst, Blocks: []Block{P("Standard", "Letterhead")}},
		},
		Footers: []HeaderFooter{
			{Type: HFDefault, Blocks: []Block{P("Standard", "Page footer")}},
		},
		Body: []Block{
			Paragraph{
				Props: ParaProps{
					Frame: &FramePr{
						Width: Mm(85), Height: Mm(40),
						X: Mm(120), Y: Mm(45),
						HAnchor: "page", VAnchor: "page",
					},
				},
				Runs: []Run{R("Beispiel GmbH"), {Items: []Inline{Break{}}}, R("3000 Bern")},
			},
			P("Heading1", "RECHTSBEGEHREN"),
			Paragraph{
				Props: ParaProps{Numbering: &NumRef{ID: listID, Level: 0}},
				Runs:  []Run{R("Erstes Begehren")},
			},
			Paragraph{
				Props: ParaProps{Numbering: &NumRef{ID: listID, Level: 0}},
				Runs:  []Run{R("Zweites Begehren")},
			},
			Paragraph{
				Props: ParaProps{Numbering: &NumRef{ID: bulletID, Level: 0}},
				Runs:  []Run{R("Ein Aufzählungspunkt")},
			},
			Paragraph{
				Props: ParaProps{Align: AlignJustify},
				Runs: []Run{
					R("Normal, "),
					RB("fett"),
					R(" und "),
					{Props: RunProps{Italic: ToggleOn}, Items: []Inline{Text("kursiv")}},
					R("."),
				},
			},
			Table{
				Widths: []Twips{Mm(40), Mm(60)},
				Borders: &TableBorders{
					Top:     &Border{Style: BorderSingle, Size: BorderPt(0.5)},
					Bottom:  &Border{Style: BorderSingle, Size: BorderPt(0.5)},
					InsideH: &Border{Style: BorderSingle, Size: BorderPt(0.5)},
				},
				Rows: []TableRow{
					{Header: true, Cells: []TableCell{Cell("Standard", "Position"), Cell("Standard", "Betrag")}},
					{Cells: []TableCell{Cell("Standard", "Werklohn"), Cell("Standard", "CHF 42'000.00")}},
					{Cells: []TableCell{{Span: 2, Blocks: []Block{P("Standard", "Über zwei Spalten")}}}},
				},
			},
		},
	}
}

func partOf(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rc.Close() }()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Fatalf("part %s not found", name)
	return ""
}

func TestRequiredPartsPresent(t *testing.T) {
	data, err := sample().Bytes()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, f := range zr.File {
		have[f.Name] = true
	}

	// Every part Word needs to open the file at all.
	for _, name := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/_rels/document.xml.rels",
		"word/styles.xml",
		"word/settings.xml",
		"word/numbering.xml",
		"word/header1.xml",
		"word/footer1.xml",
		"docProps/core.xml",
		"docProps/app.xml",
	} {
		if !have[name] {
			t.Errorf("missing part %s", name)
		}
	}
}

// The body must end with sectPr; Word treats a body without it as damaged.
func TestBodyEndsWithSectPr(t *testing.T) {
	data, _ := sample().Bytes()
	doc := partOf(t, data, "word/document.xml")
	if !strings.Contains(doc, "<w:sectPr>") {
		t.Fatal("document.xml has no sectPr")
	}
	if !strings.HasSuffix(strings.TrimSpace(doc), "</w:sectPr></w:body></w:document>") {
		t.Errorf("sectPr is not the last body element:\n...%s", tail(doc, 120))
	}
}

func TestParagraphSectionBreakStartsNewSection(t *testing.T) {
	doc := sample()
	doc.Body = []Block{
		Paragraph{
			Props: ParaProps{SectionBreak: &Section{
				Page:     A4,
				Margins:  Margins{Top: Mm(20), Bottom: Mm(20), Left: Mm(25), Right: Mm(25)},
				NextPage: true,
			}},
			Runs: []Run{R("Cover")},
		},
		P("Standard", "Body"),
	}
	doc.Section = Section{
		Page:    A4,
		Margins: Margins{Top: Mm(20), Bottom: Mm(25), Left: Mm(32), Right: Mm(32)},
	}

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	xml := partOf(t, data, "word/document.xml")
	if got := strings.Count(xml, "<w:sectPr>"); got != 2 {
		t.Errorf("got %d section properties, want 2:\n%s", got, xml)
	}
	if !strings.Contains(xml, `<w:type w:val="nextPage"/>`) {
		t.Errorf("section break is not a next-page break:\n%s", xml)
	}
	if !strings.Contains(xml, `w:left="1814"`) || !strings.Contains(xml, `w:bottom="1417"`) {
		t.Errorf("continuation margins missing from final section:\n%s", xml)
	}
}

// Header and footer references in sectPr must match relationship ids that
// actually exist, or Word silently drops the letterhead.
func TestHeaderFooterRelationshipsResolve(t *testing.T) {
	data, _ := sample().Bytes()
	doc := partOf(t, data, "word/document.xml")
	rels := partOf(t, data, "word/_rels/document.xml.rels")

	for _, want := range []string{"headerReference", "footerReference"} {
		if !strings.Contains(doc, want) {
			t.Errorf("sectPr has no %s", want)
		}
	}
	for _, target := range []string{"header1.xml", "footer1.xml", "styles.xml", "numbering.xml"} {
		if !strings.Contains(rels, `Target="`+target+`"`) {
			t.Errorf("rels missing target %s", target)
		}
	}
}

// Two lists created with AddList must not share a numId, or the second
// continues the first's numbering instead of restarting.
func TestListsGetDistinctNumIDs(t *testing.T) {
	n := Numbering{}
	first := n.AddList(DecimalList(1))
	second := n.AddList(DecimalList(1))
	if first == second {
		t.Fatalf("both lists got numId %d", first)
	}
	if first == 0 || second == 0 {
		t.Error("numId 0 means 'no numbering' and must never be issued")
	}

	third := n.NewInstance(n.Abstract[0].ID)
	if third == first || third == second {
		t.Errorf("NewInstance reused numId %d", third)
	}
	if got := n.Instances[len(n.Instances)-1].StartOverride; got != 1 {
		t.Errorf("NewInstance start override = %d, want 1", got)
	}
}

// A marginal number is a label with its own size, alignment and separator,
// none of which follow the paragraph it labels.
func TestNumLevelLabelProperties(t *testing.T) {
	n := Numbering{}
	n.AddList(AbstractNum{Levels: []NumLevel{{
		Level:          0,
		Format:         NumDecimal,
		Text:           "%1.",
		Size:           FontPt(8),
		Align:          TabRight,
		Suffix:         "space",
		Hanging:        Mm(7),
		ParagraphStyle: "Standard",
	}}})
	d := &Document{Numbering: n, Body: []Block{P("Standard", "x")}}

	got := string(d.writeNumbering())
	for _, want := range []string{
		`<w:numFmt w:val="decimal"/>`,
		`<w:lvlText w:val="%1."/>`,
		`<w:suff w:val="space"/>`,
		`<w:lvlJc w:val="right"/>`,
		`<w:sz w:val="16"/>`,   // 8pt is 16 half-points
		`<w:szCs w:val="16"/>`, // complex-script size, or Word ignores it in places
		`<w:pStyle w:val="Standard"/>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

// A level with no label formatting must not emit an empty run-properties
// element: Word treats <w:rPr/> as a formatting override of nothing.
func TestNumLevelWithoutLabelFormattingHasNoRunProps(t *testing.T) {
	n := Numbering{}
	n.AddList(AbstractNum{Levels: []NumLevel{{Level: 0, Format: NumDecimal, Text: "%1."}}})
	d := &Document{Numbering: n}
	if got := string(d.writeNumbering()); strings.Contains(got, "<w:rPr>") {
		t.Errorf("unexpected w:rPr:\n%s", got)
	}
}

// A numbered paragraph refers to its level through w:numPr; the label itself is
// never written into the text.
func TestParagraphNumberingReference(t *testing.T) {
	d := &Document{Body: []Block{Paragraph{
		Props: ParaProps{Style: "Ueberschrift2", Numbering: &NumRef{ID: 3, Level: 1}},
		Runs:  []Run{{Items: []Inline{Text("Zuständigkeit")}}},
	}}}
	got := string(d.writeDocument())
	for _, want := range []string{
		`<w:ilvl w:val="1"/>`,
		`<w:numId w:val="3"/>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

// Output must be reproducible for golden tests over the archive to mean
// anything.
func TestOutputIsDeterministic(t *testing.T) {
	first, err := sample().Bytes()
	if err != nil {
		t.Fatal(err)
	}
	second, err := sample().Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two builds of the same document produced different bytes")
	}
}

func TestTextIsEscaped(t *testing.T) {
	d := &Document{Body: []Block{P("Standard", `Müller & Co <AG> "quoted"`)}}
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, data, "word/document.xml")
	if !strings.Contains(doc, "M&#252;ller &amp; Co &lt;AG&gt;") && !strings.Contains(doc, "Müller &amp; Co &lt;AG&gt;") {
		t.Errorf("text not escaped correctly:\n%s", doc)
	}
	if strings.Contains(doc, "<AG>") {
		t.Error("unescaped angle bracket reached the output")
	}
}

// A run's spaces carry meaning when a sentence is split across runs, so w:t
// must preserve them.
func TestSpacePreserved(t *testing.T) {
	d := &Document{Body: []Block{Paragraph{Runs: []Run{R("a "), RB("b")}}}}
	data, _ := d.Bytes()
	doc := partOf(t, data, "word/document.xml")
	if !strings.Contains(doc, `xml:space="preserve"`) {
		t.Error("w:t lacks xml:space=preserve")
	}
}

// An empty cell is invalid; it must be padded with an empty paragraph.
func TestEmptyCellGetsParagraph(t *testing.T) {
	d := &Document{Body: []Block{
		Table{Widths: []Twips{Mm(50)}, Rows: []TableRow{{Cells: []TableCell{{}}}}},
	}}
	data, _ := d.Bytes()
	doc := partOf(t, data, "word/document.xml")
	if !strings.Contains(doc, "</w:tcPr><w:p></w:p></w:tc>") {
		t.Errorf("empty cell has no paragraph:\n%s", doc)
	}
}

// Relationships are scoped to their part in OPC: a header that embeds an
// image must carry its own .rels resolving the r:embed id, or Word and
// LibreOffice silently drop the picture.
func TestHeaderImageGetsPartRels(t *testing.T) {
	d := &Document{Section: Section{Page: A4}}
	png := []byte("\x89PNG\r\n\x1a\nfake")
	drawing := d.AddImage("crest", png, "png", MmEMU(30), MmEMU(10))
	d.Headers = []HeaderFooter{{
		Type:   HFFirst,
		Blocks: []Block{Paragraph{Runs: []Run{{Items: []Inline{drawing}}}}},
	}}

	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	rels := partOf(t, data, "word/_rels/header1.xml.rels")
	if !strings.Contains(rels, `Id="`+drawing.relID+`"`) {
		t.Errorf("header rels does not resolve %s:\n%s", drawing.relID, rels)
	}
	if !strings.Contains(rels, `Target="media/`) {
		t.Errorf("header rels has no media target:\n%s", rels)
	}
}

// The same image bytes used twice must be stored once.
func TestImagesDeduplicate(t *testing.T) {
	d := &Document{}
	png := []byte("\x89PNG\r\n\x1a\nfake")
	first := d.AddImage("logo", png, "png", MmEMU(30), MmEMU(10))
	second := d.AddImage("logo again", png, "png", MmEMU(30), MmEMU(10))

	if len(d.media) != 1 {
		t.Errorf("stored %d media files, want 1", len(d.media))
	}
	if first.relID != second.relID {
		t.Errorf("identical images got different rel ids: %s vs %s", first.relID, second.relID)
	}
}

// Unbalanced XML would reach Word as a repair prompt rather than an error, so
// the writer must fail loudly at construction instead.
func TestUnbalancedElementsPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for an unclosed element")
		}
	}()
	w := &xw{}
	w.open("w:p")
	_ = w.bytes()
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
