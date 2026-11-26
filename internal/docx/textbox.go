package docx

// A framed paragraph is written as a floating text box (see FramePr). The box
// needs a paragraph in the flow to hang on: it takes the next paragraph that is
// not itself framed, as Word anchors a frame to the paragraph after it; failing
// that the previous one; failing that, an empty paragraph of its own.

const (
	nsMC  = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	nsWPS = "http://schemas.microsoft.com/office/word/2010/wordprocessingShape"
)

// textBox is a run of consecutive paragraphs sharing one frame.
type textBox struct {
	frame FramePr
	paras []Paragraph
}

// writeBlocks writes a sequence of blocks — a body, a header, a table cell —
// turning framed paragraphs into text boxes on their host paragraphs.
func writeBlocks(w *xw, d *Document, blocks []Block) {
	hosted := map[int][]textBox{}
	framed := map[int]bool{}
	var pending []textBox
	lastFlow := -1
	for i := 0; i < len(blocks); {
		p, ok := blocks[i].(Paragraph)
		if ok && p.Props.Frame != nil {
			box := textBox{frame: *p.Props.Frame}
			for ; i < len(blocks); i++ {
				q, ok := blocks[i].(Paragraph)
				if !ok || q.Props.Frame == nil || *q.Props.Frame != box.frame {
					break
				}
				q.Props.Frame = nil
				box.paras = append(box.paras, q)
				framed[i] = true
			}
			pending = append(pending, box)
			continue
		}
		if ok {
			hosted[i] = append(hosted[i], pending...)
			pending = nil
			lastFlow = i
		}
		i++
	}
	if len(pending) > 0 && lastFlow >= 0 {
		hosted[lastFlow] = append(hosted[lastFlow], pending...)
		pending = nil
	}

	for i, b := range blocks {
		if framed[i] {
			continue
		}
		if p, ok := b.(Paragraph); ok {
			p.writeHosting(w, d, hosted[i])
			continue
		}
		b.writeBlock(w, d)
	}
	if len(pending) > 0 {
		Paragraph{}.writeHosting(w, d, pending)
	}
}

func (b textBox) write(w *xw, d *Document) {
	f := b.frame
	id := d.nextShapeID
	d.nextShapeID++

	width := f.Width
	if width == 0 {
		width = d.widthFrom(f)
	}
	// A box without a height grows with its content (spAutoFit). The extent
	// still has to be stated; a line per paragraph is the starting guess the
	// renderer corrects.
	height := f.Height
	if height == 0 {
		for range b.paras {
			height += Pt(14)
		}
	}

	behind := "0"
	if f.Wrap == "behind" {
		behind = "1"
	}

	w.open("w:r")
	w.open("mc:AlternateContent", a("xmlns:mc", nsMC), a("xmlns:wps", nsWPS))
	w.open("mc:Choice", a("Requires", "wps"))
	w.open("w:drawing")
	w.open("wp:anchor",
		ai("distT", 0), ai("distB", 0), ai("distL", 0), ai("distR", 0),
		ai("simplePos", 0), ai("relativeHeight", 251659264+id), a("behindDoc", behind),
		ai("locked", 0), ai("layoutInCell", 1), ai("allowOverlap", 1))
	w.empty("wp:simplePos", ai("x", 0), ai("y", 0))
	w.open("wp:positionH", a("relativeFrom", relativeFrom(f.HAnchor, "column")))
	w.open("wp:posOffset")
	w.text(itoa(int(f.X.EMU())))
	w.close("wp:posOffset")
	w.close("wp:positionH")
	w.open("wp:positionV", a("relativeFrom", relativeFrom(f.VAnchor, "paragraph")))
	w.open("wp:posOffset")
	w.text(itoa(int(f.Y.EMU())))
	w.close("wp:posOffset")
	w.close("wp:positionV")
	w.empty("wp:extent", ai("cx", width.EMU()), ai("cy", height.EMU()))
	w.empty("wp:effectExtent", ai("l", 0), ai("t", 0), ai("r", 0), ai("b", 0))
	switch f.Wrap {
	case "top_and_bottom", "notBeside", "none":
		w.empty("wp:wrapTopAndBottom")
	case "front", "behind":
		w.empty("wp:wrapNone")
	default:
		w.empty("wp:wrapSquare", a("wrapText", "bothSides"))
	}
	w.empty("wp:docPr", ai("id", id), a("name", "Text Box "+itoa(id)))
	w.empty("wp:cNvGraphicFramePr")

	w.open("a:graphic", a("xmlns:a", "http://schemas.openxmlformats.org/drawingml/2006/main"))
	w.open("a:graphicData", a("uri", nsWPS))
	w.open("wps:wsp")
	w.empty("wps:cNvSpPr", ai("txBox", 1))
	w.open("wps:spPr")
	w.open("a:xfrm")
	w.empty("a:off", ai("x", 0), ai("y", 0))
	w.empty("a:ext", ai("cx", width.EMU()), ai("cy", height.EMU()))
	w.close("a:xfrm")
	w.open("a:prstGeom", a("prst", "rect"))
	w.empty("a:avLst")
	w.close("a:prstGeom")
	w.empty("a:noFill")
	w.open("a:ln")
	w.empty("a:noFill")
	w.close("a:ln")
	w.close("wps:spPr")
	w.open("wps:txbx")
	w.open("w:txbxContent")
	for _, p := range b.paras {
		p.write(w, d)
	}
	w.close("w:txbxContent")
	w.close("wps:txbx")
	// No insets: the text sits exactly at the frame's x and y, as a frame's did.
	w.open("wps:bodyPr", a("rot", "0"), a("vert", "horz"), a("wrap", "square"),
		ai("lIns", 0), ai("tIns", 0), ai("rIns", 0), ai("bIns", 0), a("anchor", "t"))
	if f.Height == 0 {
		w.empty("a:spAutoFit")
	} else {
		w.empty("a:noAutofit")
	}
	w.close("wps:bodyPr")
	w.close("wps:wsp")
	w.close("a:graphicData")
	w.close("a:graphic")
	w.close("wp:anchor")
	w.close("w:drawing")
	w.close("mc:Choice")
	w.close("mc:AlternateContent")
	w.close("w:r")
}

// relativeFrom maps a frame anchor to DrawingML's: "page" and "margin" are the
// same word; "text" is the column horizontally and the host paragraph
// vertically. Unset means page, the frame default.
func relativeFrom(anchor, text string) string {
	switch anchor {
	case "margin":
		return "margin"
	case "text":
		return text
	default:
		return "page"
	}
}

// widthFrom is the room from the box's left edge to the right margin, for a
// frame that does not state its width.
func (d *Document) widthFrom(f FramePr) Twips {
	page := d.Section.Page
	if page.Width == 0 {
		page = A4
	}
	pageWidth := page.Width
	if page.Landscape {
		pageWidth = page.Height
	}
	m := d.Section.Margins
	left := f.X
	if f.HAnchor == "margin" || f.HAnchor == "text" {
		left += m.Left
	}
	if room := pageWidth - m.Right - left; room > 0 {
		return room
	}
	return pageWidth - m.Left - m.Right
}
