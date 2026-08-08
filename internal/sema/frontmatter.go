package sema

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	yamlast "github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// Meta is decoded frontmatter with the source position of every key, so a
// complaint about a field can point at the field.
type Meta struct {
	Values map[string]any
	// pos maps a dotted key path ("gericht.city") to its position in the file.
	pos map[string]diag.Position
	// base is where to point when a key has no recorded position.
	base diag.Position
}

// Pos returns the file position of a dotted key path. A key that is absent has
// no position, and the zero value renders as a file-level diagnostic — better
// than underlining an unrelated line the reader will try to make sense of.
func (m *Meta) Pos(path string) diag.Position {
	if p, ok := m.pos[path]; ok {
		return p
	}
	return diag.Position{}
}

// Lookup resolves a dotted key path against the decoded values.
func (m *Meta) Lookup(path string) (any, bool) {
	var cur any = m.Values
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

// decodeMeta parses frontmatter into values plus key positions.
func decodeMeta(f *parse.File, ds *diag.List) *Meta {
	m := &Meta{
		Values: map[string]any{},
		pos:    map[string]diag.Position{},
		base:   f.FrontmatterPos(1, 1),
	}
	if !f.HasFrontmatter || len(strings.TrimSpace(string(f.Frontmatter))) == 0 {
		return m
	}

	if err := yaml.Unmarshal(f.Frontmatter, &m.Values); err != nil {
		line, col := yamlErrorPos(err)
		ds.Errorf(f.Path, f.FrontmatterPos(line, col), "DOC003",
			"fix the YAML syntax; check indentation and unbalanced quotes",
			"invalid YAML frontmatter: %s", firstLine(err.Error()))
		return m
	}
	if m.Values == nil {
		m.Values = map[string]any{}
	}

	// Positions come from a second, structural parse: Unmarshal discards them.
	if file, err := yamlparser.ParseBytes(f.Frontmatter, 0); err == nil {
		for _, doc := range file.Docs {
			collectPositions(f, doc.Body, "", m.pos)
		}
	}
	return m
}

func collectPositions(f *parse.File, node yamlast.Node, prefix string, out map[string]diag.Position) {
	mapping, ok := node.(*yamlast.MappingNode)
	if !ok {
		if mv, isValue := node.(*yamlast.MappingValueNode); isValue {
			mapping = &yamlast.MappingNode{Values: []*yamlast.MappingValueNode{mv}}
		} else {
			return
		}
	}
	for _, v := range mapping.Values {
		if v.Key == nil {
			continue
		}
		tok := v.Key.GetToken()
		if tok == nil {
			continue
		}
		key := strings.TrimSpace(v.Key.String())
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		out[path] = f.FrontmatterPos(tok.Position.Line, tok.Position.Column)
		if v.Value != nil {
			collectPositions(f, v.Value, path, out)
		}
	}
}

// checkFrontmatter validates decoded metadata against the schema's field
// declarations.
func checkFrontmatter(f *parse.File, sc *schema.Schema, m *Meta, ds *diag.List) {
	checkFields(f, sc, sc.Frontmatter, m, m.Values, "", ds)
	warnUnknown(f, sc.Frontmatter, m, m.Values, "", ds)
}

func checkFields(f *parse.File, sc *schema.Schema, fields schema.Fields, m *Meta, values map[string]any, prefix string, ds *diag.List) {
	for _, name := range sortedFieldNames(fields) {
		field := fields[name]
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		raw, present := values[name]

		// A declared default fills an absent field, here rather than at render
		// time: it decides whether a required field is actually missing, and it
		// has to be in Meta.Values for the emitter to interpolate it. An
		// explicit null is left alone — on a nullable field that is a real
		// answer, and overwriting it would erase the author's "known absent".
		if field.Default != nil && !present {
			values[name] = field.Default
			raw, present = field.Default, true
		}

		if !present || raw == nil {
			switch {
			case !field.Required:
				continue
			case field.Nullable && present:
				// Explicit null on a nullable field is a real answer.
				continue
			case present:
				ds.Errorf(f.Path, m.Pos(path), "DOC004", hintOr(field, "provide a value"),
					"required field `%s` is empty", path)
			default:
				ds.Errorf(f.Path, posOrParent(m, path, prefix), "DOC004", hintOr(field, "add this field"),
					"missing required field `%s`", path)
			}
			continue
		}
		checkValue(f, sc, field, m, path, raw, ds)
	}
}

func checkValue(f *parse.File, sc *schema.Schema, field schema.Field, m *Meta, path string, raw any, ds *diag.List) {
	pos := m.Pos(path)

	if inner, isList := strings.CutPrefix(field.Type, "list<"); isList {
		elemType := strings.TrimSuffix(inner, ">")
		items, ok := raw.([]any)
		if !ok {
			ds.Errorf(f.Path, pos, "DOC005", hintOr(field, "write this as a YAML list"),
				"field `%s` must be a list, got %s", path, yamlTypeName(raw))
			return
		}
		elem := field
		elem.Type = elemType
		for i, item := range items {
			checkValue(f, sc, elem, m, fmt.Sprintf("%s[%d]", path, i), item, ds)
		}
		return
	}

	if objFields, isNamed := sc.Types[field.Type]; isNamed {
		obj, ok := raw.(map[string]any)
		if !ok {
			ds.Errorf(f.Path, pos, "DOC005", hintOr(field, fmt.Sprintf("expand this into the fields of `%s`", field.Type)),
				"field `%s` must be a `%s` object, got %s", path, field.Type, yamlTypeName(raw))
			return
		}
		checkFields(f, sc, objFields, m, obj, path, ds)
		warnUnknown(f, objFields, m, obj, path, ds)
		return
	}

	switch field.Type {
	case "", "any":
		return

	case "string":
		s, ok := raw.(string)
		if !ok {
			// The classic Swiss postal-code trap: YAML reads 5400 as an integer
			// and 0-prefixed codes lose their leading zero entirely.
			ds.Errorf(f.Path, pos, "DOC006",
				hintOr(field, fmt.Sprintf("quote it: \"%v\"", raw)),
				"field `%s` must be a string, got %s", path, yamlTypeName(raw))
			return
		}
		checkPattern(f, field, m, path, s, ds)

	case "int":
		if _, ok := raw.(uint64); ok {
			return
		}
		if _, ok := raw.(int); !ok {
			if _, ok := raw.(int64); !ok {
				ds.Errorf(f.Path, pos, "DOC006", hintOr(field, "write an unquoted whole number"),
					"field `%s` must be an integer, got %s", path, yamlTypeName(raw))
			}
		}

	case "bool":
		if _, ok := raw.(bool); !ok {
			ds.Errorf(f.Path, pos, "DOC006", hintOr(field, "write `true` or `false`"),
				"field `%s` must be a boolean, got %s", path, yamlTypeName(raw))
		}

	case "date":
		switch v := raw.(type) {
		case time.Time:
		case string:
			if _, err := time.Parse("2006-01-02", v); err != nil {
				ds.Errorf(f.Path, pos, "DOC007", hintOr(field, "use ISO format YYYY-MM-DD, e.g. 2026-08-04"),
					"field `%s` is not a valid date: %q", path, v)
			}
		default:
			ds.Errorf(f.Path, pos, "DOC007", hintOr(field, "use ISO format YYYY-MM-DD"),
				"field `%s` must be a date, got %s", path, yamlTypeName(raw))
		}

	case "enum":
		s, ok := raw.(string)
		if !ok {
			ds.Errorf(f.Path, pos, "DOC006", hintOr(field, "quote the value"),
				"field `%s` must be one of the allowed values, got %s", path, yamlTypeName(raw))
			return
		}
		for _, allowed := range field.Values {
			if s == allowed {
				return
			}
		}
		ds.Errorf(f.Path, pos, "DOC008",
			hintOr(field, "allowed: "+strings.Join(field.Values, ", ")),
			"field `%s` has invalid value %q", path, s)

	default:
		ds.Errorf(f.Path, pos, "DOC009", "fix the schema, not the document",
			"field `%s` declares unknown type %q", path, field.Type)
	}
}

func checkPattern(f *parse.File, field schema.Field, m *Meta, path, value string, ds *diag.List) {
	if field.Pattern == "" {
		return
	}
	re, err := regexp.Compile(field.Pattern)
	if err != nil {
		ds.Errorf(f.Path, m.Pos(path), "DOC009", "fix the schema, not the document",
			"field `%s` declares invalid pattern %q: %v", path, field.Pattern, err)
		return
	}
	if re.MatchString(value) {
		return
	}
	pos := m.Pos(path)
	pos.Len = len(value) + 2 // include the surrounding quotes when present
	ds.Errorf(f.Path, pos, "DOC010",
		hintOr(field, "expected pattern "+field.Pattern),
		"field `%s` has malformed value %q", path, value)
}

// warnUnknown flags fields the schema does not declare. These are usually
// typos, and a typo in an optional field is otherwise completely silent.
func warnUnknown(f *parse.File, fields schema.Fields, m *Meta, values map[string]any, prefix string, ds *diag.List) {
	for _, name := range sortedMapKeys(values) {
		if _, declared := fields[name]; declared {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		hint := "remove it, or add it to the schema"
		if near := nearest(name, fields); near != "" {
			hint = fmt.Sprintf("did you mean `%s`?", near)
		}
		ds.Warnf(f.Path, m.Pos(path), "DOC011", hint, "unknown field `%s`", path)
	}
}

// posOrParent anchors a missing-field diagnostic on the enclosing object, which
// is the nearest thing to the absent key that actually exists in the source.
func posOrParent(m *Meta, path, prefix string) diag.Position {
	if p, ok := m.pos[path]; ok {
		return p
	}
	if prefix != "" {
		if p, ok := m.pos[prefix]; ok {
			return p
		}
	}
	return diag.Position{}
}

func hintOr(field schema.Field, fallback string) string {
	if field.Hint != "" {
		return field.Hint
	}
	return fallback
}

func yamlTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int64, uint64, float64:
		return "number"
	case []any:
		return "list"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

var yamlPosRe = regexp.MustCompile(`\[(\d+):(\d+)\]`)

// yamlErrorPos digs a line and column out of a goccy error string. goccy has no
// stable typed accessor for this, so a miss falls back to the block start.
func yamlErrorPos(err error) (line, col int) {
	if mm := yamlPosRe.FindStringSubmatch(err.Error()); mm != nil {
		return atoi(mm[1]), atoi(mm[2])
	}
	return 1, 1
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
