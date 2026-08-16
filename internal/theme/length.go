package theme

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kevinzehnder/docc/internal/docx"
)

// Length is a measurement written with its unit: "20mm", "12pt", "1.5cm".
//
// Themes are edited by hand, and a bare number would mean twips — a unit nobody
// thinks in. Requiring the unit makes the file readable and makes a mistake a
// load error rather than a document that is silently 20 times too small.
type Length struct {
	twips docx.Twips
	set   bool
}

// Twips returns the measurement, or fallback when the field was absent.
func (l Length) Twips(fallback docx.Twips) docx.Twips {
	if !l.set {
		return fallback
	}
	return l.twips
}

// Points returns the measurement in whole points, or fallback when the field
// was absent. Only border spacing needs this: it is the one place OOXML
// measures in points rather than twips.
func (l Length) Points(fallback docx.Point) docx.Point {
	if !l.set {
		return fallback
	}
	return docx.BorderSpacePt(float64(l.twips) / 20)
}

// Set reports whether the field was present in the source.
func (l Length) Set() bool { return l.set }

// UnmarshalYAML parses "20mm", "12pt", "1.5cm" or "1440tw".
func (l *Length) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return fmt.Errorf("length must be a quoted string with a unit, e.g. \"20mm\", got %v", raw)
	}
	parsed, err := ParseLength(s)
	if err != nil {
		return err
	}
	*l = parsed
	return nil
}

// ParseLength converts a measurement string to twips.
func ParseLength(s string) (Length, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Length{}, nil
	}

	for _, unit := range []struct {
		suffix string
		conv   func(float64) docx.Twips
	}{
		{"mm", docx.Mm},
		{"cm", docx.Cm},
		{"pt", docx.Pt},
		{"in", func(v float64) docx.Twips { return docx.Twips(v * 1440) }},
		{"tw", func(v float64) docx.Twips { return docx.Twips(v) }},
	} {
		rest, found := strings.CutSuffix(s, unit.suffix)
		if !found {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			return Length{}, fmt.Errorf("invalid length %q: %w", s, err)
		}
		return Length{twips: unit.conv(v), set: true}, nil
	}
	return Length{}, fmt.Errorf("length %q has no unit — write mm, cm, pt, in or tw", s)
}

// FontSize is a font size written as "11pt".
type FontSize struct {
	halfPt docx.HalfPt
	set    bool
}

// HalfPt returns the size, or fallback when the field was absent.
func (f FontSize) HalfPt(fallback docx.HalfPt) docx.HalfPt {
	if !f.set {
		return fallback
	}
	return f.halfPt
}

// Set reports whether the field was present.
func (f FontSize) Set() bool { return f.set }

// UnmarshalYAML parses "11pt".
func (f *FontSize) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return fmt.Errorf("font size must be a quoted string, e.g. \"11pt\", got %v", raw)
	}
	s = strings.TrimSpace(s)
	rest, found := strings.CutSuffix(s, "pt")
	if !found {
		return fmt.Errorf("font size %q must end in pt", s)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
	if err != nil {
		return fmt.Errorf("invalid font size %q: %w", s, err)
	}
	*f = FontSize{halfPt: docx.FontPt(v), set: true}
	return nil
}

// LineHeight is line spacing written either as a multiple ("1.15") or as an
// exact length ("14pt").
type LineHeight struct {
	value docx.Twips
	rule  docx.LineRule
	set   bool
}

// Spacing returns the line component of a docx.Spacing.
func (l LineHeight) Spacing() (docx.Twips, docx.LineRule) {
	if !l.set {
		return 0, ""
	}
	return l.value, l.rule
}

// UnmarshalYAML parses "1.15" as a multiple and "14pt" as an exact height.
func (l *LineHeight) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case nil:
		return nil
	case float64:
		// Word stores a multiple in 240ths of a line.
		*l = LineHeight{value: docx.Twips(v * 240), rule: docx.LineAuto, set: true}
	case int:
		*l = LineHeight{value: docx.Twips(float64(v) * 240), rule: docx.LineAuto, set: true}
	case uint64:
		*l = LineHeight{value: docx.Twips(float64(v) * 240), rule: docx.LineAuto, set: true}
	case string:
		length, err := ParseLength(v)
		if err != nil {
			return fmt.Errorf("line height: %w", err)
		}
		*l = LineHeight{value: length.twips, rule: docx.LineExact, set: true}
	default:
		return fmt.Errorf("line height must be a number or a length, got %v", raw)
	}
	return nil
}
