package docx

import "math"

// WordprocessingML measures different things in different units, and mixing
// them silently produces a document that is wrong rather than one that fails.
// Each unit is a distinct type so the compiler catches the mix-up.
//
//	Twips  — layout: page size, margins, indents, spacing. 1/1440 inch.
//	EMU    — drawings only. 1/914400 inch.
//	HalfPt — font sizes. 1/2 point.
//	Eighth — border and rule widths. 1/8 point.

// Twips is 1/1440 of an inch, the unit for page and paragraph geometry.
type Twips int32

// EMU is an English Metric Unit, 1/914400 of an inch, used only by drawings.
type EMU int64

// HalfPt is half a point, the unit for font sizes: 11pt is HalfPt(22).
type HalfPt int

// Eighth is 1/8 of a point, the unit for border widths.
type Eighth int

const (
	twipsPerInch = 1440.0
	emuPerInch   = 914400.0
	mmPerInch    = 25.4
	ptPerInch    = 72.0
)

// Mm converts millimetres to twips. Swiss page geometry is specified in
// millimetres, so this is the common entry point.
func Mm(mm float64) Twips { return Twips(math.Round(mm / mmPerInch * twipsPerInch)) }

// Pt converts points to twips.
func Pt(pt float64) Twips { return Twips(math.Round(pt / ptPerInch * twipsPerInch)) }

// Cm converts centimetres to twips.
func Cm(cm float64) Twips { return Mm(cm * 10) }

// MmEMU converts millimetres to EMU, for image extents.
func MmEMU(mm float64) EMU { return EMU(math.Round(mm / mmPerInch * emuPerInch)) }

// PxEMU converts pixels at a given DPI to EMU.
func PxEMU(px float64, dpi float64) EMU {
	if dpi <= 0 {
		dpi = 96
	}
	return EMU(math.Round(px / dpi * emuPerInch))
}

// FontPt converts points to half-points, the unit Word stores font sizes in.
func FontPt(pt float64) HalfPt { return HalfPt(math.Round(pt * 2)) }

// BorderPt converts points to eighths of a point, for border widths.
func BorderPt(pt float64) Eighth { return Eighth(math.Round(pt * 8)) }

// A4 is the page size used by every document in this corpus.
var A4 = PageSize{Width: Mm(210), Height: Mm(297)}

// Letter is included for completeness; Swiss practice does not use it.
var Letter = PageSize{Width: Mm(215.9), Height: Mm(279.4)}
