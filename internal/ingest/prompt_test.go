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
	if !strings.Contains(p, "running footers") || !strings.Contains(p, "page number") {
		t.Error("expected the prompt to instruct skipping running headers/footers and standalone page numbers")
	}
}

func TestBuildPromptMarksRandziffernButNotCitations(t *testing.T) {
	p, err := BuildPrompt("")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(p, "[Rz N]") {
		t.Error("expected the prompt to instruct marking marginal paragraph numbers as [Rz N]")
	}
	if !strings.Contains(p, "vgl. Rz. 25") {
		t.Error("expected the prompt to give an example distinguishing a citation from a paragraph's own leading number")
	}
}
