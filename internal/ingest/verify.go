package ingest

import (
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// Verify checks the assembled document's paragraph structure against the
// Randziffern the VLM observed on the source pages.
//
// docc computes Randziffern at render time from paragraph order (see
// Schema.Render.ParagraphNumbering) — they are never stored as text — so the
// only way to check that ingest reconstructed paragraph boundaries correctly
// is to compare the count and sequence docc would render against what the
// VLM actually saw on the source pages. This is the one check unique to
// docc's render-time numbering model; no generic PDF-to-markdown tool could
// perform it.
//
// It is deliberately not a registered sema rule: there is nothing to check
// without an ingest run's observed sequence, which would need a sidecar file
// format with no other consumer. It runs once, right after conversion.
func Verify(f *parse.File, sc *schema.Schema, pages []PageResult) diag.List {
	var ds diag.List
	rule := sc.Render.ParagraphNumbering
	if rule == nil {
		return ds
	}

	observed := observedRzSeq(pages)
	expected := expectedParagraphs(f, rule)

	if len(observed) != len(expected) {
		ds.Warnf(f.Path, diag.Position{}, "ING001",
			"compare the source pages against the assembled document and fix missing or merged paragraphs",
			"docc would render %d paragraph(s) as Randziffern, but the source pages showed %d",
			len(expected), len(observed))
		return ds
	}

	// A count that happens to match can still hide a misread: the VLM's own
	// observed numbers should themselves be continuous. A gap or repeat means
	// a page was misread even though paragraph *count* lined up.
	for i := 1; i < len(observed); i++ {
		if observed[i] != observed[i-1]+1 {
			pos := diag.Position{}
			if i < len(expected) {
				pos = expected[i]
			}
			ds.Warnf(f.Path, pos, "ING002",
				"check the source page for this paragraph — it may have been misread",
				"the observed Randziffer sequence is not continuous: %d follows %d",
				observed[i], observed[i-1])
		}
	}
	return ds
}

func observedRzSeq(pages []PageResult) []int {
	var out []int
	for _, p := range pages {
		out = append(out, p.RzSeq...)
	}
	return out
}

// expectedParagraphs walks the top level of the body — the only level
// Schema.Render.ParagraphNumbering applies to — and returns the position of
// every paragraph that would receive a Randziffer, replicating the
// arrive/depart marker logic internal/emit/emit.go uses at render time.
func expectedParagraphs(f *parse.File, rule *schema.NumberingRule) []diag.Position {
	marker, inclusive := rule.Marker()
	marker = normalizeHeading(marker)
	active := marker == ""

	var out []diag.Position
	for n := f.Body.FirstChild(); n != nil; n = n.NextSibling() {
		if inclusive && !active {
			active = isMarkerHeading(f, n, marker)
		}

		if active {
			if p, ok := n.(*ast.Paragraph); ok {
				out = append(out, blockPos(f, p))
			}
		}

		if !inclusive && !active {
			active = isMarkerHeading(f, n, marker)
		}
	}
	return out
}

func isMarkerHeading(f *parse.File, n ast.Node, marker string) bool {
	if marker == "" {
		return false
	}
	h, ok := n.(*ast.Heading)
	if !ok {
		return false
	}
	return normalizeHeading(string(h.Text(f.BodySource))) == marker //nolint:staticcheck // Text is the stable API for heading content
}

func blockPos(f *parse.File, n ast.Node) diag.Position {
	if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
		return f.BodyPos(n.Lines().At(0).Start)
	}
	return diag.Position{}
}

func normalizeHeading(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
