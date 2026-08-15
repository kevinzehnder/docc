// Package project locates the .docc directory that supplies a project's
// schemas and themes.
//
// docc is the engine; the schemas, themes and house style are the project's own
// content. Keeping them apart means a theme change is a file
// edit rather than a compiler release.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DirName is the per-project configuration directory docc searches for.
const DirName = ".docc"

// Project is a resolved .docc directory.
type Project struct {
	// Root is the directory containing .docc.
	Root string
	// Dir is the .docc directory itself.
	Dir string
}

// SchemaDir returns the directory holding schema YAML files.
func (p *Project) SchemaDir() string { return filepath.Join(p.Dir, "schemas") }

// ThemeDir returns the directory holding theme definitions and their assets.
func (p *Project) ThemeDir() string { return filepath.Join(p.Dir, "themes") }

// Resolve walks up from start looking for a .docc directory, the way git finds
// .git. start may be a file or a directory.
func Resolve(start string) (*Project, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	for dir := abs; ; {
		candidate := filepath.Join(dir, DirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return &Project{Root: dir, Dir: candidate}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, fmt.Errorf("no %s directory found in %s or any parent: %w", DirName, abs, ErrNotFound)
}

// ErrNotFound signals that no project directory exists above the start path.
var ErrNotFound = errors.New("project not found")
