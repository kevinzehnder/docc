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
