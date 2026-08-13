package ingest

import "testing"

func TestNewPageResultRendersItsNodes(t *testing.T) {
	got := NewPageResult(4, []Node{
		{Kind: KindHeading, Level: 1, Text: "Heading"},
		{Kind: KindPara, Text: "Some transcribed body text."},
	})
	if got.Index != 4 {
		t.Errorf("Index = %d, want 4", got.Index)
	}
	if got.Markdown != "# Heading\n\nSome transcribed body text." {
		t.Errorf("Markdown = %q", got.Markdown)
	}
	if len(got.Nodes) != 2 {
		t.Errorf("Nodes = %d, want the 2 it was given — the elements outlive the rendering", len(got.Nodes))
	}
	if got.LowConfidence {
		t.Error("NewPageResult itself never sets LowConfidence — that is Convert's job, based on anchor availability")
	}
}

// A page that produced nothing renders to nothing, rather than to a blank line
// Assemble would go on to join with two more.
func TestNewPageResultEmpty(t *testing.T) {
	if got := NewPageResult(1, nil); got.Markdown != "" {
		t.Errorf("Markdown = %q, want empty", got.Markdown)
	}
}

// The chat backend's page crosses the seam whole and comes back byte for byte:
// until a pass needs to see inside it, parsing it would only be a chance to
// change it.
func TestRawNodeRoundTripsUnchanged(t *testing.T) {
	const page = "# Replik\n\nAd. KA Rz 6:\n\n55 Die Beklagte bestreitet.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |"
	if got := Render([]Node{{Kind: KindRaw, Text: page}}); got != page {
		t.Errorf("Render of a raw page changed it:\n got %q\nwant %q", got, page)
	}
}
