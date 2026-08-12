package ingest

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// AnchorText is one word of a PDF's born-digital text layer, positioned on
// the page. It exists to be injected into the VLM prompt as reference ground
// truth — olmOCR's "document-anchoring" technique, which measurably reduces
// hallucination versus prompting from the page image alone — not to be
// rendered itself.
type AnchorText struct {
	Text                string
	X, Y, Width, Height float64
}

// ExtractAnchors reads the born-digital text layer of one PDF page via
// pdftotext -bbox. A PDF with no text layer (a scan) or an image input
// produces no anchors and no error — the caller treats that as a
// lower-confidence page, not a failure.
func ExtractAnchors(pdfPath string, page int, timeout time.Duration) ([]AnchorText, error) {
	binary, err := findBinary("pdftotext")
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	p := strconv.Itoa(page)
	// binary comes from exec.LookPath and the paths are the caller's own
	// arguments; there is no shell involved, so no interpolation to escape.
	cmd := exec.CommandContext(ctx, binary, //nolint:gosec // fixed argv, no shell
		"-bbox", "-f", p, "-l", p, pdfPath, "-",
	)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("pdftotext timed out after %s", timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("pdftotext: %w", err)
	}
	return parseBBox(out)
}

type bboxWord struct {
	XMin float64 `xml:"xMin,attr"`
	YMin float64 `xml:"yMin,attr"`
	XMax float64 `xml:"xMax,attr"`
	YMax float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

// parseBBox scans for <word> elements by token rather than unmarshalling a
// fixed struct shape, because poppler wraps them differently across
// versions — sometimes a bare <doc><page>, sometimes a full XHTML shell
// (<html><body><doc><page>...). Only the <word> elements themselves are
// stable.
func parseBBox(b []byte) ([]AnchorText, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var out []AnchorText
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse pdftotext -bbox output: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "word" {
			continue
		}
		var w bboxWord
		if err := dec.DecodeElement(&w, &se); err != nil {
			return nil, fmt.Errorf("parse pdftotext -bbox output: %w", err)
		}
		text := strings.TrimSpace(w.Text)
		if text == "" {
			continue
		}
		out = append(out, AnchorText{
			Text: text, X: w.XMin, Y: w.YMin,
			Width: w.XMax - w.XMin, Height: w.YMax - w.YMin,
		})
	}
	return out, nil
}

// PromptText reconstructs reading-order lines for prompt injection. pdftotext
// already emits words in reading order, so this only needs to group
// consecutive words whose vertical position places them on the same line —
// it is a rough reconstruction meant to give the VLM ground-truth vocabulary
// and word order, not exact layout; the page image carries layout.
func PromptText(anchors []AnchorText) string {
	if len(anchors) == 0 {
		return ""
	}
	var lines []string
	var cur []string
	curY := anchors[0].Y
	for _, a := range anchors {
		if len(cur) > 0 && !sameLine(curY, a.Y, a.Height) {
			lines = append(lines, strings.Join(cur, " "))
			cur = nil
		}
		cur = append(cur, a.Text)
		curY = a.Y
	}
	if len(cur) > 0 {
		lines = append(lines, strings.Join(cur, " "))
	}
	return strings.Join(lines, "\n")
}

func sameLine(y1, y2, height float64) bool {
	tol := height / 2
	if tol <= 0 {
		tol = 2
	}
	diff := y1 - y2
	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}
