package ingest

import (
	"strings"
	"testing"
)

func TestBuildPromptIncludesAnchorText(t *testing.T) {
	p, err := BuildPrompt("Hello World")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(p, "Hello World") {
		t.Error("expected the anchor text to appear in the prompt")
	}
}

func TestBuildPromptOmitsAnchorSectionWhenEmpty(t *testing.T) {
	p, err := BuildPrompt("")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if strings.Contains(p, "ground truth") {
		t.Error("did not expect the anchor-text section without anchor text")
	}
}

func TestBuildPromptPreservesSpecialCharacters(t *testing.T) {
	p, err := BuildPrompt("")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(p, "ä") {
		t.Error("expected the prompt to instruct preserving special characters like umlauts")
	}
}

func TestBuildPromptSkipsRunningHeadersFooters(t *testing.T) {
	p, err := BuildPrompt("")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(p, "running footer") || !strings.Contains(p, "page number") {
		t.Error("expected the prompt to instruct skipping running headers/footers and standalone page numbers")
	}
	// The exclusion has to be scoped by position, or it swallows the
	// Randziffer in the left margin, which is also "a number by itself".
	if !strings.Contains(p, "TOP or BOTTOM EDGE") {
		t.Error("expected the header/footer rule to be scoped to the page edges")
	}
}

func TestBuildPromptMarksRandziffernButNotCitations(t *testing.T) {
	p, err := BuildPrompt("")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	// The prompt asks only for the digits: turning them into [Rz N] is done
	// in code, because the model marks them only 63-73% of the time while
	// transcribing them almost always. See rzNormalizer.
	if !strings.Contains(p, "LEFT MARGIN") || !strings.Contains(p, "Randziffer") {
		t.Error("expected the prompt to identify a Randziffer by its position in the left margin")
	}
	if !strings.Contains(p, "vgl. Rz. 25") {
		t.Error("expected the prompt to give an example distinguishing a citation from a paragraph's own leading number")
	}
	if !strings.Contains(p, "rule 1 wins over rule 2") {
		t.Error("expected the prompt to resolve the overlap between the margin rule and the page-edge rule explicitly")
	}
}
