package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lines splits a raw-string fixture. The leading newline is kept as line 0 so
// the indices in each case count from the first line of visible text.
func lines(s string) []string { return strings.Split(s, "\n") }

func TestEvidenceRegions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []Region
	}{
		{
			name: "a block runs to the next numbered paragraph",
			src: `
[Rz 3] Eine ausdrückliche Anerkennung war ausgeblieben.

BO: Christian Magnani, Mitglied der Geschäftsleitung Klägerin Parteibefragung/
Beweisaussage

Alain Luchsinger Zeugnis

[Rz 4] Die nun erstmals erfolgte Anerkennung wird zur Kenntnis genommen.
`,
			// The BO line through "Alain Luchsinger Zeugnis": an offer of
			// proof runs on across a blank line, and only the next numbered
			// paragraph ends it. The blank line before [Rz 4] is trimmed off.
			want: []Region{{Start: 3, End: 7}},
		},
		{
			name: "a heading ends a block",
			src: `
BO: Mietvertrag vom 03./07.09.2001 Beilage 3

## B. SACHVERHALT
`,
			want: []Region{{Start: 1, End: 2}},
		},
		{
			name: "the model's bold label is recognised",
			src: `
**BO:** Christian Magnani Parteibefragung

[Rz 5] Weiter ist zu sagen.
`,
			want: []Region{{Start: 1, End: 2}},
		},
		{
			name: "the spelled out label is recognised",
			src: `
Beweis:
- Wertschriftenverzeichnis der Erblasser KB 25

[Rz 9] Weiter.
`,
			want: []Region{{Start: 1, End: 3}},
		},
		{
			name: "two blocks are found separately",
			src: `
BO: Erstes Dokument Beilage 1

[Rz 2] Zwischentext.

BO: Zweites Dokument Beilage 2

[Rz 3] Weiter.
`,
			want: []Region{{Start: 1, End: 2}, {Start: 5, End: 6}},
		},
		{
			// Structuring is re-runnable; a second pass must not nest fences.
			name: "an already structured block is skipped",
			src: `
::: beweis

- [Beilage 3] Mietvertrag vom 03./07.09.2001
:::

[Rz 4] Weiter.
`,
			want: nil,
		},
		{
			name: "a document with no offers of proof yields none",
			src: `
[Rz 1] Erste Erwägung.

[Rz 2] Zweite Erwägung.
`,
			want: nil,
		},
		{
			// "Beweisaussage" as a continuation line is not a new block, and
			// the word appearing inside a sentence is not one either.
			name: "the word inside running prose does not open a block",
			src: `
[Rz 1] Die Beweislast liegt bei der Klägerin.
`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvidenceRegions(lines(tt.src))
			if len(got) != len(tt.want) {
				t.Fatalf("got %d regions %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("region %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// structureServer answers every structuring request with the same body.
func structureServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(dataFrame(reply, "stop") + "\n\ndata: [DONE]\n\n"))
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStructureRewritesBlocksAndLeavesProseAlone(t *testing.T) {
	srv := structureServer(t, "- [Beilage 7] Schreiben der Beklagten vom 20.09.2015\n- [Parteibefragung] Christian Magnani")
	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}

	src := strings.Join([]string{
		"[Rz 17] Der Ausbau erfolgte auf Kosten der Klägerin.",
		"",
		"BO: Schreiben der Beklagten vom 20.09.2015 Beilage 7",
		"Christian Magnani Parteibefragung",
		"",
		"[Rz 18] Ganz im Gegenteil.",
	}, "\n")

	got, notes, err := Structure(context.Background(), c, src, StructureOptions{})
	if err != nil {
		t.Fatalf("Structure: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("unexpected notes: %+v", notes)
	}
	for _, want := range []string{
		"::: beweis",
		"- [Beilage 7] Schreiben der Beklagten vom 20.09.2015",
		"- [Parteibefragung] Christian Magnani",
		":::",
		"[Rz 17] Der Ausbau erfolgte auf Kosten der Klägerin.",
		"[Rz 18] Ganz im Gegenteil.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "BO: Schreiben") {
		t.Errorf("the transcribed block should have been replaced:\n%s", got)
	}
}

// A block the model does not return in the required shape is worth less than
// the transcription, which at least still says what the page said.
func TestStructureKeepsTheTranscriptionWhenTheAnswerIsUnusable(t *testing.T) {
	srv := structureServer(t, "Here are the items you asked for:\n- Schreiben der Beklagten")
	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}

	src := "[Rz 1] Text.\n\nBO: Schreiben der Beklagten vom 20.09.2015 Beilage 7\n\n[Rz 2] Weiter."
	got, notes, err := Structure(context.Background(), c, src, StructureOptions{})
	if err != nil {
		t.Fatalf("Structure: %v", err)
	}
	if !strings.Contains(got, "BO: Schreiben der Beklagten") {
		t.Errorf("the original block must survive an unusable answer:\n%s", got)
	}
	if strings.Contains(got, "::: beweis") {
		t.Errorf("nothing should have been structured:\n%s", got)
	}
	if len(notes) != 1 || notes[0].Line != 3 {
		t.Fatalf("notes = %+v, want one note pointing at line 3", notes)
	}
}

func TestStructureIsIdempotent(t *testing.T) {
	srv := structureServer(t, "- [Beilage 3] Mietvertrag vom 03./07.09.2001")
	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}

	src := "[Rz 1] Text.\n\nBO: Mietvertrag vom 03./07.09.2001 Beilage 3\n\n[Rz 2] Weiter."
	once, _, err := Structure(context.Background(), c, src, StructureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	twice, _, err := Structure(context.Background(), c, once, StructureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Errorf("a second pass changed the document:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestStructureLeavesADocumentWithoutEvidenceUntouched(t *testing.T) {
	srv := structureServer(t, "- [Beilage 1] unused")
	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}

	src := "[Rz 1] Erste Erwägung.\n\n[Rz 2] Zweite Erwägung.\n"
	got, notes, err := Structure(context.Background(), c, src, StructureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Errorf("document changed:\n%s", got)
	}
	if notes != nil {
		t.Errorf("unexpected notes: %+v", notes)
	}
}

// The lead label names the block, not the evidence. Left in place it ends up
// inside an item's description, where it reads as part of the document title.
func TestStructureStripsTheLeadLabelBeforeAsking(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 && len(req.Messages[0].Content) > 0 {
			got = req.Messages[0].Content[0].Text
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(dataFrame("- [Beilage 8] Nachtrag Nr. 3 vom 03.11.2016", "stop") + "\n\ndata: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, HTTPClient: srv.Client()}
	src := "[Rz 1] Text.\n\nBO: Nachtrag Nr. 3 vom 03.11.2016 Beilage 8\n\n[Rz 2] Weiter."
	if _, _, err := Structure(context.Background(), c, src, StructureOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "BO:") {
		t.Errorf("the prompt still carries the lead label:\n%s", got)
	}
	if !strings.Contains(got, "Nachtrag Nr. 3 vom 03.11.2016 Beilage 8") {
		t.Errorf("the prompt lost the block's content:\n%s", got)
	}
}
