package emit

import (
	"fmt"
	"strings"
)

// germanAmountWords spells a money value the way a Swiss deed writes it:
// "sechsundzwanzigtausend" for 26'000.00, with any centimes appended as a
// fraction rather than spelled out.
//
// The words exist to make a figure impossible to alter after signing, which
// is exactly why they must not be typed by hand — a deed whose words and
// figures disagree is a deed with two prices. Rendering them from the figure
// removes the disagreement rather than checking for it.
func germanAmountWords(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := cents / 100
	rest := cents % 100

	words := germanNumber(whole)
	if rest != 0 {
		words = fmt.Sprintf("%s und %d/100", words, rest)
	}
	if neg {
		words = "minus " + words
	}
	return words
}

var (
	germanUnits = [...]string{
		"null", "ein", "zwei", "drei", "vier", "fünf", "sechs", "sieben",
		"acht", "neun", "zehn", "elf", "zwölf", "dreizehn", "vierzehn",
		"fünfzehn", "sechzehn", "siebzehn", "achtzehn", "neunzehn",
	}
	germanTens = [...]string{
		"", "", "zwanzig", "dreissig", "vierzig", "fünfzig",
		"sechzig", "siebzig", "achtzig", "neunzig",
	}
)

// germanNumber spells a non-negative integer. German writes everything below
// a million as one word, which is why this returns a single token.
func germanNumber(n int64) string {
	switch n {
	case 0:
		return "null"
	case 1:
		return "eins"
	}

	var b strings.Builder
	writeGroup := func(v int64, singular, plural string, separate bool) {
		if v == 0 {
			return
		}
		if v == 1 && singular != "" {
			b.WriteString(singular)
		} else {
			b.WriteString(germanBelowThousand(v))
			b.WriteString(plural)
		}
		if separate {
			b.WriteString(" ")
		}
	}

	billions := n / 1_000_000_000
	millions := (n / 1_000_000) % 1000
	thousands := (n / 1000) % 1000
	remainder := n % 1000

	writeGroup(billions, "eine Milliarde", " Milliarden", true)
	writeGroup(millions, "eine Million", " Millionen", true)
	if thousands > 0 {
		b.WriteString(germanBelowThousand(thousands))
		b.WriteString("tausend")
	}
	if remainder > 0 {
		b.WriteString(germanBelowThousand(remainder))
	}
	return strings.TrimSpace(b.String())
}

// germanBelowThousand spells 1–999 as one word.
func germanBelowThousand(n int64) string {
	if n >= 100 {
		hundreds := n / 100
		rest := n % 100
		out := germanUnits[hundreds] + "hundert"
		if rest > 0 {
			out += germanBelowThousand(rest)
		}
		return out
	}
	if n < 20 {
		return germanUnits[n]
	}
	tens := n / 10
	ones := n % 10
	if ones == 0 {
		return germanTens[tens]
	}
	// German says the unit first: einundzwanzig, sechsundzwanzig.
	return germanUnits[ones] + "und" + germanTens[tens]
}
