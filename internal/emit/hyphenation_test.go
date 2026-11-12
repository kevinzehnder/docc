package emit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/ir"
)

func TestBuildAutomaticHyphenation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			th := testTheme()
			th.Defaults.AutoHyphenation = enabled
			th.Defaults.Lang = "de-CH"
			source := &ir.Document{Meta: map[string]any{}}
			built, err := Build(source, testSchema(), th, Options{})
			if err != nil {
				t.Fatal(err)
			}
			data, err := built.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			settings := partOf(t, data, "word/settings.xml")
			if got := strings.Contains(settings, `<w:autoHyphenation w:val="true"/>`); got != enabled {
				t.Errorf("autoHyphenation = %v, want %v", got, enabled)
			}
			styles := partOf(t, data, "word/styles.xml")
			if !strings.Contains(styles, `<w:lang w:val="de-CH"/>`) {
				t.Error("proofing language did not reach document defaults")
			}
			// The setting is presentation, not an environment-dependent text
			// transformation; repeated builds must remain byte-identical.
			again, err := Build(source, testSchema(), th, Options{})
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := again.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, repeated) {
				t.Error("automatic hyphenation made output nondeterministic")
			}
		})
	}
}
