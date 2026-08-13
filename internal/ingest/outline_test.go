package ingest

import (
	"strings"
	"testing"
)

// legalOutline is the scheme testdata/schemas/legal_reference.yaml declares.
func legalOutline(t *testing.T) []OutlineRule {
	t.Helper()
	rules, err := CompileOutline([]OutlinePattern{
		{Pattern: `^[A-ZÄÖÜ][A-ZÄÖÜ\s]+:$`, Level: 1},
		{Pattern: `^(?:X{1,3}(?:IX|IV|V?I{0,3})|IX|IV|V?I{1,3}|V)\.\s+\S`, Level: 2},
		{Pattern: `^[A-ZÄÖÜ]\.\s+\S`, Level: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

// The chat backend's failure: it marked I. FORMELLES and B. KLAGEHÄUFUNG but
// left C. STREITWERT, II. MATERIELLES and A. VORBEMERKUNGEN as prose. Measured
// on assets/example_replik.pdf, pages 1-2, Qwen3.5-9B.
func TestOutlinePromotesHeadingsTheModelMissed(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}
	got := o.Apply(strings.Join([]string{
		"# BEGRÜNDUNG:",
		"",
		"## I. FORMELLES",
		"",
		"C. STREITWERT",
		"",
		"II. MATERIELLES",
		"",
		"A. VORBEMERKUNGEN",
	}, "\n"))

	for _, want := range []string{
		"# BEGRÜNDUNG:",
		"## I. FORMELLES",
		"### C. STREITWERT",
		"## II. MATERIELLES",
		"### A. VORBEMERKUNGEN",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// The layout backend's failure: thirteen headings on a document with eight,
// because the layout pass types short lines at the left margin as `title`.
// A heading matching no declared pattern is not one this type has.
func TestOutlineDemotesHeadingsTheModelInvented(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}
	got := o.Apply(strings.Join([]string{
		"## I. FORMELLES",
		"",
		"## Die vorliegende Eingabe erfolgt innert Frist.",
	}, "\n"))

	if !strings.Contains(got, "## I. FORMELLES") {
		t.Errorf("real heading lost:\n%s", got)
	}
	if strings.Contains(got, "## Die vorliegende") {
		t.Errorf("invented heading kept:\n%s", got)
	}
	// Demoted, not deleted — losing a marker is recoverable, losing the
	// sentence is not.
	if !strings.Contains(got, "Die vorliegende Eingabe erfolgt innert Frist.") {
		t.Errorf("demoted text was dropped:\n%s", got)
	}
}

// Re-levelling, not just marking: a model that marked a heading at the wrong
// depth is as wrong as one that did not mark it.
func TestOutlineCorrectsTheLevel(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}
	got := o.Apply("###### I. FORMELLES")
	if got != "## I. FORMELLES" {
		t.Errorf("got %q, want %q", got, "## I. FORMELLES")
	}
}

// Roman numerals and section letters share an alphabet, and the overlap is not
// theoretical: C and D are 100 and 500. A naive [IVXLCDM]+ puts `C. STREITWERT`
// one level too shallow, which reads perfectly well and is wrong.
func TestOutlineTellsRomanNumeralsFromSectionLetters(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}
	for line, want := range map[string]string{
		"I. FORMELLES":      "## I. FORMELLES",
		"II. MATERIELLES":   "## II. MATERIELLES",
		"IV. KOSTEN":        "## IV. KOSTEN",
		"IX. SCHLUSS":       "## IX. SCHLUSS",
		"X. ANHANG":         "## X. ANHANG",
		"A. VORBEMERKUNGEN": "### A. VORBEMERKUNGEN",
		"B. KLAGEHÄUFUNG":   "### B. KLAGEHÄUFUNG",
		"C. STREITWERT":     "### C. STREITWERT",
		"D. KOSTEN":         "### D. KOSTEN",
		"L. ANHANG":         "### L. ANHANG",
		"M. SCHLUSS":        "### M. SCHLUSS",
	} {
		if got := o.Apply(line); got != want {
			t.Errorf("Apply(%q) = %q, want %q", line, got, want)
		}
	}
}

// A document type with no declared outline, and every run without --type, must
// come through untouched.
func TestOutlineWithoutRulesChangesNothing(t *testing.T) {
	var o outlineNormalizer
	in := "## Anything At All\n\nprose\n\nC. STREITWERT"
	if got := o.Apply(in); got != in {
		t.Errorf("Apply changed the input:\n%s", got)
	}
}

// Prose that happens to open with a capital and a full stop is not a heading:
// the pattern requires the title to follow on the same line, and "A. Muster"
// as a signature block would be a real false positive if it did not.
func TestOutlineLeavesOrdinaryProseAlone(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}
	for _, line := range []string{
		"Die Beklagte bestreitet dies.",
		"Vgl. dazu BGE 145 III 72.",
		"[Rz 5] Frivol berichtet die Beklagte.",
	} {
		if got := o.Apply(line); got != line {
			t.Errorf("Apply(%q) = %q, want it unchanged", line, got)
		}
	}
}

func TestCompileOutlineRejectsABadPattern(t *testing.T) {
	if _, err := CompileOutline([]OutlinePattern{{Pattern: `^[A-Z`, Level: 1}}); err == nil {
		t.Fatal("CompileOutline accepted an invalid regexp")
	}
}
