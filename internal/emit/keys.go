package emit

// The style map's vocabulary, kept beside the code that reads it.
//
// A schema maps a markdown construct to a style id the theme defines. Which
// constructs exist is not a matter of taste — it is exactly the set of keys the
// emitter looks up, and nothing else has any effect. A mapping the emitter never
// reads is the worst kind of configuration: it validates, it renders, and it
// does nothing, so the author concludes the theme is at fault.
//
// Every key below corresponds to an `e.style(...)` call in emit.go. Adding a
// lookup there means adding it here, or `docc doctor` starts calling a real key
// unread.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/schema"
)

// StyleKey is one entry a schema's `styles:` map may set.
type StyleKey struct {
	// Key is the map key, e.g. "h1" or "div.beweis.label".
	Key string
	// Purpose says what mapping it changes.
	Purpose string
	// Fallback names the key used when this one is unset, empty when there is
	// none and the construct simply goes unstyled.
	Fallback string
}

// maxHeadingLevel is the deepest ATX heading markdown has.
const maxHeadingLevel = 6

// StyleKeys reports every style-map key that has an effect for this schema, in
// a stable order: the fixed constructs first, then the keys the schema's own
// blocks and spans bring into existence.
func StyleKeys(sc *schema.Schema) []StyleKey {
	keys := []StyleKey{
		{Key: "paragraph", Purpose: "body prose"},
		{Key: "heading", Purpose: "any heading without a level-specific mapping"},
	}
	for i := 1; i <= maxHeadingLevel; i++ {
		keys = append(keys, StyleKey{
			Key:      fmt.Sprintf("h%d", i),
			Purpose:  fmt.Sprintf("level-%d heading", i),
			Fallback: "heading",
		})
	}
	keys = append(keys,
		StyleKey{Key: "quote", Purpose: "block quote", Fallback: "paragraph"},
		StyleKey{Key: "code", Purpose: "fenced code block", Fallback: "paragraph"},
		StyleKey{Key: "table", Purpose: "table"},
		StyleKey{Key: "ordered_list", Purpose: "numbered list; may name a numbering definition instead of a style"},
		StyleKey{Key: "bullet_list", Purpose: "bulleted list; may name a numbering definition instead of a style"},
	)

	// A schema that declares no blocks or spans has not opted into the markup
	// contract, so any `:::` name and any span type is permitted and there is no
	// closed set of keys to report.
	for _, name := range sortedKeys(sc.Blocks) {
		keys = append(keys, blockKeys(name)...)
	}
	for _, name := range sortedKeys(sc.Spans) {
		keys = append(keys, StyleKey{
			Key:     "span." + name,
			Purpose: fmt.Sprintf("the `[text]{.%s}` span", name),
		})
	}
	return keys
}

// blockSuffixes are the keys a declared block brings into existence beyond
// `div.<name>` itself, each with what it does. The first four select a
// rendering pattern; mapping none renders every paragraph in `div.<name>`.
//
// Keep this in step with the `e.style("div."+d.Name+...)` lookups in emit.go. A
// suffix missing here makes `docc doctor` call a working mapping unread, which
// is worse than saying nothing.
var blockSuffixes = []struct{ suffix, purpose, fallback string }{
	{".amount", "selects amount rendering; styles the amount column", ""},
	{".line", "selects ruled rendering; styles the rule", ""},
	{".label", "selects labelled rendering; styles the tabbed label", ""},
	{".field", "selects field rendering — label first, then the value; styles the label column", ""},
	{".total", "styles the total row of amount rendering", ""},
	{".total.amount", "styles the amount cell of that total row", ""},
	{".words", "adds the amount spelled out in words; needs the theme's `formats.amount_words` as well", ""},
}

// blockKeys reports the keys one declared block brings into existence.
func blockKeys(name string) []StyleKey {
	keys := []StyleKey{{
		Key:      "div." + name,
		Purpose:  fmt.Sprintf("every paragraph of a `::: %s` block", name),
		Fallback: "paragraph",
	}}
	for _, s := range blockSuffixes {
		keys = append(keys, StyleKey{Key: "div." + name + s.suffix, Purpose: s.purpose, Fallback: s.fallback})
	}
	return keys
}

// UnreadStyleKeys reports the keys a schema maps that nothing will ever read,
// with the reason. This is the check that catches `code_span: Monospace` — a
// plausible mapping for a construct whose formatting is fixed — and a typo in a
// block name, both of which are otherwise completely silent.
//
// Keys for blocks and spans are only judged when the schema declares any: until
// it does, every `:::` name is permitted, so no `div.*` key can be called wrong.
func UnreadStyleKeys(sc *schema.Schema) []string {
	known := map[string]bool{}
	for _, k := range StyleKeys(sc) {
		known[k.Key] = true
	}

	var unread []string
	for key := range sc.Styles {
		if known[key] {
			continue
		}
		switch {
		case strings.HasPrefix(key, "div."):
			if len(sc.Blocks) == 0 {
				continue // no block contract declared; anything goes
			}
			name := blockName(key)
			if _, declared := sc.Blocks[name]; !declared {
				unread = append(unread, fmt.Sprintf(
					"%s — the schema declares no block %q", key, name))
				continue
			}
			unread = append(unread, fmt.Sprintf(
				"%s — %q is a declared block, but %q is not one of its style keys (%s)",
				key, name, strings.TrimPrefix(key, "div."+name), suffixList()))
		case strings.HasPrefix(key, "span."):
			// validateSpanStyles already rejects an undeclared span type, as an
			// error rather than a warning, and knows that the `docc-` prefix is
			// reserved and needs no declaration. Reporting it again here would
			// be weaker and would misjudge those reserved types.
			continue
		default:
			unread = append(unread, fmt.Sprintf("%s — not a construct docc styles%s", key, fixedHint(key)))
		}
	}
	sort.Strings(unread)
	return unread
}

// suffixList names the permitted `div.<name>` suffixes, for a diagnostic.
func suffixList() string {
	out := make([]string, 0, len(blockSuffixes))
	for _, s := range blockSuffixes {
		out = append(out, strings.TrimPrefix(s.suffix, "."))
	}
	return strings.Join(out, ", ")
}

// blockName recovers the block a `div.<name>[.<suffix>]` key refers to.
func blockName(key string) string {
	rest := strings.TrimPrefix(key, "div.")
	name, _, _ := strings.Cut(rest, ".")
	return name
}

// fixedHint explains the near misses: keys an author reaches for because the
// construct exists, but whose formatting the emitter fixes rather than reads
// from the theme.
func fixedHint(key string) string {
	for _, f := range FixedFormatting() {
		if f.Key == key {
			return "; " + f.Construct + " is rendered with fixed formatting (" + f.Formatting + ")"
		}
	}
	return ""
}

// BlockPattern reports how a `::: <name>` block renders, given what the schema
// maps for it. The pattern is not declared in the block's own definition — it
// is a consequence of which style key is set — so nothing else can report it.
//
// The order matches the dispatch in emit.div; mapping two of them silently
// picks the first.
func BlockPattern(sc *schema.Schema, name string) string {
	switch {
	case sc.Styles["div."+name+".amount"] != "":
		return "amount"
	case sc.Styles["div."+name+".line"] != "":
		return "ruled"
	case sc.Styles["div."+name+".label"] != "":
		return "labelled"
	case sc.Styles["div."+name+".field"] != "":
		return "field"
	default:
		return "plain"
	}
}

// FixedFormat is a construct the emitter formats itself, with no style map entry
// and no way for a theme to change it.
type FixedFormat struct {
	// Key is the mapping an author would plausibly reach for, if one exists.
	Key string
	// Construct is the markdown it applies to.
	Construct string
	// Formatting is what the emitter applies instead.
	Formatting string
}

// FixedFormatting lists the constructs a theme cannot reach. Stating them is
// the point: they are not configurable, and an author who does not know that
// spends the afternoon blaming the theme.
func FixedFormatting() []FixedFormat {
	return []FixedFormat{
		{Key: "strong", Construct: "**bold**", Formatting: "bold"},
		{Key: "emph", Construct: "*italic*", Formatting: "italic"},
		{Key: "code_span", Construct: "`inline code`", Formatting: "Courier New, otherwise inherited"},
		{Key: "link", Construct: "[a link](…)", Formatting: "colour 0000EE, single underline; rendered as text, not a hyperlink"},
		{Construct: "table borders", Formatting: "0.5pt single rule on every edge, inside and out"},
		{Construct: "table columns", Formatting: "the text width divided evenly; markdown carries no column sizing"},
	}
}
