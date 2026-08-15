package parse

import (
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantFM  bool
		wantYML string
	}{
		{
			name:    "well formed",
			src:     "---\ntitle: x\n---\n# Heading\n",
			wantFM:  true,
			wantYML: "title: x\n",
		},
		{
			name:   "unterminated",
			src:    "---\ntitle: x\n# Heading\n",
			wantFM: false,
		},
		{
			name:   "no frontmatter",
			src:    "# Heading\n",
			wantFM: false,
		},
		{
			name:    "empty frontmatter",
			src:     "---\n---\nbody\n",
			wantFM:  true,
			wantYML: "",
		},
		{
			// A horizontal rule further down must not be mistaken for a closing
			// delimiter of a block that was never opened.
			name:   "leading rule only",
			src:    "text\n\n---\n\nmore\n",
			wantFM: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := Parse("t.md", []byte(tt.src))
			if f.HasFrontmatter != tt.wantFM {
				t.Fatalf("HasFrontmatter = %v, want %v", f.HasFrontmatter, tt.wantFM)
			}
			if tt.wantFM && string(f.Frontmatter) != tt.wantYML {
				t.Errorf("Frontmatter = %q, want %q", f.Frontmatter, tt.wantYML)
			}
		})
	}
}

func TestPositionsSurviveFrontmatter(t *testing.T) {
	src := "---\ntitle: x\n---\n\n# First\n\nbody\n\n## Second\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}

	hs := f.Headings()
	if len(hs) != 2 {
		t.Fatalf("got %d headings, want 2", len(hs))
	}
	if hs[0].Text != "First" || hs[0].Level != 1 || hs[0].Pos.Line != 5 {
		t.Errorf("first heading = %q level %d line %d, want \"First\" level 1 line 5",
			hs[0].Text, hs[0].Level, hs[0].Pos.Line)
	}
	if hs[1].Text != "Second" || hs[1].Level != 2 || hs[1].Pos.Line != 9 {
		t.Errorf("second heading = %q level %d line %d, want \"Second\" level 2 line 9",
			hs[1].Text, hs[1].Level, hs[1].Pos.Line)
	}
}

// A heading inside a fenced code block is not a heading. This is the reason the
// body is parsed rather than line-scanned.
func TestHeadingsIgnoreCodeBlocks(t *testing.T) {
	src := "---\ntitle: x\n---\n\n# Real\n\n```\n# Not a heading\n```\n"
	f, _ := Parse("t.md", []byte(src))
	hs := f.Headings()
	if len(hs) != 1 {
		t.Fatalf("got %d headings, want 1: %+v", len(hs), hs)
	}
	if hs[0].Text != "Real" {
		t.Errorf("heading = %q, want \"Real\"", hs[0].Text)
	}
}

func TestDivs(t *testing.T) {
	src := "---\ntitle: x\n---\n\n::: evidence\n- one\n- two\n:::\n\nafter\n"
	f, _ := Parse("t.md", []byte(src))
	divs := f.Divs()
	if len(divs) != 1 {
		t.Fatalf("got %d divs, want 1", len(divs))
	}
	if divs[0].Name != "evidence" {
		t.Errorf("div name = %q, want \"evidence\"", divs[0].Name)
	}
	if !divs[0].Closed {
		t.Error("closed div reports Closed = false")
	}
	if n := divs[0].ChildCount(); n != 1 {
		t.Errorf("div children = %d, want 1 (the list)", n)
	}
}

func TestDivAttributes(t *testing.T) {
	src := "---\ntitle: x\n---\n\n::: partei {#verkaeufer kind=person role=veraeusserer note=\"mit Umlaut ä\"}\ntext\n:::\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	divs := f.Divs()
	if len(divs) != 1 {
		t.Fatalf("got %d divs, want 1", len(divs))
	}
	d := divs[0]
	if d.Name != "partei" {
		t.Errorf("name = %q, want partei", d.Name)
	}
	if d.Attr.ID != "verkaeufer" {
		t.Errorf("id = %q, want verkaeufer", d.Attr.ID)
	}
	for _, want := range []struct{ k, v string }{
		{"kind", "person"}, {"role", "veraeusserer"}, {"note", "mit Umlaut ä"},
	} {
		got, ok := d.Attr.Get(want.k)
		if !ok || got != want.v {
			t.Errorf("attr %s = %q (found %v), want %q", want.k, got, ok, want.v)
		}
	}
	// The id offset must point at the `#` so a caret lands under it.
	if pos := f.BodyPos(d.Attr.IDOffset); pos.Line != 5 || pos.Col != 13 {
		t.Errorf("id position = %d:%d, want 5:13", pos.Line, pos.Col)
	}
}

func TestDivAttributesWithDecoration(t *testing.T) {
	src := "---\n---\n\n::: partei {#p kind=person} :::\ntext\n:::\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	divs := f.Divs()
	if len(divs) != 1 || divs[0].Attr.ID != "p" {
		t.Fatalf("divs = %+v, want one with id p", divs)
	}
}

func TestMalformedDivAttributesAreDiagnostic(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unterminated brace", "---\n---\n\n::: partei {#p kind=person\ntext\n:::\n"},
		{"bare word", "---\n---\n\n::: partei {#p stray}\ntext\n:::\n"},
		{"unclosed quote", "---\n---\n\n::: partei {note=\"open}\ntext\n:::\n"},
		{"double id", "---\n---\n\n::: partei {#a #b}\ntext\n:::\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := Parse("t.md", []byte(tt.src))
			found := false
			for _, d := range ds {
				if d.Code == "DOC026" {
					found = true
					if d.Pos.Line != 4 {
						t.Errorf("diagnostic line = %d, want 4", d.Pos.Line)
					}
				}
			}
			if !found {
				t.Errorf("no DOC026 diagnostic: %+v", ds)
			}
		})
	}
}

// Prose that merely starts with colons must not become a div, with or without
// braces further along the line.
func TestFenceRejectsProse(t *testing.T) {
	src := "---\n---\n\n::: this is prose {not=attrs}\n\n::: also prose here\n"
	f, ds := Parse("t.md", []byte(src))
	if len(f.Divs()) != 0 {
		t.Fatalf("prose parsed as divs: %+v", f.Divs())
	}
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
}

func TestUnclosedDivIsDiagnostic(t *testing.T) {
	src := "---\n---\n\n::: evidence\n- [Beilage 1] Contract :::\n"
	_, ds := Parse("t.md", []byte(src))
	if len(ds) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %+v", len(ds), ds)
	}
	if ds[0].Code != "DOC023" || ds[0].Pos.Line != 4 {
		t.Errorf("diagnostic = %+v, want DOC023 at line 4", ds[0])
	}
}

func TestSpans(t *testing.T) {
	src := "---\n---\n\nDer Kaufpreis beträgt\n[CHF 1'250'000.00]{.preis key=kaufpreis}.\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	spans := f.Spans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	s := spans[0]
	if got := s.LiteralText(f.BodySource); got != "CHF 1'250'000.00" {
		t.Errorf("literal = %q", got)
	}
	if s.SpanType() != "preis" {
		t.Errorf("type = %q, want preis", s.SpanType())
	}
	if key, ok := s.Attr.Get("key"); !ok || key != "kaufpreis" {
		t.Errorf("key = %q (found %v), want kaufpreis", key, ok)
	}
	if pos := f.BodyPos(s.OpenOffset); pos.Line != 5 || pos.Col != 1 {
		t.Errorf("span position = %d:%d, want 5:1", pos.Line, pos.Col)
	}
}

// Byte offsets must stay correct after multi-byte characters — umlauts are
// common in this corpus.
func TestSpanOffsetsAfterUmlauts(t *testing.T) {
	src := "---\n---\n\nÜbergabe erfolgt öffentlich am [1. Oktober 2026]{.datum key=antritt} in Zürich.\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	spans := f.Spans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if got := spans[0].LiteralText(f.BodySource); got != "1. Oktober 2026" {
		t.Errorf("literal = %q", got)
	}
}

// Links and bracketed prose without an attribute block are not spans.
func TestSpanLeavesLinksAlone(t *testing.T) {
	src := "---\n---\n\nSee [the site](https://example.com) and [Beilage 1] here.\n\nNested [a [b]]{.x} stays prose.\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	if spans := f.Spans(); len(spans) != 0 {
		t.Fatalf("got %d spans, want 0: %+v", len(spans), spans)
	}
}

func TestSpanInsideDiv(t *testing.T) {
	src := "---\n---\n\n::: beweis\n**Beweis:** Auszug vom [10. August 2026]{.datum key=auszug}\n:::\n"
	f, ds := Parse("t.md", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	if spans := f.Spans(); len(spans) != 1 || spans[0].SpanType() != "datum" {
		t.Fatalf("spans = %+v, want one datum span", spans)
	}
}

func TestMalformedSpanAttributesAreDiagnostic(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unterminated brace", "---\n---\n\nPreis [CHF 100]{.preis key=kaufpreis\n"},
		{"bare word", "---\n---\n\nPreis [CHF 100]{.preis stray}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := Parse("t.md", []byte(tt.src))
			found := false
			for _, d := range ds {
				if d.Code == "DOC027" {
					found = true
					if d.Pos.Line != 4 {
						t.Errorf("diagnostic line = %d, want 4", d.Pos.Line)
					}
				}
			}
			if !found {
				t.Errorf("no DOC027 diagnostic: %+v", ds)
			}
		})
	}
}

func TestPosAt(t *testing.T) {
	src := "abc\ndef\n\nghi"
	f, _ := Parse("t.md", []byte(src))
	for _, tt := range []struct {
		offset    int
		line, col int
	}{
		{0, 1, 1},
		{2, 1, 3},
		{4, 2, 1},
		{8, 3, 1},
		{9, 4, 1},
	} {
		got := f.PosAt(tt.offset)
		if got.Line != tt.line || got.Col != tt.col {
			t.Errorf("PosAt(%d) = %d:%d, want %d:%d", tt.offset, got.Line, got.Col, tt.line, tt.col)
		}
	}
}
