package starter_test

import (
	"os"
	"path/filepath"
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
