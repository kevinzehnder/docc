package sema

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// CheckFunc is a named check selected by a schema rule.
type CheckFunc func(ctx *ruleContext)

// ruleContext is what a named check gets to work with.
type ruleContext struct {
	File   *parse.File
	Schema *schema.Schema
	Meta   *Meta
	Rule   schema.Rule
	Diags  *diag.List
}

// report emits a diagnostic under the rule's own ID and severity, letting the
// schema override the check's default wording.
func (c *ruleContext) report(pos diag.Position, defaultHint, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if c.Rule.Message != "" {
		msg = c.Rule.Message
	}
	hint := defaultHint
	if c.Rule.Hint != "" {
		hint = c.Rule.Hint
	}
	code := c.Rule.ID
	if code == "" {
		code = "DOC099"
	}
	if strings.EqualFold(c.Rule.Severity, "warning") {
		c.Diags.Warnf(c.File.Path, pos, code, hint, "%s", msg)
		return
	}
	c.Diags.Errorf(c.File.Path, pos, code, hint, "%s", msg)
}

// schemaErrorf reports a defect in the schema itself rather than in the
// document: a rule that names a missing or malformed argument. It is always
// DOC009 and always an error — the rule's own ID and severity describe findings
// about the document, and this is not one.
func (c *ruleContext) schemaErrorf(hint, format string, args ...any) {
	c.Diags.Errorf(c.File.Path, diag.Position{}, "DOC009", hint,
		"schema %q, rule %s: "+format, append([]any{c.Schema.Type, c.ruleName()}, args...)...)
}

func (c *ruleContext) ruleName() string {
	if c.Rule.ID != "" {
		return c.Rule.ID
	}
	return c.Rule.Check
}

// argString returns a string argument. A missing or non-string argument is a
// schema error, not a silent skip: a rule that does not do what its author
// wrote is worse than one that refuses to run.
func (c *ruleContext) argString(name string, required bool) (string, bool) {
	raw, ok := c.Rule.Args[name]
	if !ok {
		if required {
			c.schemaErrorf(fmt.Sprintf("add `args: { %s: ... }` to the rule", name),
				"check %q needs the argument %q", c.Rule.Check, name)
			return "", false
		}
		return "", true
	}
	s, isStr := raw.(string)
	if !isStr {
		c.schemaErrorf("write it as a string",
			"argument %q must be a string, got %T", name, raw)
		return "", false
	}
	if required && strings.TrimSpace(s) == "" {
		c.schemaErrorf(fmt.Sprintf("give `%s` a value", name),
			"argument %q is empty", name)
		return "", false
	}
	return s, true
}

// argRegexp compiles a regexp argument, falling back to def when absent.
func (c *ruleContext) argRegexp(name string, def *regexp.Regexp) (*regexp.Regexp, bool) {
	raw, ok := c.Rule.Args[name]
	if !ok {
		if def == nil {
			c.schemaErrorf(fmt.Sprintf("add `args: { %s: ... }` to the rule", name),
				"check %q needs the argument %q", c.Rule.Check, name)
			return nil, false
		}
		return def, true
	}
	s, isStr := raw.(string)
	if !isStr {
		c.schemaErrorf("quote the pattern as a string",
			"argument %q must be a regular expression, got %T", name, raw)
		return nil, false
	}
	re, err := regexp.Compile(s)
	if err != nil {
		c.schemaErrorf("fix the pattern; it is a Go regular expression",
			"argument %q is not a valid regular expression: %v", name, err)
		return nil, false
	}
	return re, true
}

// registry maps schema rule names to implementations.
var registry = map[string]CheckFunc{
	"no_placeholder_text": checkNoPlaceholders,
	"div_items_match":     checkDivItemsMatch,
	"cross_reference":     checkCrossReference,
	"no_empty_sections":   checkNoEmptySections,
}

// KnownChecks lists registered check names, for error messages and docs.
func KnownChecks() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func runRules(f *parse.File, sc *schema.Schema, m *Meta, ds *diag.List) {
	for _, rule := range sc.Rules {
		fn, ok := registry[rule.Check]
		if !ok {
			ds.Errorf(f.Path, diag.Position{}, "DOC009",
				"known checks: "+strings.Join(KnownChecks(), ", "),
				"schema %q selects unknown check %q", sc.Type, rule.Check)
			continue
		}
		fn(&ruleContext{File: f, Schema: sc, Meta: m, Rule: rule, Diags: ds})
	}
}

// placeholderRe matches an unedited template placeholder: a whole line, or a
// whole list item, that is nothing but bracketed prose. Group 1 is what the
// caret underlines — the placeholder itself, not the list marker in front of it.
var placeholderRe = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)?(\[[^\]]{3,}\]\s*[;.]?)\s*$`)

// checkNoPlaceholders finds template text the author never replaced. This is
// the single highest-value check: unedited placeholders otherwise render
// straight into a filed document.
//
// args:
//
//	pattern: what a placeholder line looks like. Defaults to bracketed prose.
func checkNoPlaceholders(c *ruleContext) {
	placeholderRe, ok := c.argRegexp("pattern", placeholderRe)
	if !ok {
		return
	}
	for i := 1; ; i++ {
		line := c.File.LineText(i)
		if line == "" && i > countLines(c.File.Source) {
			return
		}
		loc := matchSpan(placeholderRe, line)
		if loc == nil {
			continue
		}
		// Markdown links are `[text](url)`, not placeholders.
		if strings.Contains(line, "](") {
			continue
		}
		c.report(diag.Position{Line: i, Col: loc[0] + 1, Len: loc[1] - loc[0]},
			"replace this with the actual content",
			"unfilled template placeholder")
	}
}

// matchSpan returns the byte span a diagnostic should underline: the first
// capture group when the pattern has one, so a pattern can match more context
// than it points at, and otherwise the whole match. Nil means no match.
func matchSpan(re *regexp.Regexp, s string) []int {
	m := re.FindStringSubmatchIndex(s)
	if m == nil {
		return nil
	}
	if re.NumSubexp() > 0 && m[2] >= 0 {
		return []int{m[2], m[3]}
	}
	// Trailing whitespace under a caret reads as a mistake in the compiler.
	start, end := m[0], m[1]
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return []int{start, end}
}

// checkDivItemsMatch requires every item inside a named fenced div to match a
// pattern. What the div is called and what its items must look like is the
// project's convention, so both come from the schema.
//
// args:
//
//	div:     the fence name, without the colons — `evidence` for `::: evidence`
//	pattern: a Go regexp every item must match
func checkDivItemsMatch(c *ruleContext) {
	name, ok := c.argString("div", true)
	if !ok {
		return
	}
	re, ok := c.argRegexp("pattern", nil)
	if !ok {
		return
	}

	for _, div := range c.File.Divs() {
		if div.Name != name {
			continue
		}
		for _, item := range divListItems(c.File, div) {
			if re.MatchString(item.Text) {
				continue
			}
			c.report(item.Pos,
				fmt.Sprintf("every item in a `::: %s` block must match %s", name, re),
				"item in `::: %s` block does not match the required form", name)
		}
	}
}

// checkCrossReference cross-checks keys cited in the body against a list in the
// frontmatter, in both directions: cited but not listed, and listed but never
// cited.
//
// The Nth entry of the list is key N. That is the whole of the correspondence —
// the list is positional, so an entry's key is where it sits.
//
// args:
//
//	div:        the fence name whose items carry the citations
//	pattern:    a Go regexp whose capture group 1 is the cited key
//	list_field: the frontmatter list the keys index into
//	label:      what one entry is called in diagnostics; defaults to list_field
func checkCrossReference(c *ruleContext) {
	divName, ok := c.argString("div", true)
	if !ok {
		return
	}
	re, ok := c.argRegexp("pattern", nil)
	if !ok {
		return
	}
	if re.NumSubexp() < 1 {
		c.schemaErrorf("wrap the key in parentheses, e.g. `exhibit (\\d+)`",
			"argument %q needs a capture group for the referenced key", "pattern")
		return
	}
	field, ok := c.argString("list_field", true)
	if !ok {
		return
	}
	label, ok := c.argString("label", false)
	if !ok {
		return
	}
	if label == "" {
		label = field
	}

	var declared []string
	if raw, found := c.Meta.Lookup(field); found {
		if items, isList := raw.([]any); isList {
			for i := range items {
				declared = append(declared, strconv.Itoa(i+1))
			}
		}
	}

	referenced := map[string]diag.Position{}
	for _, div := range c.File.Divs() {
		if div.Name != divName {
			continue
		}
		for _, item := range divListItems(c.File, div) {
			mm := re.FindStringSubmatch(item.Text)
			if mm == nil {
				continue
			}
			if _, seen := referenced[mm[1]]; !seen {
				referenced[mm[1]] = item.Pos
			}
		}
	}

	for _, key := range sortedKeys(referenced) {
		if !slices.Contains(declared, key) {
			c.report(referenced[key],
				fmt.Sprintf("add entry %s to the `%s` list in the frontmatter", key, field),
				"%s %s is cited in the body but not listed in `%s`", label, key, field)
		}
	}
	for _, key := range declared {
		if _, cited := referenced[key]; !cited {
			c.report(c.Meta.Pos(field),
				fmt.Sprintf("cite it in a `::: %s` block, or remove it", divName),
				"%s %s is listed in `%s` but never cited in the body", label, key, field)
		}
	}
}

// checkNoEmptySections flags a heading with no content before the next heading.
// An empty section in a filed brief is an oversight, not a style choice.
//
// A heading whose next heading is deeper is a container — "BEGRÜNDUNG" over
// "Formelles" — and carries its content in its subsections, so it is exempt.
func checkNoEmptySections(c *ruleContext) {
	headings := c.File.Headings()
	for i, h := range headings {
		next := len(c.File.Source)
		if i+1 < len(headings) {
			if headings[i+1].Level > h.Level {
				continue
			}
			next = offsetOfLine(c.File, headings[i+1].Pos.Line)
		}
		start := offsetOfLine(c.File, h.Pos.Line+1)
		if start >= next || strings.TrimSpace(string(c.File.Source[start:next])) == "" {
			c.report(h.Pos, "write the section, or remove the heading",
				"section %q has no content", h.Text)
		}
	}
}

// divListItems returns one entry per line of content inside a div. Text lives
// on leaf blocks — a list item keeps its text in a child TextBlock, not on the
// item itself — so this walks to the leaves rather than assuming a depth.
func divListItems(f *parse.File, div *parse.Div) []parse.TextLine {
	var out []parse.TextLine
	_ = ast.Walk(div, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		// Lines panics on inline nodes, so descend no further than blocks.
		if !entering || n == ast.Node(div) || n.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		if n.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		for _, tl := range f.TextLines(n) {
			if strings.TrimSpace(tl.Text) != "" {
				out = append(out, tl)
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

func offsetOfLine(f *parse.File, line int) int {
	off := 0
	cur := 1
	src := f.Source
	for i := 0; i < len(src); i++ {
		if cur == line {
			return off
		}
		if src[i] == '\n' {
			cur++
			off = i + 1
		}
	}
	if cur == line {
		return off
	}
	return len(src)
}

func countLines(src []byte) int {
	n := 1
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	return n
}

// sortedKeys orders reference keys the way a reader expects: numerically when
// they are numbers, so 10 follows 9 rather than 1.
func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, aErr := strconv.Atoi(out[i])
		b, bErr := strconv.Atoi(out[j])
		if aErr == nil && bErr == nil {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}
