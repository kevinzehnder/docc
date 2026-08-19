package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// readResult mirrors readDoc for decoding: diag.Severity marshals to a string
// it cannot unmarshal from, so the diagnostics are shadowed with plain maps.
type readResult struct {
	readDoc
	Diagnostics []map[string]any `json:"diagnostics"`
}

// readOne runs `docc read` on one file and decodes the object it prints.
func readOne(t *testing.T, args ...string) (readResult, int) {
	t.Helper()
	var code int
	stdout := captureStdout(t, func() {
		code = run(append([]string{"read"}, args...))
	})
	var doc readResult
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("read output is not one JSON object: %v\n%s", err, stdout)
	}
	return doc, code
}

// TestReadAnswersTheAcceptanceQuestions drives the corpus fixture through the
// feature's own acceptance test: a consumer must be able to answer everything
// the document states from the JSON alone, with no knowledge of markdown.
func TestReadAnswersTheAcceptanceQuestions(t *testing.T) {
	doc, code := readOne(t,
		"--schema-dir", filepath.Join("..", "..", "testdata", "schemas"),
		filepath.Join("..", "..", "testdata", "good", "urkunde_valid.md"))
	if code != 0 {
		t.Fatalf("read = %d, want 0", code)
	}
	if !doc.OK || doc.DocumentType != "ch_urkunde_kaufvertrag" {
		t.Errorf("ok=%v type=%q, want ok=true type=ch_urkunde_kaufvertrag", doc.OK, doc.DocumentType)
	}

	// Every parcel the deed names, via its .grundbuch spans.
	var parcels []string
	for _, s := range doc.Spans {
		if s.Class == "grundbuch" {
			parcels = append(parcels, s.Text)
			if s.Line == 0 || s.Heading == "" {
				t.Errorf("span %q has no position or heading context: %+v", s.Text, s)
			}
		}
	}
	want := []string{"GB Baden Nr. 4711", "GB Baden Nr. 4725"}
	if len(parcels) != len(want) || parcels[0] != want[0] || parcels[1] != want[1] {
		t.Errorf("grundbuch spans = %v, want %v", parcels, want)
	}

	// A blank field is distinguishable from an absent one: null value, blank
	// true, and the schema's completion stage.
	var sawBlank bool
	for _, field := range doc.Fields {
		if field.Key == "protokoll_nr" {
			sawBlank = true
			if field.Value != nil || !field.Blank || field.Completion != "handwritten" {
				t.Errorf("protokoll_nr = %+v, want blank with null value and completion handwritten", field)
			}
		}
	}
	if !sawBlank {
		t.Error("protokoll_nr does not appear in fields")
	}

	// Dates come back typed by the schema: ISO, not the raw scalar.
	if got := doc.Frontmatter["date"]; got != "2026-09-04" {
		t.Errorf("frontmatter date = %v, want 2026-09-04", got)
	}
}

// TestReadEmitsContentDespiteDiagnostics is the semantic that makes read a
// sibling of check rather than a mode of it: a draft with open errors is
// exactly the document a review tool wants to inspect, so content is emitted,
// diagnostics ride alongside, and the exit stays 0.
func TestReadEmitsContentDespiteDiagnostics(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)

	// Missing the required `title`; text carries an escape and a hard break.
	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\n---\n\n# Summary\n\nFahrwegrecht \\[Unterhalt\\] am Weg  \nzweite Zeile\n")

	got, code := readOne(t, "--schema-dir", schemaDir, doc)
	if code != 0 {
		t.Fatalf("read = %d, want 0 even with diagnostics", code)
	}
	if got.OK || got.Errors == 0 || len(got.Diagnostics) == 0 {
		t.Errorf("ok=%v errors=%d diagnostics=%d — the missing title must be reported", got.OK, got.Errors, len(got.Diagnostics))
	}
	if len(got.Body) != 1 || got.Body[0].Heading != "Summary" {
		t.Fatalf("body = %+v, want the Summary section despite the errors", got.Body)
	}

	// Escapes resolved, hard breaks preserved as separate lines: the two
	// things a consumer would otherwise re-implement, and get wrong silently.
	para := got.Body[0].Blocks[0]
	if len(para.Lines) != 2 || para.Lines[0] != "Fahrwegrecht [Unterhalt] am Weg" || para.Lines[1] != "zweite Zeile" {
		t.Errorf("para lines = %q, want the escape resolved and the break kept", para.Lines)
	}
}
