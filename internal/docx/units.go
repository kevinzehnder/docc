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
//	Point  — the gap between a border and its text. A whole point, capped at 31.

// Twips is 1/1440 of an inch, the unit for page and paragraph geometry.
type Twips int32

// EMU is an English Metric Unit, 1/914400 of an inch, used only by drawings.
type EMU int64

// HalfPt is half a point, the unit for font sizes: 11pt is HalfPt(22).
type HalfPt int

// Eighth is 1/8 of a point, the unit for border widths.
type Eighth int

// Point is a whole point, the unit for the gap between a border and the text
// it surrounds. It is its own type because `w:space` is the one measurement in
// the writer that is neither twips nor a fraction of a point: OOXML types it
// ST_PointMeasure and Word reads it as points, so a value in twips comes out
// twenty times too large — a 6pt gap became a box 90mm deep.
type Point int

// MaxBorderSpace is the largest gap Word accepts between a border and its
// text. The schema caps ST_PointMeasure at 31 in this position, and a larger
// value makes Word offer to repair the file.
const MaxBorderSpace = 31

// Toggle is an OOXML toggle property — bold, italic, caps, small caps, strike.
// It has three states rather than two, because a style inherits through
// `based_on` and Word treats these elements as toggles along that chain:
// omitting `w:b` means "whatever the parent said", and turning bold *off*
// under a bold parent needs `w:b w:val="0"` written out.
//
// A plain bool cannot express that. It made a child style's `bold: false`
// indistinguishable from not mentioning bold at all, so a light face derived
// from a bold one silently stayed bold.
type Toggle uint8

const (
	// ToggleInherit leaves the property to the based-on style. The zero value,
	// so a struct literal that says nothing about bold changes nothing.
	ToggleInherit Toggle = iota
	// ToggleOn turns the property on.
	ToggleOn
	// ToggleOff turns it off even where the based-on style turned it on.
	ToggleOff
)

// ToggleFrom converts an optional bool — the shape a YAML decoder produces for
// a field that may be absent — into a Toggle.
func ToggleFrom(v *bool) Toggle {
	switch {
	case v == nil:
		return ToggleInherit
	case *v:
		return ToggleOn
	default:
		return ToggleOff
	}
}

// On reports whether the property is explicitly on.
func (t Toggle) On() bool { return t == ToggleOn }

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

// BorderSpacePt converts points to the border-space unit, clamped to what Word
// accepts. Clamping rather than rejecting: a theme asking for a wider gap than
// the format can express has made a judgement about spacing, not a mistake
// worth refusing a document over, and the widest legal gap is the closest the
// writer can come to honouring it.
func BorderSpacePt(pt float64) Point {
	p := int(math.Round(pt))
	switch {
	case p < 0:
		return 0
	case p > MaxBorderSpace:
		return MaxBorderSpace
	default:
		return Point(p)
	}
}

// A4 is the page size used by every document in this corpus.
var A4 = PageSize{Width: Mm(210), Height: Mm(297)}

// Letter is included for completeness; Swiss practice does not use it.
var Letter = PageSize{Width: Mm(215.9), Height: Mm(279.4)}
