// Package starter scaffolds an editable checkout of the built-in starter
// profile pack, plus sample documents.
package starter

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinzehnder/docc/internal/defaultpack"
)

// files contains the sample documents installed beside the pack checkout.
//
//go:embed all:files
var files embed.FS

const (
	assetRoot   = "files"
	examplesDir = "examples"
)

// sources lists the trees Init writes: the built-in pack at the target root,
// and this package's own examples. Both are embedded, so the file lists are
// compiled in rather than observed.
func sources() []struct {
	fsys fs.FS
	root string
	dest string
} {
	return []struct {
		fsys fs.FS
		root string
		dest string
	}{
		{defaultpack.FS(), ".", "."},
		{files, assetRoot, "."},
	}
}

// Plan reports the files Init would create beneath dir, in walk order, after
// checking that none of the paths it owns already exists. It touches nothing,
// so `docc init --dry-run` is safe to run anywhere.
func Plan(dir string) ([]string, error) {
	if dir == "" {
		dir = "."
	}
	if err := checkVacant(dir); err != nil {
		return nil, err
	}

	var planned []string
	for _, src := range sources() {
		err := fs.WalkDir(src.fsys, src.root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			planned = append(planned, target(dir, src.root, path))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return planned, nil
}

// Init creates an editable profile-pack checkout beneath dir — manifest,
// schemas, themes — plus sample documents. The result resolves like any other
// pack checkout: through its docc-profile.yaml. Init refuses to overwrite an
// existing pack or examples directory.
func Init(dir string) error {
	if dir == "" {
		dir = "."
	}
	// The collision check runs before the directory is created, so a refusal
	// leaves nothing behind — an aborted init used to leave an empty dir.
	if err := checkVacant(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}
	for _, src := range sources() {
		if err := copyTree(src.fsys, src.root, dir); err != nil {
			return err
		}
	}
	return nil
}

// checkVacant reports whether any path the starter owns already exists. A
// `.docc` binding in the target would silently shadow the checkout in
// resolution order, so it counts as occupied too.
func checkVacant(dir string) error {
	for _, name := range []string{"docc-profile.yaml", "schemas", "themes", examplesDir, ".docc"} {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
	}
	return nil
}

// target maps an embedded asset path to its destination beneath dir.
func target(dir, root, path string) string {
	rel := path
	if root != "." {
		rel = strings.TrimPrefix(path, root+"/")
	}
	return filepath.Join(dir, filepath.FromSlash(rel))
}

func copyTree(fsys fs.FS, root, dir string) error {
	return fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// The walk is over an embedded asset tree, not the filesystem, so the
		// paths below are compiled in rather than observed — there is nothing
		// for a symlink to race.
		dest := target(dir, root, path)
		if d.IsDir() {
			//nolint:gosec // G122: the walk is over embed.FS; dest is derived from a compiled-in path.
			if err := os.MkdirAll(dest, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", dest, err)
			}
			return nil
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		//nolint:gosec // G122: the walk is over embed.FS; dest is derived from a compiled-in path.
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
		}
		//nolint:gosec // dest is derived from an embedded asset path and the user's chosen project root.
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		if _, err := out.Write(data); err != nil {
			_ = out.Close()
			return fmt.Errorf("write %s: %w", dest, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		return nil
	})
}
