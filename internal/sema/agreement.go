package sema

// Cross-document consistency.
//
// The dossier problem, stated plainly: a GmbH founding is six documents that
// restate the same Firma, Sitz and Stammkapital, and nothing made them agree.
// The obvious fix is templating — one input, six outputs, values substituted
// in — and it is the wrong one for this corpus. A deed is read in context, and
// an author who writes "unter der Firma Motherstuhl GmbH" wants to see the
// company's name in that sentence while writing it, not `{{ firma }}`.
//
// So the author still writes every occurrence, and this supplies the guarantee
// the template would have given for free: the occurrences may not drift apart.
// Verification rather than substitution, which is also the stricter of the two
// — a template makes the six agree with each other, and says nothing about
// whether any of them is right.
//
// Within one document this is the `spans_agree` check. Across a set of
// documents checked together it is CrossFileDisagreements, which needs no new
// input format and no notion of a dossier: `docc check *.md` already has every
// file open at once.

import (
	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// SpanOccurrence is one appearance of a watched span type, kept so documents
// checked in one invocation can be compared against each other.
type SpanOccurrence struct {
	Type  string
	Value string
	File  string
	Pos   diag.Position
}

// WatchedSpanTypes returns the span types this schema's `spans_agree` rules
// watch. A type nobody watches is one that may legitimately differ, so the
// union of the rules is exactly the set worth comparing across files.
func WatchedSpanTypes(sc *schema.Schema) []string {
	var out []string
	seen := map[string]bool{}
	for _, rule := range sc.Rules {
		if rule.Check != "spans_agree" {
			continue
		}
		items, ok := rule.Args["spans"].([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			name, isStr := item.(string)
			if !isStr || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// WatchedSpanValues collects the occurrences of every watched span type in one
// document.
func WatchedSpanValues(f *parse.File, sc *schema.Schema) []SpanOccurrence {
	types := WatchedSpanTypes(sc)
	if len(types) == 0 {
		return nil
	}
	watched := make(map[string]bool, len(types))
	for _, name := range types {
		watched[name] = true
	}

	var out []SpanOccurrence
	for _, span := range f.Spans() {
		typ := span.SpanType()
		if !watched[typ] {
			continue
		}
		value := normalizeSpanValue(span.LiteralText(f.BodySource))
		if value == "" {
			continue // a blank is no_blank_spans' business
		}
		pos := f.BodyPos(span.Literal.Start)
		pos.Len = span.Literal.Stop - span.Literal.Start
		out = append(out, SpanOccurrence{Type: typ, Value: value, File: f.Path, Pos: pos})
	}
	return out
}

// CrossFileDisagreements reports a watched span type that says one thing in one
// document and another in the next.
//
// The first file to state a value sets it, and every later file that disagrees
// is reported once — not once per occurrence, because a Firma written five
// times in a deed is one mistake, not five, and five carets on one sheet
// obscure the one line worth looking at.
//
// A warning rather than an error: two documents checked together are not
// necessarily the same transaction. `--strict` makes it bind, which is what a
// dossier being filed should run.
func CrossFileDisagreements(occurrences []SpanOccurrence) diag.List {
	var ds diag.List

	type anchor struct {
		value string
		file  string
		line  int
	}
	first := map[string]anchor{}
	reported := map[string]bool{}

	for _, occ := range occurrences {
		prior, ok := first[occ.Type]
		if !ok {
			first[occ.Type] = anchor{value: occ.Value, file: occ.File, line: occ.Pos.Line}
			continue
		}
		if occ.Value == prior.value || occ.File == prior.file {
			continue
		}
		key := occ.File + "\x00" + occ.Type
		if reported[key] {
			continue
		}
		reported[key] = true

		ds.Warnf(occ.File, occ.Pos, "DOC029",
			"make them the same, or check the documents separately if they are different matters",
			"`.%s` says %q here but %q in %s:%d",
			occ.Type, occ.Value, prior.value, prior.file, prior.line)
	}
	return ds
}
