package ingest

import "testing"

func TestParsePageResponse(t *testing.T) {
	got := ParsePageResponse(4, "  # Heading\n\nSome transcribed body text.\n  ")
	if got.Index != 4 {
		t.Errorf("Index = %d, want 4", got.Index)
	}
	if got.Markdown != "# Heading\n\nSome transcribed body text." {
		t.Errorf("Markdown = %q", got.Markdown)
	}
	if got.LowConfidence {
		t.Error("ParsePageResponse itself never sets LowConfidence — that's Convert's job, based on anchor availability")
	}
}

func TestParsePageResponseEmpty(t *testing.T) {
	got := ParsePageResponse(1, "   \n  ")
	if got.Markdown != "" {
		t.Errorf("Markdown = %q, want empty after trimming", got.Markdown)
	}
}
