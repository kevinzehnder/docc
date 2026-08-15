package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInitCommand(t *testing.T) {
	root := t.TempDir()
	if got := run([]string{"init", root}); got != 0 {
		t.Fatalf("run(init) = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".docc", "schemas", "ch_letter.yaml")); err != nil {
		t.Fatalf("starter letter schema: %v", err)
	}
}

// TestInitHelpWritesNothing guards the bug a user actually hit: init had no flag
// set, so `docc init --help` took --help for the target directory and installed
// a starter project into a folder of that name.
func TestInitHelpWritesNothing(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	if got := run([]string{"init", "--help"}); got != 0 {
		t.Errorf("run(init --help) = %d, want 0", got)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("init --help wrote %d entries, want none: %v", len(entries), entries)
	}
}

func TestInitDryRun(t *testing.T) {
	root := t.TempDir()
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"init", "--dry-run", root})
	})
	if code != 0 {
		t.Fatalf("run(init --dry-run) = %d, want 0", code)
	}
	if !strings.Contains(out, filepath.Join(root, ".docc", "schemas", "ch_letter.yaml")) {
		t.Errorf("dry run does not list the letter schema:\n%s", out)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("--dry-run wrote %d entries, want none", len(entries))
	}
}

// TestHelpExitsZero: a help request is a successful request. Every subcommand
// used to collapse flag.ErrHelp into exit 2, which surprises scripts.
func TestHelpExitsZero(t *testing.T) {
	for _, cmd := range []string{"check", "build", "init", "doctor", "types", "themes", "describe", "example", "explain", "lsp"} {
		t.Run(cmd, func(t *testing.T) {
			var code int
			out := captureStdout(t, func() { code = run([]string{cmd, "--help"}) })
			if code != 0 {
				t.Errorf("run(%s --help) = %d, want 0", cmd, code)
			}
			if !strings.Contains(out, "docc "+cmd) {
				t.Errorf("%s --help does not print its synopsis on stdout:\n%s", cmd, out)
			}
		})
	}
}

func TestPermute(t *testing.T) {
	newSet := func() *flag.FlagSet {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.String("output", "", "")
		fs.Bool("force", false, "")
		return fs
	}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"value flag after positional", []string{"a.md", "--output", "x"}, []string{"--output", "x", "a.md"}},
		{"bool flag after positional", []string{"a.md", "--force"}, []string{"--force", "a.md"}},
		{"equals form", []string{"a.md", "--output=x"}, []string{"--output=x", "a.md"}},
		{"already ordered", []string{"--force", "a.md"}, []string{"--force", "a.md"}},
		{"double dash protects a dashed name", []string{"--force", "--", "-a.md"}, []string{"--force", "-a.md"}},
		{"unknown flag stays put for Parse to report", []string{"a.md", "--nope"}, []string{"a.md", "--nope"}},
		{"several positionals keep their order", []string{"a.md", "--force", "b.md"}, []string{"--force", "a.md", "b.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := permute(newSet(), tc.in)
			if !slices.Equal(got, tc.want) {
				t.Errorf("permute(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// write creates a file and the directories above it, for building throwaway
// projects in the tests below.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

const memoSchema = `type: memo
description: A memo.
theme: t
frontmatter:
  title:
    type: string
    required: true
body:
  - heading: Summary
    level: 1
    required: false
`

const minimalTheme = `name: t
description: A theme.
`

// TestCheckLoadsSchemaPerFile guards against resolving every file's schema from
// the first argument: two documents from different projects must each validate
// against their own project's contract.
func TestCheckLoadsSchemaPerFile(t *testing.T) {
	projA := t.TempDir()
	write(t, filepath.Join(projA, ".docc", "schemas", "memo.yaml"), "type: memo\ndescription: A memo.\n")
	docA := filepath.Join(projA, "a.md")
	write(t, docA, "---\ndocc: 1\ndocument_type: memo\n---\n\n# Hi\n")

	projB := t.TempDir()
	write(t, filepath.Join(projB, ".docc", "schemas", "note.yaml"), "type: note\ndescription: A note.\n")
	docB := filepath.Join(projB, "b.md")
	write(t, docB, "---\ndocc: 1\ndocument_type: note\n---\n\n# Hi\n")

	// Both are valid against their own project. Before the per-file fix, docB's
	// `note` type was checked against projA's schemas and failed as unknown.
	if got := run([]string{"check", docA, docB}); got != 0 {
		t.Errorf("run(check a b) = %d, want 0 — each file should use its own project's schema", got)
	}
}

// TestBuildStrictBlocksOnWarning verifies --strict promotes warnings to errors
// at build time, so a document that only warns produces no output.
func TestBuildStrictBlocksOnWarning(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	// Valid but for the missing optional "Summary" section, which warns (DOC021).
	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\ntitle: Q3\n---\n\n# Body\n")
	out := filepath.Join(dir, "memo.docx")

	if got := run([]string{"build", "--strict", "--schema-dir", schemaDir, "--theme-dir", themeDir, "--output", out, doc}); got != 1 {
		t.Fatalf("run(build --strict) = %d, want 1", got)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("--strict build should write no output, but %s exists", out)
	}
}

// TestBuildJSONErrorEmitsJSON verifies --json stays machine-readable on a
// validation failure instead of falling back to the human caret rendering.
func TestBuildJSONErrorEmitsJSON(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	// Missing the required `title`, so validation fails with an error.
	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\n---\n\n# Summary\n")
	out := filepath.Join(dir, "memo.docx")

	var code int
	stdout := captureStdout(t, func() {
		code = run([]string{"build", "--json", "--schema-dir", schemaDir, "--theme-dir", themeDir, "--output", out, doc})
	})
	if code != 1 {
		t.Fatalf("run(build --json, invalid) = %d, want 1", code)
	}
	stdout = strings.TrimSpace(stdout)
	if !strings.HasPrefix(stdout, "{") || !strings.Contains(stdout, `"ok":false`) {
		t.Errorf("stdout is not the JSON failure object:\n%s", stdout)
	}
}

// TestFlagsMayFollowTheInput covers the report's first friction point:
// `docc build file.md --output x.docx` used to fail with "expects exactly one
// input file", because Go's flag package stops at the first non-flag.
func TestFlagsMayFollowTheInput(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\ntitle: Q3\n---\n\n# Summary\n")
	out := filepath.Join(dir, "memo.docx")

	captureStdout(t, func() {
		if got := run([]string{"build", doc, "--schema-dir", schemaDir, "--theme-dir", themeDir, "--output", out}); got != 0 {
			t.Errorf("run(build <file> --output ...) = %d, want 0", got)
		}
	})
	if _, err := os.Stat(out); err != nil {
		t.Errorf("no output written: %v", err)
	}
}

// TestDoctorReportsSchemaThemeMismatch: emit.Validate used to be reachable only
// by building a valid document, so a profile that could never render looked fine
// until someone authored one.
func TestDoctorReportsSchemaThemeMismatch(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	// The schema maps h1 onto a style the theme does not define.
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema+"styles:\n  h1: NoSuchStyle\n")
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"doctor", "--json", "--schema-dir", schemaDir, "--theme-dir", themeDir})
	})
	if code != 1 {
		t.Fatalf("run(doctor) = %d, want 1 for a broken pair", code)
	}
	var got struct {
		SchemaDir string `json:"schema_dir"`
		OK        bool   `json:"ok"`
		Problems  []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"problems"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("doctor JSON: %v\n%s", err, out)
	}
	if got.SchemaDir != schemaDir {
		t.Errorf("schema_dir = %q, want %q", got.SchemaDir, schemaDir)
	}
	if got.OK || len(got.Problems) != 1 {
		t.Fatalf("want exactly one problem, got ok=%v %+v", got.OK, got.Problems)
	}
	if !strings.Contains(got.Problems[0].Message, "NoSuchStyle") {
		t.Errorf("problem does not name the missing style: %s", got.Problems[0].Message)
	}
}

func TestDoctorCleanProject(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	captureStdout(t, func() {
		if got := run([]string{"doctor", "--schema-dir", schemaDir, "--theme-dir", themeDir}); got != 0 {
			t.Errorf("run(doctor) = %d, want 0", got)
		}
	})
}

// objectSchema exercises the parts of a contract describe used to drop: a named
// object type, a conditional heading, and a `.docc-field` blank.
const objectSchema = `type: memo
description: A memo.
theme: t
types:
  party:
    name: { type: string, required: true }
    uid: { type: string, pattern: '^CHE-\d{3}$', hint: 'Swiss UID' }
frontmatter:
  title:
    type: string
    required: true
  sender:
    type: party
    required: true
  witness:
    type: party
    required: true
    nullable: true
body:
  - heading: Summary
    level: 1
    required: true
    ordered: true
    children:
      - heading: Detail
        level: 2
        required_when: 'title == "Q3"'
fields:
  signed_on:
    description: signed by hand on the day of execution
    required: true
    completion: handwritten
`

func TestDescribeReportsTheWholeContract(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	write(t, filepath.Join(schemaDir, "memo.yaml"), objectSchema)

	t.Run("json", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"describe", "--json", "--schema-dir", schemaDir, "memo"})
		})
		if code != 0 {
			t.Fatalf("run(describe --json) = %d, want 0", code)
		}
		var got describedType
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("describe JSON: %v\n%s", err, out)
		}

		var sender describedField
		for _, f := range got.Frontmatter {
			if f.Name == "sender" {
				sender = f
			}
		}
		if len(sender.Members) != 2 {
			t.Errorf("sender members = %+v, want name and uid expanded", sender.Members)
		}
		if len(got.Blanks) != 1 || got.Blanks[0].Completion != "handwritten" {
			t.Errorf("blanks = %+v, want the handwritten signed_on field", got.Blanks)
		}
		if len(got.Body) != 1 || len(got.Body[0].Children) != 1 ||
			got.Body[0].Children[0].RequiredWhen != `title == "Q3"` {
			t.Errorf("body does not carry required_when: %+v", got.Body)
		}
	})

	t.Run("human", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"describe", "--schema-dir", schemaDir, "memo"})
		})
		if code != 0 {
			t.Fatalf("run(describe) = %d, want 0", code)
		}
		for _, want := range []string{
			// The report's two confusions, both now answered in words.
			"nullable: an explicit ~ satisfies the requirement",
			`(required if title == "Q3")`,
			// And the detail the human path silently discarded.
			`pattern: ^CHE-\d{3}$`,
			"Swiss UID",
			"docc-field key=signed_on",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("describe output is missing %q:\n%s", want, out)
			}
		}
	})
}

func TestCatalogJSONIsOneObject(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)

	t.Run("types", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"types", "--json", "--schema-dir", schemaDir})
		})
		if code != 0 {
			t.Fatalf("run(types --json) = %d, want 0", code)
		}
		var got struct {
			Types []struct {
				Type string `json:"type"`
			} `json:"types"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("types JSON: %v\n%s", err, out)
		}
		if len(got.Types) != 1 || got.Types[0].Type != "memo" {
			t.Errorf("types = %+v, want memo", got.Types)
		}
	})

	t.Run("themes", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"themes", "--json", "--theme-dir", themeDir})
		})
		if code != 0 {
			t.Fatalf("run(themes --json) = %d, want 0", code)
		}
		var got struct {
			Themes []struct {
				Name string `json:"name"`
			} `json:"themes"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("themes JSON: %v\n%s", err, out)
		}
		if len(got.Themes) != 1 || got.Themes[0].Name != "t" {
			t.Errorf("themes = %+v, want t", got.Themes)
		}
	})
}

func TestWriteAtomic(t *testing.T) {
	t.Run("success writes destination", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out.txt")
		err := writeAtomic(dest, func(p string) error {
			return os.WriteFile(p, []byte("new"), 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(dest); string(b) != "new" {
			t.Errorf("dest = %q, want %q", b, "new")
		}
		assertNoTemp(t, dest)
	})

	t.Run("failure leaves an existing destination intact", func(t *testing.T) {
		dir := t.TempDir()
		dest := filepath.Join(dir, "out.txt")
		if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := writeAtomic(dest, func(p string) error {
			_ = os.WriteFile(p, []byte("partial"), 0o644)
			return io.ErrUnexpectedEOF
		})
		if err == nil {
			t.Fatal("expected the write error to propagate")
		}
		if b, _ := os.ReadFile(dest); string(b) != "old" {
			t.Errorf("dest = %q, want it untouched (%q)", b, "old")
		}
		assertNoTemp(t, dest)
	})
}

// assertNoTemp checks writeAtomic left no `.<name>.tmp-*` scratch file behind.
func assertNoTemp(t *testing.T, dest string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+filepath.Base(dest)+".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
