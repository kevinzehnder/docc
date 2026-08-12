package ingest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
