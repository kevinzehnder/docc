package ingest

import (
	"path/filepath"
	"testing"

	"github.com/kevinzehnder/docc/internal/schema"
)

// outlineFor loads a real schema from testdata and compiles its declared
// outline, so that these assertions are about the shipped YAML rather than a
// copy of it that can drift.
func outlineFor(t *testing.T, docType, scheme string) *outlineNormalizer {
	t.Helper()
	set, err := schema.Load(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	sc, err := set.Get(docType)
	if err != nil {
		t.Fatal(err)
	}
	if scheme == "" {
		scheme = sc.Outline.Default
	}
	rulesIn, ok := sc.Outline.Schemes[scheme]
	if !ok {
		t.Fatalf("%s declares no outline scheme %q", docType, scheme)
	}
	patterns := make([]OutlinePattern, 0, len(rulesIn))
	for _, r := range rulesIn {
		patterns = append(patterns, OutlinePattern{Pattern: r.Pattern, Level: r.Level})
	}
	rules, err := CompileOutline(patterns)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatalf("%s scheme %s compiled to nothing", docType, scheme)
	}
	return &outlineNormalizer{rules: rules}
}

func assertLevels(t *testing.T, o *outlineNormalizer, cases map[string]string) {
	t.Helper()
	for line, want := range cases {
		if got := o.Apply(line); got != want {
			t.Errorf("Apply(%q) = %q, want %q", line, got, want)
		}
	}
}

// The filing order: a part title, then Roman, then letters. Matches
// assets/example_replik.pdf and the UZH Merkblatt's I./A./1./a) example.
func TestLegalReferenceOutlineIsRomanOverLetters(t *testing.T) {
	assertLevels(t, outlineFor(t, "legal_reference", ""), map[string]string{
		"BEGRÜNDUNG:":       "# BEGRÜNDUNG:",
		"I. FORMELLES":      "## I. FORMELLES",
		"II. MATERIELLES":   "## II. MATERIELLES",
		"A. FRIST":          "### A. FRIST",
		"C. STREITWERT":     "### C. STREITWERT",
		"1. Vorbemerkungen": "#### 1. Vorbemerkungen",
		"a) Erstens":        "##### a) Erstens",
	})
}

// The LL.M. sheet's "übliches System" inverts the top two levels. A standalone
// I. is the ninth section here and the first part in the type above, which is
// exactly why these cannot be one declaration.
func TestKlassischOutlineInvertsTheTopTwoLevels(t *testing.T) {
	assertLevels(t, outlineFor(t, "legal_reference", "letter-first"), map[string]string{
		"A. FORMELLES":      "# A. FORMELLES",
		"B. MATERIELLES":    "# B. MATERIELLES",
		"I. Frist":          "## I. Frist",
		"IV. Kosten":        "## IV. Kosten",
		"1. Vorbemerkungen": "### 1. Vorbemerkungen",
		"a) Erstens":        "#### a) Erstens",
		"aa) Erstens":       "##### aa) Erstens",
	})
}

// The same line, levelled differently by the two types — the one assertion that
// proves the choice belongs to the document type.
func TestTheSameHeadingLevelsDifferentlyPerType(t *testing.T) {
	kley := outlineFor(t, "legal_reference", "")
	klassisch := outlineFor(t, "legal_reference", "letter-first")

	if got := kley.Apply("A. FRIST"); got != "### A. FRIST" {
		t.Errorf("legal_reference: got %q, want level 3", got)
	}
	if got := klassisch.Apply("A. FRIST"); got != "# A. FRIST" {
		t.Errorf("legal_reference_klassisch: got %q, want level 1", got)
	}
}

func TestDezimalOutlineLevelsByDepth(t *testing.T) {
	assertLevels(t, outlineFor(t, "legal_reference", "decimal"), map[string]string{
		"1 Formelles":          "# 1 Formelles",
		"1. Formelles":         "# 1. Formelles",
		"1.1 Frist":            "## 1.1 Frist",
		"1.2.1 Vorbemerkungen": "### 1.2.1 Vorbemerkungen",
		"1.2.1.3 Erstens":      "#### 1.2.1.3 Erstens",
	})
}

// `1.` opens a third-level heading and an ordered list item alike, and a Swiss
// brief's Rechtsbegehren is a numbered list of sentences. Without the length
// and punctuation constraints every prayer for relief in the corpus becomes a
// section title — measured, four spurious headings on a four-page fixture.
func TestOutlineDoesNotPromoteOrderedListItems(t *testing.T) {
	for _, docType := range []string{"legal", "legal_reference"} {
		t.Run(docType, func(t *testing.T) {
			o := outlineFor(t, docType, "")

			for _, item := range []string{
				"1. Die Beklagte sei zu verpflichten, der Klägerin CHF 42'000.00 nebst Zins zu 5 % seit 1. Juli 2024 zu bezahlen;",
				"2. unter Kosten- und Entschädigungsfolgen zulasten der Beklagten.",
				"1. Werkvertrag vom 3. März 2024, unterzeichnet von beiden Parteien.",
			} {
				if got := o.Apply(item); got != item {
					t.Errorf("list item promoted:\n  %q\n→ %q", item, got)
				}
			}

			// Where a numbered level is claimed at all, a real numbered heading
			// still has to be marked — otherwise the constraint has not tamed
			// the level, it has disabled it.
			//
			// `legal` claims no numbered level on purpose: its decimal third
			// level and its Beilagen list are the same string on the page, so
			// it marks neither. See the schema for why that is the right way
			// round.
			if docType == "legal" {
				return
			}
			if got := o.Apply("1. Werklohnforderung"); got == "1. Werklohnforderung" {
				t.Errorf("a short numbered title was not marked: %q", got)
			}
		})
	}
}

// One type carries all three, and names a default, so that a run without
// --outline still gets the common case rather than nothing.
func TestLegalReferenceCarriesEveryScheme(t *testing.T) {
	set, err := schema.Load(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	sc, err := set.Get("legal_reference")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"roman-first", "letter-first", "decimal"} {
		if _, ok := sc.Outline.Schemes[want]; !ok {
			t.Errorf("no outline scheme %q", want)
		}
	}
	if sc.Outline.Default != "roman-first" {
		t.Errorf("default scheme is %q, want roman-first", sc.Outline.Default)
	}
}

// The document belongs to somebody else. A brief outlined some way no scheme
// anticipates is unusual, not wrong, and its headings have to survive: the
// documents whose structure cannot be predicted are exactly the ones that would
// lose it if a scheme were treated as a contract.
func TestOutlineLeavesAnUnrecognizedSchemeIntact(t *testing.T) {
	o := outlineFor(t, "legal_reference", "")

	// A firm numbering its sections §1 / §1.1 matches nothing declared.
	for _, line := range []string{
		"## § 1 Formelles",
		"### § 1.1 Frist",
		"# Teil Eins — Zur Sache",
	} {
		if got := o.Apply(line); got != line {
			t.Errorf("Apply(%q) = %q, want it untouched", line, got)
		}
	}
}
