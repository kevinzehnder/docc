package parse

// This file parses pandoc-style attribute blocks: the `{...}` that can follow
// a fenced div name or an inline span. One tokenizer serves both, because the
// syntax is the same in either position:
//
//	{#id .class key=value key="quoted value"}
//
// The tokenizer is deliberately permissive about *which* ids, classes and keys
// appear — that is schema business, checked in sema. What it refuses is text
// that does not tokenize at all, so an author who writes `{key = value}` gets
// a parse diagnostic instead of a silently ignored annotation.

// Attr is one `key=value` pair in an attribute block.
type Attr struct {
	Key   string
	Value string
	// KeyOffset and ValueOffset are byte offsets into the body source, for
	// diagnostics that point at the key or at its value.
	KeyOffset   int
	ValueOffset int
}

// Class is one `.name` entry in an attribute block.
type Class struct {
	Name string
	// Offset is the byte offset of the `.` into the body source.
	Offset int
}

// AttrBlock is the parsed content of one `{...}` attribute block. Offsets
// index into the body source the block was cut from.
type AttrBlock struct {
	// ID is the `#id` entry, without the `#`. Empty when absent.
	ID       string
	IDOffset int
	// Classes lists the `.name` entries in source order.
	Classes []Class
	// Attrs lists the `key=value` entries in source order. Duplicate keys are
	// preserved here; rejecting them is a semantic decision, not a lexical one.
	Attrs []Attr

	// Err describes the first token that did not lex, anchored at ErrOffset.
	// The entries before the bad token are still populated, so diagnostics can
	// be specific about what was understood.
	Err       string
	ErrOffset int
}

// Get returns the value of the first attribute named key.
func (b *AttrBlock) Get(key string) (string, bool) {
	for _, a := range b.Attrs {
		if a.Key == key {
			return a.Value, true
		}
	}
	return "", false
}

// parseAttrBlock tokenizes the content between the braces of an attribute
// block. src excludes the braces themselves; base is the byte offset of src
// within the body source, so every recorded offset is directly reportable.
func parseAttrBlock(src []byte, base int) AttrBlock {
	var b AttrBlock
	i := 0
	fail := func(off int, msg string) AttrBlock {
		if b.Err == "" {
			b.Err, b.ErrOffset = msg, base+off
		}
		return b
	}
	for i < len(src) {
		if src[i] == ' ' || src[i] == '\t' {
			i++
			continue
		}
		start := i
		switch src[i] {
		case '#':
			word, next := scanWord(src, i+1)
			if word == "" {
				return fail(start, "`#` must be followed by an identifier")
			}
			if b.ID != "" {
				return fail(start, "only one `#id` is allowed per attribute block")
			}
			b.ID, b.IDOffset = word, base+start
			i = next
		case '.':
			word, next := scanWord(src, i+1)
			if word == "" {
				return fail(start, "`.` must be followed by a class name")
			}
			b.Classes = append(b.Classes, Class{Name: word, Offset: base + start})
			i = next
		default:
			key, next := scanWord(src, i)
			if key == "" || next >= len(src) || src[next] != '=' {
				return fail(start, "expected `#id`, `.class` or `key=value`")
			}
			val, valOff, next, ok := scanValue(src, next+1)
			if !ok {
				return fail(valOff, "attribute value is missing or its quote is never closed")
			}
			b.Attrs = append(b.Attrs, Attr{
				Key: key, Value: val,
				KeyOffset: base + start, ValueOffset: base + valOff,
			})
			i = next
		}
		if i < len(src) && src[i] != ' ' && src[i] != '\t' {
			return fail(i, "attribute entries must be separated by spaces")
		}
	}
	return b
}

// scanWord reads an identifier: letters, digits, `-`, `_` and `:`. It returns
// the word and the index of the first byte after it.
func scanWord(src []byte, i int) (string, int) {
	start := i
	for i < len(src) && isWordByte(src[i]) {
		i++
	}
	return string(src[start:i]), i
}

func isWordByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == ':':
		return true
	}
	return false
}

// scanValue reads an attribute value at i: either a `"` quoted run that may
// contain spaces, or a bare run up to the next whitespace. valOff is the
// offset of the value's first content byte, for diagnostics.
func scanValue(src []byte, i int) (val string, valOff, next int, ok bool) {
	if i < len(src) && src[i] == '"' {
		start := i + 1
		for j := start; j < len(src); j++ {
			if src[j] == '"' {
				return string(src[start:j]), start, j + 1, true
			}
		}
		return "", i, len(src), false
	}
	start := i
	for i < len(src) && src[i] != ' ' && src[i] != '\t' {
		i++
	}
	if i == start {
		return "", start, i, false
	}
	return string(src[start:i]), start, i, true
}
