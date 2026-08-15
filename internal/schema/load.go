package schema

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Load reads every *.yaml file in dir as a schema and resolves `extends`
// inheritance. Files whose name begins with "_" are treated as fragments that
// exist only to be extended.
func Load(dir string) (*Set, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read schema dir: %w", err)
	}

	raw := map[string]*Schema{}
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		sc, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		if sc.Type == "" {
			return nil, fmt.Errorf("%s: schema is missing `type`", path)
		}
		if prev, dup := raw[sc.Type]; dup {
			return nil, fmt.Errorf("%s: document type %q already declared by %s", path, sc.Type, prev.Description)
		}
		raw[sc.Type] = sc
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("no schemas found in %s", dir)
	}

	resolved := map[string]*Schema{}
	for name := range raw {
		if _, err := resolve(name, raw, resolved, nil); err != nil {
			return nil, err
		}
	}
	return &Set{byType: resolved, Root: dir}, nil
}

func loadFile(path string) (*Schema, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from a directory listing of the project's own schema dir
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var sc Schema
	if err := yaml.UnmarshalWithOptions(b, &sc, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &sc, nil
}

// resolve merges a schema with its ancestor chain. seen guards against cycles.
func resolve(name string, raw, done map[string]*Schema, seen []string) (*Schema, error) {
	if sc, ok := done[name]; ok {
		return sc, nil
	}
	sc, ok := raw[name]
	if !ok {
		return nil, fmt.Errorf("schema %q extends unknown type %q", strings.Join(seen, " -> "), name)
	}
	for _, s := range seen {
		if s == name {
			return nil, fmt.Errorf("schema inheritance cycle: %s -> %s", strings.Join(seen, " -> "), name)
		}
	}
	if sc.Extends == "" {
		done[name] = sc
		return sc, nil
	}

	parent, err := resolve(sc.Extends, raw, done, append(seen, name))
	if err != nil {
		return nil, err
	}
	merged := merge(parent, sc)
	done[name] = merged
	return merged, nil
}

// merge layers a child schema over its parent. Frontmatter fields and named
// types are merged key-wise; body structure, styles and rules are replaced
// wholesale when the child declares them, since partial override of an ordered
// structure is more confusing than restating it.
func merge(parent, child *Schema) *Schema {
	out := *child

	out.Frontmatter = Fields{}
	for k, v := range parent.Frontmatter {
		out.Frontmatter[k] = v
	}
	for k, v := range child.Frontmatter {
		out.Frontmatter[k] = v
	}

	out.Types = map[string]Fields{}
	for k, v := range parent.Types {
		out.Types[k] = v
	}
	for k, v := range child.Types {
		out.Types[k] = v
	}

	out.Blocks = map[string]BlockSpec{}
	for k, v := range parent.Blocks {
		out.Blocks[k] = v
	}
	for k, v := range child.Blocks {
		out.Blocks[k] = v
	}

	out.Spans = map[string]SpanSpec{}
	for k, v := range parent.Spans {
		out.Spans[k] = v
	}
	for k, v := range child.Spans {
		out.Spans[k] = v
	}

	if len(child.Body) == 0 {
		out.Body = parent.Body
	}
	if len(child.Rules) == 0 {
		out.Rules = parent.Rules
	}
	if child.Theme == "" {
		out.Theme = parent.Theme
	}

	// Each render rule is inherited on its own. A base that numbers paragraphs
	// and a child that adds a heading outline is the useful case; making the
	// child restate both to change one is not.
	if child.Render.HeadingNumbering == nil {
		out.Render.HeadingNumbering = parent.Render.HeadingNumbering
	}
	if child.Render.ParagraphNumbering == nil {
		out.Render.ParagraphNumbering = parent.Render.ParagraphNumbering
	}

	out.Styles = map[string]string{}
	for k, v := range parent.Styles {
		out.Styles[k] = v
	}
	for k, v := range child.Styles {
		out.Styles[k] = v
	}
	return &out
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

// LoadFS is Load against an fs.FS, used for the schemas embedded in the binary.
func LoadFS(fsys fs.FS, dir string) (*Set, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read embedded schema dir: %w", err)
	}
	raw := map[string]*Schema{}
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		b, err := fs.ReadFile(fsys, filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var sc Schema
		if err := yaml.UnmarshalWithOptions(b, &sc, yaml.Strict()); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		raw[sc.Type] = &sc
	}
	resolved := map[string]*Schema{}
	for name := range raw {
		if _, err := resolve(name, raw, resolved, nil); err != nil {
			return nil, err
		}
	}
	return &Set{byType: resolved}, nil
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinKeys[T any](m map[string]T) string {
	return strings.Join(sortedKeys(m), ", ")
}
