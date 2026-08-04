// Package parse turns a source document into YAML frontmatter plus a markdown
// block tree, preserving byte offsets so every later pass can report a real
// source position.
package parse

import (
	"bytes"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/kevinzehnder/docc/internal/diag"
)

// File is a parsed source document.
type File struct {
	// Path is the file name used in diagnostics.
	Path string
	// Source is the full original text, retained for diagnostic excerpts.
	Source []byte

	// Frontmatter is the raw YAML between the `---` delimiters, excluding them.
	Frontmatter []byte
	// FrontmatterBase is the byte offset of Frontmatter within Source, so YAML
	// parser positions can be lifted to file positions.
	FrontmatterBase int
	// HasFrontmatter reports whether a delimited block was present at all.
	HasFrontmatter bool

	// Body is the goldmark document for the markdown after the frontmatter.
	Body ast.Node
	// BodySource is the byte slice Body was parsed from; goldmark node offsets
	// index into this, not into Source.
	BodySource []byte
	// BodyBase is the offset of BodySource within Source.
	BodyBase int

	// lineStarts holds the byte offset of each line in Source.
	lineStarts []int
}

// Parse splits frontmatter from body and builds the block tree. Diagnostics are
// returned for malformed structure; a non-nil File is always returned so later
// passes can still inspect whatever was recoverable.
func Parse(path string, src []byte) (*File, diag.List) {
	var ds diag.List
	f := &File{Path: path, Source: src, lineStarts: indexLines(src)}

	body, bodyBase := src, 0
	if fm, fmBase, rest, restBase, ok := splitFrontmatter(src); ok {
		f.Frontmatter, f.FrontmatterBase, f.HasFrontmatter = fm, fmBase, true
		body, bodyBase = rest, restBase
	} else if bytes.HasPrefix(src, []byte("---")) {
		// Opened but never closed: without this the whole document silently
		// becomes body text and every frontmatter check reports "missing".
		ds.Errorf(path, diag.Position{Line: 1, Col: 1, Len: 3}, "DOC002",
			"add a closing `---` on its own line",
			"frontmatter block opened but never closed")
	} else {
		ds.Errorf(path, diag.Position{Line: 1, Col: 1}, "DOC001",
			"start the file with a `---` delimited YAML block declaring `document_type`",
			"missing YAML frontmatter")
	}

	f.BodySource, f.BodyBase = body, bodyBase
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table, divExtension{}),
	)
	f.Body = md.Parser().Parse(text.NewReader(body))
	return f, ds
}

// splitFrontmatter finds a leading `---` delimited YAML block.
func splitFrontmatter(src []byte) (fm []byte, fmBase int, rest []byte, restBase int, ok bool) {
	const delim = "---"
	if !bytes.HasPrefix(src, []byte(delim)) {
		return nil, 0, nil, 0, false
	}
	// Opening delimiter must be alone on its line.
	nl := bytes.IndexByte(src, '\n')
	if nl < 0 || strings.TrimSpace(string(src[len(delim):nl])) != "" {
		return nil, 0, nil, 0, false
	}
	start := nl + 1

	// Closing delimiter is the next line that is exactly `---`.
	for off := start; off <= len(src); {
		end := bytes.IndexByte(src[off:], '\n')
		var line []byte
		if end < 0 {
			line, end = src[off:], len(src)
		} else {
			line, end = src[off:off+end], off+end
		}
		if strings.TrimRight(string(line), " \t\r") == delim {
			bodyStart := end
			if bodyStart < len(src) {
				bodyStart++ // skip the newline that terminates the delimiter
			}
			return src[start:off], start, src[bodyStart:], bodyStart, true
		}
		off = end + 1
	}
	return nil, 0, nil, 0, false
}

// PosAt converts a byte offset in Source to a 1-indexed line and column.
func (f *File) PosAt(offset int) diag.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(f.Source) {
		offset = len(f.Source)
	}
	i := sort.SearchInts(f.lineStarts, offset+1) - 1
	if i < 0 {
		i = 0
	}
	return diag.Position{Line: i + 1, Col: offset - f.lineStarts[i] + 1}
}

// BodyPos converts an offset within BodySource to a file position.
func (f *File) BodyPos(offset int) diag.Position {
	return f.PosAt(f.BodyBase + offset)
}

// FrontmatterPos converts a 1-indexed line and column reported by the YAML
// parser (which sees only the frontmatter slice) to a file position.
func (f *File) FrontmatterPos(line, col int) diag.Position {
	if line < 1 {
		return diag.Position{}
	}
	base := f.PosAt(f.FrontmatterBase)
	return diag.Position{Line: base.Line + line - 1, Col: col}
}

// LineText returns the 1-indexed source line, without its terminator.
func (f *File) LineText(line int) string {
	if line < 1 || line > len(f.lineStarts) {
		return ""
	}
	start := f.lineStarts[line-1]
	end := len(f.Source)
	if line < len(f.lineStarts) {
		end = f.lineStarts[line] - 1
	}
	return strings.TrimRight(string(f.Source[start:end]), "\r")
}

func indexLines(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// Heading is a flattened view of one markdown heading.
type Heading struct {
	Level int
	Text  string
	Pos   diag.Position
	Node  *ast.Heading
}

// Headings walks the body in document order and returns every heading. Headings
// inside fenced code blocks are not headings and never appear here — the reason
// this walks the AST instead of scanning lines.
func (f *File) Headings() []Heading {
	var out []Heading
	_ = ast.Walk(f.Body, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, isHeading := n.(*ast.Heading)
		if !isHeading {
			return ast.WalkContinue, nil
		}
		off := 0
		if h.Lines().Len() > 0 {
			off = h.Lines().At(0).Start
		}
		out = append(out, Heading{
			Level: h.Level,
			Text:  strings.TrimSpace(string(h.Text(f.BodySource))), //nolint:staticcheck // Text is the stable API for heading content
			Pos:   f.BodyPos(off),
			Node:  h,
		})
		return ast.WalkSkipChildren, nil
	})
	return out
}

// Divs returns every fenced div in document order.
func (f *File) Divs() []*Div {
	var out []*Div
	_ = ast.Walk(f.Body, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if d, isDiv := n.(*Div); isDiv {
				out = append(out, d)
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// TextLine is one source line of a node's content with its file position.
type TextLine struct {
	Text string
	Pos  diag.Position
}

// TextLines returns the text content of a node's own lines, one entry per source
// line, paired with its file position.
func (f *File) TextLines(n ast.Node) []TextLine {
	// goldmark panics rather than returning empty for inline nodes.
	if n.Type() != ast.TypeBlock {
		return nil
	}
	var out []TextLine
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out = append(out, TextLine{
			Text: strings.TrimRight(string(seg.Value(f.BodySource)), "\r\n"),
			Pos:  f.BodyPos(seg.Start),
		})
	}
	return out
}
