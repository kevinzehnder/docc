package ingest

import "testing"

// goldenBlocks is one page exercising every path through block assembly at
// once: page furniture, a container reported alongside its children, a gutter
// column, marginal prose that is not a number, an image with no text, and one
// block of each type that renders to something other than a bare paragraph.
//
// It exists as a single fixture rather than as more cases in blocks_test.go
// because those assert behaviours — "the header is dropped" — and this asserts
// the exact bytes. The two catch different things: a refactor that preserves
// every behaviour can still reorder two blocks or lose a blank line, and this
// is the test that says so.
func goldenBlocks() []Block {
	return []Block{
		block("header", 0.131, 0.027, 0.870, 0.060, "Muster & Partner AG"),
		block("doc_title", 0.131, 0.080, 0.870, 0.110, "Replik\nin Sachen"),
		block("title", 0.131, 0.120, 0.400, 0.140, "I.  FORMELLES"),
		block("text", 0.085, 0.150, 0.102, 0.160, "55"),
		block("text", 0.131, 0.150, 0.870, 0.200, "Die vorliegende Eingabe erfolgt fristgerecht."),
		block("aside_text", 0.085, 0.210, 0.102, 0.260, "siehe Beilage 4"),
		block("list_item", 0.242, 0.270, 0.698, 0.300, "Die Beklagte sei\nzu verpflichten"),
		block("list_item", 0.242, 0.310, 0.683, 0.340, "unter Kostenfolgen"),
		block("list", 0.242, 0.270, 0.698, 0.340, "Die Beklagte sei zu verpflichten unter Kostenfolgen"),
		block("text", 0.086, 0.350, 0.102, 0.360, "56"),
		block("text", 0.131, 0.350, 0.870, 0.400, "Die Ausführungen der Beklagten werden bestritten."),
		block("equation", 0.131, 0.410, 0.870, 0.440, "a = b"),
		block("table", 0.131, 0.450, 0.870, 0.520, "<table><tr><td>x</td></tr></table>"),
		block("code", 0.131, 0.530, 0.870, 0.560, "go build ./..."),
		block("image", 0.131, 0.570, 0.870, 0.700, ""),
		block("text", 0.131, 0.710, 0.870, 0.740, ""),
		block("page_number", 0.480, 0.959, 0.520, 0.972, "- 30 -"),
		block("footer", 0.131, 0.975, 0.870, 0.985, "Replik vom 3. Mai"),
	}
}

// TestAssembleBlocksGolden pins the exact markdown one page assembles to.
func TestAssembleBlocksGolden(t *testing.T) {
	const want = "# Replik in Sachen\n\n" +
		"## I. FORMELLES\n\n" +
		"[Rz 55] Die vorliegende Eingabe erfolgt fristgerecht.\n\n" +
		"siehe Beilage 4\n\n" +
		"- Die Beklagte sei zu verpflichten\n\n" +
		"- unter Kostenfolgen\n\n" +
		"[Rz 56] Die Ausführungen der Beklagten werden bestritten.\n\n" +
		"$$\na = b\n$$\n\n" +
		"<table><tr><td>x</td></tr></table>\n\n" +
		"```\ngo build ./...\n```\n\n" +
		"<!-- image on the page here, not transcribed -->"

	if got := AssembleBlocks(goldenBlocks()); got != want {
		t.Errorf("AssembleBlocks:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
