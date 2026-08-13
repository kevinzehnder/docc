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

func TestCheckRandzifferSequence(t *testing.T) {
	body := func(paras ...string) string {
		return "---\ndocument_type: legal_reference\n---\n\n" + strings.Join(paras, "\n\n") + "\n"
	}
	rule := schema.Rule{ID: "REF010", Check: "randziffer_sequence"}

	tests := []struct {
		name    string
		src     string
		wantErr int
		wantIn  string
	}{
		{
			name: "unbroken sequence passes",
			src:  body("[Rz 1] Erste Erwägung.", "[Rz 2] Zweite Erwägung.", "[Rz 3] Dritte Erwägung."),
		},
		{
			// An extract legitimately begins partway through the document.
			name: "sequence may start anywhere",
			src:  body("[Rz 55] Erste Erwägung dieses Auszugs.", "[Rz 56] Zweite Erwägung."),
		},
		{
			name:    "a gap means lost text",
			src:     body("[Rz 1] Erste Erwägung.", "[Rz 7] Siebte Erwägung."),
			wantErr: 1,
			wantIn:  "skipping 5",
		},
		{
			name:    "a repeat means two paragraphs were merged",
			src:     body("[Rz 4] Vierte Erwägung.", "[Rz 4] Nochmals vier."),
			wantErr: 1,
			wantIn:  "repeats the previous one",
		},
		{
			name:    "going backwards means pages were reordered",
			src:     body("[Rz 9] Neunte Erwägung.", "[Rz 3] Dritte Erwägung."),
			wantErr: 1,
			wantIn:  "goes backwards",
		},
		{
			// `[Rz 9]: url` is a markdown link reference definition, not a
			// paragraph number, so it must not join the sequence.
			name: "a link reference definition is not a paragraph number",
			src:  body("[Rz 1] Erste Erwägung.", "[Rz 9]: https://example.com"),
		},
		{
			name: "a document with no markers is not a sequence",
			src:  body("Erste Erwägung ohne Nummer.", "Zweite Erwägung."),
		},
		{
			// The marker only counts at the start of a paragraph; a citation
			// inside running text is the source document quoting itself.
			name: "a citation inside a sentence is left alone",
			src:  body("[Rz 1] Wie bereits in [Rz 9] dargelegt, ist dies unzutreffend."),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, d := range run(t, tt.src, rule) {
				if d.Code == "REF010" {
					got = append(got, d.Message)
				}
			}
			if len(got) != tt.wantErr {
				t.Fatalf("REF010 count = %d, want %d: %v", len(got), tt.wantErr, got)
			}
			if tt.wantIn != "" && !strings.Contains(got[0], tt.wantIn) {
				t.Errorf("message = %q, want it to contain %q", got[0], tt.wantIn)
			}
		})
	}
}
