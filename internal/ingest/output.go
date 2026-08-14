package ingest

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteDraft atomically replaces path with content. The temporary file lives in
// path's directory, so Rename is atomic on the destination filesystem: an
// interrupted write leaves the previous draft intact rather than replacing it
// with a truncated, unclassifiable file.
func WriteDraft(path, content string) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary draft: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if _, err = f.WriteString(content); err != nil {
		return fmt.Errorf("write temporary draft: %w", err)
	}
	if err = f.Chmod(0o644); err != nil {
		return fmt.Errorf("set temporary draft permissions: %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("sync temporary draft: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close temporary draft: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace draft: %w", err)
	}
	return nil
}
