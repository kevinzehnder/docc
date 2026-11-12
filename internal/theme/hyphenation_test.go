package theme

import (
	"testing"

	"github.com/kevinzehnder/docc/internal/docx"
)

func TestStyleHyphenationOverrideInheritance(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", "name: _house\nstyles:\n  Standard: { default: true, suppress_auto_hyphens: true }\n",
		"inherited.yaml", "extends: _house\n",
		"allowed.yaml", "extends: _house\nstyles:\n  Standard: { suppress_auto_hyphens: false }\n",
		"unset.yaml", "name: unset\nstyles:\n  Standard: { default: true }\n",
	)
	for _, tc := range []struct {
		name string
		want docx.Toggle
	}{
		{"inherited", docx.ToggleOn},
		{"allowed", docx.ToggleOff},
		{"unset", docx.ToggleInherit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th, err := set.Get(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			styles := th.DocxStyles()
			if len(styles) != 1 || styles[0].Para.SuppressAutoHyphens != tc.want {
				t.Errorf("hyphenation override = %+v, want %v", styles, tc.want)
			}
		})
	}
}

func TestAutomaticHyphenationInheritance(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", "name: _house\ndefaults: { lang: de-CH, auto_hyphenation: true }\n",
		"inherited.yaml", "extends: _house\ndefaults: { font: Arial }\n",
		"disabled.yaml", "extends: _house\ndefaults: { auto_hyphenation: false }\n",
		"default.yaml", "name: default\n",
	)
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"inherited", true},
		{"disabled", false},
		{"default", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th, err := set.Get(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if th.Defaults.AutoHyphenation != tc.want {
				t.Errorf("auto_hyphenation = %v, want %v", th.Defaults.AutoHyphenation, tc.want)
			}
			if tc.name != "default" && th.Defaults.Lang != "de-CH" {
				t.Errorf("lang = %q, want inherited de-CH", th.Defaults.Lang)
			}
		})
	}
}
