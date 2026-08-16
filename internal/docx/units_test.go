package docx

import (
	"strings"
	"testing"
)

// Border spacing is the one measurement OOXML takes in whole points. Writing
// twips there produced a gap twenty times too wide, which Word renders as a
// box far from the text it is supposed to enclose.
func TestBorderSpacePtIsPointsAndClamped(t *testing.T) {
	for _, tc := range []struct {
		name string
		pt   float64
		want Point
	}{
		{"a typical gap", 6, 6},
		{"rounds to whole points", 4.4, 4},
		{"rounds up", 4.6, 5},
		{"zero stays zero", 0, 0},
		{"negative is clamped away", -3, 0},
		{"the format's ceiling", 31, 31},
		{"beyond the ceiling clamps", 120, MaxBorderSpace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := BorderSpacePt(tc.pt); got != tc.want {
				t.Errorf("BorderSpacePt(%v) = %d, want %d", tc.pt, got, tc.want)
			}
		})
	}
}

// A bordered paragraph writes its gap in points. The regression this guards is
// concrete: `space: 6pt` once emitted w:space="120", and Word read 120 points.
func TestParagraphBorderWritesSpaceInPoints(t *testing.T) {
	d := sample()
	d.Body = append(d.Body, Paragraph{
		Props: ParaProps{Style: "Standard", Borders: &ParaBorders{
			Top: &Border{Style: BorderSingle, Size: BorderPt(0.75), Space: BorderSpacePt(6)},
		}},
		Runs: []Run{{Items: []Inline{Text("boxed")}}},
	})
	data, err := d.Bytes()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	doc := partOf(t, data, "word/document.xml")
	if want := `w:space="6"`; !strings.Contains(doc, want) {
		t.Errorf("border space not written in points: want %s in\n%s", want, doc)
	}
	if strings.Contains(doc, `w:space="120"`) {
		t.Error("border space written in twips")
	}
}

// The three states have to reach the XML differently. Explicitly off is the
// one that matters: omitting the element inherits the based-on style's bold,
// so a light face derived from a bold one needs `w:val="0"` written out.
func TestToggleWritesThreeStates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		toggle Toggle
		want   string
		absent bool
	}{
		{name: "inherit writes nothing", toggle: ToggleInherit, absent: true},
		{name: "on writes a bare element", toggle: ToggleOn, want: `<w:b/>`},
		{name: "off writes val=0", toggle: ToggleOff, want: `<w:b w:val="0"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &xw{}
			w.open("w:rPr")
			writeToggle(w, "w:b", tc.toggle)
			w.close("w:rPr")
			got := string(w.bytes())
			if tc.absent {
				if strings.Contains(got, "w:b") {
					t.Errorf("inherit wrote a toggle: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("writeToggle = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestToggleFromOptionalBool(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name string
		in   *bool
		want Toggle
	}{
		{"absent inherits", nil, ToggleInherit},
		{"true is on", &yes, ToggleOn},
		{"false is an explicit off", &no, ToggleOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToggleFrom(tc.in); got != tc.want {
				t.Errorf("ToggleFrom = %v, want %v", got, tc.want)
			}
		})
	}
}
