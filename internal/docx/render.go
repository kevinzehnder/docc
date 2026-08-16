package docx

import "strconv"

func itoa(n int) string { return strconv.Itoa(n) }

// nsAttrs are the namespace declarations every WordprocessingML part carries.
// Word accepts a part declaring only what it uses, but declaring the same set
// everywhere keeps the parts uniform and diffable.
func nsAttrs() []attr {
	return []attr{
		a("xmlns:w", "http://schemas.openxmlformats.org/wordprocessingml/2006/main"),
		a("xmlns:r", "http://schemas.openxmlformats.org/officeDocument/2006/relationships"),
		a("xmlns:wp", "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"),
		a("xmlns:a", "http://schemas.openxmlformats.org/drawingml/2006/main"),
		a("xmlns:pic", "http://schemas.openxmlformats.org/drawingml/2006/picture"),
	}
}

// writeDocument renders the word/document.xml part.
func (d *Document) writeDocument() []byte {
	w := &xw{}
	w.header()
	w.open("w:document", nsAttrs()...)
	w.open("w:body")

	for _, b := range d.Body {
		b.writeBlock(w, d)
	}

	// The section properties close the body. Word treats a body without them as
	// damaged, so this is written whether or not geometry was configured.
	d.writeSectPr(w)

	w.close("w:body")
	w.close("w:document")
	return w.bytes()
}

func (d *Document) writeSectPr(w *xw) {
	d.writeSectionPr(w, d.Section)
}

// writeSectionPr writes the properties for either the final document section
// or a section break carried by a paragraph.
func (d *Document) writeSectionPr(w *xw, section Section) {
	w.open("w:sectPr")
	if section.NextPage {
		w.empty("w:type", a("w:val", "nextPage"))
	}

	for _, h := range d.Headers {
		w.empty("w:headerReference", a("w:type", string(h.hfType())), a("r:id", h.relID))
	}
	for _, f := range d.Footers {
		w.empty("w:footerReference", a("w:type", string(f.hfType())), a("r:id", f.relID))
	}

	page := section.Page
	if page.Width == 0 || page.Height == 0 {
		page = A4
	}
	pageAttrs := []attr{ai("w:w", page.Width), ai("w:h", page.Height)}
	if page.Landscape {
		pageAttrs = []attr{ai("w:w", page.Height), ai("w:h", page.Width), a("w:orient", "landscape")}
	}
	w.empty("w:pgSz", pageAttrs...)

	m := section.Margins
	w.empty("w:pgMar",
		ai("w:top", m.Top),
		ai("w:right", m.Right),
		ai("w:bottom", m.Bottom),
		ai("w:left", m.Left),
		ai("w:header", m.Header),
		ai("w:footer", m.Footer),
		ai("w:gutter", m.Gutter),
	)

	if section.Cols > 1 {
		w.empty("w:cols", ai("w:num", section.Cols))
	} else {
		w.empty("w:cols", ai("w:space", Twips(708)))
	}

	if section.TitlePage {
		w.empty("w:titlePg")
	}
	w.empty("w:docGrid", ai("w:linePitch", 360))

	w.close("w:sectPr")
}

func (h HeaderFooter) hfType() HeaderFooterType {
	if h.Type == "" {
		return HFDefault
	}
	return h.Type
}

// ---------------------------------------------------------------------------
// Paragraph
// ---------------------------------------------------------------------------

func (p Paragraph) write(w *xw, d *Document) {
	w.open("w:p")
	writeParaProps(w, p.Props, true, d)
	for _, r := range p.Runs {
		r.write(w, d)
	}
	w.close("w:p")
}

// writeParaProps emits w:pPr. inBody distinguishes a body paragraph, where an
// empty w:pPr is pointless noise, from a style definition, where it is invalid.
func writeParaProps(w *xw, p ParaProps, inBody bool, d *Document) {
	if p.isZero() {
		if !inBody {
			// Style definitions need the element present even when empty.
			w.empty("w:pPr")
		}
		return
	}

	w.open("w:pPr")

	if p.Style != "" {
		w.empty("w:pStyle", a("w:val", p.Style))
	}
	if p.KeepNext {
		w.empty("w:keepNext")
	}
	if p.KeepLines {
		w.empty("w:keepLines")
	}
	if p.PageBreak {
		w.empty("w:pageBreakBefore")
	}
	if p.Frame != nil {
		writeFramePr(w, p.Frame)
	}
	if p.Numbering != nil {
		w.open("w:numPr")
		w.empty("w:ilvl", ai("w:val", p.Numbering.Level))
		w.empty("w:numId", ai("w:val", p.Numbering.ID))
		w.close("w:numPr")
	}
	if p.Borders != nil {
		writeParaBorders(w, p.Borders)
	}
	if p.Shading != "" {
		w.empty("w:shd", a("w:val", "clear"), a("w:color", "auto"), a("w:fill", p.Shading))
	}
	if len(p.Tabs) > 0 {
		w.open("w:tabs")
		for _, t := range p.Tabs {
			al := t.Align
			if al == "" {
				al = TabLeft
			}
			attrs := []attr{a("w:val", string(al)), ai("w:pos", t.Pos)}
			if t.Leader != "" {
				attrs = append(attrs, a("w:leader", t.Leader))
			}
			w.empty("w:tab", attrs...)
		}
		w.close("w:tabs")
	}
	if p.ContextualSpacing {
		w.empty("w:contextualSpacing")
	}
	if sp := p.Spacing; sp != (Spacing{}) {
		attrs := []attr{}
		if sp.Before != 0 || sp.ExplicitBefore {
			attrs = append(attrs, ai("w:before", sp.Before))
		}
		if sp.After != 0 || sp.ExplicitAfter {
			attrs = append(attrs, ai("w:after", sp.After))
		}
		if sp.Line != 0 {
			rule := sp.LineRule
			if rule == "" {
				rule = LineAuto
			}
			attrs = append(attrs, ai("w:line", sp.Line), a("w:lineRule", string(rule)))
		}
		if len(attrs) > 0 {
			w.empty("w:spacing", attrs...)
		}
	}
	if ind := p.Indent; ind != (Indent{}) {
		attrs := []attr{}
		if ind.Left != 0 {
			attrs = append(attrs, ai("w:left", ind.Left))
		}
		if ind.Right != 0 {
			attrs = append(attrs, ai("w:right", ind.Right))
		}
		// Hanging and firstLine are mutually exclusive in the format; emitting
		// both makes Word pick one unpredictably.
		if ind.Hanging != 0 {
			attrs = append(attrs, ai("w:hanging", ind.Hanging))
		} else if ind.FirstLine != 0 {
			attrs = append(attrs, ai("w:firstLine", ind.FirstLine))
		}
		if len(attrs) > 0 {
			w.empty("w:ind", attrs...)
		}
	}
	if p.Align != "" {
		w.empty("w:jc", a("w:val", string(p.Align)))
	}
	if p.OutlineLevel != nil {
		w.empty("w:outlineLvl", ai("w:val", *p.OutlineLevel))
	}
	if p.SectionBreak != nil && d != nil {
		d.writeSectionPr(w, *p.SectionBreak)
	}

	w.close("w:pPr")
}

func (p ParaProps) isZero() bool {
	return p.Style == "" && p.Align == "" && p.Spacing == (Spacing{}) &&
		p.Indent == (Indent{}) && p.Frame == nil && p.Numbering == nil &&
		p.SectionBreak == nil && len(p.Tabs) == 0 && p.Borders == nil && !p.KeepNext && !p.KeepLines &&
		!p.PageBreak && !p.ContextualSpacing && p.Shading == "" && p.OutlineLevel == nil
}

func writeFramePr(w *xw, f *FramePr) {
	attrs := []attr{}
	if f.Width != 0 {
		attrs = append(attrs, ai("w:w", f.Width))
	}
	if f.Height != 0 {
		attrs = append(attrs, ai("w:h", f.Height), a("w:hRule", "exact"))
	}
	hAnchor := f.HAnchor
	if hAnchor == "" {
		hAnchor = "page"
	}
	vAnchor := f.VAnchor
	if vAnchor == "" {
		vAnchor = "page"
	}
	wrap := f.Wrap
	if wrap == "" {
		wrap = "around"
	}
	attrs = append(attrs,
		a("w:hAnchor", hAnchor),
		a("w:vAnchor", vAnchor),
		ai("w:x", f.X),
		ai("w:y", f.Y),
		a("w:wrap", wrap),
	)
	w.empty("w:framePr", attrs...)
}

// writeToggle emits a toggle property. Explicitly off is written as
// `w:val="0"` rather than omitted, because omitting it inherits from the
// based-on style instead of overriding it.
func writeToggle(w *xw, name string, t Toggle) {
	switch t {
	case ToggleOn:
		w.empty(name)
	case ToggleOff:
		w.empty(name, a("w:val", "0"))
	case ToggleInherit:
	}
}

func writeParaBorders(w *xw, b *ParaBorders) {
	w.open("w:pBdr")
	writeBorder(w, "w:top", b.Top)
	writeBorder(w, "w:left", b.Left)
	writeBorder(w, "w:bottom", b.Bottom)
	writeBorder(w, "w:right", b.Right)
	w.close("w:pBdr")
}

func writeBorder(w *xw, name string, b *Border) {
	if b == nil {
		return
	}
	style := b.Style
	if style == "" {
		style = BorderSingle
	}
	color := b.Color
	if color == "" {
		color = "auto"
	}
	w.empty(name,
		a("w:val", string(style)),
		ai("w:sz", b.Size),
		ai("w:space", b.Space),
		a("w:color", color),
	)
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func (r Run) write(w *xw, d *Document) {
	w.open("w:r")
	writeRunProps(w, r.Props, true)
	for _, item := range r.Items {
		item.writeInline(w, d)
	}
	w.close("w:r")
}

func writeRunProps(w *xw, r RunProps, inBody bool) {
	if r.isZero() {
		if !inBody {
			w.empty("w:rPr")
		}
		return
	}

	w.open("w:rPr")
	if r.Style != "" {
		w.empty("w:rStyle", a("w:val", r.Style))
	}
	if r.Font != "" {
		w.empty("w:rFonts", a("w:ascii", r.Font), a("w:hAnsi", r.Font), a("w:cs", r.Font))
	}
	writeToggle(w, "w:b", r.Bold)
	writeToggle(w, "w:bCs", r.Bold)
	writeToggle(w, "w:i", r.Italic)
	writeToggle(w, "w:iCs", r.Italic)
	writeToggle(w, "w:caps", r.Caps)
	writeToggle(w, "w:smallCaps", r.SmallCaps)
	writeToggle(w, "w:strike", r.Strike)
	if r.Color != "" {
		w.empty("w:color", a("w:val", r.Color))
	}
	if r.Spacing != 0 {
		w.empty("w:spacing", ai("w:val", r.Spacing))
	}
	if r.Size != 0 {
		w.empty("w:sz", ai("w:val", r.Size))
		w.empty("w:szCs", ai("w:val", r.Size))
	}
	if r.Highlight != "" {
		w.empty("w:highlight", a("w:val", r.Highlight))
	}
	if r.Underline != "" {
		w.empty("w:u", a("w:val", r.Underline))
	}
	if r.VertAlign != "" && r.VertAlign != VertBaseline {
		w.empty("w:vertAlign", a("w:val", string(r.VertAlign)))
	}
	if r.Lang != "" {
		w.empty("w:lang", a("w:val", r.Lang))
	}
	w.close("w:rPr")
}

func (r RunProps) isZero() bool {
	return r == RunProps{}
}

func (t Text) writeInline(w *xw, _ *Document) {
	// xml:space="preserve" is what stops Word collapsing the leading and
	// trailing spaces that separate runs within a sentence.
	w.open("w:t", a("xml:space", "preserve"))
	w.text(string(t))
	w.close("w:t")
}

func (Tab) writeInline(w *xw, _ *Document) { w.empty("w:tab") }

func (b Break) writeInline(w *xw, _ *Document) {
	if b.Type == "" || b.Type == BreakLine {
		w.empty("w:br")
		return
	}
	w.empty("w:br", a("w:type", string(b.Type)))
}

func (dr *Drawing) writeInline(w *xw, _ *Document) {
	w.open("w:drawing")
	w.open("wp:inline", ai("distT", 0), ai("distB", 0), ai("distL", 0), ai("distR", 0))
	w.empty("wp:extent", ai("cx", dr.Width), ai("cy", dr.Height))
	w.empty("wp:effectExtent", ai("l", 0), ai("t", 0), ai("r", 0), ai("b", 0))
	w.empty("wp:docPr", ai("id", dr.docPr), a("name", dr.Name), a("descr", dr.AltText))

	w.open("wp:cNvGraphicFramePr")
	w.empty("a:graphicFrameLocks",
		a("xmlns:a", "http://schemas.openxmlformats.org/drawingml/2006/main"),
		a("noChangeAspect", "1"),
	)
	w.close("wp:cNvGraphicFramePr")

	w.open("a:graphic", a("xmlns:a", "http://schemas.openxmlformats.org/drawingml/2006/main"))
	w.open("a:graphicData", a("uri", "http://schemas.openxmlformats.org/drawingml/2006/picture"))
	w.open("pic:pic", a("xmlns:pic", "http://schemas.openxmlformats.org/drawingml/2006/picture"))

	w.open("pic:nvPicPr")
	w.empty("pic:cNvPr", ai("id", dr.docPr), a("name", dr.Name), a("descr", dr.AltText))
	w.open("pic:cNvPicPr")
	w.empty("a:picLocks", a("noChangeAspect", "1"), a("noChangeArrowheads", "1"))
	w.close("pic:cNvPicPr")
	w.close("pic:nvPicPr")

	w.open("pic:blipFill")
	w.empty("a:blip",
		a("xmlns:r", "http://schemas.openxmlformats.org/officeDocument/2006/relationships"),
		a("r:embed", dr.relID),
	)
	w.open("a:stretch")
	w.empty("a:fillRect")
	w.close("a:stretch")
	w.close("pic:blipFill")

	w.open("pic:spPr")
	w.open("a:xfrm")
	w.empty("a:off", ai("x", 0), ai("y", 0))
	w.empty("a:ext", ai("cx", dr.Width), ai("cy", dr.Height))
	w.close("a:xfrm")
	w.open("a:prstGeom", a("prst", "rect"))
	w.empty("a:avLst")
	w.close("a:prstGeom")
	w.close("pic:spPr")

	w.close("pic:pic")
	w.close("a:graphicData")
	w.close("a:graphic")
	w.close("wp:inline")
	w.close("w:drawing")
}

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

func (t Table) write(w *xw, d *Document) {
	w.open("w:tbl")

	w.open("w:tblPr")
	if t.Style != "" {
		w.empty("w:tblStyle", a("w:val", t.Style))
	}
	total := Twips(0)
	for _, cw := range t.Widths {
		total += cw
	}
	w.empty("w:tblW", ai("w:w", total), a("w:type", "dxa"))
	if t.Indent != 0 {
		w.empty("w:tblInd", ai("w:w", t.Indent), a("w:type", "dxa"))
	}
	if t.Borders != nil {
		writeTableBorders(w, "w:tblBorders", t.Borders)
	}
	// Fixed layout: auto-fit resolves differently in Word and LibreOffice, and
	// a letter whose address block moves between renderers is not acceptable.
	w.empty("w:tblLayout", a("w:type", "fixed"))
	w.close("w:tblPr")

	w.open("w:tblGrid")
	for _, cw := range t.Widths {
		w.empty("w:gridCol", ai("w:w", cw))
	}
	w.close("w:tblGrid")

	for _, row := range t.Rows {
		row.write(w, d, t.Widths)
	}

	w.close("w:tbl")
}

func (r TableRow) write(w *xw, d *Document, widths []Twips) {
	w.open("w:tr")

	if r.Height != 0 || r.Header || r.CantSplit {
		w.open("w:trPr")
		if r.CantSplit {
			w.empty("w:cantSplit")
		}
		if r.Height != 0 {
			w.empty("w:trHeight", ai("w:val", r.Height), a("w:hRule", "atLeast"))
		}
		if r.Header {
			w.empty("w:tblHeader")
		}
		w.close("w:trPr")
	}

	col := 0
	for _, c := range r.Cells {
		span := c.Span
		if span < 1 {
			span = 1
		}
		width := Twips(0)
		for i := col; i < col+span && i < len(widths); i++ {
			width += widths[i]
		}
		col += span
		c.write(w, d, width, span)
	}

	w.close("w:tr")
}

func (c TableCell) write(w *xw, d *Document, width Twips, span int) {
	w.open("w:tc")

	w.open("w:tcPr")
	w.empty("w:tcW", ai("w:w", width), a("w:type", "dxa"))
	if span > 1 {
		w.empty("w:gridSpan", ai("w:val", span))
	}
	if c.Borders != nil {
		writeTableBorders(w, "w:tcBorders", c.Borders)
	}
	if c.Shading != "" {
		w.empty("w:shd", a("w:val", "clear"), a("w:color", "auto"), a("w:fill", c.Shading))
	}
	if c.Margins != nil {
		w.open("w:tcMar")
		w.empty("w:top", ai("w:w", c.Margins.Top), a("w:type", "dxa"))
		w.empty("w:left", ai("w:w", c.Margins.Left), a("w:type", "dxa"))
		w.empty("w:bottom", ai("w:w", c.Margins.Bottom), a("w:type", "dxa"))
		w.empty("w:right", ai("w:w", c.Margins.Right), a("w:type", "dxa"))
		w.close("w:tcMar")
	}
	if c.VAlign != "" {
		w.empty("w:vAlign", a("w:val", string(c.VAlign)))
	}
	w.close("w:tcPr")

	// Word rejects a cell whose content is empty, so an empty cell gets an
	// empty paragraph rather than nothing.
	if len(c.Blocks) == 0 {
		Paragraph{}.write(w, d)
	}
	for _, b := range c.Blocks {
		b.writeBlock(w, d)
	}

	w.close("w:tc")
}

func writeTableBorders(w *xw, name string, b *TableBorders) {
	w.open(name)
	writeBorder(w, "w:top", b.Top)
	writeBorder(w, "w:left", b.Left)
	writeBorder(w, "w:bottom", b.Bottom)
	writeBorder(w, "w:right", b.Right)
	writeBorder(w, "w:insideH", b.InsideH)
	writeBorder(w, "w:insideV", b.InsideV)
	w.close(name)
}

// ---------------------------------------------------------------------------
// Headers and footers
// ---------------------------------------------------------------------------

func (d *Document) writeHeaderFooter(hf HeaderFooter, root string) []byte {
	w := &xw{}
	w.header()
	w.open(root, nsAttrs()...)
	// A header part with no paragraph is invalid, the same as a table cell.
	if len(hf.Blocks) == 0 {
		Paragraph{}.write(w, d)
	}
	for _, b := range hf.Blocks {
		b.writeBlock(w, d)
	}
	w.close(root)
	return w.bytes()
}
