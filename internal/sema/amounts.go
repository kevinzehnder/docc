package sema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
)

// checkAmountsBalance verifies that the money in a document adds up.
//
// Two things go wrong in a deed and neither is visible to a reader working
// down the page: the parts of a price do not sum to the price, and the
// payments arranged for it do not sum to what is owed. Both are arithmetic
// over numbers that already sit in the document, so the compiler can do it.
//
// A money block lists its items with the amount first, in brackets. One item
// may be marked as the block's total with a leading `=`, mirroring the
// "= total vereinbarter Kaufpreis" line of the paper form:
//
//	::: betraege {#kaufpreis}
//	- [Fr. 820'000.00] für die Wohnung
//	- [Fr. 45'000.00] für den Autoeinstellplatz
//	- [= Fr. 865'000.00] total vereinbarter Kaufpreis
//	:::
//
// A block that settles another block's total names it:
//
//	::: betraege {#tilgung total-of=kaufpreis}
//
// and its items must sum to that block's total — the deed's way of saying
// every franc of the price is accounted for.
//
//	args:
//	  div: the block name to check (required)
func checkAmountsBalance(c *ruleContext) {
	name, ok := c.argString("div", true)
	if !ok {
		return
	}

	totals := map[string]amount{} // block id -> declared or summed total
	type pending struct {
		div   *parse.Div
		ref   string
		sum   amount
		items int
	}
	var settlements []pending

	for _, div := range c.divsNamed(name) {
		var sum amount
		var declared *amount
		var declaredPos diag.Position
		items := 0

		for _, item := range divListItems(c.File, div) {
			label, isTotal, ok := amountLabel(item.Text)
			if !ok {
				continue
			}
			value, ok := parseAmount(label)
			if !ok {
				c.report(item.Pos,
					"write the amount as a currency and a figure, e.g. \"[Fr. 26'000.00]\"",
					"amount %q is not a number this check can add up", label)
				continue
			}
			if isTotal {
				if declared != nil {
					c.report(item.Pos,
						"mark exactly one item with `=`; it is the block's total",
						"block already declares a total of %s", declared.String())
					continue
				}
				v := value
				declared = &v
				declaredPos = item.Pos
				continue
			}
			sum += value
			items++
		}

		if declared != nil && items > 0 && sum != *declared {
			c.report(declaredPos,
				fmt.Sprintf("the items above add up to %s", sum.String()),
				"declared total %s does not match the sum of the items, %s",
				declared.String(), sum.String())
		}

		if id := div.Attr.ID; id != "" {
			if declared != nil {
				totals[id] = *declared
			} else {
				totals[id] = sum
			}
		}
		if ref, has := div.Attr.Get("total-of"); has {
			settlements = append(settlements, pending{
				div: div, ref: ref, sum: sum, items: items,
			})
		}
	}

	// Settling blocks are summed *together*, not one by one: a payment
	// schedule is usually broken into a sub-section per instalment, and each
	// instalment on its own settles nothing. What has to hold is that all of
	// them together account for the total.
	type group struct {
		sum    amount
		items  int
		blocks int
		last   *parse.Div
	}
	grouped := map[string]*group{}
	var order []string
	for _, s := range settlements {
		g, seen := grouped[s.ref]
		if !seen {
			g = &group{}
			grouped[s.ref] = g
			order = append(order, s.ref)
		}
		g.sum += s.sum
		g.items += s.items
		g.blocks++
		g.last = s.div
	}

	for _, ref := range order {
		g := grouped[ref]
		want, known := totals[ref]
		if !known {
			c.report(c.File.BodyPos(g.last.OpenOffset),
				fmt.Sprintf("known money blocks: %s", strings.Join(sortedMapKeys(totals), ", ")),
				"total-of names %q, which is not a money block in this document", ref)
			continue
		}
		if g.items == 0 || g.sum == want {
			continue
		}
		spread := ""
		if g.blocks > 1 {
			spread = fmt.Sprintf(" across %d blocks", g.blocks)
		}
		c.report(c.File.BodyPos(g.last.OpenOffset),
			fmt.Sprintf("these items add up to %s%s, and %q totals %s",
				g.sum.String(), spread, ref, want.String()),
			"the amounts do not settle %q: %s is unaccounted for", ref, (want - g.sum).Abs().String())
	}
}

// amountLabel extracts the bracketed amount that opens a money item, and
// reports whether it carries the `=` total marker.
func amountLabel(text string) (label string, isTotal, ok bool) {
	t := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(t, "[") {
		return "", false, false
	}
	end := strings.IndexByte(t, ']')
	if end <= 1 {
		return "", false, false
	}
	label = strings.TrimSpace(t[1:end])
	if rest, marked := strings.CutPrefix(label, "="); marked {
		return strings.TrimSpace(rest), true, true
	}
	return label, false, true
}

// amount is a money value in hundredths, so sums are exact. Floating point
// would make a rounding difference look like a drafting error.
type amount int64

func (a amount) String() string {
	neg := a < 0
	if neg {
		a = -a
	}
	whole := int64(a) / 100
	cents := int64(a) % 100

	var groups []string
	for whole >= 1000 {
		groups = append([]string{fmt.Sprintf("%03d", whole%1000)}, groups...)
		whole /= 1000
	}
	groups = append([]string{strconv.FormatInt(whole, 10)}, groups...)

	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%s.%02d", sign, strings.Join(groups, "'"), cents)
}

func (a amount) Abs() amount {
	if a < 0 {
		return -a
	}
	return a
}

// parseAmount reads a Swiss money figure, with or without a currency in front
// of it: "Fr. 26'000.00", "CHF 850000", "1’250.50".
func parseAmount(s string) (amount, bool) {
	// Skip whatever precedes the figure — "Fr.", "CHF" — so the currency's own
	// full stop is not read as a decimal point.
	if i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' }); i > 0 {
		s = s[i:]
	}
	digits := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r == '.':
			return r
		case r == '\'', r == '’', r == ' ', r == ' ':
			return -1
		default:
			return -1
		}
	}, s)
	if digits == "" {
		return 0, false
	}

	whole, frac, hasFrac := strings.Cut(digits, ".")
	if whole == "" {
		return 0, false
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	cents := int64(0)
	if hasFrac {
		// A second separator means this is not a figure but prose.
		if strings.Contains(frac, ".") {
			return 0, false
		}
		switch len(frac) {
		case 0:
		case 1:
			c, err := strconv.ParseInt(frac, 10, 64)
			if err != nil {
				return 0, false
			}
			cents = c * 10
		case 2:
			c, err := strconv.ParseInt(frac, 10, 64)
			if err != nil {
				return 0, false
			}
			cents = c
		default:
			return 0, false
		}
	}
	return amount(units*100 + cents), true
}

// checkAmountAtLeast reports a money block whose total falls below a floor the
// document type declares.
//
// This is the one shape of error that survives every other check in this
// package: a figure transcribed wrongly but transcribed *consistently*. A
// Stammkapital of CHF 5'000 written into all six documents of a founding
// balances perfectly, agrees across every file, fills every blank — and is
// below the statutory minimum of CHF 20'000 (Art. 773 Abs. 1 OR), so the
// registry rejects the lot. Nothing in the document contradicts anything else
// in it; the contradiction is with the law, which is why the floor has to be
// stated by the document type rather than derived.
//
// The floor is written the way amounts are written everywhere else in a
// source, so a schema declares `minimum: "Fr. 20'000.00"` rather than a bare
// number in some unstated unit.
func checkAmountAtLeast(c *ruleContext) {
	name, ok := c.argString("div", true)
	if !ok {
		return
	}
	raw, ok := c.argString("minimum", true)
	if !ok {
		return
	}
	floor, ok := parseAmount(raw)
	if !ok {
		c.schemaErrorf("write it as a currency and a figure, e.g. \"Fr. 20'000.00\"",
			"argument \"minimum\" is not an amount: %q", raw)
		return
	}

	for _, div := range c.divsNamed(name) {
		total, pos, ok := divTotal(c.File, div)
		if !ok {
			continue
		}
		if total >= floor {
			continue
		}
		c.report(pos,
			"raise the amount, or check that this block is the one the floor applies to",
			"total %s is below the minimum of %s this document type requires",
			total.String(), floor.String())
	}
}

// divTotal returns a money block's total: the item marked `=` when there is
// one, otherwise the sum of its items. The position is the line worth pointing
// at — the total itself, or the block's opening when the total is a sum.
func divTotal(f *parse.File, div *parse.Div) (amount, diag.Position, bool) {
	var sum amount
	items := 0
	pos := f.BodyPos(div.OpenOffset)

	for _, item := range divListItems(f, div) {
		label, isTotal, ok := amountLabel(item.Text)
		if !ok {
			continue
		}
		value, ok := parseAmount(label)
		if !ok {
			continue
		}
		if isTotal {
			return value, item.Pos, true
		}
		sum += value
		items++
	}
	if items == 0 {
		return 0, pos, false
	}
	return sum, pos, true
}
