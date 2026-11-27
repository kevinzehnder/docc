package sema

import (
	"testing"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
)

func TestLeadingZeroListWarns(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"Antritt:\n\n01. April 2027\n", 1},     // a date read as a list
		{"- Antritt\n\n  05) Mai\n", 1},         // nested, other delimiter
		{"1. Erstens\n2. Zweitens\n", 0},        // an ordinary list
		{"01\\. April 2027\n", 0},               // escaped: a paragraph
		{"Am 01. April 2027 geht es los.\n", 0}, // not at the line start
	}
	for _, c := range cases {
		f, _ := parse.Parse("t.md", []byte(c.src))
		var ds diag.List
		checkLeadingZeroLists(f, &ds)
		if len(ds) != c.want {
			t.Errorf("%q: %d diagnostics, want %d: %v", c.src, len(ds), c.want, codes(ds))
		}
		for _, d := range ds {
			if d.Code != "DOC044" || d.Severity != diag.Warning {
				t.Errorf("%q: got %s %v, want a DOC044 warning", c.src, d.Code, d.Severity)
			}
		}
	}
}
