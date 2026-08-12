package ingest

import (
	"strings"
	"testing"
)

func TestBuildPromptStripsRandziffernInstruction(t *testing.T) {
	p, err := BuildPrompt("")
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(p, "Randziffern") {
		t.Error("expected the default prompt to mention stripping Randziffern")
	}
	if !strings.Contains(p, "===RZ===") {
		t.Error("expected the default prompt to request the RZ section")
	}
}

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

func TestBuildPlainPromptHasNoRzSection(t *testing.T) {
	p, err := BuildPlainPrompt("")
	if err != nil {
		t.Fatalf("BuildPlainPrompt: %v", err)
	}
	if strings.Contains(p, "===RZ===") || strings.Contains(p, "Randziffern") {
		t.Error("plain prompt should not mention Randziffer stripping or the RZ section")
	}
}

func TestBuildPlainPromptIncludesAnchorText(t *testing.T) {
	p, err := BuildPlainPrompt("Hello World")
	if err != nil {
		t.Fatalf("BuildPlainPrompt: %v", err)
	}
	if !strings.Contains(p, "Hello World") {
		t.Error("expected the anchor text to appear in the plain prompt too")
	}
}
