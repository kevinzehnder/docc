package docx

import (
	"strings"
	"testing"
)

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
