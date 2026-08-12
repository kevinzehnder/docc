package ingest

import (
	"testing"

	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

const verifyFixture = `# Title

## RECHTSBEGEHREN

First paragraph.

Second paragraph.

Third paragraph.
`

func TestExpectedParagraphsStartAfterHeading(t *testing.T) {
	f, _ := parse.Parse("doc.md", []byte(verifyFixture))
	rule := &schema.NumberingRule{StartAfterHeading: "RECHTSBEGEHREN"}

	got := expectedParagraphs(f, rule)
	if len(got) != 3 {
		t.Fatalf("expected 3 paragraphs after the heading, got %d", len(got))
	}
	for i, pos := range got {
		if pos.Line == 0 {
			t.Errorf("paragraph %d: expected a real source position, got line 0", i)
		}
	}
}

func TestExpectedParagraphsWholeBody(t *testing.T) {
	f, _ := parse.Parse("doc.md", []byte(verifyFixture))
	// No marker: every top-level paragraph counts, including none above the
	// heading in this fixture — there are still exactly the same three.
	got := expectedParagraphs(f, &schema.NumberingRule{})
	if len(got) != 3 {
		t.Fatalf("expected 3 paragraphs, got %d", len(got))
	}
}

func TestVerifyNoRuleProducesNoDiagnostics(t *testing.T) {
	f, _ := parse.Parse("doc.md", []byte(verifyFixture))
	sc := &schema.Schema{}
	pages := []PageResult{{RzSeq: []int{1, 2, 3}}}

	ds := Verify(f, sc, pages)
	if len(ds) != 0 {
		t.Fatalf("expected no diagnostics without a paragraph_numbering rule, got %v", ds)
	}
}

func TestVerifyMatchingCountAndSequence(t *testing.T) {
	f, _ := parse.Parse("doc.md", []byte(verifyFixture))
	sc := &schema.Schema{Render: schema.Render{
		ParagraphNumbering: &schema.NumberingRule{StartAfterHeading: "RECHTSBEGEHREN"},
	}}
	pages := []PageResult{{RzSeq: []int{1, 2, 3}}}

	ds := Verify(f, sc, pages)
	if len(ds) != 0 {
		t.Fatalf("expected no diagnostics for a matching, continuous sequence, got %v", ds)
	}
}

func TestVerifyCountMismatch(t *testing.T) {
	f, _ := parse.Parse("doc.md", []byte(verifyFixture))
	sc := &schema.Schema{Render: schema.Render{
		ParagraphNumbering: &schema.NumberingRule{StartAfterHeading: "RECHTSBEGEHREN"},
	}}
	// Only two Randziffern observed across the source pages, but the
	// assembled document has three paragraphs — a merge or split happened.
	pages := []PageResult{{RzSeq: []int{1, 2}}}

	ds := Verify(f, sc, pages)
	if len(ds) != 1 || ds[0].Code != "ING001" {
		t.Fatalf("expected exactly one ING001 diagnostic, got %v", ds)
	}
}

func TestVerifySequenceGap(t *testing.T) {
	f, _ := parse.Parse("doc.md", []byte(verifyFixture))
	sc := &schema.Schema{Render: schema.Render{
		ParagraphNumbering: &schema.NumberingRule{StartAfterHeading: "RECHTSBEGEHREN"},
	}}
	// Count matches (3), but the model's own reported sequence skips 2 —
	// it likely misread a page even though the paragraph count lined up.
	pages := []PageResult{{RzSeq: []int{1, 3, 4}}}

	ds := Verify(f, sc, pages)
	if len(ds) != 1 || ds[0].Code != "ING002" {
		t.Fatalf("expected exactly one ING002 diagnostic, got %v", ds)
	}
}
