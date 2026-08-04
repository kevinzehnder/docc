// Package sema validates a parsed document against its schema: frontmatter
// types, body structure, and named cross-cutting rules.
//
// Every pass appends to a single diagnostic list rather than stopping at the
// first problem. An author fixing one error at a time is an author running the
// compiler ten times.
package sema

import (
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// Result is the outcome of checking one document.
type Result struct {
	// DocType is the resolved document type, empty if it could not be determined.
	DocType string
	// Schema is the schema that was applied, nil if none matched.
	Schema *schema.Schema
	// Meta is the decoded frontmatter.
	Meta *Meta
	// Diagnostics holds every finding, including any from parsing.
	Diagnostics diag.List
}

// Check validates a parsed file. docTypeOverride, when non-empty, replaces the
// `document_type` declared in the frontmatter.
func Check(f *parse.File, set *schema.Set, parseDiags diag.List, docTypeOverride string) *Result {
	ds := parseDiags
	res := &Result{Diagnostics: ds}

	m := decodeMeta(f, &ds)
	res.Meta = m

	docType := docTypeOverride
	if docType == "" {
		if v, ok := m.Lookup("document_type"); ok {
			if s, isStr := v.(string); isStr {
				docType = strings.TrimSpace(s)
			}
		}
	}
	if docType == "" {
		ds.Errorf(f.Path, diag.Position{}, "DOC012",
			"add `document_type: <type>` to the frontmatter, or pass --type. Known types: "+strings.Join(set.Types(), ", "),
			"cannot determine document type")
		res.Diagnostics = ds
		return res
	}
	res.DocType = docType

	sc, err := set.Get(docType)
	if err != nil {
		ds.Errorf(f.Path, m.Pos("document_type"),
			"DOC013", "use one of: "+strings.Join(set.Types(), ", "),
			"unknown document type %q", docType)
		res.Diagnostics = ds
		return res
	}
	res.Schema = sc

	checkFrontmatter(f, sc, m, &ds)
	checkBody(f, sc, m, &ds)
	runRules(f, sc, m, &ds)

	res.Diagnostics = ds
	return res
}

func sortedFieldNames(f schema.Fields) []string {
	out := make([]string, 0, len(f))
	for k := range f {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// nearest returns the declared field name closest to name, for "did you mean"
// hints. Only close matches count; a distant one is noise.
func nearest(name string, fields schema.Fields) string {
	best, bestDist := "", 1<<30
	limit := len(name)/3 + 1
	for candidate := range fields {
		d := levenshtein(strings.ToLower(name), strings.ToLower(candidate))
		if d < bestDist {
			best, bestDist = candidate, d
		}
	}
	if bestDist <= limit {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
