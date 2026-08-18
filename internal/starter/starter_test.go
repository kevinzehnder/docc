package starter_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/profile"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
	"github.com/kevinzehnder/docc/internal/starter"
	"github.com/kevinzehnder/docc/internal/theme"
)

func TestInitCreatesWorkingStarter(t *testing.T) {
	root := t.TempDir()
	if err := starter.Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	schemas, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}
	themes, err := theme.Load(filepath.Join(root, "themes"))
	if err != nil {
		t.Fatalf("load themes: %v", err)
	}

	for _, name := range []string{"letter.md", "legal.md"} {
		path := filepath.Join(root, "examples", name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, parseDiags := parse.Parse(path, src)
		result := sema.Check(file, schemas, parseDiags, "")
		if result.Diagnostics.HasErrors() {
			t.Fatalf("%s has errors: %v", name, result.Diagnostics)
		}
		if result.Schema == nil {
			t.Fatalf("%s resolved no schema", name)
		}
		th, err := themes.Get(result.Schema.Theme)
		if err != nil {
			t.Fatalf("theme for %s: %v", name, err)
		}
		built, err := emit.Build(ir.Build(file, result.DocType, result.Meta.Values), result.Schema, th,
			emit.Options{ThemeDir: filepath.Join(root, "themes")})
		if err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		if len(built.Body) == 0 {
			t.Fatalf("build %s produced no body", name)
		}
	}
}

// The scaffold is a real pack checkout: the ordinary walk-up resolution finds
// its manifest, with no init-specific resolution path left anywhere.
// A checkout carries the pack, the examples and nothing else. `files` is the
// embed root of this package's assets, an implementation detail that has no
// business appearing in the user's project.
func TestInitCreatesNoStrayDirectories(t *testing.T) {
	root := t.TempDir()
	if err := starter.Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	slices.Sort(got)
	want := []string{"README.md", "docc-profile.yaml", "examples", "schemas", "themes"}
	if !slices.Equal(got, want) {
		t.Errorf("checkout contains %v, want %v", got, want)
	}
}

func TestInitResolvesAsPackCheckout(t *testing.T) {
	root := t.TempDir()
	t.Setenv(profile.EnvProfile, "")
	if err := starter.Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}
	resolved, err := profile.Resolve(filepath.Join(root, "examples", "letter.md"), profile.Paths{
		Config: filepath.Join(root, "xdg-config"),
		Data:   filepath.Join(root, "xdg-data"),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Source != "pack-checkout" {
		t.Fatalf("Source = %q, want pack-checkout", resolved.Source)
	}
	if resolved.PackID != "starter" {
		t.Fatalf("PackID = %q, want starter", resolved.PackID)
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	root := t.TempDir()
	if err := starter.Init(root); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := starter.Init(root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second Init error = %v, want overwrite refusal", err)
	}
}

func TestInitDoesNotOverwriteExistingExamples(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := starter.Init(root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("Init error = %v, want overwrite refusal", err)
	}
	if _, err := os.Stat(filepath.Join(root, "schemas")); !os.IsNotExist(err) {
		t.Fatalf("schemas exists after rejected init: %v", err)
	}
}

// A .docc binding in the target would shadow the checkout in resolution order,
// so init refuses rather than writing a pack nobody would resolve.
func TestInitRefusesOverAProjectBinding(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".docc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := starter.Init(root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("Init error = %v, want overwrite refusal", err)
	}
}

// Plan backs `docc init --dry-run`: the same file list Init would write, without
// writing any of it, so discovery is safe.
func TestPlanListsWithoutWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new")

	planned, err := starter.Plan(root)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(planned) == 0 {
		t.Fatal("Plan listed no files")
	}
	for _, want := range []string{
		filepath.Join(root, "docc-profile.yaml"),
		filepath.Join(root, "schemas", "ch_letter.yaml"),
		filepath.Join(root, "examples", "letter.md"),
	} {
		if !slices.Contains(planned, want) {
			t.Errorf("Plan does not list %s: %v", want, planned)
		}
	}
	// Nothing may exist yet — not even the target directory.
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("Plan created %s: %v", root, err)
	}

	if err := starter.Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, path := range planned {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Plan listed %s but Init did not write it: %v", path, err)
		}
	}
}

// A refused Init must leave nothing behind. It used to create the target
// directory before checking whether it was allowed to.
func TestInitRefusalLeavesNoDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(target, "schemas"), 0o755); err != nil {
		t.Fatal(err)
	}

	inner := filepath.Join(target, "nested")
	if err := starter.Init(inner); err != nil {
		t.Fatalf("Init into a fresh subdirectory: %v", err)
	}

	// And the refusal path itself creates nothing new.
	fresh := filepath.Join(root, "fresh")
	if err := os.MkdirAll(filepath.Join(fresh, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := starter.Init(fresh); err == nil {
		t.Fatal("Init succeeded over an existing examples directory")
	}
	if _, err := os.Stat(filepath.Join(fresh, "schemas")); !os.IsNotExist(err) {
		t.Errorf("schemas created despite the refusal: %v", err)
	}
}
