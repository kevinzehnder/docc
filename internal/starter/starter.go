// Package starter installs docc's generic starter project.
package starter

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// files contains the configuration and sample documents installed by Init.
//
//go:embed all:files
var files embed.FS

const (
	assetRoot   = "files"
	configRoot  = "docc"
	examplesDir = "examples/docc"
)

// Plan reports the files Init would create beneath dir, in walk order, after
// checking that none of the directories it owns already exists. It touches
// nothing, so `docc init --dry-run` is safe to run anywhere.
func Plan(dir string) ([]string, error) {
	if dir == "" {
		dir = "."
	}
	if err := checkVacant(dir); err != nil {
		return nil, err
	}

	var planned []string
	err := fs.WalkDir(files, assetRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == assetRoot || d.IsDir() {
			return nil
		}
		planned = append(planned, target(dir, path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return planned, nil
}

// Init creates a .docc configuration and sample documents beneath dir. It
// refuses to overwrite an existing configuration or starter-example directory.
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
	return copyTree(dir)
}

// checkVacant reports whether any directory the starter owns already exists.
func checkVacant(dir string) error {
	for _, path := range []string{filepath.Join(dir, ".docc"), filepath.Join(dir, examplesDir)} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
	}
	return nil
}

// target maps an embedded asset path to its destination beneath dir. The
// configuration is shipped as `docc/` and installed as `.docc/`, because an
// embed pattern cannot match a dot-directory.
//
// The rename has to match the directory entry itself, not only what is inside
// it. Matching `docc/` alone left the walk's own entry for `docc` unrenamed, so
// every `docc init` also created an empty `docc/` beside the real `.docc/` —
// invisible to `--dry-run`, which skips directories and so never reported it.
func target(dir, path string) string {
	rel := strings.TrimPrefix(path, assetRoot+"/")
	if rel == configRoot || strings.HasPrefix(rel, configRoot+"/") {
		rel = "." + rel
	}
	return filepath.Join(dir, filepath.FromSlash(rel))
}

func copyTree(dir string) error {
	return fs.WalkDir(files, assetRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == assetRoot {
			return nil
		}

		// The walk is over the embedded asset tree, not the filesystem, so the
		// paths below are compiled in rather than observed — there is nothing
		// for a symlink to race.
		dest := target(dir, path)
		if d.IsDir() {
			//nolint:gosec // G122: the walk is over embed.FS; dest is derived from a compiled-in path.
			if err := os.MkdirAll(dest, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", dest, err)
			}
			return nil
		}

		data, err := files.ReadFile(path)
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
