package sema_test

import (
	"path/filepath"
	"testing"

	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
)

// TestSchemaExamplesCompileClean guards the `example:` documents embedded in
// schemas: `docc example` prints them as the canonical starting point, so an
// example that fails its own schema's check is a broken promise. This is the
// drift test — change the contract and the example must follow.
func TestSchemaExamplesCompileClean(t *testing.T) {
	dirs := map[string]string{
		"testdata": filepath.Join("..", "..", "testdata", "schemas"),
		"starter":  filepath.Join("..", "defaultpack", "files", "schemas"),
	}
	for label, dir := range dirs {
		set, err := schema.Load(dir)
		if err != nil {
			t.Fatalf("load %s: %v", dir, err)
		}
		for _, docType := range set.Types() {
			sc, err := set.Get(docType)
			if err != nil {
				t.Fatal(err)
			}
			if sc.Example == "" {
				continue
			}
			t.Run(label+"/"+docType, func(t *testing.T) {
				f, ds := parse.Parse(docType+".example.md", []byte(sc.Example))
				res := sema.Check(f, set, ds, "")
				if res.DocType != docType {
					t.Errorf("example declares document_type %q, want %q", res.DocType, docType)
				}
				for _, d := range res.Diagnostics {
					t.Errorf("%s:%d %s[%s]: %s", d.File, d.Pos.Line, d.Severity, d.Code, d.Message)
				}
			})
		}
	}
}
