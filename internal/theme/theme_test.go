package theme

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kevinzehnder/docc/internal/docx"
)

// TestLoadRejectsUnknownPageSize guards the config contract: an unknown page
// size is an error, not a silent fall-back to A4.
func TestLoadRejectsUnknownPageSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.yaml"), []byte("name: t\ndescription: x\npage:\n  size: A5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for an unknown page size")
	}
	if !strings.Contains(err.Error(), "unknown page size") {
		t.Errorf("error = %q, want it to mention the unknown page size", err)
	}
}

// write is a theme file in a directory built for one inheritance test.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// loadOne builds a theme directory from name/body pairs and returns the set.
func loadOne(t *testing.T, files ...string) *Set {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < len(files); i += 2 {
		write(t, dir, files[i], files[i+1])
	}
	set, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return set
}

// loadErr builds a theme directory that is expected not to load.
func loadErr(t *testing.T, files ...string) error {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < len(files); i += 2 {
		write(t, dir, files[i], files[i+1])
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected Load to fail")
	}
	return err
}

const parentTheme = `name: _house
description: house style
page:
  size: A4
  title_page: true
  margins: { top: 20mm, left: 25mm }
defaults: { font: Arial, size: 11pt }
formats:
  date: "2. January 2006"
  months: [Januar, Februar, März, April, Mai, Juni, Juli, August, September, Oktober, November, Dezember]
styles:
  body: { size: 11pt }
  titel: { size: 14pt, bold: true }
header:
  first:
    - { style: body, text: Kanzlei }
    - { style: body, text: Bahnhofstrasse 1 }
prologue:
  - { style: body, text: parent-prologue }
`

// A child inherits everything it does not restate, and its own declarations
// win key by key rather than replacing the whole gallery.
func TestExtendsMergesMapsKeyWise(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"protokoll.yaml", `name: protokoll
extends: _house
styles:
  titel: { size: 16pt }
`)
	got, err := set.Get("protokoll")
	if err != nil {
		t.Fatal(err)
	}
	if got.Styles["titel"].Size.HalfPt(0) != docx.HalfPt(32) {
		t.Errorf("titel size = %v, want the child's 16pt", got.Styles["titel"].Size)
	}
	// The sibling style survives: a child changing one style must not drop the
	// rest of the gallery.
	if _, ok := got.Styles["body"]; !ok {
		t.Errorf("styles = %v, want the parent's body style to survive", got.Styles)
	}
	if got.Defaults.Font != "Arial" {
		t.Errorf("defaults.font = %q, want the parent's Arial", got.Defaults.Font)
	}
	if !got.Page.TitlePage {
		t.Error("page.title_page = false, want the parent's true")
	}
	if got.Page.Margins.Top.Twips(0) != docx.Mm(20) {
		t.Errorf("page.margins.top = %v, want the parent's 20mm", got.Page.Margins.Top)
	}
}

// The child is always itself. Inheriting a name would give two themes one
// identity and make the second silently unreachable.
func TestExtendsDoesNotInheritName(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"protokoll.yaml", "extends: _house\n")
	got, err := set.Get("protokoll")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "protokoll" {
		t.Errorf("name = %q, want protokoll", got.Name)
	}
}

// This is the trap the whole design turns on: a child that says nothing about
// formats must keep the parent's month names. A field-wise merge over a value
// struct cannot tell "omitted" from "zero", and the failure is a document that
// renders its dates in English.
func TestExtendsKeepsParentFormatsWhenChildOmitsThem(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"protokoll.yaml", `extends: _house
styles:
  body: { size: 10pt }
`)
	got, err := set.Get("protokoll")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Formats.Months) != 12 || got.Formats.Months[2] != "März" {
		t.Errorf("formats.months = %v, want the parent's twelve German names", got.Formats.Months)
	}
	if got.Formats.Date != "2. January 2006" {
		t.Errorf("formats.date = %q, want the parent's layout", got.Formats.Date)
	}
}

// A child may override a parent's value with the zero value. Only a document
// merge can express this; it is why inheritance works on the YAML rather than
// on the decoded struct.
func TestExtendsAllowsOverridingWithZero(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"memo.yaml", `extends: _house
page:
  title_page: false
`)
	got, err := set.Get("memo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Page.TitlePage {
		t.Error("page.title_page = true, want the child's explicit false")
	}
}

// Furniture is ordered, so a child that declares a header replaces it rather
// than interleaving lines into it.
func TestExtendsReplacesSequencesWholesale(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"brief.yaml", `extends: _house
header:
  first:
    - { style: body, text: Zweigstelle }
`)
	got, err := set.Get("brief")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Header["first"]) != 1 || got.Header["first"][0].Text != "Zweigstelle" {
		t.Errorf("header.first = %+v, want only the child's single line", got.Header["first"])
	}
	// A sequence the child never mentions is still inherited.
	if len(got.Prologue) != 1 || got.Prologue[0].Text != "parent-prologue" {
		t.Errorf("prologue = %+v, want the parent's", got.Prologue)
	}
}

// Inheritance is transitive: house style, then practice area, then type.
func TestExtendsIsTransitive(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"_notariat.yaml", `name: _notariat
extends: _house
styles:
  titel: { caps: true }
`,
		"kaufvertrag.yaml", `extends: _notariat
styles:
  titel: { size: 18pt }
`)
	got, err := set.Get("kaufvertrag")
	if err != nil {
		t.Fatal(err)
	}
	titel := got.Styles["titel"]
	if titel.Size.HalfPt(0) != docx.HalfPt(36) {
		t.Errorf("titel size = %v, want the grandchild's 18pt", titel.Size)
	}
	if titel.Caps == nil || !*titel.Caps {
		t.Error("titel caps = false, want the middle theme's true")
	}
	if titel.Bold == nil || !*titel.Bold {
		t.Error("titel bold = false, want the root's true")
	}
	if got.Defaults.Font != "Arial" {
		t.Errorf("defaults.font = %q, want the root's Arial", got.Defaults.Font)
	}
}

// A fragment exists to be extended. Selecting one is a schema mistake, and it
// must say so rather than render a half-defined theme.
func TestFragmentsAreNotSelectable(t *testing.T) {
	set := loadOne(t,
		"_house.yaml", parentTheme,
		"protokoll.yaml", "extends: _house\n")
	if _, err := set.Get("_house"); err == nil {
		t.Fatal("expected selecting a fragment to fail")
	} else if !strings.Contains(err.Error(), "fragment") {
		t.Errorf("error = %q, want it to explain that the theme is a fragment", err)
	}
	if slices.Contains(set.Names(), "_house") {
		t.Errorf("Names() = %v, want fragments omitted", set.Names())
	}
	if !slices.Contains(set.Names(), "protokoll") {
		t.Errorf("Names() = %v, want the selectable theme listed", set.Names())
	}
}

func TestExtendsUnknownThemeFails(t *testing.T) {
	err := loadErr(t, "brief.yaml", "extends: _nothing\n")
	if !strings.Contains(err.Error(), "unknown theme") {
		t.Errorf("error = %q, want it to name the unknown parent", err)
	}
}

func TestExtendsCycleFails(t *testing.T) {
	err := loadErr(t,
		"a.yaml", "extends: b\n",
		"b.yaml", "extends: a\n")
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to report the inheritance cycle", err)
	}
}

// An unknown key must still be reported against the file that wrote it, not
// against a merged document nobody can open.
func TestExtendsStillRejectsUnknownKeysPerFile(t *testing.T) {
	err := loadErr(t,
		"_house.yaml", parentTheme,
		"brief.yaml", "extends: _house\nnonsense: true\n")
	if !strings.Contains(err.Error(), "brief.yaml") {
		t.Errorf("error = %q, want it to name the offending file", err)
	}
}

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

// {{ page }} and {{ pages }} are fields Word computes, not metadata. They must
// not reach Fields(), or emit.Validate would demand a schema declare a field
// named "page" — and they must not count as empty, or a footer consisting of
// nothing but a page number would be dropped as a blank line.
func TestReservedPageFields(t *testing.T) {
	th := &Theme{Footer: map[string][]Line{
		"default": {{Text: "Seite {{ page }} von {{ pages }}"}},
	}}
	if fields := th.Fields(); len(fields) != 0 {
		t.Errorf("Fields() = %v, want none: page and pages are not schema fields", fields)
	}

	got := th.Expand("Seite {{ page }} von {{ pages }}", nil)
	if want := "Seite " + FieldPage + " von " + FieldPages; got.Text != want {
		t.Errorf("Expand() = %q, want %q", got.Text, want)
	}
	if got.AllEmpty {
		t.Error("AllEmpty = true; a page number is content, so the line stays")
	}
	if len(got.Missing) != 0 {
		t.Errorf("Missing = %v, want none", got.Missing)
	}
}

// FieldRefs carries the region each reference occurs in, which is what lets a
// contract say where a field ends up — and, by its absence, that a field is
// metadata the theme never prints.
func TestThemeFieldRefs(t *testing.T) {
	th := &Theme{
		Prologue: []Line{{Text: "{{ sender.name }}"}},
		Epilogue: []Line{{Text: "{{ signee.name }}"}},
		Header:   map[string][]Line{"first": {{Text: "{{ sender.name }}"}}},
		Footer:   map[string][]Line{"default": {{Text: "{{ date }}"}}},
	}

	got := map[string][]string{}
	for _, ref := range th.FieldRefs() {
		got[ref.Path] = append(got[ref.Path], ref.Region)
	}

	// sender.name appears twice, in two different regions.
	if want := []string{"prologue", "header:first"}; !slices.Equal(got["sender.name"], want) {
		t.Errorf("sender.name regions = %v, want %v", got["sender.name"], want)
	}
	if want := []string{"epilogue"}; !slices.Equal(got["signee.name"], want) {
		t.Errorf("signee.name regions = %v, want %v", got["signee.name"], want)
	}
	if want := []string{"footer:default"}; !slices.Equal(got["date"], want) {
		t.Errorf("date regions = %v, want %v", got["date"], want)
	}

	// Fields stays deduplicated on top of it.
	if n := len(th.Fields()); n != 3 {
		t.Errorf("Fields() = %d paths, want 3 deduplicated", n)
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

// `restart:` is how a sub-level keeps counting across its parents. Unset means
// Word's own default, which is the behaviour every existing theme relies on, so
// it must reach docx as "no element" rather than as a zero.
func TestNumFormatRestart(t *testing.T) {
	def := NumFormat{
		Format: "upperLetter", Text: "%1.)",
		Levels: []NumFormat{
			{Format: "decimal", Text: "%2.", Restart: RestartNever},
			{Format: "decimal", Text: "%3.", Restart: RestartAfterParent},
			{Format: "decimal", Text: "%4."},
		},
	}.AbstractNum()

	if got := def.Levels[1].Restart; got == nil || *got != 0 {
		t.Errorf("restart: never = %v, want a pointer to 0", got)
	}
	for _, i := range []int{0, 2, 3} {
		if got := def.Levels[i].Restart; got != nil {
			t.Errorf("level %d restart = %v, want nil (Word's default)", i, *got)
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
