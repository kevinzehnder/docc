// Package emit converts a validated document into a Word document, using the
// schema's style map to decide which theme style each markdown construct wears.
//
// The schema says "an ordered list here uses Rechtsbegehren"; the theme says
// what Rechtsbegehren looks like. Neither knows about the other, and this
// package is the only place that needs both.
package emit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
		meta:   typedMeta(sc.Frontmatter, sc.Types, doc.Meta),
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
	e.body(doc.Blocks, &e.out.Body)
	if err := e.buildFurniture(th.Epilogue, &e.out.Body); err != nil {
		return nil, fmt.Errorf("epilogue: %w", err)
	}

	e.out.Numbering = e.numbering
	return e.out, nil
}

// Validate checks a schema and a theme against each other before anything is
// rendered: every style the schema maps must exist in the theme, and every
// field the theme interpolates must be declared by the schema.
//
// Both failures are silent at render time. Word shows an unknown style as body
// text without complaint, and a placeholder naming a field that does not exist
// expands to nothing — which, because a furniture line whose fields are all
// empty is dropped, deletes the line. A typo in an address block therefore
// posts a letter with no city on it. Catching that here is the point.
func Validate(sc *schema.Schema, th *theme.Theme) error {
	var errs []error
	if err := validateStyles(sc, th); err != nil {
		errs = append(errs, err)
	}
	if err := validateFields(sc, th); err != nil {
		errs = append(errs, err)
	}
	if err := validateRender(sc, th); err != nil {
		errs = append(errs, err)
	}
	if err := validateNumbering(th); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// validateRender checks that each render rule names a list definition the theme
// actually defines, and that it says where numbering starts exactly once.
func validateRender(sc *schema.Schema, th *theme.Theme) error {
	var errs []error
	for _, r := range []struct {
		key  string
		rule *schema.NumberingRule
	}{
		{"heading_numbering", sc.Render.HeadingNumbering},
		{"paragraph_numbering", sc.Render.ParagraphNumbering},
	} {
		if r.rule == nil {
			continue
		}
		if r.rule.Definition == "" {
			errs = append(errs, fmt.Errorf("schema %q: render.%s names no definition", sc.Type, r.key))
			continue
		}
		if _, ok := th.Numbering[r.rule.Definition]; !ok {
			errs = append(errs, fmt.Errorf(
				"schema %q: render.%s names definition %q, which the theme %q does not define\ntheme defines: %s",
				sc.Type, r.key, r.rule.Definition, th.Name, strings.Join(sortedKeys(th.Numbering), ", "),
			))
		}
		if r.rule.StartAtHeading != "" && r.rule.StartAfterHeading != "" {
			errs = append(errs, fmt.Errorf(
				"schema %q: render.%s sets both start_at_heading and start_after_heading; keep one",
				sc.Type, r.key,
			))
		}
	}
	return errors.Join(errs...)
}

// validateNumbering rejects list definitions Word cannot render as written.
//
// A suffix outside the enumeration produces a repair prompt rather than a
// differently separated label; a tenth level exceeds what the format has; and a
// level that declares levels of its own is a theme author expecting a tree,
// which this is not — the entries under `levels:` are already the deeper ones.
func validateNumbering(th *theme.Theme) error {
	var bad []string
	for _, name := range sortedKeys(th.Numbering) {
		def := th.Numbering[name]
		if total := len(def.Levels) + 1; total > theme.MaxNumLevels {
			bad = append(bad, fmt.Sprintf("%s: %d levels, the maximum is %d", name, total, theme.MaxNumLevels))
		}
		for i, lvl := range def.Flatten() {
			where := name
			if i > 0 {
				where = fmt.Sprintf("%s level %d", name, i)
			}
			switch lvl.Suffix {
			case "", "tab", "space", "nothing":
			default:
				bad = append(bad, fmt.Sprintf("%s: unknown suffix %q — use tab, space or nothing", where, lvl.Suffix))
			}
			if i > 0 && len(lvl.Levels) > 0 {
				bad = append(bad, fmt.Sprintf(
					"%s: declares levels of its own; `levels:` is a flat list, so move them up beside it", where))
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("theme %q defines numbering Word cannot render:\n  %s",
		th.Name, strings.Join(bad, "\n  "))
}

func validateStyles(sc *schema.Schema, th *theme.Theme) error {
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

// validateFields resolves every {{ path }} in the theme's furniture against the
// schema's frontmatter and named types.
func validateFields(sc *schema.Schema, th *theme.Theme) error {
	var missing []string
	for _, path := range th.Fields() {
		if reason := resolveField(sc, path); reason != "" {
			missing = append(missing, fmt.Sprintf("{{ %s }} — %s", path, reason))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"theme %q interpolates fields the schema %q does not declare:\n  %s\nschema declares: %s",
		th.Name, sc.Type, strings.Join(missing, "\n  "), strings.Join(sortedKeys(sc.Frontmatter), ", "),
	)
}

// resolveField walks a dotted path through the schema, returning why it does
// not resolve, or "" when it does.
func resolveField(sc *schema.Schema, path string) string {
	segments := strings.Split(path, ".")
	fields := schema.Fields(sc.Frontmatter)
	owner := "the frontmatter"

	for i, seg := range segments {
		f, ok := fields[seg]
		if !ok {
			return fmt.Sprintf("%s declares no field %q", owner, seg)
		}
		if i == len(segments)-1 {
			return ""
		}
		next, isObject := sc.Types[f.Type]
		if !isObject {
			return fmt.Sprintf("field %q is a %s and has no member %q",
				strings.Join(segments[:i+1], "."), f.Type, segments[i+1])
		}
		fields = next
		owner = fmt.Sprintf("type %q", f.Type)
	}
	return ""
}

// isoDate is the layout a `date` field is written in. It is the schema's
// contract, checked in sema; this is the other end of it.
const isoDate = "2006-01-02"

// typedMeta returns a copy of meta with values reinterpreted according to the
// type the schema declares for them.
//
// YAML hands back a string for a date, so without this a theme's `formats.date`
// would never fire and a letterhead would print 2026-08-04 under a theme that
// asked for "4. August 2026" — configuration that is silently dead, which is
// the failure mode this compiler exists to remove. The schema is what knows a
// field is a date, and emit is the only place holding both schema and theme.
//
// A value that does not parse is left alone. sema has already reported it, and
// `--force` still has to render something.
func typedMeta(fields schema.Fields, types map[string]schema.Fields, meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	for key, value := range meta {
		f, declared := fields[key]
		if !declared {
			out[key] = value
			continue
		}
		out[key] = typedValue(f.Type, types, value)
	}
	return out
}

func typedValue(declared string, types map[string]schema.Fields, value any) any {
	if inner, isList := strings.CutPrefix(declared, "list<"); isList {
		items, ok := value.([]any)
		if !ok {
			return value
		}
		elem := strings.TrimSuffix(inner, ">")
		converted := make([]any, 0, len(items))
		for _, item := range items {
			converted = append(converted, typedValue(elem, types, item))
		}
		return converted
	}

	if declared == "date" {
		s, ok := value.(string)
		if !ok {
			return value
		}
		t, err := time.Parse(isoDate, s)
		if err != nil {
			return value
		}
		return t
	}

	if members, isObject := types[declared]; isObject {
		nested, ok := value.(map[string]any)
		if !ok {
			return value
		}
		return typedMeta(members, types, nested)
	}
	return value
}

type emitter struct {
	doc *ir.Document
	// meta is doc.Meta with values reinterpreted according to their declared
	// type. Furniture interpolates from here, never from doc.Meta.
	meta   map[string]any
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

// body renders the top level of the document, which is the only level the
// schema's render numbering applies to.
//
// Eligibility is decided here rather than inside blockTo on purpose. A
// paragraph reached recursively is a list item, a table cell, a quotation or
// the contents of a fenced div — content that already carries its own label, or
// belongs to a structure that owns it. Numbering every ir.Para the emitter
// happens to reach would put a marginal number on each Rechtsbegehren and each
// Beweismittel entry.
func (e *emitter) body(blocks []ir.Block, out *[]docx.Block) {
	headings := e.renderRule(e.schema.Render.HeadingNumbering)
	paragraphs := e.renderRule(e.schema.Render.ParagraphNumbering)

	for _, b := range blocks {
		headings.arrive(b)
		paragraphs.arrive(b)

		start := len(*out)
		e.blockTo(b, out, 0)
		produced := (*out)[start:]

		switch v := b.(type) {
		case ir.Heading:
			if headings.active {
				// Markdown level 1 is the definition's level 0.
				headings.apply(e, produced, v.Level-1)
			}
		case ir.Para:
			if paragraphs.active {
				paragraphs.apply(e, produced, 0)
			}
		}

		headings.depart(b)
		paragraphs.depart(b)
	}
}

// renderState tracks one render numbering rule as the body is walked.
type renderState struct {
	rule *schema.NumberingRule
	// marker is the heading numbering keys off, lowercased for comparison.
	marker string
	// inclusive reports that the marker heading is itself numbered.
	inclusive bool
	// active reports that numbering has started.
	active bool
	// numID is the single instance every numbered block in this rule shares.
	// One instance is the whole point: a fresh one per block would restart the
	// count, so every heading would be I. and every paragraph 1.
	numID int
	// levels is how many levels the definition declares. A heading deeper than
	// that gets no label rather than a fabricated one.
	levels int
}

func (e *emitter) renderRule(rule *schema.NumberingRule) *renderState {
	s := &renderState{rule: rule}
	if rule == nil {
		return s
	}
	heading, inclusive := rule.Marker()
	s.marker = normalizeHeading(heading)
	s.inclusive = inclusive
	// No marker means the rule covers the body from its first block.
	s.active = s.marker == ""
	return s
}

// arrive runs before a block is numbered, and starts an inclusive rule so that
// `start_at_heading: RECHTSBEGEHREN` numbers RECHTSBEGEHREN itself.
func (s *renderState) arrive(b ir.Block) {
	if s.inclusive {
		s.startIfMarker(b)
	}
}

// depart runs after a block is numbered, and starts an exclusive rule so that
// `start_after_heading: RECHTSBEGEHREN` leaves that heading unnumbered and
// begins with what follows it.
func (s *renderState) depart(b ir.Block) {
	if !s.inclusive {
		s.startIfMarker(b)
	}
}

func (s *renderState) startIfMarker(b ir.Block) {
	if s.rule == nil || s.active || s.marker == "" {
		return
	}
	if h, isHeading := b.(ir.Heading); isHeading && normalizeHeading(ir.Text(h.Inlines)) == s.marker {
		s.active = true
	}
}

// apply attaches the rule's numbering instance to the first paragraph a block
// produced, allocating the instance on first use so a document that never
// reaches the marker carries no definition it does not use.
func (s *renderState) apply(e *emitter, produced []docx.Block, level int) {
	if level < 0 {
		return
	}
	if s.numID == 0 {
		s.numID, s.levels = e.sharedNumID(s.rule.Definition)
		if s.numID == 0 {
			return
		}
	}
	if level >= s.levels {
		return
	}
	for i, blk := range produced {
		p, isPara := blk.(docx.Paragraph)
		if !isPara || p.Props.Numbering != nil {
			continue
		}
		p.Props.Numbering = &docx.NumRef{ID: s.numID, Level: level}
		produced[i] = p
		return
	}
}

// normalizeHeading matches heading text the way the body checks do: ignoring
// case and surrounding space, so a schema does not have to reproduce the
// document's capitalisation exactly.
func normalizeHeading(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
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

// sharedNumID allocates one numbering instance for a render rule and reports
// how many levels its definition declares.
//
// It shares the abstract definition with any list that names the same theme
// entry — same appearance, one definition — but takes its own instance, because
// an instance is what carries the count. Call it once per rule and keep the id.
func (e *emitter) sharedNumID(defName string) (numID, levels int) {
	themeDef, ok := e.theme.Numbering[defName]
	if !ok {
		return 0, 0 // Validate rejects this before a build reaches here.
	}
	def := themeDef.AbstractNum()

	if abstractID, known := e.abstractByName[defName]; known {
		return e.numbering.NewInstance(abstractID), len(def.Levels)
	}
	def.Name = defName
	numID = e.numbering.AddList(def)
	e.abstractByName[defName] = e.numbering.Abstract[len(e.numbering.Abstract)-1].ID
	return numID, len(def.Levels)
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
		// one: an item inside a div is styled by the div, not by the list.
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
		if err := e.furnitureLine(line, e.meta, out); err != nil {
			return err
		}
	}
	return nil
}

// repeatLine emits one paragraph per element of a list field. A list that is
// absent or empty emits nothing, so a letter without enclosures has no
// enclosures section.
func (e *emitter) repeatLine(line theme.Line, out *[]docx.Block) error {
	raw, found := lookupMeta(e.meta, line.Repeat)
	if !found {
		return nil
	}
	items, isList := raw.([]any)
	if !isList || len(items) == 0 {
		return nil
	}
	for _, item := range items {
		scope := map[string]any{"item": item}
		for k, v := range e.meta {
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

	expanded := e.theme.Expand(line.Text, meta)

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
		expanded := e.theme.Expand(r.Text, meta)
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
