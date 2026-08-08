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
	assetRoot       = "files"
	configRoot      = "docc"
	examplesDir     = "examples/docc"
	starterSkillDir = ".agents/skills/docc"
)

// Init creates a .docc configuration and sample documents beneath dir. It
// refuses to overwrite an existing configuration or starter-example directory.
func Init(dir string) error {
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	for _, path := range []string{filepath.Join(dir, ".docc"), filepath.Join(dir, examplesDir), filepath.Join(dir, starterSkillDir)} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
	}

	if err := copyTree(dir); err != nil {
		return err
	}
	return nil
}

func copyTree(dir string) error {
	return fs.WalkDir(files, assetRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == assetRoot {
			return nil
		}

		rel := strings.TrimPrefix(path, assetRoot+"/")
		switch {
		case strings.HasPrefix(rel, configRoot+"/"):
			rel = filepath.Join(".docc", strings.TrimPrefix(rel, configRoot+"/"))
		case strings.HasPrefix(rel, "agents/"):
			rel = filepath.Join(".agents", strings.TrimPrefix(rel, "agents/"))
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			return nil
		}

		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
		}
		//nolint:gosec // target is derived from an embedded asset path and the user's chosen project root.
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		if _, err := out.Write(data); err != nil {
			_ = out.Close()
			return fmt.Errorf("write %s: %w", target, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
