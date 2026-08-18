package theme

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// fieldRe matches a {{ field.path }} placeholder.
var fieldRe = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*\}\}`)

// Reserved placeholders expand to a Word field instead of metadata. A page
// number cannot be text: the document is edited after docc built it, and the
// number would then be wrong. Expand substitutes a control byte, which the
// emitter splits the line on the same way it splits on a tab.
//
// The bytes are control characters precisely because no author types one, and
// the XML writer drops any that escapes this far rather than emitting an
// invalid part.
const (
	// FieldPage marks {{ page }} — the current page number.
	FieldPage = "\x01"
	// FieldPages marks {{ pages }} — the total page count.
	FieldPages = "\x02"
)

// reservedFields are the placeholder names that are fields, not schema fields.
var reservedFields = map[string]string{
	"page":  FieldPage,
	"pages": FieldPages,
}

// HasMetaPlaceholder reports whether text interpolates at least one metadata
// field. A reserved page placeholder does not count: it always has a value, so
// it can never be the empty interpolation that drops a line.
func HasMetaPlaceholder(text string) bool {
	for _, m := range fieldRe.FindAllStringSubmatch(text, -1) {
		if _, reserved := reservedFields[strings.TrimSpace(m[1])]; !reserved {
			return true
		}
	}
	return false
}

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

// Expand substitutes {{ field.path }} placeholders from meta, rendering values
// according to the theme's formats.
//
// The syntax is deliberately field paths and nothing else. A theme that needs a
// conditional or a loop is describing logic, and logic belongs in Go where it
// can be tested — not in a configuration file that fails at render time.
//
// A nil theme formats with the defaults, which is what makes this usable from a
// test that cares about the substitution and not about the locale.
func (t *Theme) Expand(text string, meta map[string]any) Interp {
	var f Formats
	if t != nil {
		f = t.Formats
	}

	if !strings.Contains(text, "{{") {
		return Interp{Text: text}
	}

	refs := 0
	empties := 0
	var missing []string

	out := fieldRe.ReplaceAllStringFunc(text, func(match string) string {
		path := strings.TrimSpace(fieldRe.FindStringSubmatch(match)[1])
		refs++

		if marker, reserved := reservedFields[path]; reserved {
			// A field always has a value, so it counts as a filled reference:
			// a footer that is nothing but a page number must not be dropped
			// as an empty line.
			return marker
		}

		value, found := lookup(meta, path)
		if !found {
			missing = append(missing, path)
			empties++
			return ""
		}
		s := f.format(value)
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

// isoDate is the fallback layout. A theme that declares no date format gets an
// unambiguous date rather than one in some language it never asked for.
const isoDate = "2006-01-02"

// format renders a metadata value as text.
func (f Formats) format(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		return f.boolText(val)
	case time.Time:
		return f.dateText(val)
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, f.format(item))
		}
		return strings.Join(parts, f.separator())
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (f Formats) boolText(v bool) string {
	if len(f.Bool) == 2 {
		if v {
			return f.Bool[0]
		}
		return f.Bool[1]
	}
	if v {
		return "true"
	}
	return "false"
}

func (f Formats) separator() string {
	if f.ListSeparator != "" {
		return f.ListSeparator
	}
	return ", "
}

// dateText formats a date with the theme's layout and then substitutes the
// theme's own month and weekday names for Go's English ones.
func (f Formats) dateText(v time.Time) string {
	layout := f.Date
	if layout == "" {
		layout = isoDate
	}
	s := v.Format(layout)
	if r := f.nameReplacer(); r != nil {
		s = r.Replace(s)
	}
	return s
}

// The names Go's time package emits, which is the vocabulary a substitution
// has to work from. It is not a locale table: nothing here is a translation.
var (
	enMonths = [12]string{
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	}
	enWeekdays = [7]string{
		"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
	}
)

// nameReplacer builds the English → theme substitution. One Replacer, rather
// than a chain of ReplaceAll calls, because it makes a single pass and never
// rewrites its own output: "March" → "März" must not then be reconsidered as
// "Mar" → something else.
//
// Long names come first so a short name never matches inside a long one.
func (f Formats) nameReplacer() *strings.Replacer {
	if len(f.Months) == 0 && len(f.Weekdays) == 0 {
		return nil
	}
	var pairs []string
	add := func(en, local string) {
		if local != "" && local != en {
			pairs = append(pairs, en, local)
		}
	}
	for i, en := range enMonths {
		add(en, at(f.Months, i))
	}
	for i, en := range enWeekdays {
		add(en, at(f.Weekdays, i))
	}
	for i, en := range enMonths {
		add(en[:3], shortName(f.MonthsShort, f.Months, i))
	}
	for i, en := range enWeekdays {
		add(en[:3], shortName(f.WeekdaysShort, f.Weekdays, i))
	}
	if len(pairs) == 0 {
		return nil
	}
	return strings.NewReplacer(pairs...)
}

// shortName prefers a declared abbreviation and otherwise takes the first three
// runes of the long name — runes, because "März" abbreviates to "Mär".
func shortName(short, long []string, i int) string {
	if s := at(short, i); s != "" {
		return s
	}
	full := at(long, i)
	if full == "" {
		return ""
	}
	r := []rune(full)
	if len(r) <= 3 {
		return full
	}
	return string(r[:3])
}

func at(list []string, i int) string {
	if i < len(list) {
		return list[i]
	}
	return ""
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

// FieldRef is one place a theme interpolates a field: the dotted path, and the
// region of furniture it appears in — "prologue", "epilogue", "header:<key>" or
// "footer:<key>". The region is what lets a contract say where a field ends up,
// and, by its absence, that a field is metadata the theme never prints.
type FieldRef struct {
	Path   string
	Region string
}

// FieldRefs lists every field reference a theme makes, with the region it
// occurs in. A path referenced from two regions appears once per region.
//
// A line built from Runs keeps its placeholders in the runs and ignores Text,
// so both are walked. A repeat line contributes the list field it names rather
// than the `item` paths inside it: `item` is bound for one iteration and has no
// declaration of its own.
//
// The order is deterministic — prologue, epilogue, then headers and footers by
// key — because it ends up in a diagnostic.
func (t *Theme) FieldRefs() []FieldRef {
	seen := map[FieldRef]bool{}
	var out []FieldRef

	collect := func(region string, lines []Line) {
		add := func(path string) {
			ref := FieldRef{Path: path, Region: region}
			if path == "" || seen[ref] {
				return
			}
			seen[ref] = true
			out = append(out, ref)
		}
		for _, l := range lines {
			add(l.Repeat)
			add(l.IfNonempty)
			for _, text := range l.texts() {
				for _, m := range fieldRe.FindAllStringSubmatch(text, -1) {
					path := strings.TrimSpace(m[1])
					if l.Repeat != "" && (path == item || strings.HasPrefix(path, item+".")) {
						continue
					}
					if _, reserved := reservedFields[path]; reserved {
						continue
					}
					add(path)
				}
			}
		}
	}

	collect("prologue", t.Prologue)
	collect("epilogue", t.Epilogue)
	for _, key := range sortedKeys(t.Header) {
		collect("header:"+key, t.Header[key])
	}
	for _, key := range sortedKeys(t.Footer) {
		collect("footer:"+key, t.Footer[key])
	}
	return out
}

// Fields lists every field path a theme references, once each, for validation
// against a schema before anything is rendered. It is FieldRefs with the
// regions dropped and the order preserved.
func (t *Theme) Fields() []string {
	seen := map[string]bool{}
	var out []string
	for _, ref := range t.FieldRefs() {
		if seen[ref.Path] {
			continue
		}
		seen[ref.Path] = true
		out = append(out, ref.Path)
	}
	return out
}

// item is the name bound to the current element inside a repeat line.
const item = "item"

// texts returns the strings a line interpolates. Runs win over Text, matching
// what the emitter renders.
func (l Line) texts() []string {
	if len(l.Runs) == 0 {
		return []string{l.Text}
	}
	out := make([]string, 0, len(l.Runs))
	for _, r := range l.Runs {
		out = append(out, r.Text)
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
