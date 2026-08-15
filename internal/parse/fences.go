package parse

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// KindDiv is the node kind for a pandoc-style fenced div.
var KindDiv = ast.NewNodeKind("Div")

// Div is a `::: name` … `:::` block. Fenced divs carry the semantic regions of
// a document that plain markdown cannot express — a region a schema can then
// name in a rule and a theme can style. What the names mean is the project's
// business, not the parser's.
type Div struct {
	ast.BaseBlock
	// Name is the identifier after the opening colons, lowercased. A div opened
	// without a name has Name "".
	Name string
	// Attr is the parsed `{...}` attribute block on the opening fence, zero
	// when the fence has none. Attr.Err carries lexical problems — an
	// unterminated brace, a token that is not `#id`/`.class`/`key=value` — for
	// Parse to report; the div still opens so the region is not silently prose.
	Attr AttrBlock
	// OpenLine is the 1-indexed source line of the opening fence, resolved by
	// the caller against the full file.
	OpenOffset int
	// Closed reports whether the parser encountered a closing fence on its own
	// line. An unclosed div would otherwise make the rest of the document part
	// of the evidence region without a useful diagnostic.
	Closed bool
}

func (n *Div) Kind() ast.NodeKind { return KindDiv }

func (n *Div) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.Name}, nil)
}

// divParser recognises fenced divs. Opening fence is three or more colons
// followed by an optional name; closing fence is a line of colons alone.
type divParser struct{}

func (divParser) Trigger() []byte { return []byte{':'} }

func (divParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}
	fl, ok := splitFence(line[pos:])
	if !ok || fl.colons < 3 || fl.name == "" {
		// A bare `:::` at this point closes a div rather than opening one; the
		// Continue path handles that, so refuse to open here.
		return nil, parser.NoChildren
	}
	div := &Div{Name: fl.name, OpenOffset: seg.Start}
	if fl.hasAttr {
		base := seg.Start + pos + fl.attrOff
		div.Attr = parseAttrBlock(fl.attrSrc, base)
		if !fl.attrClosed && div.Attr.Err == "" {
			div.Attr.Err = "attribute block `{` is never closed"
			div.Attr.ErrOffset = base - 1
		}
	}
	reader.Advance(seg.Len() - 1)
	return div, parser.HasChildren
}

func (divParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, seg := reader.PeekLine()
	pos := pc.BlockIndent()
	if pos < 0 {
		pos = 0
	}
	if pos < len(line) {
		if fl, ok := splitFence(line[pos:]); ok && fl.colons >= 3 && fl.name == "" && !fl.hasAttr {
			if div, isDiv := node.(*Div); isDiv {
				div.Closed = true
			}
			reader.Advance(seg.Len() - 1)
			return parser.Close
		}
	}
	return parser.Continue | parser.HasChildren
}

func (divParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (divParser) CanInterruptParagraph() bool { return true }

func (divParser) CanAcceptIndentedLine() bool { return false }

// fenceLine is one recognised fence line: `::: name`, optionally followed by
// an attribute block and trailing colon decoration.
type fenceLine struct {
	colons int
	name   string
	// attrSrc is the content between the braces; attrOff is its byte offset
	// within the line slice splitFence was given.
	attrSrc    []byte
	attrOff    int
	hasAttr    bool
	attrClosed bool
}

// splitFence reports whether line begins with a colon fence, returning the
// number of leading colons, the trimmed name, and any attribute block.
func splitFence(line []byte) (fenceLine, bool) {
	i := 0
	for i < len(line) && line[i] == ':' {
		i++
	}
	if i == 0 {
		return fenceLine{}, false
	}
	rest := line[i:]
	if k := bytes.IndexByte(rest, '{'); k >= 0 {
		name := strings.TrimSpace(string(rest[:k]))
		if name == "" || strings.ContainsAny(name, " \t") {
			// `{` with no single-word name in front is prose, not a fence.
			return fenceLine{}, false
		}
		fl := fenceLine{colons: i, name: strings.ToLower(name), hasAttr: true}
		end := scanAttrEnd(rest, k+1)
		if end < 0 {
			// Unterminated attribute block: still a fence, so the author gets
			// a diagnostic instead of the region silently becoming prose.
			fl.attrSrc = bytes.TrimRight(rest[k+1:], "\r\n")
			fl.attrOff = i + k + 1
			return fl, true
		}
		// Beyond the attributes only decoration (`::: name {..} :::`) may follow.
		if strings.TrimRight(string(rest[end+1:]), ": \t\r\n") != "" {
			return fenceLine{}, false
		}
		fl.attrSrc, fl.attrOff, fl.attrClosed = rest[k+1:end], i+k+1, true
		return fl, true
	}
	trimmed := strings.TrimSpace(string(rest))
	// A trailing run of colons (`::: name :::`) is decoration, not content.
	trimmed = strings.TrimRight(trimmed, ": \t")
	if strings.ContainsAny(trimmed, " \t") {
		// Only single-word names are meaningful; anything else is prose that
		// happens to start with colons and should stay a paragraph.
		return fenceLine{}, false
	}
	return fenceLine{colons: i, name: strings.ToLower(trimmed)}, true
}

// scanAttrEnd returns the index of the `}` closing an attribute block whose
// content starts at i, skipping quoted values, or -1 when the line ends first.
func scanAttrEnd(line []byte, i int) int {
	for ; i < len(line); i++ {
		switch line[i] {
		case '}':
			return i
		case '"':
			for i++; i < len(line) && line[i] != '"'; i++ {
			}
			if i >= len(line) {
				return -1
			}
		}
	}
	return -1
}

// divExtension registers the fenced-div block parser with goldmark.
type divExtension struct{}

func (divExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(divParser{}, 100),
	))
}
