package main

// `docc read` is check's sibling: it emits what a document states rather than
// whether it is valid. The compiler already parses the document; this hands
// the result over so a consumer never regexes markdown — re-implementing the
// escaping, the hard breaks and the span syntax the format exists to make
// unambiguous.
//
// The output speaks the schema's vocabulary — block names, span classes,
// field keys, headings — and never the theme's. Styles and rendering have no
// business here; that coupling would break every consumer on a layout change.
//
// The one semantic that separates read from check: exit 0 means the document
// was parsed, even with open diagnostics. A half-finished deed with a required
// section still missing is exactly the document a review tool most wants to
// inspect. Diagnostics ride alongside in the same object; validity stays
// check's job.

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
)

const readHelp = `docc read [flags] <file.md>...

Emit what documents state, as JSON: the frontmatter with defaults applied, the
body as sections of typed blocks, every span with its source line, and blank
fields distinguished from filled ones. Text comes back as authored with
escapes resolved, and a hard break inside a block stays a separate line.

Diagnostics are reported in the same object but never gate the output: exit 0
means the document was parsed, even when it has errors. Validity stays check's
job. Several files return an array, one per file, in argument order.

flags:
`

// readDoc is the top-level object for one file. Its field names are a
// contract: agent workflows and the AGOBIS-style integrations parse them, so
// treat renaming one like renumbering a diagnostic code.
type readDoc struct {
	OK           bool           `json:"ok"`
	Path         string         `json:"path"`
	DocumentType string         `json:"document_type,omitempty"`
	Frontmatter  map[string]any `json:"frontmatter"`
	Body         []readSection  `json:"body"`
	// Spans is the flat index in document order: most consumers want "every
	// .egrid in this document", not a tree walk. Each entry carries its
	// nearest preceding heading so context survives the flattening.
	Spans []readSpan `json:"spans"`
	// Fields lists every `.docc-field` span. "Not yet known" and "known to be
	// empty" are different facts: a blank has value null and blank true.
	Fields      []readField `json:"fields"`
	Errors      int         `json:"errors"`
	Warnings    int         `json:"warnings"`
	Diagnostics diag.List   `json:"diagnostics"`
}

// readSection is a run of blocks under one heading. Sections are flat, in
// document order; nesting is recoverable from level. The preamble before any
// heading is a section without one.
type readSection struct {
	Heading string      `json:"heading,omitempty"`
	Level   int         `json:"level,omitempty"`
	Line    int         `json:"line,omitempty"`
	Blocks  []readBlock `json:"blocks"`
}

// readBlock is one body block, discriminated by Block: "para", "list",
// "table", "code", "quote", "rule", "heading" (only when nested inside
// another block), or the name of a fenced div — the schema's own vocabulary,
// since `blocks:` in a schema declares exactly those names.
type readBlock struct {
	Block string `json:"block"`
	Line  int    `json:"line,omitempty"`
	ID    string `json:"id,omitempty"`
	Level int    `json:"level,omitempty"`
	// Lines is a para's text, one entry per hard break. The break is
	// structure, not typography: flattening the lines to one string would
	// destroy the thing that made the row machine-readable.
	Lines    []string        `json:"lines,omitempty"`
	Spans    []readSpan      `json:"spans,omitempty"`
	Ordered  *bool           `json:"ordered,omitempty"`
	Start    int             `json:"start,omitempty"`
	Items    [][]readBlock   `json:"items,omitempty"`
	Header   bool            `json:"header,omitempty"`
	Align    []string        `json:"align,omitempty"`
	Rows     [][][]readBlock `json:"rows,omitempty"`
	Language string          `json:"language,omitempty"`
	Text     string          `json:"text,omitempty"`
	Blocks   []readBlock     `json:"blocks,omitempty"`
}

type readSpan struct {
	Class string `json:"class"`
	Text  string `json:"text"`
	Line  int    `json:"line,omitempty"`
	// Classes appears only when the span carries more than its type, e.g. a
	// semantic span doubling as a field marker: {.glaeubiger .docc-field}.
	Classes []string          `json:"classes,omitempty"`
	Attrs   map[string]string `json:"attrs,omitempty"`
	// Heading is set in the flat document index only.
	Heading string `json:"heading,omitempty"`
}

type readField struct {
	Key string `json:"key"`
	// Value is null while the field is blank — an empty string would erase
	// the difference between "not yet known" and "known to be empty".
	Value      *string `json:"value"`
	Blank      bool    `json:"blank"`
	Completion string  `json:"completion,omitempty"`
	Line       int     `json:"line,omitempty"`
}

func cmdRead(args []string) int {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	// Output is JSON by construction, so failure objects must be JSON too.
	cf := commonFlags{jsonOut: true}
	fs.StringVar(&cf.schemaDir, "schema-dir", "", "schema directory (default: the resolved profile's)")
	fs.StringVar(&cf.docType, "type", "", "override the frontmatter document_type")
	if code, stop := parseFlags(fs, readHelp, args); stop {
		return code
	}
	files := fs.Args()
	if len(files) == 0 {
		return failf(cf, exitUsage, "docc read: no input files")
	}

	cache := map[string]*schema.Set{}
	docs := make([]readDoc, 0, len(files))
	for _, path := range files {
		src, err := os.ReadFile(path) //nolint:gosec // the user's own argument
		if err != nil {
			return fail(cf, exitUsage, err)
		}
		set, err := loadSchemasCached(cache, cf.schemaDir, path)
		if err != nil {
			return fail(cf, exitConfig, err)
		}
		f, parseDiags := parse.Parse(displayPath(path), src)
		res := sema.Check(f, set, parseDiags, cf.docType)
		docs = append(docs, readView(f.Path, res, ir.Build(f, res.DocType, res.Meta.Values)))
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	var out any = docs
	if len(docs) == 1 {
		out = docs[0]
	}
	if err := enc.Encode(out); err != nil {
		return fail(cf, exitDiag, err)
	}
	return exitOK
}

// readView converts one checked document to its JSON form.
func readView(path string, res *sema.Result, doc *ir.Document) readDoc {
	c := &readCollector{}
	if res.Schema != nil {
		c.blanks = res.Schema.Blanks
	}

	sections := []readSection{}
	cur := readSection{Blocks: []readBlock{}}
	for _, b := range doc.Blocks {
		if h, isHeading := b.(ir.Heading); isHeading {
			if len(cur.Blocks) > 0 || cur.Heading != "" {
				sections = append(sections, cur)
			}
			text := ir.Text(h.Inlines)
			cur = readSection{Heading: text, Level: h.Level, Line: h.Line, Blocks: []readBlock{}}
			c.heading = text
			continue
		}
		cur.Blocks = append(cur.Blocks, c.block(b))
	}
	if len(cur.Blocks) > 0 || cur.Heading != "" {
		sections = append(sections, cur)
	}

	ds := res.Diagnostics
	ds.Sort()
	if ds == nil {
		ds = diag.List{}
	}
	errs, warns := ds.Counts()
	return readDoc{
		OK:           errs == 0,
		Path:         path,
		DocumentType: res.DocType,
		Frontmatter:  normalizeMeta(res.Meta.Values).(map[string]any),
		Body:         sections,
		Spans:        orEmpty(c.spans),
		Fields:       orEmpty(c.fields),
		Errors:       errs,
		Warnings:     warns,
		Diagnostics:  ds,
	}
}

// readCollector accumulates the flat span and field indexes while the body
// tree is built, tracking the nearest preceding heading for context.
type readCollector struct {
	heading string
	blanks  map[string]schema.FieldSpec
	spans   []readSpan
	fields  []readField
}

func (c *readCollector) block(b ir.Block) readBlock {
	switch v := b.(type) {
	case ir.Heading:
		// Only reached nested inside another block; top-level headings become
		// sections. It still updates the context spans are indexed under.
		c.heading = ir.Text(v.Inlines)
		return readBlock{Block: "heading", Level: v.Level, Line: v.Line, Lines: []string{c.heading}}
	case ir.Para:
		lines, spans := c.para(v.Inlines)
		return readBlock{Block: "para", Line: v.Line, Lines: lines, Spans: spans}
	case ir.List:
		items := make([][]readBlock, 0, len(v.Items))
		for _, item := range v.Items {
			items = append(items, c.blocks(item.Blocks))
		}
		ordered := v.Ordered
		return readBlock{Block: "list", Line: v.Line, Ordered: &ordered, Start: v.Start, Items: items}
	case ir.Div:
		return readBlock{Block: v.Name, ID: v.ID, Line: v.Line, Blocks: c.blocks(v.Blocks)}
	case ir.Table:
		rows := make([][][]readBlock, 0, len(v.Rows))
		for _, row := range v.Rows {
			cells := make([][]readBlock, 0, len(row.Cells))
			for _, cell := range row.Cells {
				cells = append(cells, c.blocks(cell.Blocks))
			}
			rows = append(rows, cells)
		}
		return readBlock{Block: "table", Line: v.Line, Header: v.Header, Align: v.Align, Rows: rows}
	case ir.Code:
		return readBlock{Block: "code", Line: v.Line, Language: v.Language, Text: v.Text}
	case ir.Quote:
		return readBlock{Block: "quote", Line: v.Line, Blocks: c.blocks(v.Blocks)}
	case ir.Rule:
		return readBlock{Block: "rule", Line: v.Line}
	default:
		return readBlock{Block: "unknown"}
	}
}

func (c *readCollector) blocks(bs []ir.Block) []readBlock {
	out := make([]readBlock, 0, len(bs))
	for _, b := range bs {
		out = append(out, c.block(b))
	}
	return out
}

// para flattens a paragraph's inlines to text lines split at hard breaks,
// collecting every span it passes into the paragraph's own list and the
// document indexes.
func (c *readCollector) para(inlines []ir.Inline) (lines []string, spans []readSpan) {
	var cur strings.Builder
	var walk func([]ir.Inline)
	walk = func(items []ir.Inline) {
		for _, item := range items {
			switch v := item.(type) {
			case ir.Str:
				cur.WriteString(v.Text)
			case ir.CodeSpan:
				cur.WriteString(v.Text)
			case ir.LineBreak:
				lines = append(lines, cur.String())
				cur.Reset()
			case ir.Span:
				spans = append(spans, c.span(v))
				walk(v.Inlines)
			case ir.Emph:
				walk(v.Inlines)
			case ir.Strong:
				walk(v.Inlines)
			case ir.Link:
				walk(v.Inlines)
			}
		}
	}
	walk(inlines)
	lines = append(lines, cur.String())
	return lines, spans
}

// span records one span in the paragraph, the document index, and — when it
// carries the `.docc-field` marker — the field index.
func (c *readCollector) span(v ir.Span) readSpan {
	s := readSpan{Class: v.Type, Text: ir.Text(v.Inlines), Line: v.Line, Attrs: v.Attrs}
	if len(v.Classes) > 1 {
		s.Classes = v.Classes
	}
	indexed := s
	indexed.Heading = c.heading
	c.spans = append(c.spans, indexed)

	for _, class := range v.Classes {
		if class != sema.FieldSpanType {
			continue
		}
		field := readField{Key: v.Attrs["key"], Blank: sema.IsBlank(s.Text), Line: v.Line}
		if !field.Blank {
			value := s.Text
			field.Value = &value
		}
		if spec, declared := c.blanks[field.Key]; declared {
			field.Completion = spec.Completion
			if field.Completion == "" {
				field.Completion = "before-execution"
			}
		}
		c.fields = append(c.fields, field)
		break
	}
	return s
}

// normalizeMeta rewrites YAML-decoded values for JSON: a date becomes its ISO
// day, because the schema typed it and consumers diff strings, not timestamps.
func normalizeMeta(v any) any {
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02")
	case map[string]any:
		for k, mv := range t {
			t[k] = normalizeMeta(mv)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = normalizeMeta(e)
		}
		return t
	default:
		return v
	}
}

// orEmpty turns a nil slice into an empty one, so the JSON field is [] rather
// than null — a consumer iterating "spans" should never need a null check.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
