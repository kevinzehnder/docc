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

	"github.com/kevinzehnder/docc/internal/docx"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/theme"
)

// Options configures a build.
type Options struct {
	// ThemeDir is where theme-relative image paths resolve from.
	ThemeDir string
	// Provenance records which configuration produced the document, written
	// into the file's custom properties. It answers the question a compliance
	// review actually asks of a filed document — which profile revision made
	// this — without anyone having to keep a separate ledger.
	//
	// Values come from the caller's resolved configuration, never from the
	// clock, so a rebuild stays byte-identical.
	Provenance []docx.CustomProperty
}

// Build renders a document.
func Build(doc *ir.Document, sc *schema.Schema, th *theme.Theme, opts Options) (*docx.Document, error) {
	if err := Validate(sc, th); err != nil {
		return nil, err
	}

	firstSection := docx.Section{
		Page:      th.Page.PageSize(),
		Margins:   th.Page.Margins.DocxMargins(),
		TitlePage: th.Page.TitlePage,
	}
	continuationSection := firstSection
	if th.Page.ContinuationMargins != nil {
		continuationSection.Margins = th.Page.ContinuationMargins.Merge(th.Page.Margins).DocxMargins()
		continuationSection.TitlePage = false
	}

	e := &emitter{
		doc:                 doc,
		meta:                typedMeta(sc.Frontmatter, sc.Types, doc.Meta),
		schema:              sc,
		theme:               th,
		opts:                opts,
		continuationSection: continuationSection,
		out: &docx.Document{
			Section: firstSection,
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
	e.out.Custom = opts.Provenance

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
	if err := validateSpanStyles(sc); err != nil {
		errs = append(errs, err)
	}
	if err := validateBlockPatterns(sc); err != nil {
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
	// A page break keyed to a heading the type does not declare never fires,
	// and nothing says so: the certification simply runs on from the
	// signatures. Only checkable when the type declares a body structure.
	if len(sc.Body) > 0 {
		declared := map[string]bool{}
		var walk func([]schema.BodyRule)
		walk = func(rules []schema.BodyRule) {
			for _, r := range rules {
				declared[strings.TrimSpace(strings.ToLower(r.Heading))] = true
				walk(r.Children)
			}
		}
		walk(sc.Body)
		for _, h := range sc.Render.PageBreakBeforeHeadings {
			if !declared[strings.TrimSpace(strings.ToLower(h))] {
				errs = append(errs, fmt.Errorf(
					"schema %q: render.page_break_before_headings names %q, which the body does not declare",
					sc.Type, h))
			}
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
			switch strings.ToLower(lvl.Restart) {
			case "", theme.RestartAfterParent, theme.RestartNever:
			default:
				bad = append(bad, fmt.Sprintf("%s: unknown restart %q — use %s or %s",
					where, lvl.Restart, theme.RestartAfterParent, theme.RestartNever))
			}
			if i > 0 && len(lvl.Levels) > 0 {
				bad = append(bad, fmt.Sprintf(
					"%s: declares levels of its own; `levels:` is a flat list, so move them up beside it", where))
			}
		}
	}
	// A furniture line naming a definition that does not exist would render
	// without its number and say nothing about it — an enclosures index quietly
	// losing its 1., 2., 3.
	for _, group := range [][]theme.Line{th.Prologue, th.Epilogue} {
		for _, line := range group {
			if line.Numbering == "" {
				continue
			}
			if _, ok := th.Numbering[line.Numbering]; !ok {
				bad = append(bad, fmt.Sprintf(
					"furniture line %q names numbering %q, which is not defined", line.Style, line.Numbering))
			}
		}
	}

	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("theme %q defines numbering Word cannot render:\n  %s",
		th.Name, strings.Join(bad, "\n  "))
}

// validateSpanStyles checks that every `span.<type>` style key names a span
// type the schema declares. A style mapped to a type nobody can write is a
// typo that would otherwise show up as text that is silently not styled.
// validateBlockPatterns rejects a block whose style map selects more than one
// rendering pattern. emit.div dispatches on the first key it finds, so the
// others render nothing at all — a mapping that validates, renders, and does
// nothing, which is the failure the style vocabulary exists to prevent.
//
// This is the only place that can see it: which pattern a block uses is not
// declared anywhere, it is a consequence of which key the schema set.
func validateBlockPatterns(sc *schema.Schema) error {
	names := map[string]bool{}
	for key := range sc.Styles {
		if strings.HasPrefix(key, "div.") {
			names[blockName(key)] = true
		}
	}
	var errs []error
	for _, name := range sortedKeys(names) {
		keys := PatternKeys(sc, name)
		if len(keys) < 2 {
			continue
		}
		errs = append(errs, fmt.Errorf(
			"schema %q: block %q selects %d rendering patterns (%s); only %s takes effect — map one",
			sc.Type, name, len(keys), strings.Join(keys, ", "), keys[0]))
	}
	return errors.Join(errs...)
}

func validateSpanStyles(sc *schema.Schema) error {
	if len(sc.Spans) == 0 {
		return nil
	}
	var unknown []string
	for key := range sc.Styles {
		name, ok := strings.CutPrefix(key, "span.")
		if !ok || strings.HasPrefix(name, "docc-") {
			continue
		}
		if _, declared := sc.Spans[name]; !declared {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf(
		"schema %q styles span types it does not declare:\n  %s\nschema declares: %s",
		sc.Type, strings.Join(unknown, "\n  "), strings.Join(sortedKeys(sc.Spans), ", "),
	)
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
	abstractByName      map[string]int
	continuationSection docx.Section
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
	// endBefore is the first heading outside the numbered outline.
	endBefore string
	// active reports that numbering has started.
	active bool
	// stopped reports that the end marker has been reached. It prevents a later
	// start marker from reactivating the rule.
	stopped bool
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
	s.endBefore = normalizeHeading(rule.EndBeforeHeading)
	s.inclusive = inclusive
	// No marker means the rule covers the body from its first block.
	s.active = s.marker == ""
	return s
}

// arrive runs before a block is numbered, and starts an inclusive rule so that
// `start_at_heading: RECHTSBEGEHREN` numbers RECHTSBEGEHREN itself.
func (s *renderState) arrive(b ir.Block) {
	if s.stopIfEndMarker(b) {
		return
	}
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

func (s *renderState) stopIfEndMarker(b ir.Block) bool {
	if s.rule == nil || s.stopped || s.endBefore == "" {
		return false
	}
	if h, isHeading := b.(ir.Heading); isHeading && normalizeHeading(ir.Text(h.Inlines)) == s.endBefore {
		s.active = false
		s.stopped = true
		return true
	}
	return false
}

func (s *renderState) startIfMarker(b ir.Block) {
	if s.rule == nil || s.active || s.stopped || s.marker == "" {
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

// breaksPage reports whether the schema wants this heading to start a new
// page. Matching follows the body rules: case-insensitive, whitespace
// trimmed.
func (e *emitter) breaksPage(heading string) bool {
	want := strings.TrimSpace(strings.ToLower(heading))
	if want == "" {
		return false
	}
	for _, h := range e.schema.Render.PageBreakBeforeHeadings {
		if strings.TrimSpace(strings.ToLower(h)) == want {
			return true
		}
	}
	return false
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
				p.Runs[i].Props.Bold = docx.ToggleOn
			}
		}
		if e.breaksPage(ir.Text(v.Inlines)) {
			p.Props.PageBreak = true
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
	if amountStyle := e.style("div." + d.Name + ".amount"); amountStyle != "" {
		e.amountDiv(d, out, depth, style, amountStyle, e.style("div."+d.Name+".total"))
		return
	}
	if lineStyle := e.style("div." + d.Name + ".line"); lineStyle != "" {
		e.ruledDiv(d, out, depth, style, lineStyle)
		return
	}
	if labelStyle := e.style("div." + d.Name + ".label"); labelStyle != "" {
		e.labelledDiv(d, out, depth, style, labelStyle)
		return
	}
	if fieldStyle := e.style("div." + d.Name + ".field"); fieldStyle != "" {
		e.fieldDiv(d, out, depth, style, fieldStyle)
		return
	}

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

// labelledDiv renders a labelled list item as its description, a right-aligned
// tab, then its source label. The source keeps the label first in square
// brackets because that makes the schema rule and cross-reference unambiguous;
// the page uses the more readable description-first order.
//
// A schema opts into this rendering by mapping div.<name>.label to a character
// style. Other divs retain the ordinary markdown rendering above.
func (e *emitter) labelledDiv(d ir.Div, out *[]docx.Block, depth int, style, labelStyle string) {
	for _, block := range d.Blocks {
		if list, ok := block.(ir.List); ok && !list.Ordered {
			e.labelledList(list, out, depth, style, labelStyle)
			continue
		}

		var inner []docx.Block
		e.blockTo(block, &inner, depth)
		for _, blk := range inner {
			if p, ok := blk.(docx.Paragraph); ok {
				p.Props.Style = style
				*out = append(*out, p)
				continue
			}
			*out = append(*out, blk)
		}
	}
}

// fieldDiv renders a form: a label in its own column and the value beside it.
// It is labelledDiv with the emission order reversed — tab, label, tab, then
// the item's rich remainder — which is what a Swiss registry form looks like
// and what no block pattern could produce before.
//
// The order is the whole change. splitEvidenceLabel already returns a plain
// string for the bracket and rich inlines for the rest, which is exactly the
// right way round here: a form label is plain ("Firma:") and a form value
// carries the spans the checks anchor on. Asking authors to write the value in
// the bracket instead would truncate it at its first `]` and take every
// `[…]{.firma}` with it.
//
// Reaching this from the body rather than from furniture is the point: the
// content stays in the document, so `no_blank_spans`, `spans_agree`,
// `required_spans` and every div-scoped rule keep applying to it.
func (e *emitter) fieldDiv(d ir.Div, out *[]docx.Block, depth int, style, fieldStyle string) {
	for _, block := range d.Blocks {
		if list, ok := block.(ir.List); ok && !list.Ordered {
			e.fieldList(list, out, depth, style, fieldStyle)
			continue
		}

		var inner []docx.Block
		e.blockTo(block, &inner, depth)
		for _, blk := range inner {
			if p, ok := blk.(docx.Paragraph); ok {
				p.Props.Style = style
				*out = append(*out, p)
				continue
			}
			*out = append(*out, blk)
		}
	}
}

// fieldList renders one form row per list item. The row carries no list
// numbering: a form is not a bulleted list, and the columns come from the tab
// stops and hanging indent of the row style, exactly as the amount pattern
// takes its columns from Betragszeile.
func (e *emitter) fieldList(list ir.List, out *[]docx.Block, depth int, style, fieldStyle string) {
	for _, item := range list.Items {
		first := true
		for _, block := range item.Blocks {
			if para, ok := block.(ir.Para); ok && first {
				if label, description, labelled := splitEvidenceLabel(para.Inlines); labelled {
					runs := []docx.Run{{Items: []docx.Inline{docx.Tab{}}}}
					runs = append(runs, docx.Run{
						Props: docx.RunProps{Style: fieldStyle},
						Items: []docx.Inline{docx.Text(label)},
					})
					runs = append(runs, docx.Run{Items: []docx.Inline{docx.Tab{}}})
					runs = append(runs, e.runs(description, docx.RunProps{})...)
					*out = append(*out, docx.Paragraph{
						Props: docx.ParaProps{Style: style},
						Runs:  runs,
					})
					first = false
					continue
				}
			}

			var inner []docx.Block
			e.blockTo(block, &inner, depth)
			for _, blk := range inner {
				p, isPara := blk.(docx.Paragraph)
				if !isPara {
					*out = append(*out, blk)
					continue
				}
				p.Props.Style = style
				*out = append(*out, p)
			}
			first = false
		}
	}
}

// tabStops returns the tab stops a style declares, following based_on. A
// ruled line emits one tab per stop, so a theme that wants a gap before the
// rule declares two stops rather than the emitter guessing at one.
func (e *emitter) tabStops(styleID string) []theme.Tab {
	for i := 0; i < 16 && styleID != ""; i++ {
		st, ok := e.theme.Styles[styleID]
		if !ok {
			return nil
		}
		if len(st.Tabs) > 0 {
			return st.Tabs
		}
		styleID = st.BasedOn
	}
	return nil
}

// ruledDiv renders a block whose list items each need a rule to write on: a
// signature block. The item is its own text followed by a tab, and the tab
// leader declared by the line style draws the rule across to the tab stop.
//
// Nothing about the rule is in the source, which is the point: a signature
// line is not content the author should be typing, and a document whose dots
// are literal text cannot have its signatories checked or reordered.
func (e *emitter) ruledDiv(d ir.Div, out *[]docx.Block, depth int, style, lineStyle string) {
	for _, block := range d.Blocks {
		list, isList := block.(ir.List)
		if !isList {
			var inner []docx.Block
			e.blockTo(block, &inner, depth)
			for _, blk := range inner {
				if p, ok := blk.(docx.Paragraph); ok {
					p.Props.Style = style
					*out = append(*out, p)
					continue
				}
				*out = append(*out, blk)
			}
			continue
		}

		for _, item := range list.Items {
			for _, b := range item.Blocks {
				para, isPara := b.(ir.Para)
				if !isPara {
					e.blockTo(b, out, depth)
					continue
				}
				runs := e.runs(para.Inlines, docx.RunProps{})
				for range e.tabStops(lineStyle) {
					runs = append(runs, docx.Run{Items: []docx.Inline{docx.Tab{}}})
				}
				*out = append(*out, docx.Paragraph{
					Props: docx.ParaProps{Style: lineStyle},
					Runs:  runs,
				})
			}
		}
	}
}

// amountDiv renders a money block: each item is a description and an amount,
// set in two columns so the figures align on their own tab stop.
//
// The source writes the amount first, in brackets — `- [Fr. 26'000.00] Der
// Kaufpreis beträgt` — because that is what makes the amount findable by a
// rule and by a reader scanning the markdown. The page wants the opposite
// order, so the description comes first here, then the currency at the first
// tab stop and the figure right-aligned at the second.
//
// An item whose amount begins with `=` is the block's total: it carries the
// total style, which is where a rule above the figure belongs. The marker is
// the one the paper form already used.
func (e *emitter) amountDiv(d ir.Div, out *[]docx.Block, depth int, style, amountStyle, totalStyle string) {
	for _, block := range d.Blocks {
		list, isList := block.(ir.List)
		if !isList {
			var inner []docx.Block
			e.blockTo(block, &inner, depth)
			for _, blk := range inner {
				if p, ok := blk.(docx.Paragraph); ok {
					p.Props.Style = style
					*out = append(*out, p)
					continue
				}
				*out = append(*out, blk)
			}
			continue
		}

		// A block that declares a total is a sum: its other items are the
		// parts, and parts read as a list. A block without a total states one
		// amount — an instalment, a single price — and a bullet in front of it
		// would be a list of one.
		partMarker := 0
		if hasTotal(list) {
			partMarker = e.numID("bullet_list", list)
		}

		for _, item := range list.Items {
			for _, b := range item.Blocks {
				para, isPara := b.(ir.Para)
				if !isPara {
					e.blockTo(b, out, depth)
					continue
				}
				label, description, labelled := splitEvidenceLabel(para.Inlines)
				if !labelled {
					var inner []docx.Block
					e.blockTo(b, &inner, depth)
					for _, blk := range inner {
						p, isPara := blk.(docx.Paragraph)
						if !isPara {
							*out = append(*out, blk)
							continue
						}
						p.Props.Style = style
						*out = append(*out, p)
					}
					continue
				}

				amount, isTotal := strings.CutPrefix(label, "=")
				amount = strings.TrimSpace(amount)
				currency, figure := splitAmount(amount)

				props := docx.RunProps{Style: amountStyle}
				if isTotal {
					if totalAmount := e.style("div." + d.Name + ".total.amount"); totalAmount != "" {
						props.Style = totalAmount
					}
				}
				runs := e.runs(description, docx.RunProps{})
				runs = append(runs, docx.Run{Items: []docx.Inline{docx.Tab{}}})
				runs = append(runs, docx.Run{Props: props, Items: []docx.Inline{docx.Text(currency)}})
				runs = append(runs, docx.Run{Items: []docx.Inline{docx.Tab{}}})
				runs = append(runs, docx.Run{Props: props, Items: []docx.Inline{docx.Text(figure)}})

				paraStyle := style
				if isTotal && totalStyle != "" {
					paraStyle = totalStyle
				}
				props2 := docx.ParaProps{Style: paraStyle}
				if partMarker != 0 && !isTotal {
					props2.Numbering = &docx.NumRef{ID: partMarker, Level: depth}
				}
				*out = append(*out, docx.Paragraph{Props: props2, Runs: runs})

				// The amount in words, when the schema asks for it. It is
				// rendered from the figure rather than authored, so the two
				// cannot disagree.
				//
				// No fallback to `div.<name>`: with one, the gloss appeared
				// wherever the block itself was styled, so a document type that
				// inherits a deed's theme could not switch it off. A deed is
				// read aloud and wants it; a registry form does not.
				wordsStyle := e.style("div." + d.Name + ".words")
				if e.theme.Formats.AmountWords == "" || wordsStyle == "" {
					continue
				}
				value, parsed := parseMoney(figure)
				if !parsed {
					continue
				}
				*out = append(*out, docx.Paragraph{
					Props: docx.ParaProps{Style: wordsStyle},
					Runs: []docx.Run{{Items: []docx.Inline{
						docx.Text(fmt.Sprintf(e.theme.Formats.AmountWords, germanAmountWords(value))),
					}}},
				})
			}
		}
	}
}

// parseMoney reads a Swiss figure — "26'000.00" — as hundredths. It is the
// emitter's own reader rather than a shared one because rendering must not
// fail on something the checker has already reported.
func parseMoney(figure string) (int64, bool) {
	var whole, frac int64
	var digits, fracDigits int
	seenPoint := false
	for _, r := range figure {
		switch {
		case r >= '0' && r <= '9':
			if seenPoint {
				if fracDigits == 2 {
					return 0, false
				}
				frac = frac*10 + int64(r-'0')
				fracDigits++
				continue
			}
			whole = whole*10 + int64(r-'0')
			digits++
		case r == '.':
			if seenPoint {
				return 0, false
			}
			seenPoint = true
		case r == '\'' || r == '\u2019' || r == ' ':
			// thousands separators
		default:
			return 0, false
		}
	}
	if digits == 0 {
		return 0, false
	}
	if fracDigits == 1 {
		frac *= 10
	}
	return whole*100 + frac, true
}

// hasTotal reports whether any item of a money block is marked `=`.
func hasTotal(list ir.List) bool {
	for _, item := range list.Items {
		for _, b := range item.Blocks {
			para, isPara := b.(ir.Para)
			if !isPara {
				continue
			}
			if label, _, ok := splitEvidenceLabel(para.Inlines); ok {
				if strings.HasPrefix(label, "=") {
					return true
				}
			}
		}
	}
	return false
}

// splitAmount separates a currency from its figure: "Fr. 26'000.00" becomes
// "Fr." and "26'000.00". An amount written without a currency keeps the
// figure column to itself.
func splitAmount(amount string) (currency, figure string) {
	i := strings.LastIndexByte(amount, ' ')
	if i < 0 {
		return "", amount
	}
	return strings.TrimSpace(amount[:i]), strings.TrimSpace(amount[i+1:])
}

func (e *emitter) labelledList(list ir.List, out *[]docx.Block, depth int, style, labelStyle string) {
	numID := e.numID("bullet_list", list)
	for _, item := range list.Items {
		first := true
		for _, block := range item.Blocks {
			if para, ok := block.(ir.Para); ok && first {
				if label, description, labelled := splitEvidenceLabel(para.Inlines); labelled {
					runs := e.runs(description, docx.RunProps{})
					runs = append(runs, docx.Run{Items: []docx.Inline{docx.Tab{}}})
					runs = append(runs, docx.Run{
						Props: docx.RunProps{Style: labelStyle},
						Items: []docx.Inline{docx.Text(label)},
					})
					*out = append(*out, docx.Paragraph{
						Props: docx.ParaProps{
							Style:     style,
							Numbering: &docx.NumRef{ID: numID, Level: depth},
						},
						Runs: runs,
					})
					first = false
					continue
				}
			}

			var inner []docx.Block
			e.blockTo(block, &inner, depth)
			for _, blk := range inner {
				p, isPara := blk.(docx.Paragraph)
				if !isPara {
					*out = append(*out, blk)
					continue
				}
				p.Props.Style = style
				if first {
					p.Props.Numbering = &docx.NumRef{ID: numID, Level: depth}
					first = false
				}
				*out = append(*out, p)
			}
		}
	}
}

// splitEvidenceLabel separates the leading [label] from an evidence item's
// description without disturbing any Markdown formatting in the description.
func splitEvidenceLabel(inlines []ir.Inline) (label string, description []ir.Inline, ok bool) {
	if len(inlines) == 0 {
		return "", nil, false
	}
	first, isText := inlines[0].(ir.Str)
	if !isText {
		return "", nil, false
	}
	text := strings.TrimLeft(first.Text, " \t")
	if !strings.HasPrefix(text, "[") {
		return "", nil, false
	}
	end := strings.IndexByte(text, ']')
	if end <= 1 {
		return "", nil, false
	}
	label = strings.TrimSpace(text[1:end])
	remainder := strings.TrimLeft(text[end+1:], " \t")
	if remainder != "" {
		description = append(description, ir.Str{Text: remainder})
	}
	description = append(description, inlines[1:]...)
	if label == "" || len(description) == 0 {
		return "", nil, false
	}
	return label, description, true
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
			p.Runs[j].Props.Bold = docx.ToggleOn
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
			props.Bold = docx.ToggleOn
			out = append(out, e.runs(v.Inlines, props)...)

		case ir.Emph:
			props := inherited
			props.Italic = docx.ToggleOn
			out = append(out, e.runs(v.Inlines, props)...)

		case ir.CodeSpan:
			props := inherited
			props.Font = "Courier New"
			out = append(out, docx.Run{Props: props, Items: []docx.Inline{docx.Text(v.Text)}})

		case ir.Span:
			// A span is styled only if the schema maps its type. Unmapped
			// spans render as their content: the annotation is for the
			// checker, and making it visible is the schema's choice.
			props := inherited
			if style := e.style("span." + v.Type); style != "" {
				props.Style = style
			}
			out = append(out, e.runs(v.Inlines, props)...)

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
	// One instance per definition per block of furniture, so a repeat over a
	// list numbers 1., 2., 3. and a second index elsewhere starts again at 1.
	instances := map[string]int{}
	numIDFor := func(defName string) int {
		if defName == "" {
			return 0
		}
		if id, known := instances[defName]; known {
			return id
		}
		id, _ := e.sharedNumID(defName)
		instances[defName] = id
		return id
	}

	for _, line := range lines {
		if line.IfNonempty != "" && !metaNonempty(e.meta, line.IfNonempty) {
			continue
		}
		numID := numIDFor(line.Numbering)
		if line.Repeat != "" {
			if err := e.repeatLine(line, numID, out); err != nil {
				return err
			}
			continue
		}
		if err := e.furnitureLine(line, e.meta, numID, out); err != nil {
			return err
		}
	}
	return nil
}

// metaNonempty reports whether a theme condition names a populated value.
func metaNonempty(meta map[string]any, path string) bool {
	value, found := lookupMeta(meta, path)
	if !found || value == nil {
		return false
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case bool:
		return v
	default:
		return true
	}
}

// repeatLine emits one paragraph per element of a list field. A list that is
// absent or empty emits nothing, so a letter without enclosures has no
// enclosures section.
func (e *emitter) repeatLine(line theme.Line, numID int, out *[]docx.Block) error {
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
		if err := e.furnitureLine(line, scope, numID, out); err != nil {
			return err
		}
	}
	return nil
}

// applySectionBreak turns a furniture paragraph into the final paragraph of
// the current section. The next block starts on a new page with the theme's
// continuation geometry.
func (e *emitter) applySectionBreak(line theme.Line, p *docx.Paragraph) {
	if !line.SectionBreak {
		return
	}
	ending := e.out.Section
	ending.NextPage = true
	p.Props.SectionBreak = &ending
	e.out.Section = e.continuationSection
}

func (e *emitter) furnitureLine(line theme.Line, meta map[string]any, numID int, out *[]docx.Block) error {
	if len(line.Runs) > 0 {
		return e.furnitureRunLine(line, meta, numID, out)
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
	if numID != 0 {
		p.Props.Numbering = &docx.NumRef{ID: numID}
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
	e.applySectionBreak(line, &p)

	*out = append(*out, p)
	return nil
}

// furnitureRunLine builds a paragraph whose formatting changes partway through.
//
// A run whose fields are all empty is dropped individually, so a party with no
// street loses that fragment without losing the line — and the tab or break
// that would have preceded it goes with it, rather than leaving a gap.
func (e *emitter) furnitureRunLine(line theme.Line, meta map[string]any, numID int, out *[]docx.Block) error {
	p := docx.Paragraph{
		Props: docx.ParaProps{Style: line.Style, PageBreak: line.PageBreak},
	}
	if numID != 0 {
		p.Props.Numbering = &docx.NumRef{ID: numID}
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
			Bold:   boolToggle(r.Bold),
			Italic: boolToggle(r.Italic),
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
	e.applySectionBreak(line, &p)
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

// boolToggle lifts a furniture run's plain bool. A furniture run is a leaf —
// it inherits from its character style, not from another run — so it has no
// "off" to express and absent is simply not-on.
func boolToggle(v bool) docx.Toggle {
	if v {
		return docx.ToggleOn
	}
	return docx.ToggleInherit
}
