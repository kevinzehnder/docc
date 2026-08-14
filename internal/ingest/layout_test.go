package ingest

import (
	"strings"
	"testing"
)

func TestParseLayoutReadsTheModelsBoxes(t *testing.T) {
	// A real response, from page 30 of a scanned German brief.
	raw := strings.Join([]string{
		"887 027 936 060header",
		"131 175 218 189title",
		"085 193 102 203text",
		"131 193 870 532text",
		"871 959 895 972page_number",
	}, "\n")

	blocks, err := ParseLayout(raw)
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if len(blocks) != 5 {
		t.Fatalf("got %d blocks, want 5", len(blocks))
	}

	want := []string{"header", "title", "text", "text", "page_number"}
	for i, w := range want {
		if blocks[i].Type != w {
			t.Errorf("block %d type = %q, want %q", i, blocks[i].Type, w)
		}
	}

	// Thousandths become fractions at the parse boundary, so that nothing
	// downstream has to know the layout image's size.
	if got := blocks[1].Box; got.X0 != 0.131 || got.Y0 != 0.175 || got.X1 != 0.218 || got.Y1 != 0.189 {
		t.Errorf("title box = %+v, want {0.131 0.175 0.218 0.189}", got)
	}
}

func TestParseLayoutReadsRotation(t *testing.T) {
	blocks, err := ParseLayout("131 175 218 189<|rotate_right|>table")
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if blocks[0].Angle != 90 {
		t.Errorf("angle = %d, want 90", blocks[0].Angle)
	}
	if blocks[0].Type != "table" {
		t.Errorf("type = %q, want table", blocks[0].Type)
	}
}

// A page with a hole in it reads exactly like a page the document does not
// have, so a line that does not parse has to stop the page rather than be
// skipped.
func TestParseLayoutRejectsMalformedLines(t *testing.T) {
	cases := map[string]string{
		"unknown type":       "131 175 218 189paragraph",
		"truncated mid-line": "131 175 218",
		"coordinate too big": "131 175 218 1890text",
		"inverted box":       "500 175 218 189text",
		"not a number":       "131 175 abc 189text",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseLayout(raw); err == nil {
				t.Fatalf("ParseLayout(%q) succeeded, want an error", raw)
			}
		})
	}
}

func TestParseLayoutRejectsAnEmptyResponse(t *testing.T) {
	if _, err := ParseLayout("  \n\n"); err == nil {
		t.Fatal("ParseLayout of an empty response succeeded, want an error")
	}
}

// The layout pass re-detects a region: on a real page it answered with the
// same "165 607 848 694text" line three times, and each copy was recognized
// and transcribed as its own paragraph.
func TestDedupeBlocksDropsRedetectedRegions(t *testing.T) {
	same := BBox{X0: 0.165, Y0: 0.607, X1: 0.848, Y1: 0.694}
	blocks := dedupeBlocks([]Block{
		{Type: "text", Box: BBox{X0: 0.164, Y0: 0.273, X1: 0.851, Y1: 0.378}},
		{Type: "text", Box: same},
		{Type: "text", Box: same},
		{Type: "text", Box: BBox{X0: 0.165, Y0: 0.608, X1: 0.849, Y1: 0.695}}, // off by a thousandth
		{Type: "text", Box: BBox{X0: 0.164, Y0: 0.748, X1: 0.849, Y1: 0.907}},
	})
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3: %v", len(blocks), blocks)
	}
}

// A list container and its text share a box but not a type; the container
// skip handles that pair, not the dedupe. And a caption inside a figure is
// not a re-detection.
func TestDedupeBlocksKeepsDistinctBlocks(t *testing.T) {
	blocks := dedupeBlocks([]Block{
		{Type: "list", Box: BBox{X0: 0.164, Y0: 0.186, X1: 0.851, Y1: 0.254}},
		{Type: "text", Box: BBox{X0: 0.164, Y0: 0.186, X1: 0.851, Y1: 0.254}},
		{Type: "image", Box: BBox{X0: 0.2, Y0: 0.4, X1: 0.8, Y1: 0.8}},
		{Type: "image_caption", Box: BBox{X0: 0.3, Y0: 0.75, X1: 0.7, Y1: 0.79}},
	})
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4: %v", len(blocks), blocks)
	}
}
