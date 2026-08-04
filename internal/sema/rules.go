package sema

import (
	"fmt"
	"regexp"
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

// registry maps schema rule names to implementations.
var registry = map[string]CheckFunc{
	"no_placeholder_text": checkNoPlaceholders,
	"beweis_beilage_refs": checkBeweisBeilageRefs,
	"beilagen_coverage":   checkBeilagenCoverage,
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
// whole list item, that is nothing but bracketed prose.
var placeholderRe = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)?\[[^\]]{3,}\]\s*[;.]?\s*$`)

// checkNoPlaceholders finds template text the author never replaced. This is
// the single highest-value check: unedited placeholders otherwise render
// straight into a filed document.
func checkNoPlaceholders(c *ruleContext) {
	for i := 1; ; i++ {
		line := c.File.LineText(i)
		if line == "" && i > countLines(c.File.Source) {
			return
		}
		if !placeholderRe.MatchString(line) {
			continue
		}
		// Markdown links are `[text](url)`, not placeholders.
		if strings.Contains(line, "](") {
			continue
		}
		col := strings.Index(line, "[") + 1
		c.report(diag.Position{Line: i, Col: col, Len: len(strings.TrimSpace(line)) - col + 1},
			"replace this with the actual content",
			"unfilled template placeholder")
	}
}

// beilageRefRe matches the trailing `// Beilage 3` reference on an evidence item.
var beilageRefRe = regexp.MustCompile(`//\s*Beilage\s+(\d+)\s*$`)

// checkBeweisBeilageRefs requires every item inside a `::: beweis` block to name
// the exhibit it relies on.
func checkBeweisBeilageRefs(c *ruleContext) {
	for _, div := range c.File.Divs() {
		if div.Name != "beweis" {
			continue
		}
		for _, item := range divListItems(c.File, div) {
			if beilageRefRe.MatchString(item.Text) {
				continue
			}
			c.report(item.Pos,
				`append a reference, e.g. "// Beilage 3"`,
				"Beweismittel without a Beilage reference")
		}
	}
}

// checkBeilagenCoverage cross-checks the exhibit numbers cited in the body
// against the `beilagen` list in the frontmatter, in both directions.
func checkBeilagenCoverage(c *ruleContext) {
	declared := map[int]bool{}
	if raw, ok := c.Meta.Lookup("beilagen"); ok {
		if items, isList := raw.([]any); isList {
			for i := range items {
				declared[i+1] = true
			}
		}
	}

	referenced := map[int]diag.Position{}
	for _, div := range c.File.Divs() {
		if div.Name != "beweis" {
			continue
		}
		for _, item := range divListItems(c.File, div) {
			mm := beilageRefRe.FindStringSubmatch(item.Text)
			if mm == nil {
				continue
			}
			n, err := strconv.Atoi(mm[1])
			if err != nil {
				continue
			}
			if _, seen := referenced[n]; !seen {
				referenced[n] = item.Pos
			}
		}
	}

	for _, n := range sortedInts(referenced) {
		if !declared[n] {
			c.report(referenced[n],
				fmt.Sprintf("add entry %d to the `beilagen` list in the frontmatter", n),
				"Beilage %d is cited in the body but not listed in `beilagen`", n)
		}
	}
	for _, n := range sortedInts(declared) {
		if _, cited := referenced[n]; !cited {
			c.report(c.Meta.Pos("beilagen"),
				"cite it in a `::: beweis` block, or remove it",
				"Beilage %d is listed in `beilagen` but never cited in the body", n)
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

func sortedInts[T any](m map[int]T) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
