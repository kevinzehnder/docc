package parse

import (
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
	colons, name, ok := splitFence(line[pos:])
	if !ok || colons < 3 || name == "" {
		// A bare `:::` at this point closes a div rather than opening one; the
		// Continue path handles that, so refuse to open here.
		return nil, parser.NoChildren
	}
	reader.Advance(seg.Len() - 1)
	return &Div{Name: name, OpenOffset: seg.Start}, parser.HasChildren
}

func (divParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, seg := reader.PeekLine()
	pos := pc.BlockIndent()
	if pos < 0 {
		pos = 0
	}
	if pos < len(line) {
		if colons, name, ok := splitFence(line[pos:]); ok && colons >= 3 && name == "" {
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

// splitFence reports whether line begins with a colon fence, returning the
// number of leading colons and the trimmed name that follows.
func splitFence(line []byte) (colons int, name string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ':' {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	rest := strings.TrimSpace(string(line[i:]))
	// A trailing run of colons (`::: name :::`) is decoration, not content.
	rest = strings.TrimRight(rest, ": \t")
	if strings.ContainsAny(rest, " \t") {
		// Only single-word names are meaningful; anything else is prose that
		// happens to start with colons and should stay a paragraph.
		return 0, "", false
	}
	return i, strings.ToLower(rest), true
}

// divExtension registers the fenced-div block parser with goldmark.
type divExtension struct{}

func (divExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(divParser{}, 100),
	))
}
