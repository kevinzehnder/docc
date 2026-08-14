package ingest

import (
	"strings"
	"testing"
)

// block is a terse constructor for the tests below: the geometry matters and
// the rest is noise.
func block(kind string, x0, y0, x1, y1 float64, text string) Block {
	return Block{Type: kind, Box: BBox{X0: x0, Y0: y0, X1: x1, Y1: y1}, Text: text}
}

// The running header and the page number are on the page and not in the
// document. The chat backend asks a model to leave them out and gets it right
// about two thirds of the time; here it is a type check.
func TestAssembleBlocksDropsPageFurniture(t *testing.T) {
	md := Render(Nodes([]Block{
		block("header", 0.887, 0.027, 0.936, 0.060, "Muster & Partner AG"),
		block("text", 0.131, 0.193, 0.870, 0.532, "Der Kläger bestreitet."),
		block("page_number", 0.871, 0.959, 0.895, 0.972, "- 30 -"),
		block("footer", 0.131, 0.975, 0.870, 0.985, "Replik vom 3. Mai"),
	}))

	if !strings.Contains(md, "Der Kläger bestreitet.") {
		t.Fatalf("body text missing:\n%s", md)
	}
	for _, gone := range []string{"Muster & Partner", "- 30 -", "Replik vom"} {
		if strings.Contains(md, gone) {
			t.Errorf("furniture %q survived:\n%s", gone, md)
		}
	}
}

// A heading is a heading because the layout pass said so, not because the model
// felt like typing a "#" on this page and not the next.
func TestAssembleBlocksMarksHeadings(t *testing.T) {
	md := Render(Nodes([]Block{
		block("title", 0.131, 0.175, 0.218, 0.189, "Zu Rz. 80"),
		block("text", 0.131, 0.193, 0.870, 0.532, "Richtig ist, dass ..."),
	}))
	if !strings.HasPrefix(md, "## Zu Rz. 80\n\n") {
		t.Fatalf("heading not marked:\n%s", md)
	}
}

// A number alone in the gutter is a marginal paragraph number and belongs to
// the paragraph it sits beside.
func TestAssembleBlocksMarksGutterNumbers(t *testing.T) {
	md := Render(Nodes([]Block{
		block("title", 0.131, 0.175, 0.218, 0.189, "Zu Rz. 80"),
		block("text", 0.085, 0.193, 0.102, 0.203, "55"),
		block("text", 0.131, 0.193, 0.870, 0.532, "Richtig ist, dass ..."),
		block("text", 0.086, 0.744, 0.102, 0.753, "56"),
		block("text", 0.131, 0.745, 0.870, 0.836, "Die Ausführungen ..."),
	}))

	if !strings.Contains(md, "[Rz 55] Richtig ist") {
		t.Errorf("first Randziffer not attached:\n%s", md)
	}
	if !strings.Contains(md, "[Rz 56] Die Ausführungen") {
		t.Errorf("second Randziffer not attached:\n%s", md)
	}
	// The number must not also survive as a paragraph of its own.
	for _, line := range strings.Split(md, "\n") {
		if strings.TrimSpace(line) == "55" {
			t.Errorf("gutter number left as its own paragraph:\n%s", md)
		}
	}
}

// A gutter column recognized as one block carries every number on the page,
// and each must reach its own paragraph. Page 6 of a real Replik came back as
// a single aside_text "25\n26\n27\n28" spanning half the page; binding every
// number at the block's top attached 25 and silently dropped 26 through 28.
func TestAssembleBlocksSplitsMergedGutterColumn(t *testing.T) {
	md := Render(Nodes([]Block{
		block("aside_text", 0.165, 0.153, 0.184, 0.652, "25\n26\n27\n28"),
		block("text", 0.221, 0.150, 0.854, 0.293, "Damit hatte die Vermieterschaft ..."),
		block("text", 0.223, 0.303, 0.519, 0.318, "BO: Mietvertrag vom 03./07.09.2001"),
		block("text", 0.768, 0.303, 0.841, 0.317, "Beilage 3"),
		block("text", 0.222, 0.336, 0.852, 0.406, "Ein Zustimmungserfordernis gilt ..."),
		block("text", 0.223, 0.418, 0.518, 0.432, "BO: Mietvertrag vom 03./07.09.2001"),
		block("text", 0.768, 0.418, 0.841, 0.432, "Beilage 3"),
		block("text", 0.221, 0.450, 0.853, 0.593, "Angesprochen ist hier die Übertragung ..."),
		block("text", 0.223, 0.604, 0.518, 0.619, "BO: Mietvertrag vom 03./07.09.2001"),
		block("text", 0.768, 0.604, 0.841, 0.618, "Beilage 3"),
		block("text", 0.221, 0.636, 0.853, 0.887, "Gleichwohl kam die Klägerin ..."),
	}))

	for _, want := range []string{
		"[Rz 25] Damit hatte",
		"[Rz 26] Ein Zustimmungserfordernis",
		"[Rz 27] Angesprochen ist",
		"[Rz 28] Gleichwohl kam",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q:\n%s", want, md)
		}
	}
}

// An interpolated gutter position is fuzzy, and an offer of proof sits between
// paragraphs exactly where the fuzz lands. Page 16 of a real Replik attached
// [Rz 63] to "BO: Christian Magnani ..." because the interpolated y for 63
// fell a line short of its paragraph and the BO block's bottom caught it.
func TestAssembleBlocksMergedGutterSkipsEvidenceRows(t *testing.T) {
	md := Render(Nodes([]Block{
		block("text", 0.221, 0.149, 0.853, 0.310, "Noch am 8. Mai 2023 ..."),
		block("text", 0.221, 0.320, 0.841, 0.354, "BO: Christian Magnani, Mitglied der Geschäftsleitung Klägerin"),
		block("aside_text", 0.163, 0.373, 0.181, 0.846, "62\n63\n64\n65"),
		block("text", 0.220, 0.370, 0.853, 0.513, "Eine EBIT-Klausel war ..."),
		block("text", 0.221, 0.524, 0.841, 0.556, "BO: Christian Magnani, Mitglied der Geschäftsleitung Klägerin"),
		block("text", 0.282, 0.568, 0.838, 0.582, "E-Mail-Korrespondenz der Parteien vom 03./19.4.2024 Beilage 15"),
		block("text", 0.220, 0.601, 0.850, 0.655, "Die Behauptung einer Zerstörung ..."),
		block("text", 0.220, 0.671, 0.852, 0.814, "Tatsächlich dürfte die Situation ..."),
		block("text", 0.220, 0.832, 0.851, 0.903, "Die folgende umfangreiche Kritik ..."),
	}))

	for _, want := range []string{
		"[Rz 62] Eine EBIT-Klausel",
		"[Rz 63] Die Behauptung",
		"[Rz 64] Tatsächlich dürfte",
		"[Rz 65] Die folgende",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "[Rz 63] BO:") {
		t.Errorf("number attached to an offer of proof:\n%s", md)
	}
}

// A narrow block in the margin that says something other than a number is a
// marginal note, and throwing it away would lose document text.
func TestAssembleBlocksKeepsMarginalProse(t *testing.T) {
	md := Render(Nodes([]Block{
		block("aside_text", 0.085, 0.193, 0.102, 0.400, "siehe Beilage 4"),
		block("text", 0.131, 0.193, 0.870, 0.532, "Der Kläger bestreitet."),
	}))
	if !strings.Contains(md, "siehe Beilage 4") {
		t.Fatalf("marginal note dropped:\n%s", md)
	}
}

// The layout pass reports a container and its children both. Recognizing both
// transcribes the same lines twice, which turned a Rechtsbegehren with two
// prayers into four on the round-trip fixture.
func TestAssembleBlocksSkipsContainerBlocks(t *testing.T) {
	md := Render(Nodes([]Block{
		block("list_item", 0.242, 0.594, 0.698, 0.627, "Die Beklagte sei zu verpflichten"),
		block("list_item", 0.242, 0.631, 0.683, 0.665, "unter Kosten- und Entschädigungsfolgen"),
		// The container spans both, and holds no text of its own that is not
		// already in them.
		block("list", 0.242, 0.594, 0.698, 0.685, "Die Beklagte sei zu verpflichten unter Kosten- und Entschädigungsfolgen"),
	}))

	if got := strings.Count(md, "Die Beklagte sei zu verpflichten"); got != 1 {
		t.Errorf("text appears %d times, want 1:\n%s", got, md)
	}
	if got := strings.Count(md, "unter Kosten- und Entschädigungsfolgen"); got != 1 {
		t.Errorf("second item appears %d times, want 1:\n%s", got, md)
	}
}

func TestAssembleBlocksAnnouncesImages(t *testing.T) {
	md := Render(Nodes([]Block{
		block("image", 0.131, 0.193, 0.870, 0.532, ""),
		block("text", 0.131, 0.600, 0.870, 0.700, "Abbildung 1 zeigt ..."),
	}))
	if !strings.Contains(md, "<!-- image on the page here, not transcribed -->") {
		t.Fatalf("image not announced:\n%s", md)
	}
}

// The body's left edge is read off the page rather than assumed, so that an
// indented quotation does not turn every paragraph after it into a margin note.
func TestBodyLeftIgnoresOneIndentedBlock(t *testing.T) {
	blocks := []Block{
		block("text", 0.131, 0.10, 0.870, 0.20, "a"),
		block("text", 0.200, 0.21, 0.800, 0.30, "indented quotation"),
		block("text", 0.131, 0.31, 0.870, 0.40, "b"),
	}
	if got := bodyLeft(blocks); got != 0.131 {
		t.Errorf("bodyLeft = %v, want 0.131", got)
	}
}
