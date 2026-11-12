package docx

import (
	"strings"
	"testing"
)

func TestParagraphHyphenationOverride(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value Toggle
		want  string
	}{
		{"inherit", ToggleInherit, ""},
		{"suppress", ToggleOn, "<w:suppressAutoHyphens/>"},
		{"allow", ToggleOff, `<w:suppressAutoHyphens w:val="0"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sample()
			// This must be emitted even if it is the only paragraph property.
			doc.Body = []Block{Paragraph{Props: ParaProps{SuppressAutoHyphens: tc.value}, Runs: []Run{R("Test")}}}
			data, err := doc.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			body := partOf(t, data, "word/document.xml")
			if tc.want == "" {
				if strings.Contains(body, "suppressAutoHyphens") {
					t.Fatal("an inherited setting must not emit an override")
				}
			} else if !strings.Contains(body, tc.want) {
				t.Errorf("missing %s in %s", tc.want, body)
			}
		})
	}
}

func TestAutomaticHyphenationSettings(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			doc := sample()
			doc.AutoHyphenation = enabled
			data, err := doc.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			settings := partOf(t, data, "word/settings.xml")
			if got := strings.Contains(settings, `<w:autoHyphenation w:val="true"/>`); got != enabled {
				t.Fatalf("autoHyphenation = %v, want %v: %s", got, enabled, settings)
			}
			if !enabled && strings.Contains(settings, "autoHyphenation") {
				t.Fatal("disabled setting must preserve the existing settings XML")
			}
		})
	}
}
