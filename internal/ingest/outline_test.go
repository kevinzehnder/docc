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

// The layout backend marks thirteen headings on a document with eight, and this
// deliberately does not fix that. Unmarking whatever matches no rule would, and
// would also strip the real structure out of any brief written to a convention
// nobody anticipated — which is the case a transcription of somebody else's
// document has to survive. A scheme is a baseline, not a contract.
func TestOutlineLeavesUnrecognizedHeadingsAlone(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}
	in := strings.Join([]string{
		"## I. FORMELLES",
		"",
		"## Die vorliegende Eingabe erfolgt innert Frist.",
	}, "\n")

	got := o.Apply(in)
	if got != in {
		t.Errorf("Apply changed a page it recognized only part of:\ngot:\n%s\nwant:\n%s", got, in)
	}
}

// strict is the caller saying the scheme is this document's, not a guess about
// it. Then a heading matching no rule is one the model invented, and unmarking
// it is right — which is what takes the round-trip fixture from fifteen
// headings to its actual eight.
func TestOutlineStrictUnmarksWhatItDoesNotRecognize(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t), strict: true}
	got := o.Apply(strings.Join([]string{
		"## I. FORMELLES",
		"",
		"## EINSCHREIBEN",
		"",
		"## Die vorliegende Eingabe erfolgt innert Frist.",
	}, "\n"))

	if !strings.Contains(got, "## I. FORMELLES") {
		t.Errorf("a recognized heading was lost:\n%s", got)
	}
	for _, gone := range []string{"## EINSCHREIBEN", "## Die vorliegende"} {
		if strings.Contains(got, gone) {
			t.Errorf("strict kept %q:\n%s", gone, got)
		}
	}
	// Unmarked, never deleted: losing a marker is recoverable, losing the line
	// is not.
	for _, kept := range []string{"EINSCHREIBEN", "Die vorliegende Eingabe erfolgt innert Frist."} {
		if !strings.Contains(got, kept) {
			t.Errorf("strict dropped the text %q:\n%s", kept, got)
		}
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

// The scheme decides depth; the backend decided what is a heading. Neither
// question involves a "#", which is the point of doing this on elements.
func TestOutlineApplyNodesLevelsAClassifiedHeading(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}

	// The legal_reference scheme puts a shouting title at level 1, roman
	// numerals at 2 and letters at 3 — the inversion schema.Outline exists for.
	got := o.ApplyNodes([]Node{
		{Kind: KindHeading, Level: 2, Text: "BEGRÜNDUNG:"},
		{Kind: KindHeading, Level: 2, Text: "I. FORMELLES"},
		{Kind: KindHeading, Level: 2, Text: "A. FRIST"},
	})
	for i, want := range []int{1, 2, 3} {
		if got[i].Level != want {
			t.Errorf("%q is level %d, want %d", got[i].Text, got[i].Level, want)
		}
	}
}

// A title the layout pass typed `text`. Dropping this would lose every heading
// the backend failed to classify, which is most of them on a scanned brief.
func TestOutlineApplyNodesPromotesAParagraph(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}

	got := o.ApplyNodes([]Node{{Kind: KindPara, Text: "II. MATERIELLES"}})
	if got[0].Kind != KindHeading || got[0].Level != 2 {
		t.Errorf("got kind %v level %d, want a level 2 heading", got[0].Kind, got[0].Level)
	}
}

// Only under --outline-strict, and the text survives either way: a transcription
// is of somebody else's brief, and a scheme nobody confirmed is a guess.
func TestOutlineApplyNodesDemotesOnlyWhenStrict(t *testing.T) {
	rules := legalOutline(t)
	invented := Node{Kind: KindHeading, Level: 2, Text: "Muster & Partner AG"}

	lenient := (&outlineNormalizer{rules: rules}).ApplyNodes([]Node{invented})
	if lenient[0].Kind != KindHeading {
		t.Error("an unrecognized heading was demoted without --outline-strict")
	}

	strict := (&outlineNormalizer{rules: rules, strict: true}).ApplyNodes([]Node{invented})
	if strict[0].Kind != KindPara {
		t.Error("--outline-strict did not demote a heading matching no rule")
	}
	if strict[0].Text != invented.Text {
		t.Errorf("text changed to %q; it is kept either way", strict[0].Text)
	}
}

// The chat backend's page has no elements to consult, so it goes through the
// string form — the same code, on the only representation that path has.
func TestOutlineApplyNodesFallsBackForARawPage(t *testing.T) {
	o := outlineNormalizer{rules: legalOutline(t)}

	// Marked at the wrong depth by the model, and re-levelled to the scheme's.
	got := o.ApplyNodes([]Node{{Kind: KindRaw, Text: "# I. FORMELLES\n\nDie Eingabe erfolgt fristgerecht."}})
	if !strings.HasPrefix(got[0].Text, "## I. FORMELLES") {
		t.Errorf("raw page not re-levelled:\n%s", got[0].Text)
	}
}
