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

	schemas, err := schema.Load(filepath.Join(root, ".docc", "schemas"))
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}
	themes, err := theme.Load(filepath.Join(root, ".docc", "themes"))
	if err != nil {
		t.Fatalf("load themes: %v", err)
	}

	for _, name := range []string{"letter.md", "legal.md"} {
		path := filepath.Join(root, "examples", "docc", name)
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
			emit.Options{ThemeDir: filepath.Join(root, ".docc", "themes")})
		if err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		if len(built.Body) == 0 {
			t.Fatalf("build %s produced no body", name)
		}
	}

	skill, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "docc", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if !strings.Contains(string(skill), "name: docc") {
		t.Fatal("installed skill has no docc frontmatter")
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
	if err := os.MkdirAll(filepath.Join(root, "examples", "docc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := starter.Init(root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("Init error = %v, want overwrite refusal", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".docc")); !os.IsNotExist(err) {
		t.Fatalf(".docc exists after rejected init: %v", err)
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
	if !slices.Contains(planned, filepath.Join(root, ".docc", "schemas", "ch_letter.yaml")) {
		t.Errorf("Plan does not list the letter schema: %v", planned)
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
	if err := os.MkdirAll(filepath.Join(target, ".docc"), 0o755); err != nil {
		t.Fatal(err)
	}

	inner := filepath.Join(target, "nested")
	if err := starter.Init(inner); err != nil {
		t.Fatalf("Init into a fresh subdirectory: %v", err)
	}

	// And the refusal path itself creates nothing new.
	fresh := filepath.Join(root, "fresh")
	if err := os.MkdirAll(filepath.Join(fresh, "examples", "docc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := starter.Init(fresh); err == nil {
		t.Fatal("Init succeeded over an existing examples/docc")
	}
	if _, err := os.Stat(filepath.Join(fresh, ".docc")); !os.IsNotExist(err) {
		t.Errorf(".docc created despite the refusal: %v", err)
	}
}
