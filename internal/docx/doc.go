// Package docx writes Word documents.
//
// It builds a .docx from scratch — no template to fill, no dependencies beyond
// the standard library. A document is assembled as a value and serialised:
//
//	d := &docx.Document{
//		Section: docx.Section{
//			Page:    docx.A4,
//			Margins: docx.Margins{Top: docx.Mm(20), Bottom: docx.Mm(20), Left: docx.Mm(26), Right: docx.Mm(15)},
//		},
//		Styles: []docx.Style{{ID: "Standard", Name: "Standard", Default: true}},
//		Body: []docx.Block{
//			docx.P("Standard", "Sehr geehrte Damen und Herren,"),
//		},
//	}
//	err := d.Write("letter.docx")
//
// # Determinism
//
// Identical input produces byte-identical output: archive timestamps are fixed,
// parts are written in sorted order, and identifiers are assigned by position
// rather than by counter state. Golden tests over the archive are therefore
// meaningful, and a rebuild that changes bytes changed something real.
//
// # Units
//
// WordprocessingML measures different things in different units. Each is a
// distinct type so they cannot be mixed by accident: [Twips] for layout, [EMU]
// for drawings, [HalfPt] for font sizes, [Eighth] for border widths. Construct
// them with [Mm], [Pt], [Cm], [MmEMU] and [FontPt] rather than by conversion.
//
// # What Word rejects
//
// Several structures are invalid rather than merely ugly, and this package
// fills them in rather than letting a document reach Word broken:
//
//   - a body without section properties
//   - a table cell or header part containing no paragraph
//   - an unbalanced element, which surfaces as a repair prompt rather than an
//     error, so [Document.Bytes] panics on one at construction time instead
//
// # Numbering
//
// Word's list numbering is a two-level indirection: a paragraph names a numId,
// which names an abstractNumId, which holds the levels. Two lists that must
// number independently need two numIds. Use [Numbering.AddList] and
// [Numbering.NewInstance] rather than building the tables by hand.
package docx
