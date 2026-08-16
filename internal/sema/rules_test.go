package sema

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// run applies one rule to one source and returns what it found.
func run(t *testing.T, src string, rule schema.Rule) diag.List {
	t.Helper()
	f, ds := parse.Parse("test.md", []byte(src))
	sc := &schema.Schema{Type: "test", Rules: []schema.Rule{rule}}
	m := decodeMeta(f, &ds)
	runRules(f, sc, m, &ds)
	return ds
}

func codes(ds diag.List) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Code)
	}
	return out
}

func messages(ds diag.List) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.Code + ": " + d.Message + " | " + d.Hint + "\n")
	}
	return b.String()
}

const evidenceDoc = `---
document_type: test
attachments:
  - Contract
  - Minutes
---

# Facts

::: evidence
- Contract of 3 March // Exhibit 1
- Correspondence between the parties
:::
`

// A check that names an argument the schema never supplied is a defect in the
// schema, and must say so rather than silently doing nothing.
func TestMissingArgIsSchemaError(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{ID: "X001", Check: "div_items_match"})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC009" {
		t.Fatalf("codes = %v, want [DOC009]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Message, `"div"`) {
		t.Errorf("message does not name the missing argument: %q", ds[0].Message)
	}
}

func TestRequiredDivReportsAtConfiguredHeading(t *testing.T) {
	src := `---
document_type: test
---

# Grundstückbeschrieb

Kein Auszug.
`
	ds := run(t, src, schema.Rule{
		ID:    "X002",
		Check: "required_div",
		Args:  map[string]any{"div": "grundstueck", "anchor_heading": "Grundstückbeschrieb"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "X002" {
		t.Fatalf("codes = %v, want [X002]\n%s", got, messages(ds))
	}
	if got := ds[0].Pos.Line; got != 5 {
		t.Errorf("diagnostic line = %d, want 5", got)
	}
}

func TestRequiredDivAcceptsPresentBlock(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X003",
		Check: "required_div",
		Args:  map[string]any{"div": "evidence"},
	})
	if len(ds) != 0 {
		t.Fatalf("present div reported:\n%s", messages(ds))
	}
}

func TestMalformedPatternIsSchemaError(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X002",
		Check: "div_items_match",
		Args:  map[string]any{"div": "evidence", "pattern": "([unclosed"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC009" {
		t.Fatalf("codes = %v, want [DOC009]\n%s", got, messages(ds))
	}
}

func TestNonStringArgIsSchemaError(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X003",
		Check: "div_items_match",
		Args:  map[string]any{"div": 42, "pattern": "x"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC009" {
		t.Fatalf("codes = %v, want [DOC009]\n%s", got, messages(ds))
	}
}

// The div name comes from the schema, so a rule aimed at another div must leave
// this one alone.
func TestDivItemsMatchOnlyItsOwnDiv(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X004",
		Check: "div_items_match",
		Args:  map[string]any{"div": "somethingelse", "pattern": `// Exhibit \d+$`},
	})
	if len(ds) != 0 {
		t.Fatalf("want no diagnostics, got:\n%s", messages(ds))
	}
}

func TestDivItemsMatchFlagsUnmatchedItem(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X005",
		Check: "div_items_match",
		Args:  map[string]any{"div": "evidence", "pattern": `//\s*Exhibit\s+\d+\s*$`},
	})
	if len(ds) != 1 {
		t.Fatalf("want 1 diagnostic, got:\n%s", messages(ds))
	}
	if ds[0].Code != "X005" {
		t.Errorf("code = %q, want X005", ds[0].Code)
	}
	if ds[0].Pos.Line != 12 {
		t.Errorf("line = %d, want 12 (the unreferenced item)", ds[0].Pos.Line)
	}
}

func TestDivItemsMatchTreatsWrappedListItemAsOneEvidence(t *testing.T) {
	src := `---
document_type: test
attachments:
  - Contract
---

::: evidence
- [Beilage 1] Contract of 3
  March
- [Augenschein] The disputed property
:::
`
	ds := run(t, src, schema.Rule{
		ID:    "X010",
		Check: "div_items_match",
		Args: map[string]any{
			"div":     "evidence",
			"pattern": `^\s*\[[^\]\r\n]+\]\s+\S`,
		},
	})
	if len(ds) != 0 {
		t.Fatalf("wrapped labelled evidence should pass, got:\n%s", messages(ds))
	}

	ds = run(t, src, schema.Rule{
		ID:    "X011",
		Check: "cross_reference",
		Args: map[string]any{
			"div":        "evidence",
			"pattern":    `(?i)^\s*\[Beilage\s+(\d+)\]`,
			"list_field": "attachments",
			"label":      "Beilage",
		},
	})
	if len(ds) != 0 {
		t.Fatalf("only [Beilage N] should cross-reference attachments, got:\n%s", messages(ds))
	}
}

func TestDivItemsMatchRequiresLabel(t *testing.T) {
	src := `---
document_type: test
---

::: evidence
- Contract of 3 March
:::
`
	ds := run(t, src, schema.Rule{
		ID:    "X012",
		Check: "div_items_match",
		Args: map[string]any{
			"div":     "evidence",
			"pattern": `^\s*\[[^\]\r\n]+\]\s+\S`,
		},
	})
	if len(ds) != 1 {
		t.Fatalf("want one unlabelled-evidence diagnostic, got:\n%s", messages(ds))
	}
	if ds[0].Pos.Line != 6 {
		t.Errorf("line = %d, want 6", ds[0].Pos.Line)
	}
}

// Both directions: a key cited but not listed, and one listed but never cited.
func TestCrossReferenceBothDirections(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:       "X006",
		Check:    "cross_reference",
		Severity: "warning",
		Args: map[string]any{
			"div":        "evidence",
			"pattern":    `//\s*Exhibit\s+(\d+)`,
			"list_field": "attachments",
			"label":      "Exhibit",
		},
	})
	if len(ds) != 1 {
		t.Fatalf("want 1 diagnostic, got:\n%s", messages(ds))
	}
	if !strings.Contains(ds[0].Message, "Exhibit 2 is listed") {
		t.Errorf("message = %q, want the uncited entry 2", ds[0].Message)
	}

	// Cite an exhibit that the frontmatter does not list.
	src := strings.Replace(evidenceDoc, "// Exhibit 1", "// Exhibit 7", 1)
	ds = run(t, src, schema.Rule{
		ID:       "X006",
		Check:    "cross_reference",
		Severity: "warning",
		Args: map[string]any{
			"div":        "evidence",
			"pattern":    `//\s*Exhibit\s+(\d+)`,
			"list_field": "attachments",
			"label":      "Exhibit",
		},
	})
	if len(ds) != 3 {
		t.Fatalf("want 3 diagnostics (entries 1 and 2 uncited, exhibit 7 unlisted), got:\n%s", messages(ds))
	}
	if !strings.Contains(messages(ds), "Exhibit 7 is cited in the body but not listed") {
		t.Errorf("missing the cited-but-unlisted finding:\n%s", messages(ds))
	}
}

// A pattern without a capture group cannot name the key it is meant to match.
func TestCrossReferenceNeedsCaptureGroup(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X007",
		Check: "cross_reference",
		Args: map[string]any{
			"div":        "evidence",
			"pattern":    `//\s*Exhibit\s+\d+`,
			"list_field": "attachments",
		},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC009" {
		t.Fatalf("codes = %v, want [DOC009]\n%s", got, messages(ds))
	}
}

const placeholderDoc = `---
document_type: test
---

# Facts

- [describe what happened]
- see the [handbook](https://example.ch/handbook)
- TODO: write this
`

func TestNoPlaceholdersDefaultPattern(t *testing.T) {
	ds := run(t, placeholderDoc, schema.Rule{ID: "X008", Check: "no_placeholder_text"})
	if len(ds) != 1 {
		t.Fatalf("want 1 diagnostic, got:\n%s", messages(ds))
	}
	// The caret underlines the placeholder, not the list marker before it.
	if ds[0].Pos.Line != 7 || ds[0].Pos.Col != 3 {
		t.Errorf("pos = %d:%d, want 7:3", ds[0].Pos.Line, ds[0].Pos.Col)
	}
	if ds[0].Pos.Len != len("[describe what happened]") {
		t.Errorf("len = %d, want %d", ds[0].Pos.Len, len("[describe what happened]"))
	}
}

func TestNoPlaceholdersCustomPattern(t *testing.T) {
	ds := run(t, placeholderDoc, schema.Rule{
		ID:    "X009",
		Check: "no_placeholder_text",
		Args:  map[string]any{"pattern": `^\s*-\s+(TODO:.*)$`},
	})
	if len(ds) != 1 {
		t.Fatalf("want 1 diagnostic, got:\n%s", messages(ds))
	}
	if ds[0].Pos.Line != 9 {
		t.Errorf("line = %d, want 9", ds[0].Pos.Line)
	}
}

// A `fields:` blank is content; a semantic span's blank is a missing fact.
// Without this check a row of underscores satisfied `required_spans` exactly
// as well as a name did — found by drafting a real founding whose Heimatort
// nobody had looked up, which passed `check` in two documents.
func TestNoBlankSpans(t *testing.T) {
	sc := &schema.Schema{
		Type: "deed",
		Spans: map[string]schema.SpanSpec{
			"heimatort": {}, "name": {},
		},
		Fields: map[string]schema.FieldSpec{
			"datum": {Completion: "handwritten"},
		},
		Rules: []schema.Rule{{ID: "X060", Check: "no_blank_spans", Severity: "error"}},
	}

	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"underscores are a blank", "von [__________]{.heimatort}.", true},
		{"dots are a blank", "von [......]{.heimatort}.", true},
		{"an empty span is a blank", "von []{.heimatort}.", true},
		{"a real value is not", "von [Baden AG]{.heimatort}.", false},
		{"a value containing a dash is not", "von [Rüti-Winkel]{.heimatort}.", false},
		// The exemption that makes the two mechanisms coexist.
		{"a field blank is content", "am [______]{.docc-field key=datum}.", false},
		{"a field blank behind a type is content", "von [____]{.heimatort .docc-field key=datum}.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "---\ndocc: 1\ndocument_type: deed\n---\n\n" + tc.body + "\n"
			f, ds := parse.Parse("deed.md", []byte(src))
			runRules(f, sc, nil, &ds)

			var got bool
			for _, d := range ds {
				if d.Code == "X060" {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("blank-span reported = %v, want %v (%q)\n%+v", got, tc.want, tc.body, ds)
			}
		})
	}
}
