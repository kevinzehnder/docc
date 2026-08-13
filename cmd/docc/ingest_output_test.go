package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/ingest"
)

func TestIngestOutputPath(t *testing.T) {
	if got := ingestOutputPath("a/scan.pdf", ""); got != "a/scan.md" {
		t.Errorf("ingestOutputPath = %q, want a/scan.md", got)
	}
	if got := ingestOutputPath("a/scan.pdf", "out.md"); got != "out.md" {
		t.Errorf("--output should win, got %q", got)
	}
}

func TestCheckIngestOutput(t *testing.T) {
	dir := t.TempDir()

	// Nothing there yet: nothing to protect.
	fresh := filepath.Join(dir, "new.md")
	if err := checkIngestOutput(fresh, false); err != nil {
		t.Errorf("checkIngestOutput on a missing file: %v", err)
	}

	// Ingest's own untouched output — re-running a conversion is routine.
	generated := filepath.Join(dir, "gen.md")
	writeTestFile(t, generated, ingest.Assemble(
		[]ingest.PageResult{{Index: 1, Markdown: "text"}},
		ingest.AssembleOptions{SourceFile: "x.pdf"},
	))
	if err := checkIngestOutput(generated, false); err != nil {
		t.Errorf("re-running over ingest's own draft should be allowed: %v", err)
	}

	// A file somebody has worked on: the banner is gone.
	edited := filepath.Join(dir, "edited.md")
	writeTestFile(t, edited, "---\ndocc: 1\ndocument_type: legal\n---\n\n# Klage\n\nHand-written.\n")
	err := checkIngestOutput(edited, false)
	if err == nil {
		t.Fatal("overwriting an edited document must be refused — the edits are unrecoverable")
	}
	for _, want := range []string{"--output", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %s as the way forward", err, want)
		}
	}
	if err := checkIngestOutput(edited, true); err != nil {
		t.Errorf("--force must override the guard: %v", err)
	}

	// A partial draft is ingest's own output, but it holds pages a resume
	// converts a different range for and will never reproduce.
	partial := filepath.Join(dir, "partial.md")
	writeTestFile(t, partial, ingest.Assemble(
		[]ingest.PageResult{{Index: 1, Markdown: "text"}},
		ingest.AssembleOptions{
			SourceFile: "x.pdf",
			Incomplete: &ingest.Incomplete{Completed: 1, Total: 9, NextPage: 2, LastPage: 9, Reason: "interrupted"},
		},
	))
	err = checkIngestOutput(partial, false)
	if err == nil {
		t.Fatal("overwriting a partial draft must be refused — those pages are not reproducible")
	}
	if !strings.Contains(err.Error(), "partial draft") {
		t.Errorf("error %q should say what makes this file worth protecting", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Ingest has to work with no schema at all — it is a transcription tool first.
// Where a schema does exist, it is the only thing that knows whether the
// document being produced will generate its own paragraph numbers.
func TestRandzifferPolicy(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "schemas")

	tests := []struct {
		name      string
		docType   string
		schemaDir string
		wantStrip bool
		noteHas   string
	}{
		{
			name: "no type means no schema to consult, so transcribe faithfully",
		},
		{
			name:      "a schema that generates paragraph numbers drops the source's own",
			docType:   "legal",
			schemaDir: dir,
			wantStrip: true,
			noteHas:   "render time",
		},
		{
			name:      "a reference schema keeps them, because they are the citation key",
			docType:   "legal_reference",
			schemaDir: dir,
		},
		{
			name:      "an unknown type is not fatal — it keeps them and says why",
			docType:   "nonesuch",
			schemaDir: dir,
			noteHas:   "keeping",
		},
		{
			name:      "a missing schema directory is not fatal either",
			docType:   "legal",
			schemaDir: filepath.Join(t.TempDir(), "absent"),
			noteHas:   "keeping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strip, note := randzifferPolicy(tt.docType, tt.schemaDir, ".")
			if strip != tt.wantStrip {
				t.Errorf("strip = %v, want %v (note: %q)", strip, tt.wantStrip, note)
			}
			if tt.noteHas != "" && !strings.Contains(note, tt.noteHas) {
				t.Errorf("note = %q, want it to mention %q", note, tt.noteHas)
			}
			if tt.noteHas == "" && note != "" {
				t.Errorf("unexpected note %q", note)
			}
		})
	}
}
