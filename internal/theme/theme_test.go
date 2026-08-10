package theme

import (
	"testing"
	"time"

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
			var th *Theme // the defaults; this test is about substitution, not locale
			got := th.Expand(tt.in, meta)
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
	var th *Theme
	got := th.Expand("{{ absent.field }}", map[string]any{})
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
		Epilogue: []Line{
			{Text: "{{ signee.name }}"},
			{Text: "Enclosures", IfNonempty: "attachments"},
		},
		Header: map[string][]Line{"first": {{Text: "{{ sender.name }}"}}},
	}
	fields := th.Fields()

	want := map[string]bool{
		"sender.name": true, "sender.city": true,
		"date": true, "signee.name": true, "attachments": true,
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

// `levels:` is a flat list: the definition is level 0 and each entry is the
// next one down. Reading it as a tree gave two levels both claiming ilvl 1,
// and Word renders the loser's %3 placeholder as literal text.
func TestNumFormatLevelsAreFlat(t *testing.T) {
	def := NumFormat{
		Format: "upperRoman", Text: "%1.", Style: "Ueberschrift1",
		Levels: []NumFormat{
			{Format: "upperLetter", Text: "%2.", Style: "Ueberschrift2"},
			{Format: "decimal", Text: "%3.", Style: "Ueberschrift3"},
		},
	}.AbstractNum()

	if len(def.Levels) != 3 {
		t.Fatalf("got %d levels, want 3", len(def.Levels))
	}
	if def.MultiLevelType != "multilevel" {
		t.Errorf("MultiLevelType = %q, want multilevel", def.MultiLevelType)
	}
	for i, want := range []struct {
		format docx.NumFormat
		text   string
		style  string
	}{
		{docx.NumUpperRoman, "%1.", "Ueberschrift1"},
		{docx.NumUpperLetter, "%2.", "Ueberschrift2"},
		{docx.NumDecimal, "%3.", "Ueberschrift3"},
	} {
		got := def.Levels[i]
		if got.Level != i {
			t.Errorf("level %d reported ilvl %d", i, got.Level)
		}
		if got.Format != want.format || got.Text != want.text || got.ParagraphStyle != want.style {
			t.Errorf("level %d = %+v, want %v/%q/%q", i, got, want.format, want.text, want.style)
		}
	}
}

// Word's numbering has nine levels. A definition declaring more is truncated
// rather than emitted as XML Word rejects; emit.Validate reports it.
func TestNumFormatCapsAtNineLevels(t *testing.T) {
	def := NumFormat{Levels: make([]NumFormat, 20)}.AbstractNum()
	if len(def.Levels) != MaxNumLevels {
		t.Errorf("got %d levels, want %d", len(def.Levels), MaxNumLevels)
	}
}

// A marginal number carries its own size, alignment and separator.
func TestNumFormatLabelProperties(t *testing.T) {
	var size FontSize
	if err := size.UnmarshalYAML(func(v any) error { *v.(*any) = "8pt"; return nil }); err != nil {
		t.Fatal(err)
	}
	def := NumFormat{
		Format: "decimal", Text: "%1.",
		Size: size, Align: "right", Suffix: "space", Style: "Standard",
	}.AbstractNum()

	lvl := def.Levels[0]
	if lvl.Size != docx.FontPt(8) {
		t.Errorf("Size = %v, want %v", lvl.Size, docx.FontPt(8))
	}
	if lvl.Align != docx.TabRight {
		t.Errorf("Align = %q, want %q", lvl.Align, docx.TabRight)
	}
	if lvl.Suffix != "space" {
		t.Errorf("Suffix = %q, want space", lvl.Suffix)
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

// A theme that declares no formats must render something unambiguous rather
// than something in a language it never chose.
func TestFormatsDefault(t *testing.T) {
	var th *Theme
	meta := map[string]any{
		"date":   time.Date(2024, time.March, 3, 0, 0, 0, 0, time.UTC),
		"urgent": true,
		"calm":   false,
		"items":  []any{"a", "b"},
	}

	tests := []struct{ in, want string }{
		{"{{ date }}", "2024-03-03"},
		{"{{ urgent }}", "true"},
		{"{{ calm }}", "false"},
		{"{{ items }}", "a, b"},
	}
	for _, tt := range tests {
		if got := th.Expand(tt.in, meta).Text; got != tt.want {
			t.Errorf("%s = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A configured theme supplies the layout and the names; the engine holds no
// locale of its own.
func TestFormatsConfigured(t *testing.T) {
	th := &Theme{Formats: Formats{
		Date:          "2. January 2006",
		Bool:          []string{"ja", "nein"},
		ListSeparator: "; ",
		Months: []string{
			"Januar", "Februar", "März", "April", "Mai", "Juni",
			"Juli", "August", "September", "Oktober", "November", "Dezember",
		},
		Weekdays: []string{
			"Sonntag", "Montag", "Dienstag", "Mittwoch",
			"Donnerstag", "Freitag", "Samstag",
		},
	}}
	meta := map[string]any{
		"date":   time.Date(2024, time.March, 3, 0, 0, 0, 0, time.UTC),
		"urgent": true,
		"calm":   false,
		"items":  []any{"a", "b"},
	}

	tests := []struct{ in, want string }{
		{"{{ date }}", "3. März 2024"},
		{"{{ urgent }}", "ja"},
		{"{{ calm }}", "nein"},
		{"{{ items }}", "a; b"},
	}
	for _, tt := range tests {
		if got := th.Expand(tt.in, meta).Text; got != tt.want {
			t.Errorf("%s = %q, want %q", tt.in, got, tt.want)
		}
	}

	// A weekday and a short month in one layout, to prove the substitution is a
	// single pass: "Mar" must not be reconsidered after "March" became "März".
	th.Formats.Date = "Monday, 2 Jan 2006"
	got := th.Expand("{{ date }}", meta).Text
	if got != "Sonntag, 3 Mär 2024" {
		t.Errorf("date = %q, want %q", got, "Sonntag, 3 Mär 2024")
	}
}
