package ingest

import (
	"strings"
	"testing"
)

func TestRZNormalizerMarksASequence(t *testing.T) {
	var r rzNormalizer
	got := r.Apply("1 Die vorliegende Eingabe erfolgt innert Frist.\n\n2 Daran ändert nichts.")

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
	got := r.Apply("7 Die Beklagte gibt die Rechtsprechung wieder.\n\n2010 wurde der Vertrag geschlossen.\n\n8 Weiter ist zu sagen.")

	if !strings.Contains(got, "[Rz 7] Die Beklagte") {
		t.Errorf("the first number should start the sequence:\n%s", got)
	}
	if strings.Contains(got, "[Rz 2010]") {
		t.Errorf("a year was marked as a paragraph number:\n%s", got)
	}
	if !strings.Contains(got, "[Rz 8] Weiter") {
		t.Errorf("the year must not break the sequence for the number that follows:\n%s", got)
	}
}

// A run continues across pages, so the counter cannot reset per page — and a
// --pages 30 run legitimately opens partway through the document.
func TestRZNormalizerSpansPagesAndStartsAnywhere(t *testing.T) {
	var r rzNormalizer
	first := r.Apply("55 Erste Erwägung auf dieser Seite.")
	second := r.Apply("56 Zweite Erwägung, nächste Seite.")

	if !strings.Contains(first, "[Rz 55]") {
		t.Errorf("a range run should accept its first number as the start:\n%s", first)
	}
	if !strings.Contains(second, "[Rz 56]") {
		t.Errorf("the sequence must carry across pages:\n%s", second)
	}
}

// The model marks some of them itself. Those must advance the same counter,
// or the next bare number stops continuing the sequence and is left behind.
func TestRZNormalizerAdoptsMarkersTheModelWrote(t *testing.T) {
	var r rzNormalizer
	got := r.Apply("[Rz 3] Vom Modell markiert.\n\n4 Von uns zu markieren.")

	if strings.Count(got, "[Rz 3]") != 1 {
		t.Errorf("an existing marker must be left exactly as it is:\n%s", got)
	}
	if !strings.Contains(got, "[Rz 4] Von uns") {
		t.Errorf("a bare number after a model-written marker should continue it:\n%s", got)
	}
}

func TestRZNormalizerIgnoresListsHeadingsAndProse(t *testing.T) {
	var r rzNormalizer
	r.last, r.found = 6, true

	in := strings.Join([]string{
		"## 7 FORMELLES",              // a heading, not a paragraph
		"1. Erstens ist zu sagen",     // an ordered list item
		"7. Die Beklagte behauptet",   // list syntax, not a Randziffer
		"Die Frist von 7 Tagen läuft", // a number inside a sentence
		"7 Die Beklagte gibt wieder",  // the real one
	}, "\n")
	got := r.Apply(in)

	if n := strings.Count(got, "[Rz "); n != 1 {
		t.Errorf("expected exactly one marker, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "[Rz 7] Die Beklagte gibt wieder") {
		t.Errorf("the paragraph opening with a bare number should be marked:\n%s", got)
	}
}

func TestRZNormalizerLeavesADocumentWithoutRandziffernUntouched(t *testing.T) {
	var r rzNormalizer
	in := "# Klage\n\nErste Erwägung ohne Nummer.\n\nZweite Erwägung.\n"
	if got := r.Apply(in); got != in {
		t.Errorf("a document with no paragraph numbers must come back unchanged:\n%s", got)
	}
}
