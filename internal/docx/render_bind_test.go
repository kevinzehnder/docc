package docx

import "testing"

func TestBindAfter(t *testing.T) {
	d := &Document{NonBreakingAfter: []string{"Fr.", "CHF"}}
	for in, want := range map[string]string{
		"den Betrag von Fr. 1'000'000.00 und CHF 5": "den Betrag von Fr. 1'000'000.00 und CHF 5",
		"Fr. 800'000.00": "Fr. 800'000.00",
		// No digit after it, or inside a longer word: left alone.
		"in Fr. Franken, GFr. 12": "in Fr. Franken, GFr. 12",
		"Fr.":                     "Fr.",
	} {
		if got := d.bindAfter(in); got != want {
			t.Errorf("bindAfter(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (*Document)(nil).bindAfter("Fr. 1"); got != "Fr. 1" {
		t.Errorf("nil document changed text: %q", got)
	}
}
