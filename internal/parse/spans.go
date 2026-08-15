package parse

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// KindSpan is the node kind for an inline semantic annotation.
var KindSpan = ast.NewNodeKind("Span")

// Span is an inline semantic annotation: `[literal]{.type key=value}`. The
// bracketed text is the authored document content, kept verbatim; the
// attribute block makes it machine-checkable. A span never changes rendering —
// it exists for validation, consistency and reference resolution.
type Span struct {
	ast.BaseInline
	// Literal is the byte range of the bracketed text within the body source.
	// The same range is attached as a child text node, so the IR builder
	// renders the authored content without knowing what a span is.
	Literal text.Segment
	// Attr is the parsed attribute block. Attr.Classes carries the span's
	// type as its first entry; whether one is present is sema's business.
	Attr AttrBlock
	// OpenOffset is the byte offset of the `[` within the body source.
	OpenOffset int
}

func (n *Span) Kind() ast.NodeKind { return KindSpan }

func (n *Span) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Type": n.SpanType()}, nil)
}

// SpanType returns the span's semantic type: the first `.class` in the
// attribute block, or "" when the author wrote none.
func (n *Span) SpanType() string {
	if len(n.Attr.Classes) == 0 {
		return ""
	}
	return n.Attr.Classes[0].Name
}

// LiteralText returns the authored text between the brackets.
func (n *Span) LiteralText(source []byte) string {
	return string(n.Literal.Value(source))
}

// spanParser recognises `[literal]{...}`. It runs before the link parser on
// the shared `[` trigger and declines anything that is not immediately
// followed by an attribute block, so links and footnote-style prose are
// untouched.
type spanParser struct{}

func (spanParser) Trigger() []byte { return []byte{'['} }

func (spanParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, seg := block.PeekLine()
	closing := -1
	for i := 1; i < len(line); i++ {
		if line[i] == '[' {
			// A nested bracket is link-shaped, not a span; let the link parser
			// have the whole thing.
			return nil
		}
		if line[i] == ']' {
			closing = i
			break
		}
	}
	// A span is bracketed text with its attribute block on the same line.
	if closing < 0 || closing+1 >= len(line) || line[closing+1] != '{' {
		return nil
	}

	span := &Span{
		Literal:    text.NewSegment(seg.Start+1, seg.Start+closing),
		OpenOffset: seg.Start,
	}
	attrStart := closing + 2
	end := scanAttrEnd(line, attrStart)
	consumed := end + 1
	if end < 0 {
		// Unterminated attribute block: claim the rest of the line so the
		// annotation cannot silently degrade to prose, and report it.
		content := bytes.TrimRight(line[attrStart:], "\r\n")
		span.Attr = parseAttrBlock(content, seg.Start+attrStart)
		if span.Attr.Err == "" {
			span.Attr.Err = "attribute block `{` is never closed"
			span.Attr.ErrOffset = seg.Start + closing + 1
		}
		consumed = attrStart + len(content)
	} else {
		span.Attr = parseAttrBlock(line[attrStart:end], seg.Start+attrStart)
	}
	span.AppendChild(span, ast.NewTextSegment(span.Literal))
	block.Advance(consumed)
	return span
}
