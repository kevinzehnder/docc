package ingest

import (
	"testing"

	"github.com/kevinzehnder/docc/internal/testpdf"
)

func writeMinimalPDF(t *testing.T, dir, text string) string {
	t.Helper()
	return testpdf.Write(t, dir, text)
}

func writeMultiPagePDF(t *testing.T, dir string, n int) string {
	t.Helper()
	return testpdf.WriteMultiPage(t, dir, n)
}
