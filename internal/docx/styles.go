package docx

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
	w := &xw{}
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
	attrs := []attr{a("w:type", string(typ)), a("w:styleId", s.ID)}
	if s.Default {
		attrs = append(attrs, a("w:default", "1"))
	}
	w.open("w:style", attrs...)

	name := s.Name
	if name == "" {
		name = s.ID
	}
	w.empty("w:name", a("w:val", name))
	if s.BasedOn != "" {
		w.empty("w:basedOn", a("w:val", s.BasedOn))
	}
	if s.Next != "" {
		w.empty("w:next", a("w:val", s.Next))
	}
	if s.LinkedTo != "" {
		w.empty("w:link", a("w:val", s.LinkedTo))
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
