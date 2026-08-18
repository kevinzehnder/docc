package emit

import (
	"path/filepath"
	"testing"

	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
	"github.com/kevinzehnder/docc/internal/theme"
)

// TestStarterProfile exercises the starter pack embedded in the binary — the
// profile `docc init` copies out and every unconfigured docc resolves — which
// testdata/ does not cover at all.
//
// The two corpora answer different questions and neither substitutes for the
// other. testdata/ is the engine's regression suite: small fixtures chosen to
// pin one behaviour each, with committed golden XML. The starter is what a new
// user is handed, so a style renamed in a theme, a check renamed in the
// registry, or a schema mapping a key the emitter stopped reading has to fail
// here rather than on someone's first `docc build`.
//
// This ran against the firm's own .docc/ until that moved to its own private
// pack repository, which now runs the same assertions in its own CI against a
// pinned docc release. What remains here is the starter pack embedded in the
// binary: the pack every unconfigured docc resolves, so it must always
// validate and build.
//
// It asserts what `docc doctor --strict` asserts, plus the two properties a
// profile's example is supposed to guarantee forever: it validates, and it
// builds.
func TestStarterProfile(t *testing.T) {
	root := filepath.Join("..", "defaultpack", "files")

	schemas, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}
	themeDir := filepath.Join(root, "themes")
	themes, err := theme.Load(themeDir)
	if err != nil {
		t.Fatalf("load themes: %v", err)
	}

	types := schemas.Types()
	if len(types) == 0 {
		t.Fatal("no document types in the embedded starter pack")
	}

	for _, name := range types {
		t.Run(name, func(t *testing.T) {
			sc, err := schemas.Get(name)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}

			// A type with no theme is check-only by design — the shared
			// fragments. It has nothing to render and nothing to agree with.
			if sc.Theme == "" {
				return
			}

			th, err := themes.Get(sc.Theme)
			if err != nil {
				t.Fatalf("theme %q: %v", sc.Theme, err)
			}
			// The schema-against-theme agreement check: every style the schema
			// maps exists, every {{ field }} the furniture interpolates
			// resolves, every numbering definition a render rule names is
			// there. This is the half of `doctor` that catches a profile whose
			// wiring rotted.
			if err := Validate(sc, th); err != nil {
				t.Fatalf("schema and theme disagree: %v", err)
			}

			if sc.Example == "" {
				t.Fatal("a buildable type must carry an `example:` — it is the profile's own regression test")
			}

			buildable := func(t *testing.T, label, src string) {
				t.Helper()
				f, parseDiags := parse.Parse(label, []byte(src))
				res := sema.Check(f, schemas, parseDiags, "")
				if res.Diagnostics.HasErrors() {
					for _, d := range res.Diagnostics {
						t.Errorf("%s: %s: %s", label, d.Code, d.Message)
					}
					t.Fatal("does not validate")
				}
				if _, err := Build(
					ir.Build(f, res.DocType, res.Meta.Values),
					res.Schema, th, Options{ThemeDir: themeDir},
				); err != nil {
					t.Fatalf("%s: build: %v", label, err)
				}
			}
			buildable(t, name+" example", sc.Example)

			// The skeleton `docc example --blank` hands an author must still be
			// a valid draft: a blank is content, not missing content. If this
			// fails, the starting point the tool offers is one the tool
			// rejects.
			//
			// Warnings count. `docc check --strict` is what a pack's CI runs,
			// so a warning on the skeleton is a failing build for whoever
			// starts from it — and asserting only HasErrors() is how a
			// `spans_agree` warning on every blank stayed invisible here.
			skeleton := sema.BlankFields(sc.Example)
			f, parseDiags := parse.Parse(name+" skeleton", []byte(skeleton))
			res := sema.Check(f, schemas, parseDiags, "")
			if len(res.Diagnostics) > 0 {
				for _, d := range res.Diagnostics {
					t.Errorf("skeleton: %s[%s]: %s", d.Severity, d.Code, d.Message)
				}
				t.Fatal("`example --blank` does not pass `check --strict`")
			}
		})
	}
}
