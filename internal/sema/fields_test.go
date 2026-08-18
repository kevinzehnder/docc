package sema

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

func fieldSchema() *schema.Schema {
	return &schema.Schema{
		Type: "test",
		Blanks: map[string]schema.FieldSpec{
			"beurkundungsdatum": {Required: true, Completion: "handwritten"},
			"protokollnummer":   {Required: true, Completion: "before-execution"},
			"bemerkung":         {},
		},
	}
}

const urkundeFields = `---
---

Die Urkunde wurde am
[____________________]{.docc-field key=beurkundungsdatum}
unterzeichnet.

Protokollnummer:
[____________________]{.docc-field key=protokollnummer}
`

func TestBlankFieldsPassCheck(t *testing.T) {
	ds := checkMarkupOn(t, urkundeFields, fieldSchema())
	if len(ds) != 0 {
		t.Fatalf("check should accept blank fields:\n%s", messages(ds))
	}
}

func TestAbsentRequiredField(t *testing.T) {
	src := "---\n---\n\nProtokollnummer: [_____]{.docc-field key=protokollnummer}\n"
	ds := checkMarkupOn(t, src, fieldSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC038" {
		t.Fatalf("codes = %v, want [DOC038]\n%s", got, messages(ds))
	}
	d := ds[0]
	if d.Key != "beurkundungsdatum" || !strings.Contains(d.Expected, "key=beurkundungsdatum") {
		t.Errorf("diagnostic = %+v, want the missing beurkundungsdatum with expected syntax", d)
	}
}

func TestFieldSpanWithoutKey(t *testing.T) {
	src := "---\n---\n\nDatum: [_____]{.docc-field}\n"
	ds := checkMarkupOn(t, src, &schema.Schema{Type: "test"})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC040" {
		t.Fatalf("codes = %v, want [DOC040]\n%s", got, messages(ds))
	}
}

func TestUndeclaredFieldKey(t *testing.T) {
	src := "---\n---\n\nDie Urkunde vom [___]{.docc-field key=beurkundungsdatum} mit\n[___]{.docc-field key=protokolnummer} liegt vor.\n"
	ds := checkMarkupOn(t, src, fieldSchema())
	found := false
	for _, d := range ds {
		if d.Code == "DOC040" && strings.Contains(d.Message, "protokolnummer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no DOC040 for the undeclared key:\n%s", messages(ds))
	}
}

func TestInvalidCompletionIsSchemaBug(t *testing.T) {
	sc := &schema.Schema{
		Type:   "test",
		Blanks: map[string]schema.FieldSpec{"x": {Completion: "later"}},
	}
	ds := checkMarkupOn(t, "---\n---\n\n[___]{.docc-field key=x}\n", sc)
	if got := codes(ds); len(got) != 1 || got[0] != "DOC041" {
		t.Fatalf("codes = %v, want [DOC041]\n%s", got, messages(ds))
	}
}

// docc-field needs no spans: declaration even in a schema that declares spans.
func TestFieldSpanTypeIsReserved(t *testing.T) {
	sc := fieldSchema()
	sc.Spans = map[string]schema.SpanSpec{"datum": {}}
	ds := checkMarkupOn(t, urkundeFields, sc)
	if len(ds) != 0 {
		t.Fatalf("reserved type flagged:\n%s", messages(ds))
	}
}

func TestSemanticFieldUsesBothContracts(t *testing.T) {
	sc := fieldSchema()
	sc.Spans = map[string]schema.SpanSpec{"datum": {}}
	src := strings.Replace(urkundeFields,
		"[____________________]{.docc-field key=protokollnummer}",
		"[____________________]{.datum .docc-field key=protokollnummer}", 1)
	if ds := checkMarkupOn(t, src, sc); len(ds) != 0 {
		t.Fatalf("semantic field rejected:\n%s", messages(ds))
	}
	if ds := completionOn(t, src, sc); len(ds) != 1 || ds[0].Code != "DOC039" || ds[0].Key != "protokollnummer" {
		t.Fatalf("semantic field completion = %+v, want DOC039 for protokollnummer", ds)
	}
}

func completionOn(t *testing.T, src string, sc *schema.Schema) diag.List {
	t.Helper()
	f, ds := parse.Parse("test.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("source does not parse: %+v", ds)
	}
	CheckCompletion(f, sc, &ds)
	return ds
}

func TestCompletionBlocksBlankBeforeExecution(t *testing.T) {
	ds := completionOn(t, urkundeFields, fieldSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC039" {
		t.Fatalf("codes = %v, want [DOC039]\n%s", got, messages(ds))
	}
	if ds[0].Key != "protokollnummer" {
		t.Errorf("key = %q, want protokollnummer — the handwritten date may stay blank", ds[0].Key)
	}
}

func TestCompletionAcceptsFilledField(t *testing.T) {
	src := strings.Replace(urkundeFields, "[____________________]{.docc-field key=protokollnummer}",
		"[2026/417]{.docc-field key=protokollnummer}", 1)
	ds := completionOn(t, src, fieldSchema())
	if len(ds) != 0 {
		t.Fatalf("filled field flagged:\n%s", messages(ds))
	}
}
