package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProfileUseInstallsAndBindsAPack(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for profile packs")
	}
	store := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(store, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(store, "data"))

	repo := t.TempDir()
	write(t, filepath.Join(repo, "docc-profile.yaml"), "format: 1\nid: firm\nschemas: schemas\nthemes: themes\n")
	write(t, filepath.Join(repo, "schemas", "memo.yaml"), memoSchema)
	write(t, filepath.Join(repo, "themes", "t.yaml"), minimalTheme)
	gitCommand(t, repo, "init")
	gitCommand(t, repo, "add", ".")
	gitCommand(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "profile")

	root := filepath.Join(t.TempDir(), "documents")
	if got := run([]string{"profile", "use", "--project", root, repo}); got != 0 {
		t.Fatalf("run(profile use) = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".docc", "profile.yaml")); err != nil {
		t.Fatalf("profile binding: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".docc", "profile.lock")); err != nil {
		t.Fatalf("profile lock: %v", err)
	}

	doc := filepath.Join(root, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\ntitle: Q3\n---\n\n# Summary\n")
	if got := run([]string{"check", doc}); got != 0 {
		t.Errorf("run(check profile-backed document) = %d, want 0", got)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"profile", "status", "--project", root, "--check-remote", "--json"})
	})
	if code != 0 {
		t.Fatalf("run(profile status --check-remote) = %d", code)
	}
	var status struct {
		RemoteCommit string `json:"remote_commit"`
		Stale        bool   `json:"stale"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("profile status JSON: %v\n%s", err, out)
	}
	if status.RemoteCommit == "" || status.Stale {
		t.Errorf("profile status = %+v, want current remote commit", status)
	}

	write(t, filepath.Join(repo, "README.md"), "new profile revision\n")
	gitCommand(t, repo, "add", ".")
	gitCommand(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "update")
	out = captureStdout(t, func() {
		code = run([]string{"profile", "status", "--project", root, "--check-remote", "--json"})
	})
	if code != 0 || json.Unmarshal([]byte(out), &status) != nil || !status.Stale {
		t.Fatalf("status should identify the newer remote revision: code=%d status=%+v output=%s", code, status, out)
	}
	if got := run([]string{"profile", "update", "--project", root}); got != 0 {
		t.Fatalf("run(profile update) = %d, want 0", got)
	}
	out = captureStdout(t, func() {
		code = run([]string{"profile", "status", "--project", root, "--check-remote", "--json"})
	})
	if code != 0 || json.Unmarshal([]byte(out), &status) != nil || status.Stale {
		t.Fatalf("updated profile should be current: code=%d status=%+v output=%s", code, status, out)
	}
}

func gitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

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

// `docc --version` is what a CI step reaches for first, and what every other
// tool on the PATH answers. It must say exactly what the subcommand says.
func TestVersionFlagMatchesSubcommand(t *testing.T) {
	var flagCode, cmdCode int
	fromFlag := captureStdout(t, func() { flagCode = run([]string{"--version"}) })
	fromCmd := captureStdout(t, func() { cmdCode = run([]string{"version"}) })

	if flagCode != 0 || cmdCode != 0 {
		t.Fatalf("exit codes = %d and %d, want 0", flagCode, cmdCode)
	}
	if fromFlag != fromCmd {
		t.Errorf("--version printed %q, version printed %q", fromFlag, fromCmd)
	}
	if !strings.HasPrefix(fromFlag, "docc ") {
		t.Errorf("--version = %q, want it to name the program", fromFlag)
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

// Usage and configuration failures used to share exit code 2, so a caller could
// not tell "you typed it wrong" — retry differently — from "your project is
// wrong" — no invocation will help.
func TestExitCodesSeparateUsageFromConfiguration(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme)
	doc := filepath.Join(dir, "memo.md")
	write(t, doc, "---\ndocc: 1\ndocument_type: memo\ntitle: Q3\n---\n\n# Summary\n")

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no input file", []string{"build"}, exitUsage},
		{"two input files", []string{"build", doc, doc}, exitUsage},
		{"unknown output format", []string{"build", "--to", "rtf", doc}, exitUsage},
		{"unreadable input", []string{"build", filepath.Join(dir, "absent.md")}, exitUsage},
		{"unknown diagnostic code", []string{"explain", "DOC999"}, exitUsage},
		{"too many codes", []string{"explain", "DOC001", "DOC002"}, exitUsage},

		{"no project", []string{"check", "--schema-dir", filepath.Join(dir, "absent"), doc}, exitConfig},
		{"unknown document type", []string{"describe", "--schema-dir", schemaDir, "nosuch"}, exitConfig},
		{"unknown theme", []string{"build", "--schema-dir", schemaDir, "--theme-dir", themeDir, "--theme", "nosuch", doc}, exitConfig},
		{"schema declares no example", []string{"example", "--schema-dir", schemaDir, "memo"}, exitConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var code int
			captureStdout(t, func() { code = run(tc.args) })
			if code != tc.want {
				t.Errorf("run(%v) = %d, want %d", tc.args, code, tc.want)
			}
		})
	}
}

// Every failure path must stay on the JSON stream when --json is given.
// Previously only build had a failure object; everything else printed human text
// to stderr and left stdout empty, so a consumer saw success-shaped silence.
func TestJSONFailureObjects(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema)

	cases := []struct {
		name string
		args []string
		kind string
	}{
		{"config", []string{"describe", "--json", "--schema-dir", schemaDir, "nosuch"}, "config"},
		{"usage", []string{"explain", "--json", "DOC999"}, "usage"},
		{"types with no project", []string{"types", "--json", "--schema-dir", filepath.Join(dir, "absent")}, "config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() { run(tc.args) })
			var got struct {
				OK    bool   `json:"ok"`
				Kind  string `json:"kind"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("stdout is not a JSON failure object: %v\n%s", err, out)
			}
			if got.OK {
				t.Error(`"ok" should be false`)
			}
			if got.Kind != tc.kind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.kind)
			}
			if got.Error == "" {
				t.Error("the failure object carries no message")
			}
		})
	}
}

// describe and example resolve a project from the working directory, because
// their positional argument is a type name rather than a file. --from is the way
// to describe a type from outside its own tree without naming both directories.
func TestDescribeFromAnotherProject(t *testing.T) {
	proj := t.TempDir()
	write(t, filepath.Join(proj, ".docc", "schemas", "memo.yaml"), memoSchema)
	t.Chdir(t.TempDir()) // somewhere with no .docc above it

	var code int
	captureStdout(t, func() { code = run([]string{"describe", "memo"}) })
	if code != exitConfig {
		t.Errorf("describe without --from = %d, want %d", code, exitConfig)
	}

	out := captureStdout(t, func() { code = run([]string{"describe", "--from", proj, "memo"}) })
	if code != 0 {
		t.Fatalf("describe --from = %d, want 0", code)
	}
	if !strings.Contains(out, "memo — A memo.") {
		t.Errorf("--from did not resolve the other project:\n%s", out)
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
	if code != exitConfig {
		t.Fatalf("run(doctor) = %d, want %d — a profile that cannot render is a configuration error", code, exitConfig)
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

// A style mapping nothing reads is silent in every other way — it validates, it
// renders, and it changes nothing — so doctor is the only place it can surface.
func TestDoctorWarnsOnUnreadStyleKeys(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	themeDir := filepath.Join(dir, "themes")
	write(t, filepath.Join(schemaDir, "memo.yaml"), memoSchema+
		"blocks:\n  beweis: {}\nstyles:\n  code_span: Mono\n  div.bewies: Typo\n")
	write(t, filepath.Join(themeDir, "t.yaml"), minimalTheme+
		"styles:\n  Mono: { font: Courier New }\n  Typo: {}\n")

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"doctor", "--json", "--schema-dir", schemaDir, "--theme-dir", themeDir})
	})
	// Warnings alone do not fail the report.
	if code != 0 {
		t.Fatalf("run(doctor) = %d, want 0 — unread keys are warnings", code)
	}
	var got struct {
		Warnings []struct {
			Message string `json:"message"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("doctor JSON: %v\n%s", err, out)
	}
	if len(got.Warnings) != 2 {
		t.Fatalf("want 2 warnings, got %+v", got.Warnings)
	}
	joined := out
	if !strings.Contains(joined, "Courier New") {
		t.Error("the code_span warning does not say what the fixed formatting is")
	}
	if !strings.Contains(joined, `declares no block \"bewies\"`) {
		t.Error("the typo'd block name is not reported")
	}

	// --strict is the caller asking for the warnings to bind.
	captureStdout(t, func() {
		code = run([]string{"doctor", "--strict", "--schema-dir", schemaDir, "--theme-dir", themeDir})
	})
	if code != exitConfig {
		t.Errorf("run(doctor --strict) = %d, want %d", code, exitConfig)
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
blocks:
  betraege: {}
fields:
  signed_on:
    description: signed by hand on the day of execution
    required: true
    completion: handwritten
styles:
  h1: Heading1
  div.betraege.amount: Amount
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

		// Which rendering pattern a block uses is a consequence of the style map
		// and is declared nowhere in the block itself.
		if len(got.Blocks) != 1 || got.Blocks[0].Pattern != "amount" {
			t.Errorf("blocks do not report the rendering pattern: %+v", got.Blocks)
		}
		if len(got.Styles.Mapped) != 2 {
			t.Errorf("styles.mapped = %+v, want h1 and div.betraege.amount", got.Styles.Mapped)
		}
		if len(got.Styles.Fixed) == 0 {
			t.Error("styles.fixed is empty; the unreachable constructs are the point")
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
			// The ceiling: constructs no theme can reach.
			"fixed formatting (no theme can change these)",
			"Courier New",
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

// A schema names its own diagnostic codes for the checks it selects, and the
// engine has never heard of them. `docc describe` prints them beside the
// DOC0xx ones and they look identical, so `docc explain` is the obvious next
// move — and it used to answer "no explanation", leaving the check's meaning
// discoverable only by deliberately breaking a document.
func TestExplainResolvesSchemaOwnedCodes(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schemas")
	write(t, filepath.Join(schemaDir, "memo.yaml"), `
type: memo
description: A memo.
frontmatter:
  document_type: { type: string, required: true }
rules:
  - id: MEM007
    check: no_placeholder_text
    severity: error
    message: a placeholder survived into a filed memo
    hint: replace it with the real wording
`)

	t.Run("with --type", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"explain", "MEM007", "--type", "memo", "--schema-dir", schemaDir})
		})
		if code != 0 {
			t.Fatalf("run(explain MEM007) = %d, want 0", code)
		}
		for _, want := range []string{
			"MEM007", "memo", "no_placeholder_text",
			"a placeholder survived into a filed memo",
			"replace it with the real wording",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("explain output missing %q:\n%s", want, out)
			}
		}
	})

	// Without --type it searches the project, because an author holding only a
	// code from a diagnostic does not yet know which type declared it.
	t.Run("without --type", func(t *testing.T) {
		var code int
		out := captureStdout(t, func() {
			code = run([]string{"explain", "MEM007", "--schema-dir", schemaDir})
		})
		if code != 0 {
			t.Fatalf("run(explain MEM007) = %d, want 0", code)
		}
		if !strings.Contains(out, "no_placeholder_text") {
			t.Errorf("explain did not find the code by searching:\n%s", out)
		}
	})

	// A code nobody declares still fails, and now says where else to look.
	t.Run("genuinely unknown", func(t *testing.T) {
		if code := run([]string{"explain", "ZZZ999", "--schema-dir", schemaDir}); code != exitUsage {
			t.Errorf("run(explain ZZZ999) = %d, want %d", code, exitUsage)
		}
	})
}
