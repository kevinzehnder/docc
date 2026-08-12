package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommand(t *testing.T) {
	root := t.TempDir()
	if got := run([]string{"init", root}); got != 0 {
		t.Fatalf("run(init) = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".docc", "schemas", "letter.yaml")); err != nil {
		t.Fatalf("starter letter schema: %v", err)
	}
}

func TestParsePageRange(t *testing.T) {
	cases := []struct {
		in          string
		first, last int
		wantErr     bool
	}{
		{in: "", first: 0, last: 0},
		{in: "5", first: 5, last: 5},
		{in: "3-7", first: 3, last: 7},
		{in: " 3 - 7 ", first: 3, last: 7},
		{in: "0", wantErr: true},
		{in: "7-3", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "3-", wantErr: true},
	}
	for _, c := range cases {
		first, last, err := parsePageRange(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePageRange(%q): expected an error, got first=%d last=%d", c.in, first, last)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePageRange(%q): %v", c.in, err)
			continue
		}
		if first != c.first || last != c.last {
			t.Errorf("parsePageRange(%q) = (%d, %d), want (%d, %d)", c.in, first, last, c.first, c.last)
		}
	}
}
