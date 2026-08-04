package docx

import (
	"bytes"
	"fmt"
	"strings"
)

// xw writes XML with correct escaping and attribute order preserved as given.
//
// encoding/xml is not used for output: WordprocessingML needs exact namespace
// prefixes and, in places, a specific attribute order, and expressing that
// through struct tags is harder to read and easier to get subtly wrong than
// writing the elements directly.
type xw struct {
	b     bytes.Buffer
	stack []string
}

type attr struct {
	k, v string
}

// a builds an attribute.
func a(k, v string) attr { return attr{k, v} }

// ai builds an attribute from any integer-like value.
func ai[T ~int | ~int32 | ~int64](k string, v T) attr {
	return attr{k, fmt.Sprintf("%d", int64(v))}
}

func (w *xw) header() {
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
}

// open writes a start tag and pushes it onto the stack.
func (w *xw) open(name string, attrs ...attr) {
	w.writeTag(name, attrs, false)
	w.stack = append(w.stack, name)
}

// close closes the most recently opened element, verifying it matches.
func (w *xw) close(name string) {
	if len(w.stack) == 0 {
		panic("docx: close " + name + " with empty element stack")
	}
	top := w.stack[len(w.stack)-1]
	if top != name {
		panic("docx: close " + name + " but " + top + " is open")
	}
	w.stack = w.stack[:len(w.stack)-1]
	w.b.WriteString("</")
	w.b.WriteString(name)
	w.b.WriteString(">")
}

// empty writes a self-closing element.
func (w *xw) empty(name string, attrs ...attr) {
	w.writeTag(name, attrs, true)
}

// text writes escaped character data.
func (w *xw) text(s string) {
	w.b.WriteString(escapeText(s))
}

func (w *xw) writeTag(name string, attrs []attr, selfClose bool) {
	w.b.WriteString("<")
	w.b.WriteString(name)
	for _, at := range attrs {
		w.b.WriteString(" ")
		w.b.WriteString(at.k)
		w.b.WriteString(`="`)
		w.b.WriteString(escapeAttr(at.v))
		w.b.WriteString(`"`)
	}
	if selfClose {
		w.b.WriteString("/>")
		return
	}
	w.b.WriteString(">")
}

// bytes returns the document, panicking if any element was left open — an
// unbalanced part makes Word show a repair prompt rather than an error, so it
// is worth catching at the point of construction.
func (w *xw) bytes() []byte {
	if len(w.stack) != 0 {
		panic("docx: unclosed elements: " + strings.Join(w.stack, ", "))
	}
	return w.b.Bytes()
}

// escapeText escapes character data. Control characters that XML 1.0 forbids
// are dropped: a stray one makes the whole part unreadable, and no document
// author put it there on purpose.
func escapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\t', '\n', '\r':
			b.WriteRune(r)
		default:
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0xfffe || r == 0xffff {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeAttr escapes an attribute value.
func escapeAttr(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t':
			b.WriteString("&#x9;")
		case '\n':
			b.WriteString("&#xA;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
