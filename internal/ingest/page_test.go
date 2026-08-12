package ingest

import "testing"

func TestParsePageResponseWellFormed(t *testing.T) {
	raw := "===MARKDOWN===\n# Klage\n\nErste Erwägung.\n===RZ===\n```json\n{\"randziffern\": [1, 2]}\n```\n"

	got := ParsePageResponse(3, raw)
	if got.Index != 3 {
		t.Errorf("Index = %d, want 3", got.Index)
	}
	if got.Markdown != "# Klage\n\nErste Erwägung." {
		t.Errorf("Markdown = %q", got.Markdown)
	}
	if len(got.RzSeq) != 2 || got.RzSeq[0] != 1 || got.RzSeq[1] != 2 {
		t.Errorf("RzSeq = %v, want [1 2]", got.RzSeq)
	}
	if got.LowConfidence {
		t.Errorf("LowConfidence = true for a well-formed response")
	}
}

func TestParsePageResponseMissingMarkers(t *testing.T) {
	got := ParsePageResponse(1, "just some prose the model wrote instead of following the format")
	if !got.LowConfidence {
		t.Errorf("expected LowConfidence for a response with no markers")
	}
	if got.Markdown == "" {
		t.Errorf("expected the raw response to still be captured as markdown")
	}
}

func TestParsePageResponseBadRzJSON(t *testing.T) {
	raw := "===MARKDOWN===\nSome text.\n===RZ===\nnot json at all\n"
	got := ParsePageResponse(1, raw)
	if !got.LowConfidence {
		t.Errorf("expected LowConfidence when the RZ section is not parseable JSON")
	}
	if got.Markdown != "Some text." {
		t.Errorf("Markdown = %q, want the body preserved despite the bad RZ section", got.Markdown)
	}
}

func TestParsePageResponseEmptyRzSequence(t *testing.T) {
	raw := "===MARKDOWN===\nNo numbered paragraphs on this page.\n===RZ===\n```json\n{\"randziffern\": []}\n```\n"
	got := ParsePageResponse(2, raw)
	if got.LowConfidence {
		t.Errorf("an empty but well-formed RZ list should not be low-confidence")
	}
	if len(got.RzSeq) != 0 {
		t.Errorf("RzSeq = %v, want empty", got.RzSeq)
	}
}
