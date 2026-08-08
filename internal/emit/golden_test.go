package emit

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
	"github.com/kevinzehnder/docc/internal/theme"
)

var update = flag.Bool("update", false, "rewrite golden files")

// goldenParts lists the archive members worth committing: everything directly
// under word/, which is document.xml, styles.xml, numbering.xml and whatever
// headers and footers the theme produced. Discovering them rather than naming
// them is what makes a theme that grows a header show up here.
//
// The rest of the package — content types, relationships, app properties — is
// fixed scaffolding that says nothing about how a document was rendered.
func goldenParts(names []string) []string {
	var out []string
	for _, name := range names {
		rest, under := strings.CutPrefix(name, "word/")
		if under && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".xml") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// TestBuildGolden renders every document in testdata/good and compares the parts
// that carry the rendering to committed files.
//
// This is affordable only because the writer is deterministic: fixed archive
// timestamps, sorted parts, identifiers assigned by position. A diff here is a
// real change in output, never noise.
//
// It does not replace `task test:roundtrip`. Golden files prove the bytes did
// not change; only a renderer proves Word will open them.
func TestBuildGolden(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")

	schemas, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}
	themeDir := filepath.Join(root, "themes")
	themes, err := theme.Load(themeDir)
	if err != nil {
		t.Fatalf("load themes: %v", err)
	}

	docs, err := filepath.Glob(filepath.Join(root, "good", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("no documents in testdata/good")
	}

	for _, doc := range docs {
		name := filepath.Base(doc)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(doc)
			if err != nil {
				t.Fatal(err)
			}

			f, parseDiags := parse.Parse(name, src)
			res := sema.Check(f, schemas, parseDiags, "")
			if res.Diagnostics.HasErrors() {
				t.Fatalf("%s does not validate; fix the fixture before comparing output", name)
			}
			if res.Schema == nil {
				t.Fatal("no schema resolved")
			}
			th, err := themes.Get(res.Schema.Theme)
			if err != nil {
				t.Fatalf("schema %q: %v", res.Schema.Type, err)
			}

			built, err := Build(
				ir.Build(f, res.DocType, res.Meta.Values),
				res.Schema, th, Options{ThemeDir: themeDir},
			)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			data, err := built.Bytes()
			if err != nil {
				t.Fatalf("write: %v", err)
			}

			dir := filepath.Join(root, "golden", name[:len(name)-len(".md")])
			parts := goldenParts(partNames(t, data))

			if *update {
				// Removed wholesale: a theme that loses its footer must lose the
				// golden file too, or the corpus keeps asserting on a part that
				// is no longer produced.
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				assertNoExtraGoldens(t, dir, parts)
			}

			for _, part := range parts {
				got := indentXML(partOf(t, data, part))
				golden := filepath.Join(dir, filepath.Base(part))

				if *update {
					if err := os.WriteFile(golden, []byte(got), 0o644); err != nil { //nolint:gosec // test fixture
						t.Fatal(err)
					}
					continue
				}

				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("missing golden file (run: task test:golden:update): %v", err)
				}
				if !bytes.Equal(want, []byte(got)) {
					t.Errorf("%s changed — review the diff, then regenerate", part)
				}
			}
		})
	}
}

// assertNoExtraGoldens catches the other direction: a part that stopped being
// produced leaves a committed file behind, and nothing else would notice.
func assertNoExtraGoldens(t *testing.T, dir string, parts []string) {
	t.Helper()
	expected := map[string]bool{}
	for _, p := range parts {
		expected[filepath.Base(p)] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no goldens yet; the per-part read reports it with a better hint
	}
	for _, e := range entries {
		if !expected[e.Name()] {
			t.Errorf("%s is committed but no longer produced — regenerate", filepath.Join(dir, e.Name()))
		}
	}
}

// indentXML puts each element on its own line, so a golden diff points at the
// element that changed rather than at one enormous line.
//
// It is a text transform on purpose. Re-serialising through encoding/xml would
// rewrite OOXML's namespace prefixes, and the golden would then record what the
// test did to the bytes rather than what the writer produced.
func indentXML(s string) string {
	return strings.ReplaceAll(s, "><", ">\n<") + "\n"
}
