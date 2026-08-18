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

// The mirror of TestMissingArgIsSchemaError: an argument the check never reads
// is just as much a schema defect, and used to be silent. `anchour_heading`
// passes every loader — `args:` is the one map yaml.Strict() cannot see into —
// and the rule then anchors somewhere the author never chose.
func TestUnknownArgIsSchemaError(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X001",
		Check: "required_div",
		Args:  map[string]any{"div": "evidence", "anchour_heading": "Facts"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC009" {
		t.Fatalf("codes = %v, want [DOC009]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Message, `"anchour_heading"`) {
		t.Errorf("message does not name the argument: %q", ds[0].Message)
	}
	if !strings.Contains(ds[0].Hint, "anchor_heading") {
		t.Errorf("hint does not list what the check reads: %q", ds[0].Hint)
	}
}

// A check with no arguments at all says so, rather than listing nothing.
func TestUnknownArgOnArglessCheck(t *testing.T) {
	ds := run(t, evidenceDoc, schema.Rule{
		ID:    "X002",
		Check: "no_empty_sections",
		Args:  map[string]any{"div": "evidence"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC009" {
		t.Fatalf("codes = %v, want [DOC009]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Hint, "takes no arguments") {
		t.Errorf("hint = %q", ds[0].Hint)
	}
}

// Every registered check needs a description and an argument list beside it,
// because both are read as documentation: `docc describe` prints the first and
// the schema author is told the second when they mistype a key.
func TestRegistryTablesCoverEveryCheck(t *testing.T) {
	for name := range registry {
		if checkDescriptions[name] == "" {
			t.Errorf("check %q has no entry in checkDescriptions", name)
		}
		if _, ok := checkArgs[name]; !ok {
			t.Errorf("check %q has no entry in checkArgs", name)
		}
	}
	for name := range checkArgs {
		if _, ok := registry[name]; !ok {
			t.Errorf("checkArgs names %q, which is not a registered check", name)
		}
	}
	for name := range checkDescriptions {
		if _, ok := registry[name]; !ok {
			t.Errorf("checkDescriptions names %q, which is not a registered check", name)
		}
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
		Blanks: map[string]schema.FieldSpec{
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

// Opt-in per span type, because some types are supposed to differ: a
// Kaufvertrag's `.name` spans are the Verkäufer and the Käufer.
func TestSpansAgree(t *testing.T) {
	sc := &schema.Schema{
		Type:  "deed",
		Spans: map[string]schema.SpanSpec{"firma": {}, "name": {}},
		Rules: []schema.Rule{{
			ID: "X070", Check: "spans_agree", Severity: "warning",
			Args: map[string]any{"spans": []any{"firma"}},
		}},
	}

	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"a Firma spelled two ways", "[Motherstuhl]{.firma} und [Mutterstuhl]{.firma}.", true},
		{"the same Firma twice", "[Motherstuhl]{.firma} und [Motherstuhl]{.firma}.", false},
		{"line breaks are not disagreement", "[Motherstuhl]{.firma} und [Motherstuhl]{.firma}.", false},
		// The reason the check is opt-in rather than automatic.
		{"an unwatched type may differ", "[Anna]{.name} und [Peter]{.name}.", false},
		// A blank is not a value, here as in no_blank_spans. `docc example
		// --blank` hands out a skeleton full of these, and comparing one
		// against a filled occurrence made the tool warn on its own output.
		{
			"a field blank beside a value",
			"[____]{.firma .docc-field key=firma} und [Motherstuhl]{.firma}.",
			false,
		},
		{
			"a bare blank beside a value",
			"[________]{.firma} und [Motherstuhl]{.firma}.",
			false,
		},
		{
			"two field blanks",
			"[____]{.firma .docc-field key=firma} und [______]{.firma .docc-field key=firma}.",
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "---\ndocc: 1\ndocument_type: deed\n---\n\n" + tc.body + "\n"
			f, ds := parse.Parse("deed.md", []byte(src))
			runRules(f, sc, nil, &ds)

			var got bool
			for _, d := range ds {
				if d.Code == "X070" {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("disagreement reported = %v, want %v (%q)\n%+v", got, tc.want, tc.body, ds)
			}
		})
	}
}

// The one error every other check accepts: a figure transcribed wrongly but
// transcribed consistently. It balances, it agrees across files, it fills
// every blank — and it is below the statutory floor.
func TestAmountAtLeast(t *testing.T) {
	sc := &schema.Schema{
		Type:   "deed",
		Blocks: map[string]schema.BlockSpec{"betraege": {}},
		Rules: []schema.Rule{{
			ID: "X080", Check: "amount_at_least", Severity: "error",
			Args: map[string]any{"div": "betraege", "minimum": "Fr. 20'000.00"},
		}},
	}

	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{
			"a declared total below the floor",
			"::: betraege\n- [Fr. 5'000.00] gezeichnet\n- [= Fr. 5'000.00] Stammkapital\n:::",
			true,
		},
		{
			"a declared total at the floor",
			"::: betraege\n- [Fr. 20'000.00] gezeichnet\n- [= Fr. 20'000.00] Stammkapital\n:::",
			false,
		},
		{
			"above the floor",
			"::: betraege\n- [Fr. 50'000.00] gezeichnet\n- [= Fr. 50'000.00] Stammkapital\n:::",
			false,
		},
		// A block need not declare a total; the sum of its items is one.
		{
			"a summed total below the floor",
			"::: betraege\n- [Fr. 6'000.00] eine Einlage\n- [Fr. 4'000.00] noch eine\n:::",
			true,
		},
		{
			"a summed total that clears it",
			"::: betraege\n- [Fr. 12'000.00] eine Einlage\n- [Fr. 8'000.00] noch eine\n:::",
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "---\ndocc: 1\ndocument_type: deed\n---\n\n" + tc.body + "\n"
			f, ds := parse.Parse("deed.md", []byte(src))
			runRules(f, sc, nil, &ds)

			var got bool
			for _, d := range ds {
				if d.Code == "X080" {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("below-minimum reported = %v, want %v\n%+v", got, tc.want, ds)
			}
		})
	}
}

// A rule scoped to a block no document contains reports nothing, and nothing
// is what a passing document looks like. This is the class the Gründungsurkunde
// fell into: its Stammkapital rewritten as prose, and the Art. 773 floor never
// evaluated once.
func TestUnguardedDivRules(t *testing.T) {
	amounts := schema.Rule{
		ID: "X090", Check: "amounts_balance",
		Args: map[string]any{"div": "betraege"},
	}

	t.Run("unpaired rule is reported", func(t *testing.T) {
		sc := &schema.Schema{Type: "deed", Rules: []schema.Rule{amounts}}
		got := UnguardedDivRules(sc)
		if len(got) != 1 {
			t.Fatalf("findings = %v, want one", got)
		}
		if !strings.Contains(got[0], "betraege") || !strings.Contains(got[0], "X090") {
			t.Errorf("finding names neither the block nor the rule: %q", got[0])
		}
	})

	t.Run("required_div guarantees the block", func(t *testing.T) {
		sc := &schema.Schema{Type: "deed", Rules: []schema.Rule{
			{ID: "X091", Check: "required_div", Args: map[string]any{"div": "betraege"}},
			amounts,
		}}
		if got := UnguardedDivRules(sc); len(got) != 0 {
			t.Errorf("findings = %v, want none", got)
		}
	})

	// The conditional case has to stay expressible: a Rechtsschrift arguing
	// only a point of law offers no exhibits.
	t.Run("on_missing says the absence is deliberate", func(t *testing.T) {
		rule := amounts
		rule.Args = map[string]any{"div": "betraege", "on_missing": "ignore"}
		sc := &schema.Schema{Type: "deed", Rules: []schema.Rule{rule}}
		if got := UnguardedDivRules(sc); len(got) != 0 {
			t.Errorf("findings = %v, want none", got)
		}
	})
}

// on_missing: error is the runtime half of the same fix — the rule itself says
// that having nothing to check is the finding.
func TestOnMissingDiv(t *testing.T) {
	src := "---\ndocc: 1\ndocument_type: deed\n---\n\n# Kapital\n\nFr. 3'000.00 als Stammkapital.\n"

	for _, tc := range []struct {
		name  string
		mode  any
		codes []string
	}{
		{"unset stays silent", nil, nil},
		{"ignore stays silent", "ignore", nil},
		{"error reports the absent block", "error", []string{"X100"}},
		{"an unknown mode is a schema error", "maybe", []string{"DOC009"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"div": "betraege", "minimum": "Fr. 20'000.00"}
			if tc.mode != nil {
				args["on_missing"] = tc.mode
			}
			ds := run(t, src, schema.Rule{ID: "X100", Check: "amount_at_least", Args: args})
			got := codes(ds)
			if len(got) != len(tc.codes) {
				t.Fatalf("codes = %v, want %v\n%s", got, tc.codes, messages(ds))
			}
			for i, want := range tc.codes {
				if got[i] != want {
					t.Fatalf("codes = %v, want %v\n%s", got, tc.codes, messages(ds))
				}
			}
		})
	}
}

const anchoredDoc = `---
document_type: test
kaeufer:
  name: Anna Muster-Berger
---

# Parteien

Die Käuferschaft, [Anna Muster-Berger]{.name}, erwirbt vom Verkäufer.

# Unterschriften

- [Anna Muster]{.name}:
`

// The copied-deed case: every occurrence agrees with every other one, so
// spans_agree is content, and none of them is blank, so the field machinery is
// content. Only the frontmatter knows who the buyer actually is.
func TestSpanMatchesFieldCatchesStaleValue(t *testing.T) {
	ds := run(t, anchoredDoc, schema.Rule{
		ID:    "X010",
		Check: "span_matches_field",
		Args:  map[string]any{"span": "name", "field": "kaeufer.name"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "X010" {
		t.Fatalf("codes = %v, want [X010]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Message, `"Anna Muster"`) {
		t.Errorf("message does not quote the stale value: %q", ds[0].Message)
	}
	if !strings.Contains(ds[0].Message, `"Anna Muster-Berger"`) {
		t.Errorf("message does not quote the authority: %q", ds[0].Message)
	}
	if ds[0].Pos.Line == 0 {
		t.Error("the diagnostic must point at the span, not at the file")
	}
}

// A rule anchored to a field the document never sets checked nothing, and must
// say so rather than exit clean.
func TestSpanMatchesFieldReportsMissingAnchor(t *testing.T) {
	ds := run(t, anchoredDoc, schema.Rule{
		ID:    "X011",
		Check: "span_matches_field",
		Args:  map[string]any{"span": "name", "field": "verkaeufer.name"},
	})
	if got := codes(ds); len(got) != 1 || got[0] != "X011" {
		t.Fatalf("codes = %v, want [X011]\n%s", got, messages(ds))
	}
}

// A blank the author was told to leave visible is not a disagreement, or
// `docc example --blank` would fail this tool's own check.
func TestSpanMatchesFieldIgnoresFieldBlanks(t *testing.T) {
	src := `---
document_type: test
kaeufer:
  name: Anna Muster-Berger
---

# Parteien

Die Käuferschaft, [Anna Muster-Berger]{.name}, und [____]{.name .docc-field key=zweitkaeufer}.
`
	ds := run(t, src, schema.Rule{
		ID:    "X012",
		Check: "span_matches_field",
		Args:  map[string]any{"span": "name", "field": "kaeufer.name"},
	})
	if len(ds) != 0 {
		t.Fatalf("expected no diagnostics, got:\n%s", messages(ds))
	}
}
