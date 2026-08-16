package sema

import (
	"strings"
	"testing"
)

// A skeleton is the example with its decisions taken back out: `--blank`
// empties every field marker and leaves everything else, including the
// attribute block and any semantic class riding in front of the marker.
func TestBlankFieldsEmptiesMarkers(t *testing.T) {
	src := "---\ndocc: 1\n---\n\n" +
		"Die Gesellschaft bezweckt [den Handel]{.docc-field key=zweck}.\n\n" +
		"Unter der Firma [Muster Bau]{.firma .docc-field key=firma} GmbH.\n\n" +
		"Sitz in [Baden]{.sitz}, geführt von [Max Muster]{.name}.\n"

	got := BlankFields(src)

	for _, want := range []string{
		"[____________]{.docc-field key=zweck}",
		"[____________]{.firma .docc-field key=firma}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("BlankFields did not empty a marker: want %q in\n%s", want, got)
		}
	}
	// A span that is not a field is content, not a decision, and stays put.
	for _, want := range []string{"[Baden]{.sitz}", "[Max Muster]{.name}"} {
		if !strings.Contains(got, want) {
			t.Errorf("BlankFields emptied a plain span: want %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "den Handel") || strings.Contains(got, "Muster Bau") {
		t.Errorf("BlankFields left a field's value behind:\n%s", got)
	}
}
