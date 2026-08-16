package sema

import (
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/diag"
)

// The dossier check: the author writes every occurrence, and the compiler
// refuses to let the documents drift apart. This is the case a templating
// system would have prevented by construction — and the one that actually
// happened, a Firma spelled two ways across a six-document founding.
func TestCrossFileDisagreements(t *testing.T) {
	occ := func(typ, val, file string, line int) SpanOccurrence {
		return SpanOccurrence{Type: typ, Value: val, File: file, Pos: diag.Position{Line: line, Col: 1}}
	}

	t.Run("reports a value that differs between files", func(t *testing.T) {
		ds := CrossFileDisagreements([]SpanOccurrence{
			occ("firma", "Motherstuhl", "urkunde.md", 32),
			occ("firma", "Mutterstuhl", "stampa.md", 13),
		})
		if len(ds) != 1 {
			t.Fatalf("want 1 diagnostic, got %d: %+v", len(ds), ds)
		}
		if ds[0].Code != "DOC029" {
			t.Errorf("want DOC029, got %s", ds[0].Code)
		}
		for _, want := range []string{"Mutterstuhl", "Motherstuhl", "urkunde.md"} {
			if !strings.Contains(ds[0].Message, want) {
				t.Errorf("message missing %q: %s", want, ds[0].Message)
			}
		}
	})

	t.Run("agreement is silent", func(t *testing.T) {
		ds := CrossFileDisagreements([]SpanOccurrence{
			occ("firma", "Motherstuhl", "urkunde.md", 32),
			occ("firma", "Motherstuhl", "stampa.md", 13),
			occ("firma", "Motherstuhl", "statuten.md", 18),
		})
		if len(ds) != 0 {
			t.Errorf("want silence, got %+v", ds)
		}
	})

	// A Firma written five times in one deed is one mistake, not five.
	t.Run("one report per file and type", func(t *testing.T) {
		ds := CrossFileDisagreements([]SpanOccurrence{
			occ("firma", "Motherstuhl", "urkunde.md", 32),
			occ("firma", "Mutterstuhl", "stampa.md", 13),
			occ("firma", "Mutterstuhl", "stampa.md", 40),
			occ("firma", "Mutterstuhl", "stampa.md", 51),
		})
		if len(ds) != 1 {
			t.Errorf("want 1 diagnostic for the file, got %d: %+v", len(ds), ds)
		}
	})

	// Repetition within one file is `spans_agree`'s business, not this pass's.
	t.Run("ignores disagreement inside a single file", func(t *testing.T) {
		ds := CrossFileDisagreements([]SpanOccurrence{
			occ("firma", "Motherstuhl", "urkunde.md", 32),
			occ("firma", "Mutterstuhl", "urkunde.md", 55),
		})
		if len(ds) != 0 {
			t.Errorf("want silence, got %+v", ds)
		}
	})
}
