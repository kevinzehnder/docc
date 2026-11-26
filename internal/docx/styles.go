package docx

import (
	"fmt"
	"strings"
)

// StyleType is the kind of thing a style applies to.
type StyleType string

const (
	StyleParagraph StyleType = "paragraph"
	StyleCharacter StyleType = "character"
	StyleTable     StyleType = "table"
	StyleNumbering StyleType = "numbering"
)

// Style is a named style definition.
type Style struct {
	// ID is the identifier the body refers to (w:pStyle w:val). Keep it free of
	// spaces; Word tolerates spaces but tooling generally does not.
	ID string
	// Name is the display name shown in Word's style gallery.
	Name string
	Type StyleType
	// BasedOn is the ID of the parent style.
	BasedOn string
	// Next is the ID of the style applied to the following paragraph, which is
	// what makes pressing Enter after a heading return to body text.
	Next string
	// LinkedTo pairs a paragraph style with its character style.
	LinkedTo string
	// Para and Run are the formatting this style contributes.
	Para ParaProps
	Run  RunProps
	// QFormat lists the style in Word's gallery.
	QFormat bool
	// Default marks this the default style for its type. Exactly one paragraph
	// style should set it.
	Default bool
	// Hidden keeps the style out of the gallery.
	Hidden bool
	// UIPriority orders the style in the gallery; lower sorts first.
	UIPriority int
}

// writeStyles renders the styles.xml part.
func (d *Document) writeStyles() []byte {
	w := &xw{ids: d.styleIDs}
	w.header()
	w.open("w:styles", nsAttrs()...)

	w.open("w:docDefaults")
	w.open("w:rPrDefault")
	writeRunProps(w, d.Defaults.Run, false)
	w.close("w:rPrDefault")
	w.open("w:pPrDefault")
	writeParaProps(w, d.Defaults.Paragraph, false, nil)
	w.close("w:pPrDefault")
	w.close("w:docDefaults")

	for _, s := range d.Styles {
		writeStyle(w, s)
	}

	w.close("w:styles")
	return w.bytes()
}

func writeStyle(w *xw, s Style) {
	typ := s.Type
	if typ == "" {
		typ = StyleParagraph
	}
	attrs := []attr{a("w:type", string(typ)), a("w:styleId", w.styleID(s.ID))}
	if s.Default {
		attrs = append(attrs, a("w:default", "1"))
	}
	w.open("w:style", attrs...)

	name := s.Name
	if name == "" {
		name = s.ID
	}
	// Word writes the default paragraph style as "Normal" in every language
	// and shows it localised ("Standard"); a localised name here would read
	// as a second, custom style.
	if isDefaultParagraph(s) {
		name = "Normal"
	}
	w.empty("w:name", a("w:val", name))
	if s.BasedOn != "" {
		w.empty("w:basedOn", a("w:val", w.styleID(s.BasedOn)))
	}
	if s.Next != "" {
		w.empty("w:next", a("w:val", w.styleID(s.Next)))
	}
	if s.LinkedTo != "" {
		w.empty("w:link", a("w:val", w.styleID(s.LinkedTo)))
	}
	if s.UIPriority > 0 {
		w.empty("w:uiPriority", ai("w:val", s.UIPriority))
	}
	if s.QFormat {
		w.empty("w:qFormat")
	}
	if s.Hidden {
		w.empty("w:semiHidden")
		w.empty("w:unhideWhenUsed")
	}

	// A character style carries no paragraph properties; emitting an empty
	// w:pPr inside one is invalid.
	if typ != StyleCharacter {
		writeParaProps(w, s.Para, false, nil)
	}
	writeRunProps(w, s.Run, false)

	w.close("w:style")
}

// builtinStyleIDs maps the lower-cased name of a Word built-in style to the
// styleId Word itself writes for it. Word recognises a built-in by name, but
// tools that address styles by ID (WordCraft's para.heading1 looks up
// "Heading1") only find one written under its canonical ID. The rule is the
// English name with spaces removed and words capitalised; "Normal Table" and
// the TOC levels are Word's own exceptions.
var builtinStyleIDs = func() map[string]string {
	m := map[string]string{
		"normal":                 "Normal",
		"title":                  "Title",
		"subtitle":               "Subtitle",
		"quote":                  "Quote",
		"intense quote":          "IntenseQuote",
		"list paragraph":         "ListParagraph",
		"list bullet":            "ListBullet",
		"list number":            "ListNumber",
		"list":                   "List",
		"no spacing":             "NoSpacing",
		"caption":                "Caption",
		"header":                 "Header",
		"footer":                 "Footer",
		"footnote text":          "FootnoteText",
		"footnote reference":     "FootnoteReference",
		"endnote text":           "EndnoteText",
		"endnote reference":      "EndnoteReference",
		"body text":              "BodyText",
		"body text indent":       "BodyTextIndent",
		"block text":             "BlockText",
		"plain text":             "PlainText",
		"signature":              "Signature",
		"closing":                "Closing",
		"salutation":             "Salutation",
		"date":                   "Date",
		"page number":            "PageNumber",
		"default paragraph font": "DefaultParagraphFont",
		"strong":                 "Strong",
		"emphasis":               "Emphasis",
		"subtle emphasis":        "SubtleEmphasis",
		"intense emphasis":       "IntenseEmphasis",
		"subtle reference":       "SubtleReference",
		"intense reference":      "IntenseReference",
		"book title":             "BookTitle",
		"hyperlink":              "Hyperlink",
		"normal table":           "TableNormal",
		"table grid":             "TableGrid",
		"no list":                "NoList",
		"toc heading":            "TOCHeading",
	}
	for i := 1; i <= 9; i++ {
		m[fmt.Sprintf("heading %d", i)] = fmt.Sprintf("Heading%d", i)
		m[fmt.Sprintf("toc %d", i)] = fmt.Sprintf("TOC%d", i)
	}
	return m
}()

// canonicalStyleIDs returns the renames that put built-in styles under Word's
// IDs: a style named like a built-in gets that ID, and the default paragraph
// style becomes "Normal". The first style in order claims a canonical ID; a
// style whose own ID is taken by such a claim gets "_2", "_3", … appended.
// Callers keep using their own IDs throughout; only the writer applies this.
func canonicalStyleIDs(styles []Style) map[string]string {
	ids := map[string]string{}
	taken := map[string]bool{}
	claimed := make([]bool, len(styles))
	for i, s := range styles {
		c := builtinStyleIDs[strings.ToLower(s.Name)]
		if isDefaultParagraph(s) {
			c = "Normal"
		}
		if c == "" || taken[c] {
			continue
		}
		taken[c] = true
		claimed[i] = true
		if c != s.ID {
			ids[s.ID] = c
		}
	}
	var displaced []int
	for i, s := range styles {
		switch {
		case claimed[i]:
		case taken[s.ID]:
			displaced = append(displaced, i)
		default:
			taken[s.ID] = true
		}
	}
	for _, i := range displaced {
		id := styles[i].ID
		c := id
		for n := 2; taken[c]; n++ {
			c = fmt.Sprintf("%s_%d", id, n)
		}
		taken[c] = true
		ids[id] = c
	}
	return ids
}

func isDefaultParagraph(s Style) bool {
	return s.Default && (s.Type == "" || s.Type == StyleParagraph)
}
