package theme

import (
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/docx"
)

// DocxStyles converts the theme's style table. Output order is sorted by name
// so the generated styles.xml stays stable across builds.
func (t *Theme) DocxStyles() []docx.Style {
	ids := make([]string, 0, len(t.Styles))
	for id := range t.Styles {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]docx.Style, 0, len(ids))
	for _, id := range ids {
		out = append(out, t.Styles[id].docxStyle(id))
	}
	return out
}

func (s Style) docxStyle(id string) docx.Style {
	name := s.Name
	if name == "" {
		name = id
	}
	typ := docx.StyleParagraph
	if strings.EqualFold(s.Type, "character") {
		typ = docx.StyleCharacter
	}

	st := docx.Style{
		ID:         id,
		Name:       name,
		Type:       typ,
		BasedOn:    s.BasedOn,
		Next:       s.Next,
		Default:    s.Default,
		QFormat:    true,
		UIPriority: s.UIPriority,
		Run:        s.runProps(),
	}
	if typ != docx.StyleCharacter {
		st.Para = s.paraProps()
	}
	return st
}

func (s Style) runProps() docx.RunProps {
	return docx.RunProps{
		Font:      s.Font,
		Size:      s.Size.HalfPt(0),
		Bold:      docx.ToggleFrom(s.Bold),
		Italic:    docx.ToggleFrom(s.Italic),
		Underline: s.Underline,
		Caps:      docx.ToggleFrom(s.Caps),
		SmallCaps: docx.ToggleFrom(s.SmallCaps),
		Color:     s.Color,
	}
}

func (s Style) paraProps() docx.ParaProps {
	line, rule := s.Spacing.Line.Spacing()
	p := docx.ParaProps{
		Align: docx.Align(alignName(s.Align)),
		Spacing: docx.Spacing{
			Before:         s.Spacing.Before.Twips(0),
			After:          s.Spacing.After.Twips(0),
			Line:           line,
			LineRule:       rule,
			ExplicitBefore: s.Spacing.Before.Set(),
			ExplicitAfter:  s.Spacing.After.Set(),
		},
		Indent: docx.Indent{
			Left:      s.Indent.Left.Twips(0),
			Right:     s.Indent.Right.Twips(0),
			FirstLine: s.Indent.FirstLine.Twips(0),
			Hanging:   s.Indent.Hanging.Twips(0),
		},
		KeepNext:          s.KeepNext,
		KeepLines:         s.KeepLines,
		PageBreak:         s.PageBreakBefore,
		ContextualSpacing: s.ContextualSpacing,
		Shading:           s.Shading,
		Tabs:              docxTabs(s.Tabs),
		Borders:           s.Borders.docx(),
	}
	if s.OutlineLevel > 0 {
		// Word counts outline levels from zero.
		lvl := s.OutlineLevel - 1
		p.OutlineLevel = &lvl
	}
	return p
}

func docxTabs(tabs []Tab) []docx.TabStop {
	if len(tabs) == 0 {
		return nil
	}
	out := make([]docx.TabStop, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, docx.TabStop{
			Pos:    t.Pos.Twips(0),
			Align:  docx.TabAlign(tabAlignName(t.Align)),
			Leader: t.Leader,
		})
	}
	return out
}

func (b *Borders) docx() *docx.ParaBorders {
	if b == nil {
		return nil
	}
	out := &docx.ParaBorders{
		Top:    b.Top.docx(),
		Bottom: b.Bottom.docx(),
		Left:   b.Left.docx(),
		Right:  b.Right.docx(),
	}
	if out.Top == nil && out.Bottom == nil && out.Left == nil && out.Right == nil {
		return nil
	}
	return out
}

func (e *BorderEdge) docx() *docx.Border {
	if e == nil {
		return nil
	}
	style := docx.BorderStyle(e.Style)
	if style == "" {
		style = docx.BorderSingle
	}
	width := e.Width
	if width == 0 {
		width = 0.5
	}
	return &docx.Border{
		Style: style,
		Size:  docx.BorderPt(width),
		Space: e.Space.Points(0),
		Color: e.Color,
	}
}

// defaultIndent is half an inch per nesting level, the conventional list
// indent. Levels are capped at nine by the format, so this cannot overflow.
func defaultIndent(level int) docx.Twips {
	if level < 0 {
		level = 0
	}
	if level > 8 {
		level = 8
	}
	return docx.Twips(720 * (level + 1))
}

// alignName maps a theme's `align:` to Word's justification. An explicit
// `left` is *not* the same as no alignment at all: a style based on a justified
// one inherits the justification unless it says otherwise, so the only way to
// left-align it is to emit `w:jc w:val="left"`. Folding the two together made
// `align: left` a no-op — the very thing the theme reference recommends for
// address blocks and party entries. `tabAlignName` below always got this right.
func alignName(s string) string {
	switch strings.ToLower(s) {
	case "":
		return ""
	case "left":
		return string(docx.AlignLeft)
	case "center", "centre":
		return string(docx.AlignCenter)
	case "right":
		return string(docx.AlignRight)
	case "justify", "both":
		return string(docx.AlignJustify)
	default:
		return s
	}
}

// restartLevel maps a theme's `restart:` to w:lvlRestart. Unset means "leave
// the element out", which is Word's own default and today's behaviour;
// RestartNever is the only other value a theme can ask for, so the numbering
// vocabulary stays two words rather than a level index a theme author would
// have to reason about. validateNumbering rejects anything else.
func restartLevel(s string) *int {
	if strings.EqualFold(strings.TrimSpace(s), RestartNever) {
		never := 0
		return &never
	}
	return nil
}

func tabAlignName(s string) string {
	switch strings.ToLower(s) {
	case "", "left":
		return string(docx.TabLeft)
	case "center", "centre":
		return string(docx.TabCenter)
	case "right":
		return string(docx.TabRight)
	case "decimal":
		return string(docx.TabDecimal)
	default:
		return s
	}
}

// MaxNumLevels is how many levels Word's numbering has. It is a fixed sequence,
// not an open-ended tree.
const MaxNumLevels = 9

// The values a level's `restart:` takes.
const (
	// RestartAfterParent is Word's default: the level restarts whenever the
	// level above it increments.
	RestartAfterParent = "after-parent"
	// RestartNever keeps one counter running across every parent.
	RestartNever = "never"
)

// Flatten returns the definition's levels in order: the definition itself is
// level 0 and Levels[i] is level i+1.
//
// Levels is a flat list, not a tree. Treating it as one and recursing produced
// two levels both claiming ilvl 1, which Word resolves by rendering the second
// one's %3 placeholder as literal text — a definition that looks right in YAML
// and wrong on the page.
func (n NumFormat) Flatten() []NumFormat {
	out := make([]NumFormat, 0, len(n.Levels)+1)
	out = append(out, n)
	out = append(out, n.Levels...)
	if len(out) > MaxNumLevels {
		out = out[:MaxNumLevels]
	}
	return out
}

// AbstractNum converts a theme list definition.
func (n NumFormat) AbstractNum() docx.AbstractNum {
	def := docx.AbstractNum{}
	for level, f := range n.Flatten() {
		format := docx.NumFormat(f.Format)
		if format == "" {
			format = docx.NumDecimal
		}
		def.Levels = append(def.Levels, docx.NumLevel{
			Level:          level,
			Format:         format,
			Text:           f.Text,
			Start:          f.Start,
			Indent:         f.Indent.Twips(defaultIndent(level)),
			Hanging:        f.Hanging.Twips(docx.Twips(360)),
			Font:           f.Font,
			Size:           f.Size.HalfPt(0),
			Align:          docx.TabAlign(tabAlignName(f.Align)),
			Suffix:         f.Suffix,
			Restart:        restartLevel(f.Restart),
			ParagraphStyle: f.Style,
		})
	}

	if len(def.Levels) > 1 {
		def.MultiLevelType = "multilevel"
	} else {
		def.MultiLevelType = "singleLevel"
	}
	return def
}
