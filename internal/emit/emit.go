// Package emit converts a validated document into a Word document, using the
// schema's style map to decide which theme style each markdown construct wears.
//
// The schema says "an ordered list here uses Rechtsbegehren"; the theme says
// what Rechtsbegehren looks like. Neither knows about the other, and this
// package is the only place that needs both.
package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/theme"
	"github.com/kevinzehnder/docc/pkg/docx"
)

// Options configures a build.
type Options struct {
	// ThemeDir is where theme-relative image paths resolve from.
	ThemeDir string
}

// Build renders a document.
func Build(doc *ir.Document, sc *schema.Schema, th *theme.Theme, opts Options) (*docx.Document, error) {
	if err := Validate(sc, th); err != nil {
		return nil, err
	}

	e := &emitter{
		doc:    doc,
		schema: sc,
		theme:  th,
		opts:   opts,
		out: &docx.Document{
			Section: docx.Section{
				Page:      th.Page.PageSize(),
				Margins:   th.Page.Margins.DocxMargins(),
				TitlePage: th.Page.TitlePage,
			},
			Defaults: docx.Defaults{
				Run: docx.RunProps{
					Font: th.Defaults.Font,
					Size: th.Defaults.Size.HalfPt(docx.FontPt(11)),
					Lang: th.Defaults.Lang,
				},
			},
			Styles: th.DocxStyles(),
		},
		abstractByName: map[string]int{},
	}

	e.out.Properties = docx.Properties{
		Title:   metaString(doc.Meta, "title"),
		Subject: metaString(doc.Meta, "subject", "betreff"),
		Creator: metaString(doc.Meta, "sender.name", "author"),
	}

	if err := e.buildHeadersFooters(); err != nil {
		return nil, err
	}
	if err := e.buildFurniture(th.Prologue, &e.out.Body); err != nil {
		return nil, fmt.Errorf("prologue: %w", err)
	}
	e.blocks(doc.Blocks, &e.out.Body, 0)
	if err := e.buildFurniture(th.Epilogue, &e.out.Body); err != nil {
		return nil, fmt.Errorf("epilogue: %w", err)
	}

	e.out.Numbering = e.numbering
	return e.out, nil
}

// Validate reports style-map entries that name a style the theme does not
// define. Word renders an unknown style as body text without complaint, which
// is precisely the silent failure this compiler exists to remove.
func Validate(sc *schema.Schema, th *theme.Theme) error {
	var missing []string
	for key, styleID := range sc.Styles {
		if styleID == "" {
			continue
		}
		if _, ok := th.Styles[styleID]; ok {
			continue
		}
		if _, ok := th.Numbering[styleID]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("%s -> %q", key, styleID))
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	known := make([]string, 0, len(th.Styles))
	for id := range th.Styles {
		known = append(known, id)
	}
	sort.Strings(known)

	return fmt.Errorf(
		"schema %q maps to styles the theme %q does not define:\n  %s\ntheme defines: %s",
		sc.Type, th.Name, strings.Join(missing, "\n  "), strings.Join(known, ", "),
	)
}

type emitter struct {
	doc    *ir.Document
	schema *schema.Schema
	theme  *theme.Theme
	opts   Options
	out    *docx.Document

	numbering docx.Numbering
	// abstractByName caches the abstract definition per theme list name, so
	// every list of a kind shares one definition.
	abstractByName map[string]int
}

// style resolves a style-map key to a style id, falling back through a chain of
// alternatives before giving up.
func (e *emitter) style(keys ...string) string {
	for _, k := range keys {
		if id, ok := e.schema.Styles[k]; ok && id != "" {
			return id
		}
	}
	return ""
}

func (e *emitter) blocks(blocks []ir.Block, out *[]docx.Block, depth int) {
	for _, b := range blocks {
		e.blockTo(b, out, depth)
	}
}

func (e *emitter) blockTo(b ir.Block, out *[]docx.Block, depth int) {
	switch v := b.(type) {
	case ir.Heading:
		style := e.style(fmt.Sprintf("h%d", v.Level), "heading")
		p := docx.Paragraph{
			Props: docx.ParaProps{Style: style},
			Runs:  e.runs(v.Inlines, docx.RunProps{}),
		}
		if style == "" {
			// Without a mapped style the heading would be indistinguishable
			// from body text, so carry the emphasis directly.
			p.Props.KeepNext = true
			for i := range p.Runs {
				p.Runs[i].Props.Bold = true
			}
		}
		*out = append(*out, p)

	case ir.Para:
		*out = append(*out, docx.Paragraph{
			Props: docx.ParaProps{Style: e.style("paragraph")},
			Runs:  e.runs(v.Inlines, docx.RunProps{}),
		})

	case ir.List:
		e.list(v, out, depth, "")

	case ir.Div:
		e.div(v, out, depth)

	case ir.Quote:
		style := e.style("quote", "paragraph")
		var inner []docx.Block
		e.blocks(v.Blocks, &inner, depth)
		for _, blk := range inner {
			if p, ok := blk.(docx.Paragraph); ok {
				if p.Props.Style == "" || p.Props.Style == e.style("paragraph") {
					p.Props.Style = style
				}
				p.Props.Indent.Left += docx.Mm(10)
				*out = append(*out, p)
				continue
			}
			*out = append(*out, blk)
		}

	case ir.Code:
		style := e.style("code", "paragraph")
		// Each source line becomes its own paragraph: a single paragraph with
		// breaks would reflow, and preformatted text must not.
		for _, line := range strings.Split(strings.TrimRight(v.Text, "\n"), "\n") {
			*out = append(*out, docx.Paragraph{
				Props: docx.ParaProps{Style: style},
				Runs:  []docx.Run{{Props: docx.RunProps{Font: "Courier New"}, Items: []docx.Inline{docx.Text(line)}}},
			})
		}

	case ir.Rule:
		*out = append(*out, docx.Paragraph{
			Props: docx.ParaProps{
				Borders: &docx.ParaBorders{
					Bottom: &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
				},
			},
		})

	case ir.Table:
		*out = append(*out, e.table(v, depth))
	}
}

// list emits a list, allocating a fresh numId at the top level so consecutive
// lists restart rather than continuing one another.
func (e *emitter) list(l ir.List, out *[]docx.Block, depth int, numName string) {
	if numName == "" {
		numName = "ordered_list"
		if !l.Ordered {
			numName = "bullet_list"
		}
	}
	numID := e.numID(numName, l)

	for _, item := range l.Items {
		first := true
		for _, b := range item.Blocks {
			// Nested lists keep the parent's numbering instance and step down a
			// level; a fresh instance would restart the sub-numbering.
			if nested, isList := b.(ir.List); isList {
				e.nestedList(nested, out, depth+1, numID)
				continue
			}

			var buf []docx.Block
			e.blockTo(b, &buf, depth)
			for _, blk := range buf {
				p, isPara := blk.(docx.Paragraph)
				if !isPara {
					*out = append(*out, blk)
					continue
				}
				if first {
					p.Props.Numbering = &docx.NumRef{ID: numID, Level: depth}
					p.Props.Style = e.listStyle(numName, p.Props.Style)
					first = false
				}
				*out = append(*out, p)
			}
		}
	}
}

func (e *emitter) nestedList(l ir.List, out *[]docx.Block, depth int, numID int) {
	for _, item := range l.Items {
		first := true
		for _, b := range item.Blocks {
			if nested, isList := b.(ir.List); isList {
				e.nestedList(nested, out, depth+1, numID)
				continue
			}
			var buf []docx.Block
			e.blockTo(b, &buf, depth)
			for _, blk := range buf {
				p, isPara := blk.(docx.Paragraph)
				if !isPara {
					*out = append(*out, blk)
					continue
				}
				if first {
					p.Props.Numbering = &docx.NumRef{ID: numID, Level: depth}
					first = false
				}
				*out = append(*out, p)
			}
		}
	}
}

// listStyle returns the paragraph style for a list, preferring the schema's
// mapping over whatever the block carried.
func (e *emitter) listStyle(numName, fallback string) string {
	if id, ok := e.schema.Styles[numName]; ok && id != "" {
		if _, isStyle := e.theme.Styles[id]; isStyle {
			return id
		}
		// The mapping names a list definition rather than a paragraph style;
		// the definition may still nominate one.
		if def, isNum := e.theme.Numbering[id]; isNum && def.Style != "" {
			return def.Style
		}
	}
	return fallback
}

// numID returns a numbering instance for a list, creating the abstract
// definition on first use and a fresh instance for every top-level list.
func (e *emitter) numID(numName string, l ir.List) int {
	defName := numName
	if id, ok := e.schema.Styles[numName]; ok && id != "" {
		if _, isNum := e.theme.Numbering[id]; isNum {
			defName = id
		}
	}

	abstractID, known := e.abstractByName[defName]
	if !known {
		var def docx.AbstractNum
		if themeDef, ok := e.theme.Numbering[defName]; ok {
			def = themeDef.AbstractNum()
		} else if l.Ordered {
			def = docx.DecimalList(9)
		} else {
			def = docx.BulletList(9)
		}
		def.Name = defName
		numID := e.numbering.AddList(def)
		abstractID = e.numbering.Abstract[len(e.numbering.Abstract)-1].ID
		e.abstractByName[defName] = abstractID
		return numID
	}
	return e.numbering.NewInstance(abstractID)
}

func (e *emitter) div(d ir.Div, out *[]docx.Block, depth int) {
	style := e.style("div."+d.Name, "paragraph")

	var inner []docx.Block
	e.blocks(d.Blocks, &inner, depth)

	for _, blk := range inner {
		p, isPara := blk.(docx.Paragraph)
		if !isPara {
			*out = append(*out, blk)
			continue
		}
		// A div's style applies to its paragraphs unless a list already claimed
		// one, since a Beweismittel entry is styled by the div, not the list.
		if p.Props.Numbering == nil || e.style("div."+d.Name) != "" {
			p.Props.Style = style
		}
		*out = append(*out, p)
	}
}

func (e *emitter) table(t ir.Table, depth int) docx.Table {
	cols := 0
	for _, row := range t.Rows {
		if len(row.Cells) > cols {
			cols = len(row.Cells)
		}
	}
	if cols == 0 {
		cols = 1
	}

	// Distribute the text width evenly: markdown carries no column sizing.
	m := e.theme.Page.Margins.DocxMargins()
	textWidth := e.theme.Page.PageSize().Width - m.Left - m.Right
	widths := make([]docx.Twips, cols)
	for i := range widths {
		widths[i] = textWidth / docx.Twips(cols)
	}

	out := docx.Table{
		Widths: widths,
		Style:  e.style("table"),
		Borders: &docx.TableBorders{
			Top:     &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
			Bottom:  &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
			Left:    &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
			Right:   &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
			InsideH: &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
			InsideV: &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
		},
	}

	for i, row := range t.Rows {
		r := docx.TableRow{Header: t.Header && i == 0}
		for j, cell := range row.Cells {
			var blocks []docx.Block
			e.blocks(cell.Blocks, &blocks, depth)
			c := docx.TableCell{Blocks: blocks, VAlign: docx.VAlignTop}
			if j < len(t.Align) {
				applyCellAlign(&c, t.Align[j])
			}
			if r.Header {
				boldCell(&c)
			}
			r.Cells = append(r.Cells, c)
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}

func applyCellAlign(c *docx.TableCell, align string) {
	if align == "" {
		return
	}
	for i, b := range c.Blocks {
		if p, ok := b.(docx.Paragraph); ok {
			p.Props.Align = docx.Align(align)
			if align == "center" {
				p.Props.Align = docx.AlignCenter
			}
			c.Blocks[i] = p
		}
	}
}

func boldCell(c *docx.TableCell) {
	for i, b := range c.Blocks {
		p, ok := b.(docx.Paragraph)
		if !ok {
			continue
		}
		for j := range p.Runs {
			p.Runs[j].Props.Bold = true
		}
		c.Blocks[i] = p
	}
}

// runs converts inlines, carrying inherited formatting down the tree.
func (e *emitter) runs(inlines []ir.Inline, inherited docx.RunProps) []docx.Run {
	var out []docx.Run
	for _, item := range inlines {
		switch v := item.(type) {
		case ir.Str:
			if v.Text == "" {
				continue
			}
			out = append(out, docx.Run{Props: inherited, Items: []docx.Inline{docx.Text(v.Text)}})

		case ir.Strong:
			props := inherited
			props.Bold = true
			out = append(out, e.runs(v.Inlines, props)...)

		case ir.Emph:
			props := inherited
			props.Italic = true
			out = append(out, e.runs(v.Inlines, props)...)

		case ir.CodeSpan:
			props := inherited
			props.Font = "Courier New"
			out = append(out, docx.Run{Props: props, Items: []docx.Inline{docx.Text(v.Text)}})

		case ir.Link:
			// Rendered as styled text rather than a real hyperlink: a legal
			// document is printed, and a live link adds a relationship for no
			// benefit on paper.
			props := inherited
			props.Color = "0000EE"
			props.Underline = "single"
			out = append(out, e.runs(v.Inlines, props)...)

		case ir.LineBreak:
			out = append(out, docx.Run{Props: inherited, Items: []docx.Inline{docx.Break{}}})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Furniture
// ---------------------------------------------------------------------------

func (e *emitter) buildHeadersFooters() error {
	for _, key := range sortedKeys(e.theme.Header) {
		var blocks []docx.Block
		if err := e.buildFurniture(e.theme.Header[key], &blocks); err != nil {
			return fmt.Errorf("header %q: %w", key, err)
		}
		e.out.Headers = append(e.out.Headers, docx.HeaderFooter{
			Type: docx.HeaderFooterType(key), Blocks: blocks,
		})
	}
	for _, key := range sortedKeys(e.theme.Footer) {
		var blocks []docx.Block
		if err := e.buildFurniture(e.theme.Footer[key], &blocks); err != nil {
			return fmt.Errorf("footer %q: %w", key, err)
		}
		e.out.Footers = append(e.out.Footers, docx.HeaderFooter{
			Type: docx.HeaderFooterType(key), Blocks: blocks,
		})
	}
	return nil
}

// buildFurniture expands the fixed lines of a theme against the document's
// metadata.
func (e *emitter) buildFurniture(lines []theme.Line, out *[]docx.Block) error {
	for _, line := range lines {
		if line.Repeat != "" {
			if err := e.repeatLine(line, out); err != nil {
				return err
			}
			continue
		}
		if err := e.furnitureLine(line, e.doc.Meta, out); err != nil {
			return err
		}
	}
	return nil
}

// repeatLine emits one paragraph per element of a list field. A list that is
// absent or empty emits nothing, so a letter without enclosures has no
// enclosures section.
func (e *emitter) repeatLine(line theme.Line, out *[]docx.Block) error {
	raw, found := lookupMeta(e.doc.Meta, line.Repeat)
	if !found {
		return nil
	}
	items, isList := raw.([]any)
	if !isList || len(items) == 0 {
		return nil
	}
	for _, item := range items {
		scope := map[string]any{"item": item}
		for k, v := range e.doc.Meta {
			if k != "item" {
				scope[k] = v
			}
		}
		if err := e.furnitureLine(line, scope, out); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) furnitureLine(line theme.Line, meta map[string]any, out *[]docx.Block) error {
	if len(line.Runs) > 0 {
		return e.furnitureRunLine(line, meta, out)
	}

	expanded := theme.Expand(line.Text, meta)

	// A line whose every field is empty is dropped: a recipient with no
	// organisation should not leave a blank line in the address block.
	if line.Image == nil && expanded.AllEmpty && line.Omit() {
		return nil
	}

	p := docx.Paragraph{
		Props: docx.ParaProps{
			Style:     line.Style,
			PageBreak: line.PageBreak,
		},
	}
	if line.Frame != nil {
		p.Props.Frame = &docx.FramePr{
			Width:   line.Frame.Width.Twips(0),
			Height:  line.Frame.Height.Twips(0),
			X:       line.Frame.X.Twips(0),
			Y:       line.Frame.Y.Twips(0),
			HAnchor: line.Frame.HAnchor,
			VAnchor: line.Frame.VAnchor,
			Wrap:    line.Frame.Wrap,
		}
	}
	for _, t := range line.Tabs {
		p.Props.Tabs = append(p.Props.Tabs, docx.TabStop{
			Pos:    t.Pos.Twips(0),
			Align:  docx.TabAlign(t.Align),
			Leader: t.Leader,
		})
	}

	if line.Image != nil {
		drawing, err := e.image(*line.Image)
		if err != nil {
			return err
		}
		p.Runs = append(p.Runs, docx.Run{Items: []docx.Inline{drawing}})
	}
	if expanded.Text != "" {
		p.Runs = append(p.Runs, furnitureRuns(expanded.Text)...)
	}

	*out = append(*out, p)
	return nil
}

// furnitureRunLine builds a paragraph whose formatting changes partway through.
//
// A run whose fields are all empty is dropped individually, so a party with no
// street loses that fragment without losing the line — and the tab or break
// that would have preceded it goes with it, rather than leaving a gap.
func (e *emitter) furnitureRunLine(line theme.Line, meta map[string]any, out *[]docx.Block) error {
	p := docx.Paragraph{
		Props: docx.ParaProps{Style: line.Style, PageBreak: line.PageBreak},
	}
	for _, t := range line.Tabs {
		p.Props.Tabs = append(p.Props.Tabs, docx.TabStop{
			Pos:    t.Pos.Twips(0),
			Align:  docx.TabAlign(t.Align),
			Leader: t.Leader,
		})
	}

	// Literal runs — a label such as "vertreten durch" — do not justify a
	// paragraph on their own. Only a run that interpolated a real value counts
	// as content, so a party with no representative loses the whole line rather
	// than keeping a dangling label.
	fieldRuns, filledRuns, anyText := 0, 0, false
	for _, r := range line.Runs {
		expanded := theme.Expand(r.Text, meta)
		if expanded.Refs > 0 {
			fieldRuns++
		}
		if expanded.AllEmpty && r.Omit() {
			continue
		}
		if expanded.Refs > 0 && strings.TrimSpace(expanded.Text) != "" {
			filledRuns++
		}

		props := docx.RunProps{
			Style:  r.Style,
			Bold:   r.Bold,
			Italic: r.Italic,
			Size:   r.Size.HalfPt(0),
			Color:  r.Color,
		}
		if r.Break {
			p.Runs = append(p.Runs, docx.Run{Props: props, Items: []docx.Inline{docx.Break{}}})
		}
		if r.Tab {
			p.Runs = append(p.Runs, docx.Run{Props: props, Items: []docx.Inline{docx.Tab{}}})
		}
		if expanded.Text != "" {
			p.Runs = append(p.Runs, docx.Run{Props: props, Items: []docx.Inline{docx.Text(expanded.Text)}})
			anyText = true
		}
	}

	if line.Omit() {
		// A line whose every placeholder came up empty is dropped, as is one
		// that produced nothing but tabs and breaks.
		if fieldRuns > 0 && filledRuns == 0 {
			return nil
		}
		if !anyText {
			return nil
		}
	}
	*out = append(*out, p)
	return nil
}

// lookupMeta resolves a dotted path against nested metadata.
func lookupMeta(meta map[string]any, path string) (any, bool) {
	var cur any = meta
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// furnitureRuns splits a furniture line on tab markers so a right-aligned date
// can share a paragraph with left-aligned text.
func furnitureRuns(text string) []docx.Run {
	parts := strings.Split(text, "\t")
	var out []docx.Run
	for i, part := range parts {
		if i > 0 {
			out = append(out, docx.Run{Items: []docx.Inline{docx.Tab{}}})
		}
		if part != "" {
			out = append(out, docx.Run{Items: []docx.Inline{docx.Text(part)}})
		}
	}
	return out
}

func (e *emitter) image(img theme.Image) (*docx.Drawing, error) {
	path := img.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.opts.ThemeDir, path)
	}
	data, err := os.ReadFile(path) //nolint:gosec // path comes from the project's own theme
	if err != nil {
		return nil, fmt.Errorf("theme image: %w", err)
	}
	ext := strings.TrimPrefix(filepath.Ext(path), ".")

	width := docx.EMU(int64(img.Width.Twips(docx.Mm(40))) * 635)
	height := docx.EMU(int64(img.Height.Twips(docx.Mm(15))) * 635)

	name := img.Alt
	if name == "" {
		name = filepath.Base(path)
	}
	return e.out.AddImage(name, data, ext, width, height), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// metaString returns the first non-empty value among the given dotted paths.
func metaString(meta map[string]any, paths ...string) string {
	for _, path := range paths {
		var cur any = meta
		found := true
		for _, part := range strings.Split(path, ".") {
			obj, ok := cur.(map[string]any)
			if !ok {
				found = false
				break
			}
			cur, ok = obj[part]
			if !ok {
				found = false
				break
			}
		}
		if !found {
			continue
		}
		if s, ok := cur.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
