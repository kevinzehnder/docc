package sema

import (
	"fmt"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// checkBody verifies that the headings the schema requires are present, and
// that declared subsections sit under the right parent.
//
// Body rules lean deliberately towards warnings. Legal documents vary in shape
// and a compiler that refuses to build a valid brief because it lacks a
// conventional heading is worse than no compiler.
func checkBody(f *parse.File, sc *schema.Schema, m *Meta, ds *diag.List) {
	if len(sc.Body) == 0 {
		return
	}
	headings := f.Headings()
	checkRules(f, sc.Body, headings, m, nil, ds)
}

// checkRules matches one level of body rules against the headings available in
// scope. scope is nil at the top level, otherwise the headings beneath a
// matched parent.
func checkRules(f *parse.File, rules []schema.BodyRule, headings []parse.Heading, m *Meta, parent *parse.Heading, ds *diag.List) {
	lastIndex := -1
	for _, rule := range rules {
		idx := findHeading(headings, rule)
		if idx < 0 {
			reportMissing(f, rule, m, parent, headings, ds)
			continue
		}

		if rule.Ordered && idx < lastIndex {
			ds.Warnf(f.Path, headings[idx].Pos, "DOC022",
				"move this section back into the order the schema declares",
				"heading %q appears out of order", rule.Heading)
		}
		lastIndex = idx

		if len(rule.Children) > 0 {
			checkRules(f, rule.Children, sectionOf(headings, idx), m, &headings[idx], ds)
		}
	}
}

func reportMissing(f *parse.File, rule schema.BodyRule, m *Meta, parent *parse.Heading, headings []parse.Heading, ds *diag.List) {
	required, why := isRequired(rule, m)
	if !required && rule.RequiredWhen == "" && !rule.Required {
		// Optional section, simply absent. Not worth a diagnostic.
		return
	}

	where := diag.Position{}
	switch {
	case parent != nil:
		where = parent.Pos
	case len(headings) > 0:
		where = headings[0].Pos
	}

	hint := fmt.Sprintf("add a level-%d heading %q", rule.Level, rule.Heading)
	msg := fmt.Sprintf("missing required section %q", rule.Heading)
	if why != "" {
		msg += " (" + why + ")"
	}
	if required {
		ds.Errorf(f.Path, where, "DOC020", hint, "%s", msg)
		return
	}
	ds.Warnf(f.Path, where, "DOC021", hint, "missing conventional section %q", rule.Heading)
}

// isRequired evaluates Required and RequiredWhen. The returned reason is shown
// to explain a conditional requirement.
func isRequired(rule schema.BodyRule, m *Meta) (bool, string) {
	if rule.RequiredWhen == "" {
		return rule.Required, ""
	}
	field, want, ok := parseCondition(rule.RequiredWhen)
	if !ok {
		return rule.Required, ""
	}
	got, present := m.Lookup(field)
	if !present {
		return false, ""
	}
	if s, isStr := got.(string); isStr && s == want {
		return true, fmt.Sprintf("required because %s is %q", field, want)
	}
	return false, ""
}

// parseCondition understands the single supported form: `field == "value"`.
// Anything richer belongs in a named Go rule, not in a schema expression
// language nobody wants to maintain.
func parseCondition(expr string) (field, value string, ok bool) {
	parts := strings.SplitN(expr, "==", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	field = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	value = strings.Trim(value, `"'`)
	return field, value, field != "" && value != ""
}

// findHeading locates a heading matching the rule, comparing case-insensitively
// so RECHTSBEGEHREN and Rechtsbegehren are the same section.
func findHeading(headings []parse.Heading, rule schema.BodyRule) int {
	want := strings.ToLower(strings.TrimSpace(rule.Heading))
	for i, h := range headings {
		if rule.Level > 0 && h.Level != rule.Level {
			continue
		}
		if strings.ToLower(h.Text) == want {
			return i
		}
	}
	return -1
}

// sectionOf returns the headings nested under headings[idx] — every following
// heading until one at the same or shallower level.
func sectionOf(headings []parse.Heading, idx int) []parse.Heading {
	level := headings[idx].Level
	for i := idx + 1; i < len(headings); i++ {
		if headings[i].Level <= level {
			return headings[idx+1 : i]
		}
	}
	return headings[idx+1:]
}
