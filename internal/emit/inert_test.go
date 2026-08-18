package emit

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/theme"
)

func TestInertFurnitureFlags(t *testing.T) {
	no, yes := false, true

	th := &theme.Theme{
		Name: "test",
		Prologue: []theme.Line{
			// Inert: a literal line has no placeholder to come up empty, so
			// neither true nor false changes anything.
			{Style: "Fixed", Text: "Beilagen", OmitIfEmpty: &no},
			{Style: "AlsoFixed", Text: "Beilagen", OmitIfEmpty: &yes},
			// Load-bearing: this is what the flag is for.
			{Style: "Addr", Text: "{{ recipient.organisation }}", OmitIfEmpty: &no},
			// Inert: an image line is never dropped.
			{Style: "Logo", Image: &theme.Image{Path: "logo.png"}, OmitIfEmpty: &no},
		},
		Epilogue: []theme.Line{
			// Load-bearing: the literal label goes only when the value it
			// introduces is empty, which is the whole point of the line flag.
			{Style: "Vertreter", OmitIfEmpty: &no, Runs: []theme.LineRun{
				{Text: "vertreten durch "},
				{Text: "{{ vertreter }}"},
			}},
			// Inert on the line: no run interpolates, and one produces text.
			// Inert on the run: a literal run has nothing to be empty.
			{Style: "Betreff", OmitIfEmpty: &no, Runs: []theme.LineRun{
				{Text: "betreffend ", OmitIfEmpty: &no},
				{Text: "Kaufvertrag"},
			}},
		},
	}

	got := InertFurnitureFlags(th)
	if len(got) != 5 {
		t.Fatalf("got %d findings, want 5:\n%s", len(got), strings.Join(got, "\n"))
	}
	for _, want := range []string{
		"prologue line 1 (Fixed)",
		"prologue line 2 (AlsoFixed)",
		"prologue line 4 (Logo)",
		"epilogue line 2 (Betreff)",
		`the run "betreffend "`,
	} {
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("no finding mentions %q:\n%s", want, strings.Join(got, "\n"))
		}
	}
	// The two load-bearing flags must not be reported: flagging them would
	// teach an author to delete the thing that drops a dangling label.
	for _, unwanted := range []string{"Addr", "Vertreter"} {
		if strings.Contains(strings.Join(got, "\n"), unwanted) {
			t.Errorf("reported a load-bearing flag on %s:\n%s", unwanted, strings.Join(got, "\n"))
		}
	}
}

// The shipped packs must stay clean, or the warning is noise from the first
// `docc doctor` a new user runs.
func TestStarterThemesCarryNoInertFlags(t *testing.T) {
	themes, err := theme.Load("../defaultpack/files/themes")
	if err != nil {
		t.Fatalf("load themes: %v", err)
	}
	for _, name := range themes.Names() {
		th, _ := themes.Get(name)
		if found := InertFurnitureFlags(th); len(found) > 0 {
			t.Errorf("theme %s:\n  %s", name, strings.Join(found, "\n  "))
		}
	}
}
