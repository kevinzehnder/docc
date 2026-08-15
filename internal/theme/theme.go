// Package theme defines the visual side of a document type: page geometry,
// named styles, list definitions, and the fixed furniture — letterhead, address
// block, footer — that surrounds the body.
//
// The split against schema is deliberate. A schema says a legal brief has a
// RECHTSBEGEHREN section and that its list uses the style "Rechtsbegehren"; a
// theme says what "Rechtsbegehren" looks like and where the letterhead sits.
// Changing the house style therefore touches no schema, and adding a document
// type touches no theme.
package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/kevinzehnder/docc/internal/docx"
)

// Theme is one visual definition, loaded from a YAML file.
type Theme struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`

	Page      Page                 `yaml:"page"`
	Defaults  Defaults             `yaml:"defaults"`
	Formats   Formats              `yaml:"formats"`
	Styles    map[string]Style     `yaml:"styles"`
	Numbering map[string]NumFormat `yaml:"numbering"`

	// Header and Footer are keyed by "first", "default" or "even".
	Header map[string][]Line `yaml:"header"`
	Footer map[string][]Line `yaml:"footer"`

	// Prologue is placed at the top of the body, before the rendered markdown.
	// It carries the letter furniture: address block, date line, subject.
	Prologue []Line `yaml:"prologue"`
	// Epilogue is placed after the body: closing, signature, enclosures.
	Epilogue []Line `yaml:"epilogue"`
}

// Page is the sheet and its margins.
type Page struct {
	// Size is "A4" or "Letter". Width and Height override it.
	Size      string  `yaml:"size"`
	Width     Length  `yaml:"width"`
	Height    Length  `yaml:"height"`
	Landscape bool    `yaml:"landscape"`
	Margins   Margins `yaml:"margins"`
	// ContinuationMargins override the first section's margins after a
	// furniture line creates a section break. Omitted sides retain Margins.
	ContinuationMargins *Margins `yaml:"continuation_margins"`
	// TitlePage gives the first page its own header and footer, which a
	// letterhead needs: the logo belongs on page one, not on every page.
	TitlePage bool `yaml:"title_page"`
}

// Margins are the page margins.
type Margins struct {
	Top    Length `yaml:"top"`
	Bottom Length `yaml:"bottom"`
	Left   Length `yaml:"left"`
	Right  Length `yaml:"right"`
	Header Length `yaml:"header"`
	Footer Length `yaml:"footer"`
	Gutter Length `yaml:"gutter"`
}

// Defaults are the document-wide font settings applied before any style.
type Defaults struct {
	Font string   `yaml:"font"`
	Size FontSize `yaml:"size"`
	Lang string   `yaml:"lang"`
}

// Formats says how non-string metadata is rendered into text. This is
// presentation, and presentation is the theme's business: the day order, the
// month names and the word for true differ per document, and none of them
// belong in the engine.
//
// The month and weekday names are supplied here rather than looked up from a
// locale, because a locale database is a dependency and a table is four lines
// of YAML.
type Formats struct {
	// Date is a Go reference layout, e.g. "2. January 2006". Defaults to ISO
	// 8601 — a theme that says nothing should render something unambiguous.
	Date string `yaml:"date"`
	// Bool is [true, false]. Defaults to ["true", "false"].
	Bool []string `yaml:"bool"`
	// ListSeparator joins a list field into one line. Defaults to ", ".
	ListSeparator string `yaml:"list_separator"`
	// AmountWords is how a money block writes an amount in words, with %s for
	// the spelled figure: "(Franken %s)". Empty means amounts are not spelled
	// out. The speller is German, which is what this corpus needs; a theme in
	// another language leaves this unset.
	AmountWords string `yaml:"amount_words"`

	// Months translates the twelve month names in calendar order. Short names
	// are the first three characters unless MonthsShort gives them.
	Months      []string `yaml:"months"`
	MonthsShort []string `yaml:"months_short"`
	// Weekdays translates the seven day names, Sunday first, matching Go's
	// week. Short names default to the first three characters.
	Weekdays      []string `yaml:"weekdays"`
	WeekdaysShort []string `yaml:"weekdays_short"`
}

// Style is a named style definition.
type Style struct {
	// Name is the display name in Word's gallery. Defaults to the key.
	Name    string `yaml:"name"`
	BasedOn string `yaml:"based_on"`
	Next    string `yaml:"next"`
	Default bool   `yaml:"default"`
	// Type is "paragraph" or "character"; defaults to paragraph.
	Type string `yaml:"type"`

	// Character formatting.
	Font      string   `yaml:"font"`
	Size      FontSize `yaml:"size"`
	Bold      bool     `yaml:"bold"`
	Italic    bool     `yaml:"italic"`
	Underline string   `yaml:"underline"`
	Caps      bool     `yaml:"caps"`
	SmallCaps bool     `yaml:"small_caps"`
	Color     string   `yaml:"color"`

	// Paragraph formatting.
	Align   string   `yaml:"align"`
	Spacing Spacing  `yaml:"spacing"`
	Indent  Indent   `yaml:"indent"`
	Tabs    []Tab    `yaml:"tabs"`
	Borders *Borders `yaml:"borders"`
	// OutlineLevel sets the heading depth for the navigation pane and PDF
	// bookmarks. 1 is the top level; omit for body text.
	OutlineLevel int `yaml:"outline_level"`
	// KeepNext stops a heading being stranded at the foot of a page.
	KeepNext          bool   `yaml:"keep_next"`
	KeepLines         bool   `yaml:"keep_lines"`
	PageBreakBefore   bool   `yaml:"page_break_before"`
	ContextualSpacing bool   `yaml:"contextual_spacing"`
	Shading           string `yaml:"shading"`
	UIPriority        int    `yaml:"ui_priority"`
}

// Spacing is vertical paragraph spacing.
type Spacing struct {
	Before Length     `yaml:"before"`
	After  Length     `yaml:"after"`
	Line   LineHeight `yaml:"line"`
}

// Indent is horizontal paragraph indentation.
type Indent struct {
	Left      Length `yaml:"left"`
	Right     Length `yaml:"right"`
	FirstLine Length `yaml:"first_line"`
	Hanging   Length `yaml:"hanging"`
}

// Tab is a tab stop.
type Tab struct {
	Pos    Length `yaml:"pos"`
	Align  string `yaml:"align"`
	Leader string `yaml:"leader"`
}

// Borders are paragraph borders. A bottom border alone is how a rule is drawn
// under a letterhead.
type Borders struct {
	Top    *BorderEdge `yaml:"top"`
	Bottom *BorderEdge `yaml:"bottom"`
	Left   *BorderEdge `yaml:"left"`
	Right  *BorderEdge `yaml:"right"`
}

// BorderEdge is one edge.
type BorderEdge struct {
	Style string  `yaml:"style"`
	Width float64 `yaml:"width"` // points
	Space Length  `yaml:"space"`
	Color string  `yaml:"color"`
}

// NumFormat is a list definition referenced by a schema's style map.
type NumFormat struct {
	// Format is "decimal", "lowerLetter", "upperLetter", "lowerRoman",
	// "upperRoman", "bullet" or "none".
	Format string `yaml:"format"`
	// Text is the rendered label; "%1." yields "1.".
	Text    string `yaml:"text"`
	Start   int    `yaml:"start"`
	Indent  Length `yaml:"indent"`
	Hanging Length `yaml:"hanging"`
	Font    string `yaml:"font"`
	// Size sets the label's font size independently of the text it labels, which
	// is what a marginal number needs: small digit, normal prose.
	Size FontSize `yaml:"size"`
	// Align positions the label within the space the hanging indent reserves:
	// "left" (default), "center", "right" or "decimal".
	Align string `yaml:"align"`
	// Suffix is what separates the label from the text: "tab" (default),
	// "space" or "nothing".
	Suffix string `yaml:"suffix"`
	// Style is the paragraph style applied to items at this level.
	Style string `yaml:"style"`
	// Levels defines deeper levels. Level 0 is this definition itself.
	Levels []NumFormat `yaml:"levels"`
}

// Line is one paragraph of fixed furniture. Its text may interpolate document
// metadata with {{ field.path }}.
type Line struct {
	Style string `yaml:"style"`
	Text  string `yaml:"text"`
	// Runs expresses a paragraph whose formatting changes partway through — a
	// party entry is a bold name, a tab, the role in normal weight, then the
	// address a size smaller. When set, Text is ignored.
	Runs []LineRun `yaml:"runs"`
	// Frame positions the line, and any that follow it in the same block, as a
	// floating frame. This is how the address block reaches the envelope
	// window without a text box.
	Frame *Frame `yaml:"frame"`
	// Image embeds a picture from the theme directory.
	Image *Image `yaml:"image"`
	// Repeat names a list field, emitting one paragraph per element. Inside
	// the text, {{ item }} is the element.
	Repeat string `yaml:"repeat"`
	// IfNonempty emits this line only when the named metadata field has a value.
	// It is for furniture that introduces a repeated list (for example, an
	// enclosures heading) and must disappear with an empty list.
	IfNonempty string `yaml:"if_nonempty"`
	// Numbering names a definition in the theme's `numbering:` map, giving the
	// line a Word list number. Every line naming the same definition within one
	// block of furniture shares an instance, so a `repeat` over a list comes out
	// as 1., 2., 3. — which is what an enclosures index is.
	//
	// The label is Word numbering, not text: the index renumbers itself when an
	// entry is added, and a cross-reference check can still read the underlying
	// list.
	Numbering string `yaml:"numbering"`
	// OmitIfEmpty drops the line when every field it interpolates is empty.
	// Defaults to true: a recipient without an organisation should not leave a
	// blank line in the address block.
	OmitIfEmpty *bool `yaml:"omit_if_empty"`
	// PageBreak starts a new page before this line.
	PageBreak bool `yaml:"page_break"`
	// SectionBreak ends the current section after this line and starts the next
	// one on a new page. It activates page.continuation_margins.
	SectionBreak bool `yaml:"section_break"`
	// Tab inserts a tab before the text, for a right-aligned date line.
	Tabs []Tab `yaml:"tabs"`
}

// LineRun is one formatting span within a furniture line.
type LineRun struct {
	// Style names a character style defined in the theme.
	Style  string   `yaml:"style"`
	Text   string   `yaml:"text"`
	Bold   bool     `yaml:"bold"`
	Italic bool     `yaml:"italic"`
	Size   FontSize `yaml:"size"`
	Color  string   `yaml:"color"`
	// Tab emits a tab stop before the text.
	Tab bool `yaml:"tab"`
	// Break emits a line break before the text.
	Break bool `yaml:"break"`
	// OmitIfEmpty drops just this run when its interpolation is empty, leaving
	// the rest of the line intact. Defaults to true.
	OmitIfEmpty *bool `yaml:"omit_if_empty"`
}

// Omit reports whether an empty run should be dropped.
func (r LineRun) Omit() bool {
	if r.OmitIfEmpty == nil {
		return true
	}
	return *r.OmitIfEmpty
}

// Frame positions a paragraph absolutely.
type Frame struct {
	X       Length `yaml:"x"`
	Y       Length `yaml:"y"`
	Width   Length `yaml:"width"`
	Height  Length `yaml:"height"`
	HAnchor string `yaml:"h_anchor"`
	VAnchor string `yaml:"v_anchor"`
	Wrap    string `yaml:"wrap"`
}

// Image is a picture placed in a line.
type Image struct {
	// Path is relative to the theme file's directory.
	Path   string `yaml:"path"`
	Width  Length `yaml:"width"`
	Height Length `yaml:"height"`
	Alt    string `yaml:"alt"`
}

// Set is the themes available to a project.
type Set struct {
	byName map[string]*Theme
	// Dir is where the themes were loaded from, used to resolve image paths.
	Dir string
}

// Get returns a theme by name.
func (s *Set) Get(name string) (*Theme, error) {
	if s == nil || len(s.byName) == 0 {
		return nil, fmt.Errorf("no themes loaded")
	}
	t, ok := s.byName[name]
	if !ok {
		return nil, fmt.Errorf("unknown theme %q (known: %s)", name, strings.Join(s.Names(), ", "))
	}
	return t, nil
}

// Names lists the loaded themes in sorted order.
func (s *Set) Names() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.byName))
	for k := range s.byName {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Load reads every *.yaml file in dir as a theme.
func Load(dir string) (*Set, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read theme dir: %w", err)
	}

	set := &Set{byName: map[string]*Theme{}, Dir: dir}
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path) //nolint:gosec // listing of the project's own theme dir
		if err != nil {
			return nil, err
		}
		var t Theme
		if err := yaml.UnmarshalWithOptions(b, &t, yaml.Strict()); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if t.Name == "" {
			t.Name = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		}
		// An unknown page size must fail loudly rather than fall back to A4:
		// `size: A5` silently rendering as A4 is exactly the kind of production
		// surprise a compiler exists to prevent. Omitting it stays a documented
		// default; naming one that does not exist is an error.
		if s := strings.ToLower(t.Page.Size); s != "" && s != "a4" && s != "letter" {
			return nil, fmt.Errorf("%s: unknown page size %q — use A4 or Letter, or set page.width and page.height", path, t.Page.Size)
		}
		if prev, dup := set.byName[t.Name]; dup {
			return nil, fmt.Errorf("%s: theme %q already declared (%s)", path, t.Name, prev.Description)
		}
		set.byName[t.Name] = &t
	}
	if len(set.byName) == 0 {
		return nil, fmt.Errorf("no themes found in %s", dir)
	}
	return set, nil
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

// PageSize resolves the page geometry.
func (p Page) PageSize() docx.PageSize {
	size := docx.A4
	switch strings.ToLower(p.Size) {
	case "", "a4":
		size = docx.A4
	case "letter":
		size = docx.Letter
	}
	if p.Width.Set() {
		size.Width = p.Width.Twips(size.Width)
	}
	if p.Height.Set() {
		size.Height = p.Height.Twips(size.Height)
	}
	size.Landscape = p.Landscape
	return size
}

// DocxMargins converts theme margins, defaulting to values that keep text on
// the page rather than to zero.
// Merge fills unset override sides from base.
func (m Margins) Merge(base Margins) Margins {
	out := base
	if m.Top.Set() {
		out.Top = m.Top
	}
	if m.Bottom.Set() {
		out.Bottom = m.Bottom
	}
	if m.Left.Set() {
		out.Left = m.Left
	}
	if m.Right.Set() {
		out.Right = m.Right
	}
	if m.Header.Set() {
		out.Header = m.Header
	}
	if m.Footer.Set() {
		out.Footer = m.Footer
	}
	if m.Gutter.Set() {
		out.Gutter = m.Gutter
	}
	return out
}

func (m Margins) DocxMargins() docx.Margins {
	return docx.Margins{
		Top:    m.Top.Twips(docx.Mm(20)),
		Bottom: m.Bottom.Twips(docx.Mm(20)),
		Left:   m.Left.Twips(docx.Mm(25)),
		Right:  m.Right.Twips(docx.Mm(20)),
		Header: m.Header.Twips(docx.Mm(10)),
		Footer: m.Footer.Twips(docx.Mm(10)),
		Gutter: m.Gutter.Twips(0),
	}
}

// Omit reports whether a line should be dropped when its interpolation is
// empty. Absent means yes.
func (l Line) Omit() bool {
	if l.OmitIfEmpty == nil {
		return true
	}
	return *l.OmitIfEmpty
}
