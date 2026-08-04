package sema_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden checks every document under testdata/{good,bad} against the
// schemas in testdata/schemas and compares the rendered diagnostics to a
// committed .golden file.
//
// The corpus is the regression suite: a schema or rule change that alters any
// message shows up here as a diff rather than as a surprise in a real brief.
func TestGolden(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	set, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}

	docs, err := filepath.Glob(filepath.Join(root, "*", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("no documents in testdata")
	}

	for _, doc := range docs {
		t.Run(filepath.Base(filepath.Dir(doc))+"/"+filepath.Base(doc), func(t *testing.T) {
			src, err := os.ReadFile(doc)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Base(doc)

			f, parseDiags := parse.Parse(name, src)
			res := sema.Check(f, set, parseDiags, "")

			var buf bytes.Buffer
			srcFn := func(string) string { return string(src) }
			if err := res.Diagnostics.Render(&buf, srcFn, false); err != nil {
				t.Fatal(err)
			}

			golden := doc[:len(doc)-len(".md")] + ".golden"
			if *update {
				if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil { //nolint:gosec // test fixture
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run: task test:golden:update): %v", err)
			}
			if !bytes.Equal(want, buf.Bytes()) {
				t.Errorf("diagnostics changed\n--- want ---\n%s\n--- got ---\n%s", want, buf.Bytes())
			}
		})
	}
}

// TestGoodDocumentsHaveNoErrors guards the corpus itself: a document in
// testdata/good that starts producing errors means either a real regression or
// a fixture that was never valid.
func TestGoodDocumentsHaveNoErrors(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	set, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}

	docs, err := filepath.Glob(filepath.Join(root, "good", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		src, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		f, parseDiags := parse.Parse(filepath.Base(doc), src)
		res := sema.Check(f, set, parseDiags, "")
		if res.Diagnostics.HasErrors() {
			var buf bytes.Buffer
			_ = res.Diagnostics.Render(&buf, func(string) string { return string(src) }, false)
			t.Errorf("%s should be clean:\n%s", doc, buf.String())
		}
	}
}
