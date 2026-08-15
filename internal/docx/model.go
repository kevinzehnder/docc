package docx

// Document is a Word document under construction. Build it up, then hand it to
// Write or Bytes.
type Document struct {
	// Body holds the document content in order.
	Body []Block
	// Section describes page geometry and header/footer attachment. Word stores
	// it as the final element of the body.
	Section Section
	// Styles are the named styles the body refers to. A paragraph naming a
	// style that is not defined here still renders, but without formatting.
	Styles []Style
	// Numbering holds list definitions. Empty means no numbering.xml part.
	Numbering Numbering
	// Headers and Footers are attached through Section.
	Headers []HeaderFooter
	Footers []HeaderFooter
	// Defaults are the document-wide run and paragraph defaults.
	Defaults Defaults
	// Properties are the core document properties (title, author).
	Properties Properties
	// Custom are document properties Word shows under File > Info > Advanced.
	// They are written in the order given, so output stays deterministic.
	Custom []CustomProperty

	media []mediaFile
}

// CustomProperty is one named value in the custom document properties part.
// It is where provenance belongs: which profile, at which revision, produced
// this file. Word displays it and leaves it alone.
type CustomProperty struct {
	Name  string
	Value string
}

// Properties are the core document properties Word shows in its file dialog.
type Properties struct {
	Title       string
	Subject     string
	Creator     string
	Description string
	Keywords    string
	// Created and Modified are ISO 8601 timestamps. Leave empty for a
	// deterministic build; a zero timestamp is written instead.
	Created  string
	Modified string
}

// Defaults are the docDefaults applied before any style.
type Defaults struct {
	Run       RunProps
	Paragraph ParaProps
}

// Block is a top-level body element: a paragraph or a table.
type Block interface {
	writeBlock(w *xw, d *Document)
}

// ---------------------------------------------------------------------------
// Section
// ---------------------------------------------------------------------------

// PageSize is the physical page.
type PageSize struct {
	Width, Height Twips
	Landscape     bool
}

// Margins are page margins. Header and Footer are the distance from the page
// edge to the header and footer bands, not to the text.
type Margins struct {
	Top, Right, Bottom, Left Twips
	Header, Footer, Gutter   Twips
}

// HeaderFooterType selects which pages a header or footer applies to.
type HeaderFooterType string

const (
	// HFDefault applies to every page not covered by another type.
	HFDefault HeaderFooterType = "default"
	// HFFirst applies to the first page only, and requires
	// Section.TitlePage to be set.
	HFFirst HeaderFooterType = "first"
	// HFEven applies to even pages, and requires evenAndOddHeaders in
	// settings, which this package sets whenever an even header exists.
	HFEven HeaderFooterType = "even"
)

// HeaderFooter is the content of one header or footer part.
type HeaderFooter struct {
	Type   HeaderFooterType
	Blocks []Block

	// relID and partName are assigned during packaging.
	relID    string
	partName string
}

// Section is the document's page setup.
type Section struct {
	Page    PageSize
	Margins Margins
	// NextPage starts this section on a new page when it appears as a paragraph
	// section break. It is ignored for the final document section.
	NextPage bool
	// TitlePage enables a distinct first-page header, which a letterhead
	// normally needs: the logo appears once, not on every page.
	TitlePage bool
	// Cols is the number of text columns. Zero means one column.
	Cols int
}

// ---------------------------------------------------------------------------
// Paragraph
// ---------------------------------------------------------------------------

// Align is horizontal paragraph alignment.
type Align string

const (
	AlignLeft    Align = "left"
	AlignCenter  Align = "center"
	AlignRight   Align = "right"
	AlignJustify Align = "both"
)

// LineRule selects how Spacing.Line is interpreted.
type LineRule string

const (
	// LineAuto treats Line as a multiple of single spacing, in 240ths.
	LineAuto LineRule = "auto"
	// LineExact fixes the line height at Line twips.
	LineExact LineRule = "exact"
	// LineAtLeast makes Line a minimum.
	LineAtLeast LineRule = "atLeast"
)

// Spacing is vertical paragraph spacing.
//
// ExplicitBefore and ExplicitAfter force the value to be written even when it
// is zero. Without them a style could never override an inherited spacing back
// to nothing: an omitted attribute inherits, and zero is indistinguishable from
// absent in a plain int.
type Spacing struct {
	Before, After  Twips
	Line           Twips
	LineRule       LineRule
	ExplicitBefore bool
	ExplicitAfter  bool
}

// Indent is horizontal paragraph indentation. FirstLine and Hanging are
// mutually exclusive; Hanging wins if both are set.
type Indent struct {
	Left, Right Twips
	FirstLine   Twips
	Hanging     Twips
}

// FramePr positions a paragraph as a floating frame. This is how the address
// block lands in the envelope window: a frame reflows correctly and needs a
// fraction of the XML that an equivalent text box does.
type FramePr struct {
	Width, Height Twips
	// X and Y are absolute offsets from HAnchor and VAnchor.
	X, Y Twips
	// HAnchor and VAnchor are "page", "margin" or "text".
	HAnchor, VAnchor string
	// Wrap is "around", "none", "notBeside", "auto".
	Wrap string
}

// ParaProps is paragraph formatting. The zero value adds nothing.
type ParaProps struct {
	Style     string
	Align     Align
	Spacing   Spacing
	Indent    Indent
	Frame     *FramePr
	Numbering *NumRef
	// SectionBreak ends the current section after this paragraph. Its page
	// geometry applies to the section that just ended; the document's final
	// Section applies to the following content.
	SectionBreak *Section
	Tabs         []TabStop
	Borders      *ParaBorders
	// KeepNext keeps this paragraph on the same page as the next, which is
	// what stops a heading being orphaned at a page break.
	KeepNext  bool
	KeepLines bool
	PageBreak bool
	// ContextualSpacing suppresses spacing between paragraphs of the same
	// style, the usual want for list items.
	ContextualSpacing bool
	// Shading is a background fill as a hex colour without "#".
	Shading string
	// OutlineLevel sets the heading level for the navigation pane and for PDF
	// bookmarks; 0 means body text. Word uses 0-8 for levels 1-9.
	OutlineLevel *int
}

// TabAlign is the alignment of a tab stop.
type TabAlign string

const (
	TabLeft    TabAlign = "left"
	TabCenter  TabAlign = "center"
	TabRight   TabAlign = "right"
	TabDecimal TabAlign = "decimal"
)

// TabStop is a single tab stop.
type TabStop struct {
	Pos   Twips
	Align TabAlign
	// Leader is "none", "dot", "hyphen", "underscore".
	Leader string
}

// BorderStyle names a border line style.
type BorderStyle string

const (
	BorderNone   BorderStyle = "none"
	BorderSingle BorderStyle = "single"
	BorderDouble BorderStyle = "double"
	BorderDotted BorderStyle = "dotted"
	BorderDashed BorderStyle = "dashed"
)

// Border is one edge.
type Border struct {
	Style BorderStyle
	Size  Eighth
	Space Twips
	Color string
}

// ParaBorders are the four edges of a paragraph. A single bottom border is the
// usual way to draw the rule under a letterhead.
type ParaBorders struct {
	Top, Bottom, Left, Right *Border
}

// Paragraph is a block of inline content.
type Paragraph struct {
	Props ParaProps
	Runs  []Run
}

func (p Paragraph) writeBlock(w *xw, d *Document) { p.write(w, d) }

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

// VertAlign raises or lowers text.
type VertAlign string

const (
	VertBaseline    VertAlign = "baseline"
	VertSuperscript VertAlign = "superscript"
	VertSubscript   VertAlign = "subscript"
)

// RunProps is character formatting. The zero value adds nothing.
type RunProps struct {
	Style     string
	Font      string
	Size      HalfPt
	Bold      bool
	Italic    bool
	Underline string // "single", "double", "none"
	Strike    bool
	Color     string
	// Highlight is a named Word highlight colour, e.g. "yellow". It is how an
	// unfilled field is made impossible to miss.
	Highlight string
	Caps      bool
	SmallCaps bool
	VertAlign VertAlign
	// Spacing is inter-character spacing in twips.
	Spacing Twips
	// Lang tags the text's language so spell check behaves.
	Lang string
}

// Run is a span of inline content sharing one set of properties.
type Run struct {
	Props RunProps
	Items []Inline
}

// Inline is content inside a run: text, a tab, a break, or a drawing.
type Inline interface {
	writeInline(w *xw, d *Document)
}

// Text is literal text. Leading and trailing spaces are preserved.
type Text string

// Tab advances to the next tab stop.
type Tab struct{}

// BreakType selects what a Break breaks.
type BreakType string

const (
	BreakLine   BreakType = "textWrapping"
	BreakPage   BreakType = "page"
	BreakColumn BreakType = "column"
)

// Break is a line, page or column break.
type Break struct{ Type BreakType }

// Drawing is an inline image.
type Drawing struct {
	// Name identifies the image in the media store, set by AddImage.
	Name string
	// Width and Height are the display size.
	Width, Height EMU
	// AltText is the accessibility description.
	AltText string

	relID string
	docPr int
}

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

// Table is a simple grid. Column widths are fixed; the layout is not
// auto-fitted, because auto-fit renders differently across Word and
// LibreOffice and this corpus needs predictable output.
type Table struct {
	// Widths are the column widths, in order. Required.
	Widths []Twips
	Rows   []TableRow
	// Borders apply to the whole table.
	Borders *TableBorders
	// Indent shifts the table right.
	Indent Twips
	// Style names a table style.
	Style string
}

func (t Table) writeBlock(w *xw, d *Document) { t.write(w, d) }

// TableBorders are the outer edges plus the inner grid lines.
type TableBorders struct {
	Top, Bottom, Left, Right *Border
	InsideH, InsideV         *Border
}

// TableRow is one row.
type TableRow struct {
	Cells []TableCell
	// Height is a minimum row height; zero means automatic.
	Height Twips
	// Header repeats the row at the top of each page the table spans.
	Header bool
	// CantSplit keeps the row from breaking across pages.
	CantSplit bool
}

// VAlign is vertical alignment within a cell.
type VAlign string

const (
	VAlignTop    VAlign = "top"
	VAlignCenter VAlign = "center"
	VAlignBottom VAlign = "bottom"
)

// TableCell is one cell. A cell must contain at least one paragraph; an empty
// Blocks slice is filled with one at write time, because Word rejects an empty
// cell.
type TableCell struct {
	Blocks []Block
	// Span is the number of grid columns this cell covers. Zero means one.
	Span    int
	VAlign  VAlign
	Shading string
	Borders *TableBorders
	// Margins override the table cell margins.
	Margins *CellMargins
}

// CellMargins are the insets of a table cell.
type CellMargins struct {
	Top, Right, Bottom, Left Twips
}

// ---------------------------------------------------------------------------
// Convenience constructors
// ---------------------------------------------------------------------------

// P builds a paragraph in the named style from plain text. Passing an empty
// string produces an empty paragraph, which is a legitimate spacer.
func P(style, text string) Paragraph {
	p := Paragraph{Props: ParaProps{Style: style}}
	if text != "" {
		p.Runs = []Run{{Items: []Inline{Text(text)}}}
	}
	return p
}

// R builds a run of plain text.
func R(text string) Run {
	return Run{Items: []Inline{Text(text)}}
}

// RB builds a bold run of plain text.
func RB(text string) Run {
	return Run{Props: RunProps{Bold: true}, Items: []Inline{Text(text)}}
}

// Cell builds a table cell holding a single paragraph.
func Cell(style, text string) TableCell {
	return TableCell{Blocks: []Block{P(style, text)}}
}
