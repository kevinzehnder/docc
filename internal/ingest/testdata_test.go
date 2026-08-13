package ingest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMinimalPDF builds a hand-rolled single-page PDF containing text, with
// correctly computed xref offsets, so raster.go and anchor.go's real
// pdftoppm/pdftotext calls have something genuine to operate on without the
// test depending on LibreOffice or any other docc-external build step.
func writeMinimalPDF(t *testing.T, dir, text string) string {
	t.Helper()

	var buf bytes.Buffer
	offsets := make([]int, 6) // index 0 unused, objects are 1..5

	buf.WriteString("%PDF-1.4\n")

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	offsets[3] = buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] " +
		"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n")

	offsets[4] = buf.Len()
	buf.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	offsets[5] = buf.Len()
	content := fmt.Sprintf("BT /F1 18 Tf 20 100 Td (%s) Tj ET", text)
	fmt.Fprintf(&buf, "5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)

	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 6\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	buf.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF", xrefOffset)

	path := filepath.Join(dir, "fixture.pdf")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeMultiPagePDF builds an n-page PDF the same way, each page carrying its
// own number. Convert's partial-save behaviour only exists across pages, so
// testing it needs a document that has more than one.
//
// Objects are laid out as catalog, page tree, font, then a page and a content
// stream per page.
func writeMultiPagePDF(t *testing.T, dir string, n int) string {
	t.Helper()

	count := 3 + 2*n
	var buf bytes.Buffer
	offsets := make([]int, count+1) // index 0 unused

	pageObj := func(i int) int { return 4 + 2*i }
	contentObj := func(i int) int { return 5 + 2*i }

	buf.WriteString("%PDF-1.4\n")

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	kids := make([]string, 0, n)
	for i := range n {
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj(i)))
	}
	offsets[2] = buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), n)

	offsets[3] = buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	for i := range n {
		offsets[pageObj(i)] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] "+
			"/Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>\nendobj\n", pageObj(i), contentObj(i))

		offsets[contentObj(i)] = buf.Len()
		content := fmt.Sprintf("BT /F1 18 Tf 20 100 Td (Page %d) Tj ET", i+1)
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", contentObj(i), len(content), content)
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", count+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\n", count+1)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF", xrefOffset)

	path := filepath.Join(dir, "multipage.pdf")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
