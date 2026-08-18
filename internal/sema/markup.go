package sema

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// checkMarkup validates the semantic markup — fenced blocks and inline spans —
// against the schema's `blocks:` and `spans:` declarations.
//
// A schema that declares no blocks leaves divs unchecked, and likewise for
// spans: the declarations are the opt-in, so existing schemas keep their
// meaning until they adopt the contract.
func checkMarkup(f *parse.File, sc *schema.Schema, ds *diag.List) {
	checkUniqueIDs(f, ds)
	checkRefs(f, ds)
	checkDocFields(f, sc, ds)
	if len(sc.Blocks) > 0 {
		checkBlocks(f, sc, ds)
	}
	if len(sc.Spans) > 0 {
		checkSpans(f, sc, ds)
	}
}

// checkUniqueIDs reports every reuse of a `#id`. This runs regardless of
// declarations: two regions with one name is meaningless under any schema, and
// references will resolve against these ids.
func checkUniqueIDs(f *parse.File, ds *diag.List) {
	first := map[string]*parse.Div{}
	for _, div := range f.Divs() {
		id := div.Attr.ID
		if id == "" {
			continue
		}
		prev, dup := first[id]
		if !dup {
			first[id] = div
			continue
		}
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: idPos(f, div), Severity: diag.Error, Code: "DOC034",
			Message: fmt.Sprintf("block id %q is already used by `::: %s`", id, prev.Name),
			Hint:    "give every block a unique `#id`; references resolve against it",
			Block:   id,
			Related: []diag.Location{{File: f.Path, Pos: idPos(f, prev)}},
		})
	}
}

// checkRefs resolves every span `ref=` against the document's block ids. Like
// id uniqueness this runs regardless of schema declarations: a reference the
// author wrote is a reference the author wants resolved — checking it is
// resolution, not type validation.
func checkRefs(f *parse.File, ds *diag.List) {
	ids := map[string]bool{}
	for _, div := range f.Divs() {
		if div.Attr.ID != "" {
			ids[div.Attr.ID] = true
		}
	}
	for _, span := range f.Spans() {
		ref, ok := span.Attr.Get("ref")
		if !ok || ids[ref] {
			continue
		}
		hint := "no block declares an id; add `{#" + ref + "}` to the block this refers to"
		expected := ""
		if len(ids) > 0 {
			known := sortedMapKeys(ids)
			hint = "known ids: #" + strings.Join(known, ", #")
			expected = fmt.Sprintf("[%s]{.%s ref=%s}", span.LiteralText(f.BodySource), span.SpanType(), known[0])
		}
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: refPos(f, span), Severity: diag.Error, Code: "DOC037",
			Message:  fmt.Sprintf("reference %q does not resolve to any block id", ref),
			Hint:     hint,
			Expected: expected,
		})
	}
}

func checkBlocks(f *parse.File, sc *schema.Schema, ds *diag.List) {
	for _, div := range f.Divs() {
		spec, known := sc.Blocks[div.Name]
		if !known {
			ds.Add(diag.Diagnostic{
				File: f.Path, Pos: f.BodyPos(div.OpenOffset), Severity: diag.Error, Code: "DOC030",
				Message: fmt.Sprintf("schema %q does not declare a block %q", sc.Type, div.Name),
				Hint:    "declared blocks: " + strings.Join(sortedMapKeys(sc.Blocks), ", "),
				Block:   div.Name,
			})
			continue
		}
		checkBlockAttrs(f, div, spec, ds)
		required := spec.RequiredSpans
		if len(spec.Variants) > 0 {
			variant, ok := checkDiscriminator(f, sc, div, spec, ds)
			if !ok {
				continue
			}
			required = variant.RequiredSpans
		}
		checkRequiredSpans(f, div, required, ds)
	}
}

// checkBlockAttrs reports attribute keys the block does not declare. `#id` is
// always permitted; so is the discriminator.
func checkBlockAttrs(f *parse.File, div *parse.Div, spec schema.BlockSpec, ds *diag.List) {
	permitted := map[string]bool{}
	if spec.Discriminator != "" {
		permitted[spec.Discriminator] = true
	}
	for _, a := range spec.Attributes {
		permitted[a] = true
	}
	for _, a := range div.Attr.Attrs {
		if permitted[a.Key] {
			continue
		}
		hint := "remove the attribute"
		if len(permitted) > 0 {
			hint = "permitted attributes: " + strings.Join(sortedMapKeys(permitted), ", ")
		}
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: attrPos(f, a), Severity: diag.Error, Code: "DOC035",
			Message: fmt.Sprintf("block %q does not permit the attribute %q", div.Name, a.Key),
			Hint:    hint,
			Block:   blockLabel(div),
		})
	}
}

// checkDiscriminator resolves which variant a block uses.
func checkDiscriminator(f *parse.File, sc *schema.Schema, div *parse.Div, spec schema.BlockSpec, ds *diag.List) (schema.BlockVariant, bool) {
	if spec.Discriminator == "" {
		// A spec with variants but no discriminator cannot be satisfied by any
		// document; that is the schema author's bug, not the document's.
		ds.Errorf(f.Path, diag.Position{}, "DOC036",
			fmt.Sprintf("add `discriminator: <attribute>` to block %q in schema %q", div.Name, sc.Type),
			"schema %q declares variants for block %q but no discriminator", sc.Type, div.Name)
		return schema.BlockVariant{}, false
	}
	value, ok := div.Attr.Get(spec.Discriminator)
	if !ok {
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: f.BodyPos(div.OpenOffset), Severity: diag.Error, Code: "DOC032",
			Message: fmt.Sprintf("block %q is missing its %q attribute", div.Name, spec.Discriminator),
			Hint:    fmt.Sprintf("one of: %s", strings.Join(sortedMapKeys(spec.Variants), ", ")),
			Expected: fmt.Sprintf("::: %s {%s=%s}", div.Name, spec.Discriminator,
				sortedMapKeys(spec.Variants)[0]),
			Block: blockLabel(div),
		})
		return schema.BlockVariant{}, false
	}
	variant, known := spec.Variants[value]
	if !known {
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: f.BodyPos(div.OpenOffset), Severity: diag.Error, Code: "DOC032",
			Message: fmt.Sprintf("block %q has no variant %s=%q", div.Name, spec.Discriminator, value),
			Hint:    fmt.Sprintf("one of: %s", strings.Join(sortedMapKeys(spec.Variants), ", ")),
			Block:   blockLabel(div),
		})
		return schema.BlockVariant{}, false
	}
	return variant, true
}

// checkRequiredSpans verifies that every span type the block (or its variant)
// requires appears somewhere inside the block.
func checkRequiredSpans(f *parse.File, div *parse.Div, required []string, ds *diag.List) {
	present := map[string]bool{}
	for _, span := range spansIn(div) {
		present[span.SpanType()] = true
	}
	for _, want := range required {
		if present[want] {
			continue
		}
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: f.BodyPos(div.OpenOffset), Severity: diag.Error, Code: "DOC033",
			Message:  fmt.Sprintf("block %q is missing a required `.%s` span", blockLabel(div), want),
			Hint:     fmt.Sprintf("annotate the value inside the block: `[...]{.%s}`", want),
			Expected: fmt.Sprintf("[...]{.%s}", want),
			Block:    blockLabel(div),
		})
	}
}

func checkSpans(f *parse.File, sc *schema.Schema, ds *diag.List) {
	present := map[string]bool{}
	for _, span := range f.Spans() {
		name := span.SpanType()
		present[name] = true
		// `docc-` types are compiler-owned and need no declaration.
		if strings.HasPrefix(name, "docc-") {
			continue
		}
		if name == "" {
			ds.Add(diag.Diagnostic{
				File: f.Path, Pos: spanPos(f, span), Severity: diag.Error, Code: "DOC031",
				Message:  "span has no type class",
				Hint:     "declared span types: " + strings.Join(sortedMapKeys(sc.Spans), ", "),
				Expected: fmt.Sprintf("[%s]{.%s}", span.LiteralText(f.BodySource), sortedMapKeys(sc.Spans)[0]),
			})
			continue
		}
		if _, known := sc.Spans[name]; !known {
			ds.Add(diag.Diagnostic{
				File: f.Path, Pos: classPos(f, span), Severity: diag.Error, Code: "DOC031",
				Message: fmt.Sprintf("schema %q does not declare a span type %q", sc.Type, name),
				Hint:    "declared span types: " + strings.Join(sortedMapKeys(sc.Spans), ", "),
			})
		}
	}

	// An absence has no line to underline, so this is a file-level diagnostic
	// rather than a caret under something unrelated. The `Expected` line is
	// what the author is missing, which is the actionable half.
	for _, name := range sortedMapKeys(sc.Spans) {
		if !sc.Spans[name].Required || present[name] {
			continue
		}
		ds.Add(diag.Diagnostic{
			File: f.Path, Pos: diag.Position{}, Severity: diag.Error, Code: "DOC042",
			Message:  fmt.Sprintf("no `.%s` appears in the document, and the schema requires one", name),
			Hint:     "state the value in the prose and mark it: " + sc.Spans[name].Description,
			Expected: fmt.Sprintf("[text]{.%s}", name),
		})
	}
}

// spansIn collects every span in the block's subtree, at any depth — a span in
// a list item inside the block still belongs to the block.
func spansIn(div *parse.Div) []*parse.Span {
	var out []*parse.Span
	_ = ast.Walk(div, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if s, isSpan := n.(*parse.Span); isSpan {
				out = append(out, s)
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// blockLabel identifies a block in messages: its id when it has one, else its
// kind. "erwerberin" reads better than "the third partei block".
func blockLabel(div *parse.Div) string {
	if div.Attr.ID != "" {
		return div.Attr.ID
	}
	return div.Name
}

func idPos(f *parse.File, div *parse.Div) diag.Position {
	pos := f.BodyPos(div.Attr.IDOffset)
	pos.Len = len(div.Attr.ID) + 1
	return pos
}

func attrPos(f *parse.File, a parse.Attr) diag.Position {
	pos := f.BodyPos(a.KeyOffset)
	pos.Len = len(a.Key)
	return pos
}

func spanPos(f *parse.File, s *parse.Span) diag.Position {
	pos := f.BodyPos(s.OpenOffset)
	pos.Len = s.Literal.Stop - s.Literal.Start + 2
	return pos
}

// refPos underlines the ref value itself.
func refPos(f *parse.File, s *parse.Span) diag.Position {
	for _, a := range s.Attr.Attrs {
		if a.Key == "ref" {
			pos := f.BodyPos(a.ValueOffset)
			pos.Len = len(a.Value)
			return pos
		}
	}
	return f.BodyPos(s.OpenOffset)
}

func classPos(f *parse.File, s *parse.Span) diag.Position {
	pos := f.BodyPos(s.Attr.Classes[0].Offset)
	pos.Len = len(s.Attr.Classes[0].Name) + 1
	return pos
}
