package sema

import (
	"fmt"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// FieldSpanType is the reserved span type for intentionally incomplete
// fields. The `docc-` prefix marks compiler-owned span types: they need no
// `spans:` declaration and schemas cannot redefine them.
const FieldSpanType = "docc-field"

// completionHandwritten may stay blank through build — a human fills it in on
// paper. Everything else must be completed before the document is built.
const completionHandwritten = "handwritten"

const completionBeforeExecution = "before-execution"

// checkDocFields validates spans carrying `.docc-field` at check time: every
// field span carries a key, declared fields exist where required, and the schema's
// completion values are ones the compiler knows. Whether a blank is *allowed*
// is build business — see CheckCompletion.
func checkDocFields(f *parse.File, sc *schema.Schema, ds *diag.List) {
	present := map[string]bool{}
	for _, span := range fieldSpans(f) {
		key, ok := span.Attr.Get("key")
		if !ok || key == "" {
			ds.Add(diag.Diagnostic{
				File: f.Path, Pos: spanPos(f, span), Severity: diag.Error, Code: "DOC040",
				Message:  "field span has no key",
				Hint:     "the key names the field: `key=<name>`",
				Expected: fmt.Sprintf("[____________]{.%s key=<name>}", FieldSpanType),
			})
			continue
		}
		present[key] = true
		if len(sc.Blanks) > 0 {
			if _, declared := sc.Blanks[key]; !declared {
				ds.Add(diag.Diagnostic{
					File: f.Path, Pos: spanPos(f, span), Severity: diag.Error, Code: "DOC040",
					Message: fmt.Sprintf("schema %q does not declare a field %q", sc.Type, key),
					Hint:    "declared fields: " + strings.Join(sortedMapKeys(sc.Blanks), ", "),
					Key:     key,
				})
			}
		}
	}

	for _, name := range sortedMapKeys(sc.Blanks) {
		spec := sc.Blanks[name]
		switch spec.Completion {
		case "", completionHandwritten, completionBeforeExecution:
		default:
			ds.Errorf(f.Path, diag.Position{}, "DOC041",
				fmt.Sprintf("use `%s` or `%s`", completionHandwritten, completionBeforeExecution),
				"schema %q declares field %q with unknown completion %q",
				sc.Type, name, spec.Completion)
		}
		if spec.Required && !present[name] {
			ds.Add(diag.Diagnostic{
				File: f.Path, Pos: diag.Position{}, Severity: diag.Error, Code: "DOC038",
				Message:  fmt.Sprintf("required field %q does not appear in the document", name),
				Hint:     "a blank is content: write it visibly and annotate it",
				Expected: fmt.Sprintf("[____________]{.%s key=%s}", FieldSpanType, name),
				Key:      name,
			})
		}
	}
}

// CheckCompletion is the build-stage half of field checking: a blank field
// whose completion is not "handwritten" must not reach a rendered document.
// `check` accepts these blanks — drafting with them is the point — so the
// caller invokes this only when actually building.
func CheckCompletion(f *parse.File, sc *schema.Schema, ds *diag.List) {
	if len(sc.Blanks) == 0 {
		return
	}
	for _, span := range fieldSpans(f) {
		key, ok := span.Attr.Get("key")
		if !ok {
			continue // already DOC040 at check time
		}
		spec, declared := sc.Blanks[key]
		if !declared || spec.Completion == completionHandwritten {
			continue
		}
		if !IsBlank(span.LiteralText(f.BodySource)) {
			continue
		}
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: spanPos(f, span), Severity: diag.Error, Code: "DOC039",
			Message: fmt.Sprintf("field %q is blank but must be completed before the document is built", key),
			Hint: fmt.Sprintf("fill in the value, or declare `completion: %s` if it is completed on paper",
				completionHandwritten),
			Key: key,
		})
	}
}

// fieldSpans returns every span carrying `.docc-field` in document order.
// The marker can be a second class after a semantic type, for example
// `[SIX SIS AG]{.glaeubiger .docc-field key=glaeubiger_name}`.
func fieldSpans(f *parse.File) []*parse.Span {
	var out []*parse.Span
	for _, span := range f.Spans() {
		if span.HasClass(FieldSpanType) {
			out = append(out, span)
		}
	}
	return out
}

// IsBlank reports whether a field's literal is an unfilled blank: empty, or
// nothing but underscores and spaces. Exported because `docc read` must make
// the same call when it reports a field as blank rather than filled.
func IsBlank(literal string) bool {
	for _, r := range literal {
		if r != '_' && r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}
