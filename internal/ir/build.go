package ir

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	gast "github.com/yuin/goldmark/extension/ast"

	"github.com/kevinzehnder/docc/internal/parse"
)

// Build converts a parsed file into the typed document.
func Build(f *parse.File, docType string, meta map[string]any) *Document {
	if meta == nil {
		meta = map[string]any{}
	}
	return &Document{
		Type:   docType,
		Meta:   meta,
		Blocks: blocks(f, f.Body),
	}
}

// blocks converts the children of a container node.
func blocks(f *parse.File, parent ast.Node) []Block {
	var out []Block
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if b := block(f, n); b != nil {
			out = append(out, b)
		}
	}
	return out
}

func block(f *parse.File, n ast.Node) Block {
	switch v := n.(type) {
	case *ast.Heading:
		return Heading{Level: v.Level, Line: lineOf(f, v), Inlines: inlines(f, v)}

	case *ast.Paragraph:
		return Para{Line: lineOf(f, v), Inlines: inlines(f, v)}

	case *ast.TextBlock:
		// A tight list item's content is a TextBlock rather than a Paragraph.
		return Para{Line: lineOf(f, v), Inlines: inlines(f, v)}

	case *ast.List:
		list := List{Ordered: v.IsOrdered(), Start: v.Start, Line: lineOf(f, v)}
		for item := v.FirstChild(); item != nil; item = item.NextSibling() {
			list.Items = append(list.Items, ListItem{Blocks: blocks(f, item)})
		}
		return list

	case *parse.Div:
		return Div{Name: v.Name, ID: v.Attr.ID, Line: lineOf(f, v), Blocks: blocks(f, v)}

	case *ast.Blockquote:
		return Quote{Line: lineOf(f, v), Blocks: blocks(f, v)}

	case *ast.FencedCodeBlock:
		return Code{
			Language: string(v.Language(f.BodySource)),
			Line:     lineOf(f, v),
			Text:     rawLines(f, v),
		}

	case *ast.CodeBlock:
		return Code{Line: lineOf(f, v), Text: rawLines(f, v)}

	case *ast.ThematicBreak:
		return Rule{Line: lineOf(f, v)}

	case *gast.Table:
		return table(f, v)

	case *ast.HTMLBlock:
		// Raw HTML has no meaning in a Word document; dropping it is more
		// honest than emitting the markup as visible text.
		return nil

	default:
		return nil
	}
}

// lineOf returns the 1-based source line a block starts on. Containers
// (lists, tables, quotes) record no lines of their own, so the first
// descendant that does supplies the position. Zero means no line is known,
// which only a thematic break produces in practice.
func lineOf(f *parse.File, n ast.Node) int {
	if d, isDiv := n.(*parse.Div); isDiv {
		return f.BodyPos(d.OpenOffset).Line
	}
	if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
		return f.BodyPos(n.Lines().At(0).Start).Line
	}
	// A table cell holds inline children with no block lines above them; the
	// first text segment is the position that exists.
	if t, isText := n.(*ast.Text); isText {
		return f.BodyPos(t.Segment.Start).Line
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if line := lineOf(f, c); line > 0 {
			return line
		}
	}
	return 0
}

func table(f *parse.File, t *gast.Table) Table {
	out := Table{Line: lineOf(f, t)}
	for _, al := range t.Alignments {
		out.Align = append(out.Align, alignName(al))
	}
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []Cell
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, Cell{Blocks: []Block{Para{Inlines: inlines(f, cell)}}})
		}
		if _, isHeader := row.(*gast.TableHeader); isHeader {
			out.Header = true
		}
		out.Rows = append(out.Rows, Row{Cells: cells})
	}
	return out
}

func alignName(a gast.Alignment) string {
	switch a {
	case gast.AlignLeft:
		return "left"
	case gast.AlignCenter:
		return "center"
	case gast.AlignRight:
		return "right"
	default:
		return ""
	}
}

// rawLines returns a code block's literal text.
func rawLines(f *parse.File, n ast.Node) string {
	var b strings.Builder
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		b.Write(seg.Value(f.BodySource))
	}
	return b.String()
}

// inlines converts the inline children of a block node.
func inlines(f *parse.File, parent ast.Node) []Inline {
	var out []Inline
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		out = append(out, inline(f, n)...)
	}
	return merge(out)
}

func inline(f *parse.File, n ast.Node) []Inline {
	switch v := n.(type) {
	case *ast.Text:
		seg := v.Segment
		text := unescape(string(seg.Value(f.BodySource)))
		items := []Inline{Str{Text: text}}
		// A hard break is a property of the text node, not a node of its own.
		if v.HardLineBreak() {
			items = append(items, LineBreak{})
		} else if v.SoftLineBreak() {
			// A soft break is a newline in the source, which reflows as a
			// space rather than as a break.
			items = append(items, Str{Text: " "})
		}
		return items

	case *ast.String:
		return []Inline{Str{Text: string(v.Value)}}

	case *ast.Emphasis:
		children := inlines(f, v)
		if v.Level >= 2 {
			return []Inline{Strong{Inlines: children}}
		}
		return []Inline{Emph{Inlines: children}}

	case *ast.CodeSpan:
		return []Inline{CodeSpan{Text: inlineText(f, v)}}

	case *parse.Span:
		span := Span{
			Type:    v.SpanType(),
			Line:    f.BodyPos(v.OpenOffset).Line,
			Inlines: inlines(f, v),
		}
		for _, class := range v.Attr.Classes {
			span.Classes = append(span.Classes, class.Name)
		}
		for _, a := range v.Attr.Attrs {
			if span.Attrs == nil {
				span.Attrs = map[string]string{}
			}
			if _, dup := span.Attrs[a.Key]; !dup {
				span.Attrs[a.Key] = a.Value
			}
		}
		return []Inline{span}

	case *ast.Link:
		return []Inline{Link{Inlines: inlines(f, v), URL: string(v.Destination)}}

	case *ast.AutoLink:
		url := string(v.URL(f.BodySource))
		return []Inline{Link{Inlines: []Inline{Str{Text: url}}, URL: url}}

	case *ast.Image:
		// An inline image in the body is not something this pipeline places;
		// its alt text is the meaningful part.
		return inlines(f, v)

	case *ast.RawHTML:
		return nil

	default:
		if n.Type() == ast.TypeInline {
			return inlines(f, n)
		}
		return nil
	}
}

// inlineText collects the literal text of an inline node's children.
func inlineText(f *parse.File, n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			seg := t.Segment
			b.Write(seg.Value(f.BodySource))
		}
	}
	return b.String()
}

// unescape resolves CommonMark backslash escapes: a backslash before ASCII
// punctuation stands for the punctuation itself. goldmark leaves escapes in
// its text segments and resolves them in its own renderers, so this pipeline
// must do the same or `\[Unterhalt\]` reaches the rendered document verbatim.
// Entity references (`&amp;`) stay literal: nobody writes them in this corpus,
// and resolving them correctly means carrying the HTML5 entity table.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// isASCIIPunct matches CommonMark's ASCII punctuation set, the characters a
// backslash may escape.
func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}

// merge joins adjacent Str nodes. goldmark splits text at every soft break and
// entity, and one run per fragment would bloat the output for no benefit.
func merge(items []Inline) []Inline {
	if len(items) < 2 {
		return items
	}
	out := make([]Inline, 0, len(items))
	for _, item := range items {
		s, isStr := item.(Str)
		if !isStr || len(out) == 0 {
			out = append(out, item)
			continue
		}
		prev, prevIsStr := out[len(out)-1].(Str)
		if !prevIsStr {
			out = append(out, item)
			continue
		}
		out[len(out)-1] = Str{Text: prev.Text + s.Text}
	}
	return out
}
