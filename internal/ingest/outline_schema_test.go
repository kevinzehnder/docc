package ingest

import (
	"path/filepath"
	"testing"

	"github.com/kevinzehnder/docc/internal/schema"
)

// outlineFor loads a real schema from testdata and compiles its declared
// outline, so that these assertions are about the shipped YAML rather than a
// copy of it that can drift.
func outlineFor(t *testing.T, docType string) *outlineNormalizer {
	t.Helper()
	set, err := schema.Load(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	sc, err := set.Get(docType)
	if err != nil {
		t.Fatal(err)
	}
	patterns := make([]OutlinePattern, 0, len(sc.Outline))
	for _, r := range sc.Outline {
		patterns = append(patterns, OutlinePattern{Pattern: r.Pattern, Level: r.Level})
	}
	rules, err := CompileOutline(patterns)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatalf("%s declares no outline", docType)
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
	assertLevels(t, outlineFor(t, "legal_reference"), map[string]string{
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
	assertLevels(t, outlineFor(t, "legal_reference_klassisch"), map[string]string{
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
	kley := outlineFor(t, "legal_reference")
	klassisch := outlineFor(t, "legal_reference_klassisch")

	if got := kley.Apply("A. FRIST"); got != "### A. FRIST" {
		t.Errorf("legal_reference: got %q, want level 3", got)
	}
	if got := klassisch.Apply("A. FRIST"); got != "# A. FRIST" {
		t.Errorf("legal_reference_klassisch: got %q, want level 1", got)
	}
}

func TestDezimalOutlineLevelsByDepth(t *testing.T) {
	assertLevels(t, outlineFor(t, "legal_reference_dezimal"), map[string]string{
		"1 Formelles":          "# 1 Formelles",
		"1. Formelles":         "# 1. Formelles",
		"1.1 Frist":            "## 1.1 Frist",
		"1.2.1 Vorbemerkungen": "### 1.2.1 Vorbemerkungen",
		"1.2.1.3 Erstens":      "#### 1.2.1.3 Erstens",
	})
}

// The variants inherit everything but the outline: same required frontmatter,
// same randziffer_sequence rule, and above all no render.paragraph_numbering,
// which is what keeps a reference document's numbers as citation keys.
func TestOutlineVariantsInheritTheRestOfTheType(t *testing.T) {
	set, err := schema.Load(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"legal_reference_klassisch", "legal_reference_dezimal"} {
		sc, err := set.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		if sc.Render.ParagraphNumbering != nil {
			t.Errorf("%s generates paragraph numbers — a reference document must keep the source's", name)
		}
		if _, ok := sc.Frontmatter["cite_as"]; !ok {
			t.Errorf("%s did not inherit cite_as", name)
		}
		if len(sc.Rules) == 0 {
			t.Errorf("%s did not inherit randziffer_sequence", name)
		}
	}
}
