// Package ir is the typed document that sits between validation and emission.
//
// The markdown AST is shaped by the syntax that produced it; the emitter cares
// about document structure. Converting once, here, keeps goldmark's types out
// of the emitter and gives the whole pipeline one thing to test against.
package ir

// Document is a validated, normalised document.
type Document struct {
	// Type is the resolved document type.
	Type string
	// Meta is the frontmatter with defaults applied and dates parsed.
	Meta map[string]any
	// Blocks is the body.
	Blocks []Block
}

// Block is a body-level element.
type Block interface{ block() }

// Heading is a section heading. Level is 1-based.
type Heading struct {
	Level   int
	Inlines []Inline
}

// Para is a paragraph of prose.
type Para struct {
	Inlines []Inline
}

// List is an ordered or bulleted list. Nested lists appear as blocks inside an
// item rather than as a flattened level, so the emitter can assign numbering
// levels from the nesting depth.
type List struct {
	Ordered bool
	// Start is the first number of an ordered list; zero means 1.
	Start int
	Items []ListItem
}

// ListItem is one entry of a list.
type ListItem struct {
	Blocks []Block
}

// Div is a fenced region: `::: name`.
type Div struct {
	Name   string
	Blocks []Block
}

// Table is a grid. The first row is a header when Header is set.
type Table struct {
	Header bool
	// Align holds the per-column alignment: "", "left", "center", "right".
	Align []string
	Rows  []Row
}

// Row is one table row.
type Row struct {
	Cells []Cell
}

// Cell is one table cell.
type Cell struct {
	Blocks []Block
}

// Code is a preformatted block.
type Code struct {
	Language string
	Text     string
}

// Rule is a horizontal rule.
type Rule struct{}

// Quote is a block quotation.
type Quote struct {
	Blocks []Block
}

func (Heading) block() {}
func (Para) block()    {}
func (List) block()    {}
func (Div) block()     {}
func (Table) block()   {}
func (Code) block()    {}
func (Rule) block()    {}
func (Quote) block()   {}

// Inline is content within a paragraph.
type Inline interface{ inline() }

// Str is literal text.
type Str struct{ Text string }

// Emph is italic.
type Emph struct{ Inlines []Inline }

// Strong is bold.
type Strong struct{ Inlines []Inline }

// CodeSpan is inline preformatted text.
type CodeSpan struct{ Text string }

// LineBreak is a hard line break within a paragraph.
type LineBreak struct{}

// Link is a hyperlink.
type Link struct {
	Inlines []Inline
	URL     string
}

func (Str) inline()       {}
func (Emph) inline()      {}
func (Strong) inline()    {}
func (CodeSpan) inline()  {}
func (LineBreak) inline() {}
func (Link) inline()      {}

// Text flattens inlines to their plain text, for diagnostics and for cases
// where formatting cannot be represented.
func Text(inlines []Inline) string {
	var b []byte
	var walk func([]Inline)
	walk = func(items []Inline) {
		for _, item := range items {
			switch v := item.(type) {
			case Str:
				b = append(b, v.Text...)
			case CodeSpan:
				b = append(b, v.Text...)
			case Emph:
				walk(v.Inlines)
			case Strong:
				walk(v.Inlines)
			case Link:
				walk(v.Inlines)
			case LineBreak:
				b = append(b, ' ')
			}
		}
	}
	walk(inlines)
	return string(b)
}
