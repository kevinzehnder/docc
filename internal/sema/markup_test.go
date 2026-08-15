package sema

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
)

// checkMarkupOn parses src and runs the markup pass under sc.
func checkMarkupOn(t *testing.T, src string, sc *schema.Schema) diag.List {
	t.Helper()
	f, ds := parse.Parse("test.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("source does not parse: %+v", ds)
	}
	checkMarkup(f, sc, &ds)
	return ds
}

func parteiSchema() *schema.Schema {
	return &schema.Schema{
		Type: "test",
		Blocks: map[string]schema.BlockSpec{
			"beweis": {},
			"partei": {
				Discriminator: "kind",
				Attributes:    []string{"role"},
				Variants: map[string]schema.BlockVariant{
					"person":  {RequiredSpans: []string{"name", "geburtsdatum", "adresse"}},
					"company": {RequiredSpans: []string{"firma", "sitz", "uid", "adresse"}},
				},
			},
		},
		Spans: map[string]schema.SpanSpec{
			"name": {}, "geburtsdatum": {}, "adresse": {},
			"firma": {}, "sitz": {}, "uid": {}, "datum": {},
		},
	}
}

const validPartei = `---
document_type: test
---

::: partei {#verkaeufer kind=person role=veraeusserer}
Herr [Max Muster]{.name}, geboren am
[12. April 1975]{.geburtsdatum}, wohnhaft an der
[Musterstrasse 10, 5400 Baden]{.adresse}
:::
`

func TestValidBlockAndSpansPass(t *testing.T) {
	ds := checkMarkupOn(t, validPartei, parteiSchema())
	if len(ds) != 0 {
		t.Fatalf("diagnostics on valid document:\n%s", messages(ds))
	}
}

func TestUndeclaredBlock(t *testing.T) {
	src := "---\n---\n\n::: zeugen\ntext\n:::\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC030" {
		t.Fatalf("codes = %v, want [DOC030]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Hint, "beweis, partei") {
		t.Errorf("hint should list declared blocks: %q", ds[0].Hint)
	}
}

func TestMissingDiscriminator(t *testing.T) {
	src := "---\n---\n\n::: partei {#p}\n[Max]{.name}\n:::\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC032" {
		t.Fatalf("codes = %v, want [DOC032]\n%s", got, messages(ds))
	}
	if ds[0].Expected == "" {
		t.Error("DOC032 for a missing discriminator should carry an expected example")
	}
}

func TestUnknownVariant(t *testing.T) {
	src := "---\n---\n\n::: partei {#p kind=verein}\n[Max]{.name}\n:::\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC032" {
		t.Fatalf("codes = %v, want [DOC032]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Hint, "company, person") {
		t.Errorf("hint should list variants: %q", ds[0].Hint)
	}
}

func TestMissingRequiredSpans(t *testing.T) {
	src := `---
---

::: partei {#erwerberin kind=company}
Die [Beispiel AG]{.firma}, mit Sitz in [Zürich]{.sitz}, mit
Geschäftsadresse [Beispielweg 4, 8001 Zürich]{.adresse}
:::
`
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC033" {
		t.Fatalf("codes = %v, want [DOC033]\n%s", got, messages(ds))
	}
	d := ds[0]
	if !strings.Contains(d.Message, ".uid") || d.Block != "erwerberin" || d.Expected != "[...]{.uid}" {
		t.Errorf("diagnostic = %+v, want missing .uid on block erwerberin", d)
	}
}

func TestUnknownAttribute(t *testing.T) {
	src := "---\n---\n\n::: beweis {seite=4}\ntext\n:::\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC035" {
		t.Fatalf("codes = %v, want [DOC035]\n%s", got, messages(ds))
	}
	// The caret must sit under the attribute key itself.
	if ds[0].Pos.Line != 4 || ds[0].Pos.Len != len("seite") {
		t.Errorf("pos = %+v, want line 4 underlining `seite`", ds[0].Pos)
	}
}

func TestDuplicateID(t *testing.T) {
	src := "---\n---\n\n::: beweis {#a}\nx\n:::\n\n::: beweis {#a}\ny\n:::\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC034" {
		t.Fatalf("codes = %v, want [DOC034]\n%s", got, messages(ds))
	}
	if len(ds[0].Related) != 1 || ds[0].Related[0].Pos.Line != 4 {
		t.Errorf("related = %+v, want the first #a at line 4", ds[0].Related)
	}
	if ds[0].Pos.Line != 8 {
		t.Errorf("pos = %+v, want the duplicate at line 8", ds[0].Pos)
	}
}

func TestUntypedSpan(t *testing.T) {
	src := "---\n---\n\nPreis [CHF 100]{key=kaufpreis} gilt.\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC031" {
		t.Fatalf("codes = %v, want [DOC031]\n%s", got, messages(ds))
	}
}

func TestUnknownSpanType(t *testing.T) {
	src := "---\n---\n\nAm [1. Mai 2026]{.datm key=x} passiert es.\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC031" {
		t.Fatalf("codes = %v, want [DOC031]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Hint, "datum") {
		t.Errorf("hint should list declared span types: %q", ds[0].Hint)
	}
}

func TestRefResolves(t *testing.T) {
	src := "---\n---\n\n::: beweis {#vertrag}\nx\n:::\n\nGemäss [Vertrag]{.datum ref=vertrag} gilt es.\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if len(ds) != 0 {
		t.Fatalf("diagnostics on resolving ref:\n%s", messages(ds))
	}
}

func TestDanglingRef(t *testing.T) {
	src := "---\n---\n\n::: beweis {#vertrag}\nx\n:::\n\nGemäss [Vertrag]{.datum ref=vertrga} gilt es.\n"
	ds := checkMarkupOn(t, src, parteiSchema())
	if got := codes(ds); len(got) != 1 || got[0] != "DOC037" {
		t.Fatalf("codes = %v, want [DOC037]\n%s", got, messages(ds))
	}
	d := ds[0]
	if !strings.Contains(d.Hint, "#vertrag") {
		t.Errorf("hint should list known ids: %q", d.Hint)
	}
	// Caret under the value `vertrga` on line 8.
	if d.Pos.Line != 8 || d.Pos.Len != len("vertrga") {
		t.Errorf("pos = %+v, want line 8 underlining the ref value", d.Pos)
	}
}

// A dangling ref is an error even when the schema declares nothing: a
// reference the author wrote is a reference the author wants resolved.
func TestDanglingRefWithoutDeclarations(t *testing.T) {
	src := "---\n---\n\n[Erwerberin]{.partei ref=erwerberin} kauft.\n"
	ds := checkMarkupOn(t, src, &schema.Schema{Type: "test"})
	if got := codes(ds); len(got) != 1 || got[0] != "DOC037" {
		t.Fatalf("codes = %v, want [DOC037]\n%s", got, messages(ds))
	}
	if !strings.Contains(ds[0].Hint, "no block declares an id") {
		t.Errorf("hint = %q", ds[0].Hint)
	}
}

// A schema that declares neither blocks nor spans leaves markup unchecked:
// the declarations are the opt-in.
func TestUndeclaredSchemaIsPermissive(t *testing.T) {
	src := "---\n---\n\n::: anything {#x foo=bar}\n[text]{.whatever key=k}\n:::\n"
	ds := checkMarkupOn(t, src, &schema.Schema{Type: "test"})
	if len(ds) != 0 {
		t.Fatalf("diagnostics without declarations:\n%s", messages(ds))
	}
}

// Variants without a discriminator are a schema bug and must say so.
func TestVariantsWithoutDiscriminator(t *testing.T) {
	sc := &schema.Schema{
		Type: "test",
		Blocks: map[string]schema.BlockSpec{
			"partei": {Variants: map[string]schema.BlockVariant{"person": {}}},
		},
	}
	src := "---\n---\n\n::: partei\nx\n:::\n"
	ds := checkMarkupOn(t, src, sc)
	if got := codes(ds); len(got) != 1 || got[0] != "DOC036" {
		t.Fatalf("codes = %v, want [DOC036]\n%s", got, messages(ds))
	}
}
