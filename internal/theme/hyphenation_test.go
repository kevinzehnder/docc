package theme

import "testing"

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
