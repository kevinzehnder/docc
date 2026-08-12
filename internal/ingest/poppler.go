package ingest

import (
	"fmt"
	"os/exec"
	"strings"
)

// findBinary is the exec.LookPath pattern internal/emit/pdf.go uses for
// soffice, applied to poppler-utils: try each name in turn, and fail with an
// actionable message rather than a bare "not found".
func findBinary(names ...string) (string, error) {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found on PATH — install poppler-utils", strings.Join(names, "/"))
}
