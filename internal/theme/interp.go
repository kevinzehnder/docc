package theme

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// fieldRe matches a {{ field.path }} placeholder.
var fieldRe = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*\}\}`)

// Interp is the result of expanding a furniture line's text.
type Interp struct {
	// Text is the expanded string.
	Text string
	// AllEmpty reports that the line referenced at least one field and every
	// one of them was empty — the signal that an optional line, such as a
	// recipient's organisation, should be dropped rather than left blank.
	AllEmpty bool
	// Missing lists referenced paths that do not exist in the metadata at all.
	// A field that is absent is a theme bug; a field that is empty is not.
	Missing []string
	// Refs counts the placeholders the text contained. Zero means the text is a
	// literal — a label like "vertreten durch" that only earns its place when
	// the field beside it has a value.
	Refs int
}

// Expand substitutes {{ field.path }} placeholders from meta.
//
// The syntax is deliberately field paths and nothing else. A theme that needs a
// conditional or a loop is describing logic, and logic belongs in Go where it
// can be tested — not in a configuration file that fails at render time.
func Expand(text string, meta map[string]any) Interp {
	if !strings.Contains(text, "{{") {
		return Interp{Text: text}
	}

	refs := 0
	empties := 0
	var missing []string

	out := fieldRe.ReplaceAllStringFunc(text, func(match string) string {
		path := strings.TrimSpace(fieldRe.FindStringSubmatch(match)[1])
		refs++

		value, found := lookup(meta, path)
		if !found {
			missing = append(missing, path)
			empties++
			return ""
		}
		s := format(value)
		if strings.TrimSpace(s) == "" {
			empties++
		}
		return s
	})

	return Interp{
		Text:     collapse(out),
		AllEmpty: refs > 0 && empties == refs,
		Missing:  missing,
		Refs:     refs,
	}
}

// lookup resolves a dotted path against nested maps.
func lookup(meta map[string]any, path string) (any, bool) {
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

// format renders a metadata value as text.
func format(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		if val {
			return "ja"
		}
		return "nein"
	case time.Time:
		return val.Format("2. January 2006")
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, format(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// collapse tidies the whitespace an empty substitution leaves behind, so a
// missing organisation does not turn "Muster GmbH, Bern" into ", Bern".
func collapse(s string) string {
	s = strings.TrimSpace(s)
	for _, fix := range []struct{ from, to string }{
		{"  ", " "},
		{" ,", ","},
		{", ,", ","},
		{",,", ","},
	} {
		for strings.Contains(s, fix.from) {
			s = strings.ReplaceAll(s, fix.from, fix.to)
		}
	}
	return strings.Trim(s, " ,")
}

// Fields lists every field path a theme references, for validation against a
// schema before anything is rendered.
func (t *Theme) Fields() []string {
	seen := map[string]bool{}
	var out []string

	collect := func(lines []Line) {
		for _, l := range lines {
			for _, m := range fieldRe.FindAllStringSubmatch(l.Text, -1) {
				path := strings.TrimSpace(m[1])
				if !seen[path] {
					seen[path] = true
					out = append(out, path)
				}
			}
		}
	}

	collect(t.Prologue)
	collect(t.Epilogue)
	for _, lines := range t.Header {
		collect(lines)
	}
	for _, lines := range t.Footer {
		collect(lines)
	}
	return out
}
