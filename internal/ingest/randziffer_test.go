package ingest

import (
	"strings"
	"testing"
)

// apply runs the normalizer over one or more pages and joins the result, so a
// test that does not care about page boundaries can read one string.
func apply(r *rzNormalizer, pages ...string) string {
	return strings.Join(r.Apply(pages), "\n")
}

func TestRZNormalizerMarksASequence(t *testing.T) {
	var r rzNormalizer
	got := apply(&r, "1 Die vorliegende Eingabe erfolgt innert Frist.\n\n2 Daran ändert nichts.")

	for _, want := range []string{"[Rz 1] Die vorliegende", "[Rz 2] Daran ändert"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// The sequence check is the whole safeguard: a paragraph opening with a year
// is not a paragraph number.
func TestRZNormalizerLeavesNonSequentialNumbersAlone(t *testing.T) {
	var r rzNormalizer
	got := apply(&r, "7 Die Beklagte gibt die Rechtsprechung wieder.\n\n2010 wurde der Vertrag geschlossen.\n\n8 Weiter ist zu sagen.")

	if !strings.Contains(got, "[Rz 7] Die Beklagte") {
		t.Errorf("the first number of the chain should be marked:\n%s", got)
	}
	if strings.Contains(got, "[Rz 2010]") {
		t.Errorf("a year was marked as a paragraph number:\n%s", got)
	}
	if !strings.Contains(got, "[Rz 8] Weiter") {
		t.Errorf("the year must not break the chain for the number that follows:\n%s", got)
	}
}

// The failure this was rewritten for. A letterhead postal code opens a line
// exactly like a Randziffer does, and it comes first. Trusting the first
// candidate made "5400 Baden" the document's paragraph 5400, after which the
// real 1, 2, 3 and 4 were all rejected for not continuing from 5401 — every
// model and both backends scored 1 of 4 on this fixture, and the one they
// "found" was the postal code.
func TestRZNormalizerIsNotFooledByALetterheadPostcode(t *testing.T) {
	var r rzNormalizer
	got := apply(&r,
		"Muster & Partner AG\n\n5400 Baden\n\n5000 Aarau, Musterstrasse 1",
		"1 Die örtliche Zuständigkeit ergibt sich aus dem Sitz.\n\n2 Die Parteien schlossen einen Werkvertrag.",
		"3 Die Klägerin hat das Werk abgeliefert.\n\n4 Die Klage ist gutzuheissen.",
	)

	for _, want := range []string{"[Rz 1] Die örtliche", "[Rz 2] Die Parteien", "[Rz 3] Die Klägerin", "[Rz 4] Die Klage"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, gone := range []string{"[Rz 5400]", "[Rz 5000]"} {
		if strings.Contains(got, gone) {
			t.Errorf("a postal code was marked as a paragraph number %q:\n%s", gone, got)
		}
	}
}

// A run continues across pages, so the chain cannot be found per page — and a
// --pages 30 run legitimately opens partway through the document.
func TestRZNormalizerSpansPagesAndStartsAnywhere(t *testing.T) {
	var r rzNormalizer
	pages := r.Apply([]string{
		"55 Erste Erwägung auf dieser Seite.",
		"56 Zweite Erwägung, nächste Seite.",
	})

	if !strings.Contains(pages[0], "[Rz 55]") {
		t.Errorf("a range run should accept its first number as the start:\n%s", pages[0])
	}
	if !strings.Contains(pages[1], "[Rz 56]") {
		t.Errorf("the sequence must carry across pages:\n%s", pages[1])
	}
}

// Converting one page of a brief offers a single candidate and nothing to
// check it against. It is still believed, because that page is a real use of
// --pages — but only below rzLoneMax, which is where a paragraph count lives
// and a postal code or a year does not.
func TestRZNormalizerBelievesALonePlausibleNumber(t *testing.T) {
	var r rzNormalizer
	got := apply(&r, "7 Die Beklagte gibt die Rechtsprechung wieder.")
	if !strings.Contains(got, "[Rz 7] Die Beklagte") {
		t.Errorf("a lone plausible paragraph number should be marked:\n%s", got)
	}
}

func TestRZNormalizerRejectsALoneImplausibleNumber(t *testing.T) {
	var r rzNormalizer
	for _, line := range []string{
		"5400 Baden ist der Sitz der Beklagten.",
		"2010 wurde der Vertrag geschlossen.",
	} {
		if got := apply(&r, line); strings.Contains(got, "[Rz") {
			t.Errorf("a lone four-digit number was marked:\n%s", got)
		}
	}
}

// The model marks some of them itself. Those belong to the same chain, or the
// next bare number stops continuing the sequence and is left behind.
func TestRZNormalizerAdoptsMarkersTheModelWrote(t *testing.T) {
	var r rzNormalizer
	got := apply(&r, "[Rz 3] Vom Modell markiert.\n\n4 Von uns zu markieren.")

	if strings.Count(got, "[Rz 3]") != 1 {
		t.Errorf("an existing marker must be left exactly as it is:\n%s", got)
	}
	if !strings.Contains(got, "[Rz 4] Von uns") {
		t.Errorf("a bare number after a model-written marker should continue it:\n%s", got)
	}
}

func TestRZNormalizerIgnoresListsHeadingsAndProse(t *testing.T) {
	var r rzNormalizer
	in := strings.Join([]string{
		"## 7 FORMELLES",              // a heading, not a paragraph
		"1. Erstens ist zu sagen",     // an ordered list item
		"7. Die Beklagte behauptet",   // list syntax, not a Randziffer
		"Die Frist von 7 Tagen läuft", // a number inside a sentence
		"7 Die Beklagte gibt wieder",  // the real one
		"8 Und weiter",                // its successor, so the chain is believable
	}, "\n")
	got := apply(&r, in)

	if n := strings.Count(got, "[Rz "); n != 2 {
		t.Errorf("expected exactly two markers, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "[Rz 7] Die Beklagte gibt wieder") {
		t.Errorf("the paragraph opening with a bare number should be marked:\n%s", got)
	}
}

func TestRZNormalizerLeavesADocumentWithoutRandziffernUntouched(t *testing.T) {
	var r rzNormalizer
	in := "# Klage\n\nErste Erwägung ohne Nummer.\n\nZweite Erwägung.\n"
	if got := apply(&r, in); got != in {
		t.Errorf("a document with no paragraph numbers must come back unchanged:\n%s", got)
	}
}

// A draft destined to become one of our own documents must carry no paragraph
// numbers in source: the schema generates them at render time, so a
// transcribed one would print twice and go stale the first time a section
// moved.
func TestRZNormalizerStripMode(t *testing.T) {
	r := rzNormalizer{strip: true}
	got := apply(&r, "1 Die vorliegende Eingabe erfolgt innert Frist.\n\n2 Daran ändert nichts.")

	if strings.Contains(got, "[Rz") || strings.Contains(got, "\n2 ") {
		t.Errorf("strip mode left a paragraph number behind:\n%s", got)
	}
	for _, want := range []string{"Die vorliegende Eingabe", "Daran ändert nichts"} {
		if !strings.Contains(got, want) {
			t.Errorf("strip mode lost prose %q:\n%s", want, got)
		}
	}
}

// Whichever way the model wrote them, stripping has to remove both forms and
// keep one sequence — otherwise a marker the model produced survives into a
// document whose numbers are generated.
func TestRZNormalizerStripsMarkersTheModelWrote(t *testing.T) {
	r := rzNormalizer{strip: true}
	got := apply(&r, "[Rz 3] Vom Modell markiert.\n\n4 Von uns zu entfernen.")

	if strings.Contains(got, "[Rz 3]") || strings.Contains(got, "\n4 ") {
		t.Errorf("strip mode left a number behind:\n%s", got)
	}
	if !strings.Contains(got, "Vom Modell markiert.") || !strings.Contains(got, "Von uns zu entfernen.") {
		t.Errorf("strip mode lost prose:\n%s", got)
	}
}

// A year is not a paragraph number in either mode.
func TestRZNormalizerStripLeavesProseNumbersAlone(t *testing.T) {
	r := rzNormalizer{strip: true}
	got := apply(&r, "1 Erste Erwägung.\n\n2 Zweite Erwägung.\n\n2010 wurde der Vertrag geschlossen.")

	if !strings.Contains(got, "2010 wurde der Vertrag") {
		t.Errorf("strip mode removed a number that was part of the prose:\n%s", got)
	}
}

// para is a numbered body element as a backend hands it over: the gutter number
// already located and separated from the text.
func para(text string, rz *int) Node {
	return Node{Kind: KindPara, Text: text, SourceNumber: rz}
}

// A number the backend found in the gutter needs no guessing — it joins the
// chain as a link that is already proved. This is the half that used to be a
// regular expression re-reading a marker the assembler had just written.
func TestApplyNodesKeepsACorroboratedGutterNumber(t *testing.T) {
	rz := rzNormalizer{}
	n1, n2 := 55, 56
	got := rz.ApplyNodes([][]Node{
		{para("Richtig ist, dass ...", &n1)},
		{para("Die Ausführungen ...", &n2)},
	})

	for p, want := range []int{55, 56} {
		if got[p][0].SourceNumber == nil {
			t.Fatalf("page %d lost its number", p+1)
		}
		if *got[p][0].SourceNumber != want {
			t.Errorf("page %d numbered %d, want %d", p+1, *got[p][0].SourceNumber, want)
		}
	}
}

// The postal code. A single number nothing corroborates loses its marker, and
// the chain is what decides — the lesson this file was written around.
func TestApplyNodesDropsAnUncorroboratedNumber(t *testing.T) {
	rz := rzNormalizer{}
	postcode, one, two := 5400, 1, 2
	got := rz.ApplyNodes([][]Node{{
		para("Baden", &postcode),
		para("Die vorliegende Eingabe ...", &one),
		para("Daran ändert nichts ...", &two),
	}})

	if got[0][0].SourceNumber != nil {
		t.Errorf("5400 Baden was numbered %d — no chain corroborates it", *got[0][0].SourceNumber)
	}
	for i, want := range map[int]int{1: 1, 2: 2} {
		if got[0][i].SourceNumber == nil || *got[0][i].SourceNumber != want {
			t.Errorf("element %d should carry %d", i, want)
		}
	}
}

// A number the backend merged into the body text, rather than reading as its
// own block in the gutter.
func TestApplyNodesLiftsANumberOutOfTheText(t *testing.T) {
	rz := rzNormalizer{}
	got := rz.ApplyNodes([][]Node{{
		para("55 Richtig ist, dass ...", nil),
		para("56 Die Ausführungen ...", nil),
	}})

	if got[0][0].SourceNumber == nil || *got[0][0].SourceNumber != 55 {
		t.Fatalf("number not lifted out of the text: %+v", got[0][0])
	}
	if got[0][0].Text != "Richtig ist, dass ..." {
		t.Errorf("text is %q, want the number removed", got[0][0].Text)
	}
}

// A draft destined to become one of our own carries no paragraph numbers in
// source: docc generates those at render time, and a transcribed one would
// print twice and go stale the first time a section moved.
func TestApplyNodesStripsForOurOwnDocuments(t *testing.T) {
	rz := rzNormalizer{strip: true}
	n1, n2 := 55, 56
	got := rz.ApplyNodes([][]Node{{
		para("Richtig ist, dass ...", &n1),
		para("Die Ausführungen ...", &n2),
	}})

	for i, n := range got[0] {
		if n.SourceNumber != nil {
			t.Errorf("element %d kept the number %d under strip", i, *n.SourceNumber)
		}
	}
	if got[0][0].Text != "Richtig ist, dass ..." {
		t.Errorf("text is %q", got[0][0].Text)
	}
}

// The chat backend's page has no elements, so it goes through the string form.
func TestApplyNodesFallsBackForRawPages(t *testing.T) {
	rz := rzNormalizer{}
	got := rz.ApplyNodes([][]Node{
		{{Kind: KindRaw, Text: "55 Richtig ist, dass ..."}},
		{{Kind: KindRaw, Text: "56 Die Ausführungen ..."}},
	})

	if !strings.HasPrefix(got[0][0].Text, "[Rz 55] ") {
		t.Errorf("raw page 1 not marked: %q", got[0][0].Text)
	}
	if !strings.HasPrefix(got[1][0].Text, "[Rz 56] ") {
		t.Errorf("raw page 2 not marked: %q", got[1][0].Text)
	}
}
