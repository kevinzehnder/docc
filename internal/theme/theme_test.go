package theme

import (
	"testing"

	"github.com/kevinzehnder/docc/pkg/docx"
)

func TestParseLength(t *testing.T) {
	tests := []struct {
		in      string
		want    docx.Twips
		wantErr bool
	}{
		{in: "20mm", want: docx.Mm(20)},
		{in: "1.5cm", want: docx.Cm(1.5)},
		{in: "12pt", want: docx.Pt(12)},
		{in: "1in", want: 1440},
		{in: "708tw", want: 708},
		{in: " 20mm ", want: docx.Mm(20)},
		// A bare number is rejected rather than guessed at: silently reading it
		// as twips would render a 20mm margin as a hairline.
		{in: "20", wantErr: true},
		{in: "20px", wantErr: true},
		{in: "abcmm", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseLength(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseLength(%q) succeeded, want error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLength(%q): %v", tt.in, err)
			continue
		}
		if got.Twips(0) != tt.want {
			t.Errorf("ParseLength(%q) = %d, want %d", tt.in, got.Twips(0), tt.want)
		}
	}
}

// An absent length and an explicit zero are different: the first inherits, the
// second overrides.
func TestLengthDistinguishesZeroFromAbsent(t *testing.T) {
	zero, err := ParseLength("0mm")
	if err != nil {
		t.Fatal(err)
	}
	if !zero.Set() {
		t.Error(`"0mm" should be marked as set`)
	}
	if zero.Twips(999) != 0 {
		t.Errorf("explicit zero returned the fallback: %d", zero.Twips(999))
	}

	var absent Length
	if absent.Set() {
		t.Error("an unparsed length should not be marked as set")
	}
	if absent.Twips(999) != 999 {
		t.Error("an absent length should return the fallback")
	}
}

func TestExpand(t *testing.T) {
	meta := map[string]any{
		"sender": map[string]any{
			"name": "Beispiel AG",
			"city": "Aarau",
		},
		"recipient": map[string]any{
			"name":         "Hans Beispiel",
			"organization": "",
		},
		"date": "2026-08-04",
	}

	tests := []struct {
		name         string
		in           string
		wantText     string
		wantAllEmpty bool
		wantRefs     int
	}{
		{
			name:     "single field",
			in:       "{{ sender.name }}",
			wantText: "Beispiel AG",
			wantRefs: 1,
		},
		{
			name:     "two fields and a literal",
			in:       "{{ sender.city }}, {{ date }}",
			wantText: "Aarau, 2026-08-04",
			wantRefs: 2,
		},
		{
			name:         "every field empty",
			in:           "{{ recipient.organization }}",
			wantText:     "",
			wantAllEmpty: true,
			wantRefs:     1,
		},
		{
			// A literal has no fields, so it can never be "all empty" — which is
			// what stops a label surviving on its own.
			name:     "literal only",
			in:       "vertreten durch",
			wantText: "vertreten durch",
			wantRefs: 0,
		},
		{
			// An empty field must not leave orphaned punctuation behind.
			name:     "empty field tidied",
			in:       "{{ recipient.name }}, {{ recipient.organization }}",
			wantText: "Hans Beispiel",
			wantRefs: 2,
		},
		{
			name:         "unknown path",
			in:           "{{ nope.missing }}",
			wantText:     "",
			wantAllEmpty: true,
			wantRefs:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Expand(tt.in, meta)
			if got.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", got.Text, tt.wantText)
			}
			if got.AllEmpty != tt.wantAllEmpty {
				t.Errorf("AllEmpty = %v, want %v", got.AllEmpty, tt.wantAllEmpty)
			}
			if got.Refs != tt.wantRefs {
				t.Errorf("Refs = %d, want %d", got.Refs, tt.wantRefs)
			}
		})
	}
}

func TestExpandReportsMissingPaths(t *testing.T) {
	got := Expand("{{ absent.field }}", map[string]any{})
	if len(got.Missing) != 1 || got.Missing[0] != "absent.field" {
		t.Errorf("Missing = %v, want [absent.field]", got.Missing)
	}
}

// A theme's field references are collected so they can be checked against a
// schema before anything renders.
func TestThemeFields(t *testing.T) {
	th := &Theme{
		Prologue: []Line{
			{Text: "{{ sender.name }}"},
			{Text: "{{ sender.city }}, {{ date }}"},
		},
		Epilogue: []Line{{Text: "{{ signee.name }}"}},
		Header:   map[string][]Line{"first": {{Text: "{{ sender.name }}"}}},
	}
	fields := th.Fields()

	want := map[string]bool{
		"sender.name": true, "sender.city": true,
		"date": true, "signee.name": true,
	}
	if len(fields) != len(want) {
		t.Errorf("got %d fields %v, want %d", len(fields), fields, len(want))
	}
	for _, f := range fields {
		if !want[f] {
			t.Errorf("unexpected field %q", f)
		}
	}
}

func TestLineHeightMultipleVersusExact(t *testing.T) {
	var multiple LineHeight
	if err := multiple.UnmarshalYAML(func(v any) error {
		*v.(*any) = 1.15
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	line, rule := multiple.Spacing()
	if rule != docx.LineAuto {
		t.Errorf("a bare number should be a multiple, got rule %q", rule)
	}
	if line != docx.Twips(1.15*240) {
		t.Errorf("line = %d, want %d", line, docx.Twips(1.15*240))
	}

	var exact LineHeight
	if err := exact.UnmarshalYAML(func(v any) error {
		*v.(*any) = "14pt"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	line, rule = exact.Spacing()
	if rule != docx.LineExact {
		t.Errorf("a length should be an exact height, got rule %q", rule)
	}
	if line != docx.Pt(14) {
		t.Errorf("line = %d, want %d", line, docx.Pt(14))
	}
}
