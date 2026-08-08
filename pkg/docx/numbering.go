package docx

// Numbering holds list definitions.
//
// Word's numbering is a two-level indirection that trips people up: a body
// paragraph refers to a numId, a numId refers to an abstractNumId, and the
// abstract definition holds the nine levels. Two lists that must number
// independently need two numIds pointing at the same abstract definition —
// sharing one numId makes the second list continue the first.
type Numbering struct {
	Abstract []AbstractNum
	// Instances map a numId to an abstract definition. Use AddList to create
	// one rather than appending by hand.
	Instances []NumInstance
}

// AbstractNum is a list definition: nine levels of format and indentation.
type AbstractNum struct {
	// ID must be unique within the document.
	ID int
	// Name is optional and shown in Word's list gallery.
	Name string
	// MultiLevelType is "singleLevel", "multilevel" or "hybridMultilevel".
	MultiLevelType string
	Levels         []NumLevel
}

// NumInstance binds a numId used by paragraphs to an abstract definition.
type NumInstance struct {
	// ID is the numId body paragraphs refer to.
	ID int
	// AbstractID is the AbstractNum.ID it uses.
	AbstractID int
	// StartOverride restarts level 0 at this number when non-zero.
	StartOverride int
}

// NumFormat is a numbering format.
type NumFormat string

const (
	NumDecimal     NumFormat = "decimal"
	NumLowerLetter NumFormat = "lowerLetter"
	NumUpperLetter NumFormat = "upperLetter"
	NumLowerRoman  NumFormat = "lowerRoman"
	NumUpperRoman  NumFormat = "upperRoman"
	NumBullet      NumFormat = "bullet"
	NumNone        NumFormat = "none"
)

// NumLevel is one level of a list definition.
type NumLevel struct {
	// Level is the zero-based depth.
	Level  int
	Format NumFormat
	// Text is the rendered label. "%1." yields "1.", "%1.%2" yields "1.1".
	// The digit refers to the one-based level, not the zero-based one.
	Text string
	// Start is the first number, defaulting to 1.
	Start int
	Align TabAlign
	// Indent is the left indent of the text, and Hanging the distance the
	// number hangs to its left.
	Indent  Twips
	Hanging Twips
	// Font overrides the label font, needed for bullet glyphs: "Symbol" for
	// a filled bullet, "Courier New" for "o".
	Font string
	// Size overrides the label's font size, independently of the paragraph it
	// labels. A marginal number is set smaller than the prose beside it.
	Size HalfPt
	// Suffix is what follows the label: "tab" (default), "space" or "nothing".
	Suffix string
	// Bold and Italic format the label independently of the text.
	Bold   bool
	Italic bool
	// ParagraphStyle links this level to a paragraph style.
	ParagraphStyle string
}

// NumRef points a paragraph at a list level.
type NumRef struct {
	ID    int
	Level int
}

// AddList registers an abstract definition and returns a fresh numId bound to
// it. Call it once per list that must number independently.
func (n *Numbering) AddList(def AbstractNum) int {
	if def.ID == 0 {
		def.ID = n.nextAbstractID()
	}
	n.Abstract = append(n.Abstract, def)
	id := n.nextInstanceID()
	n.Instances = append(n.Instances, NumInstance{ID: id, AbstractID: def.ID})
	return id
}

// NewInstance returns another numId bound to an existing abstract definition,
// so a second list restarts rather than continuing the first.
func (n *Numbering) NewInstance(abstractID int) int {
	id := n.nextInstanceID()
	n.Instances = append(n.Instances, NumInstance{ID: id, AbstractID: abstractID})
	return id
}

func (n *Numbering) nextAbstractID() int {
	max := -1
	for _, d := range n.Abstract {
		if d.ID > max {
			max = d.ID
		}
	}
	return max + 1
}

// nextInstanceID starts at 1: Word treats numId 0 as "no numbering", so an
// instance with that id would silently do nothing.
func (n *Numbering) nextInstanceID() int {
	max := 0
	for _, i := range n.Instances {
		if i.ID > max {
			max = i.ID
		}
	}
	return max + 1
}

// empty reports whether there is nothing to write.
func (n Numbering) empty() bool {
	return len(n.Abstract) == 0 && len(n.Instances) == 0
}

// writeNumbering renders the numbering.xml part.
func (d *Document) writeNumbering() []byte {
	w := &xw{}
	w.header()
	w.open("w:numbering", nsAttrs()...)

	// Word requires every abstractNum to precede every num.
	for _, def := range d.Numbering.Abstract {
		writeAbstractNum(w, def)
	}
	for _, inst := range d.Numbering.Instances {
		w.open("w:num", ai("w:numId", inst.ID))
		w.empty("w:abstractNumId", ai("w:val", inst.AbstractID))
		if inst.StartOverride > 0 {
			w.open("w:lvlOverride", ai("w:ilvl", 0))
			w.empty("w:startOverride", ai("w:val", inst.StartOverride))
			w.close("w:lvlOverride")
		}
		w.close("w:num")
	}

	w.close("w:numbering")
	return w.bytes()
}

func writeAbstractNum(w *xw, def AbstractNum) {
	w.open("w:abstractNum", ai("w:abstractNumId", def.ID))
	multi := def.MultiLevelType
	if multi == "" {
		if len(def.Levels) > 1 {
			multi = "multilevel"
		} else {
			multi = "singleLevel"
		}
	}
	w.empty("w:multiLevelType", a("w:val", multi))
	if def.Name != "" {
		w.empty("w:name", a("w:val", def.Name))
	}
	for _, lvl := range def.Levels {
		writeNumLevel(w, lvl)
	}
	w.close("w:abstractNum")
}

func writeNumLevel(w *xw, lvl NumLevel) {
	w.open("w:lvl", ai("w:ilvl", lvl.Level))

	start := lvl.Start
	if start == 0 {
		start = 1
	}
	w.empty("w:start", ai("w:val", start))

	format := lvl.Format
	if format == "" {
		format = NumDecimal
	}
	w.empty("w:numFmt", a("w:val", string(format)))

	if lvl.ParagraphStyle != "" {
		w.empty("w:pStyle", a("w:val", lvl.ParagraphStyle))
	}

	text := lvl.Text
	if text == "" && format != NumNone {
		text = "%" + itoa(lvl.Level+1) + "."
	}
	w.empty("w:lvlText", a("w:val", text))

	if lvl.Suffix != "" {
		w.empty("w:suff", a("w:val", lvl.Suffix))
	}

	align := lvl.Align
	if align == "" {
		align = TabLeft
	}
	w.empty("w:lvlJc", a("w:val", string(align)))

	if lvl.Indent != 0 || lvl.Hanging != 0 {
		w.open("w:pPr")
		w.empty("w:ind", ai("w:left", lvl.Indent), ai("w:hanging", lvl.Hanging))
		w.close("w:pPr")
	}

	if lvl.Font != "" || lvl.Bold || lvl.Italic || lvl.Size != 0 {
		w.open("w:rPr")
		if lvl.Bold {
			w.empty("w:b")
		}
		if lvl.Italic {
			w.empty("w:i")
		}
		if lvl.Font != "" {
			w.empty("w:rFonts",
				a("w:ascii", lvl.Font),
				a("w:hAnsi", lvl.Font),
				a("w:cs", lvl.Font),
				a("w:hint", "default"),
			)
		}
		if lvl.Size != 0 {
			w.empty("w:sz", ai("w:val", lvl.Size))
			w.empty("w:szCs", ai("w:val", lvl.Size))
		}
		w.close("w:rPr")
	}

	w.close("w:lvl")
}

// DecimalList is a conventional numbered list: 1. 2. 3. with each level
// indented a further half inch.
func DecimalList(levels int) AbstractNum {
	if levels < 1 {
		levels = 1
	}
	if levels > 9 {
		levels = 9
	}
	def := AbstractNum{MultiLevelType: "multilevel"}
	for i := range levels {
		def.Levels = append(def.Levels, NumLevel{
			Level:   i,
			Format:  NumDecimal,
			Text:    "%" + itoa(i+1) + ".",
			Indent:  Twips(int32(720 * (i + 1))),
			Hanging: Twips(360),
		})
	}
	return def
}

// BulletList is a conventional bulleted list.
func BulletList(levels int) AbstractNum {
	if levels < 1 {
		levels = 1
	}
	if levels > 9 {
		levels = 9
	}
	glyphs := []struct{ text, font string }{
		{"", "Symbol"},
		{"o", "Courier New"},
		{"", "Wingdings"},
	}
	def := AbstractNum{MultiLevelType: "hybridMultilevel"}
	for i := range levels {
		g := glyphs[i%len(glyphs)]
		def.Levels = append(def.Levels, NumLevel{
			Level:   i,
			Format:  NumBullet,
			Text:    g.text,
			Font:    g.font,
			Indent:  Twips(int32(720 * (i + 1))),
			Hanging: Twips(360),
		})
	}
	return def
}
