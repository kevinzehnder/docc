package ingest

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseBBox(t *testing.T) {
	xml := `<doc>
<page width="612.000000" height="792.000000">
<word xMin="20.000000" yMin="100.000000" xMax="60.000000" yMax="112.000000">Hello</word>
<word xMin="64.000000" yMin="100.000000" xMax="110.000000" yMax="112.000000">World</word>
</page>
</doc>`

	anchors, err := parseBBox([]byte(xml))
	if err != nil {
		t.Fatalf("parseBBox: %v", err)
	}
	if len(anchors) != 2 {
		t.Fatalf("got %d anchors, want 2", len(anchors))
	}
	if anchors[0].Text != "Hello" || anchors[1].Text != "World" {
		t.Errorf("anchors = %+v", anchors)
	}
	if anchors[0].Width != 40 {
		t.Errorf("Width = %v, want 40 (xMax - xMin)", anchors[0].Width)
	}
}

func TestParseBBoxNoPages(t *testing.T) {
	anchors, err := parseBBox([]byte(`<doc></doc>`))
	if err != nil {
		t.Fatalf("parseBBox: %v", err)
	}
	if anchors != nil {
		t.Errorf("expected nil anchors for a page-less document, got %v", anchors)
	}
}

func TestPromptTextGroupsLines(t *testing.T) {
	anchors := []AnchorText{
		{Text: "Hello", X: 20, Y: 100, Height: 12},
		{Text: "World", X: 64, Y: 100.5, Height: 12}, // same line, tiny jitter
		{Text: "Second", X: 20, Y: 130, Height: 12},  // clearly a new line
		{Text: "line", X: 60, Y: 130, Height: 12},
	}
	got := PromptText(anchors)
	want := "Hello World\nSecond line"
	if got != want {
		t.Errorf("PromptText = %q, want %q", got, want)
	}
}

func TestPromptTextEmpty(t *testing.T) {
	if got := PromptText(nil); got != "" {
		t.Errorf("PromptText(nil) = %q, want empty", got)
	}
}

func TestSameLine(t *testing.T) {
	cases := []struct {
		y1, y2, height float64
		want           bool
	}{
		{100, 100.5, 12, true},
		{100, 112, 12, false},
		{100, 100, 0, true}, // zero height falls back to a small fixed tolerance
	}
	for _, c := range cases {
		if got := sameLine(c.y1, c.y2, c.height); got != c.want {
			t.Errorf("sameLine(%v, %v, %v) = %v, want %v", c.y1, c.y2, c.height, got, c.want)
		}
	}
}

// TestExtractAnchorsRealBinary exercises the actual pdftotext -bbox call
// against a hand-built PDF, since parseBBox alone cannot catch a flag or
// output-format mismatch with the real tool.
func TestExtractAnchorsRealBinary(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not on PATH")
	}
	pdfPath := writeMinimalPDF(t, t.TempDir(), "Anchor Test")

	anchors, err := ExtractAnchors(pdfPath, 1, 10*time.Second)
	if err != nil {
		t.Fatalf("ExtractAnchors: %v", err)
	}
	if len(anchors) == 0 {
		t.Fatal("expected at least one anchor word from the fixture PDF")
	}
	got := PromptText(anchors)
	if !strings.Contains(got, "Anchor") {
		t.Errorf("PromptText(ExtractAnchors(...)) = %q, want it to contain the fixture text", got)
	}
}

func TestExtractAnchorsOutOfRangePage(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not on PATH")
	}
	pdfPath := writeMinimalPDF(t, t.TempDir(), "One page only")

	// pdftotext itself rejects a page range past the document's end (exit
	// 99), so this is an error case, not an empty-result case — the caller
	// (Convert) only ever requests pages RenderPages actually produced.
	if _, err := ExtractAnchors(pdfPath, 2, 10*time.Second); err == nil {
		t.Fatal("expected an error requesting a page past the end of the document")
	}
}
