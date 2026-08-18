package profile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestXDGPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/config")
	t.Setenv("XDG_DATA_HOME", "relative-data")
	t.Setenv("XDG_CACHE_HOME", "/cache")
	got := xdgPaths("/home/alice", os.Getenv)
	if got.Config != "/config" || got.Data != "/home/alice/.local/share" || got.Cache != "/cache" {
		t.Fatalf("xdgPaths = %+v", got)
	}
}

func TestLoadPackRejectsEscapingPaths(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, manifestName), "format: 1\nid: example\nschemas: ../schemas\nthemes: themes\n")
	if _, err := LoadPack(root); err == nil || !strings.Contains(err.Error(), "relative path") {
		t.Fatalf("LoadPack error = %v, want relative-path error", err)
	}
}

func TestResolveProjectBinding(t *testing.T) {
	paths := testPaths(t)
	ref := installFixturePack(t, paths, "firm", "0123456789abcdef0123456789abcdef01234567")
	root := t.TempDir()
	if err := WriteProject(root, ref); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(filepath.Join(root, "docs", "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "project-profile" || got.Reference == nil || *got.Reference != ref {
		t.Fatalf("Resolve = %+v, want project profile %v", got, ref)
	}
	if got.SchemaDir != filepath.Join(paths.Data, "docc", "profiles", ref.ID, ref.Commit, "schemas") {
		t.Errorf("SchemaDir = %q", got.SchemaDir)
	}
}

func TestResolveDefaultWithoutProject(t *testing.T) {
	paths := testPaths(t)
	ref := installFixturePack(t, paths, "default", "fedcba9876543210fedcba9876543210fedcba98")
	if err := SetDefault(paths, ref); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(filepath.Join(t.TempDir(), "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "user-default" || got.Reference == nil || *got.Reference != ref {
		t.Fatalf("Resolve = %+v, want user default %v", got, ref)
	}
}

func TestWriteProjectRefusesLegacyDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, projectDirName(), "schemas"), 0o755); err != nil {
		t.Fatal(err)
	}
	ref := Reference{ID: "firm", Source: "https://example.test/profiles.git", Commit: "0123456789abcdef0123456789abcdef01234567"}
	if err := WriteProject(root, ref); err == nil || !strings.Contains(err.Error(), "existing .docc/schemas") {
		t.Fatalf("WriteProject error = %v, want legacy collision", err)
	}
}

func TestInstallMakesValidatedImmutableRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for profile installation")
	}
	repo := t.TempDir()
	write(t, filepath.Join(repo, manifestName), "format: 1\nid: firm\nschemas: schemas\nthemes: themes\n")
	write(t, filepath.Join(repo, "schemas", "memo.yaml"), "type: memo\ndescription: memo\ntheme: t\n")
	write(t, filepath.Join(repo, "themes", "t.yaml"), "name: t\ndescription: theme\n")
	gitTest(t, repo, "init")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")

	paths := testPaths(t)
	ref, err := Install(context.Background(), paths, repo, "", Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != "firm" || len(ref.Commit) != 40 {
		t.Fatalf("Install = %+v", ref)
	}
	dir, err := paths.PackDir(ref.ID, ref.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("installed pack retains .git: %v", err)
	}
	if _, err := LoadPack(dir); err != nil {
		t.Fatalf("installed pack is invalid: %v", err)
	}
	remote, err := RemoteCommit(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if remote != ref.Commit {
		t.Errorf("RemoteCommit = %s, want %s", remote, ref.Commit)
	}
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return Paths{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data"), Cache: filepath.Join(root, "cache")}
}

func installFixturePack(t *testing.T, paths Paths, id, commit string) Reference {
	t.Helper()
	ref := Reference{ID: id, Source: "https://example.test/" + id + ".git", Commit: commit}
	dir, err := paths.PackDir(id, commit)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, manifestName), "format: 1\nid: "+id+"\nschemas: schemas\nthemes: themes\n")
	if err := os.MkdirAll(filepath.Join(dir, "schemas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return ref
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func projectDirName() string { return ".docc" }

// writePackCheckout lays out a pack repository: the manifest at its root and
// the two directories it names.
func writePackCheckout(t *testing.T, root, id string) {
	t.Helper()
	write(t, filepath.Join(root, manifestName), "format: 1\nid: "+id+"\nschemas: schemas\nthemes: themes\n")
	for _, dir := range []string{"schemas", "themes"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// A pack repository has no .docc — its schemas and themes are the product, not
// one project's local configuration — so every command in a pack checkout used
// to need --schema-dir and --theme-dir. The manifest already names both.
func TestResolvePackCheckout(t *testing.T) {
	paths := testPaths(t)
	root := t.TempDir()
	writePackCheckout(t, root, "firm")

	// From a file several directories down, the way authoring actually happens.
	got, err := Resolve(filepath.Join(root, "documents", "gruendung", "urkunde.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "pack-checkout" {
		t.Fatalf("Source = %q, want pack-checkout (%+v)", got.Source, got)
	}
	if got.Root != root || got.SchemaDir != filepath.Join(root, "schemas") || got.ThemeDir != filepath.Join(root, "themes") {
		t.Errorf("Resolve = %+v, want the pack's own directories", got)
	}
	// Nothing is pinned: you are working on the pack, not consuming a revision
	// of it, so there is no commit to record in a built document.
	if got.Reference != nil {
		t.Errorf("Reference = %+v, want none for a checkout", got.Reference)
	}
}

// A .docc binding is an explicit statement about which profile applies. A pack
// that happens to sit above it does not get to override that.
func TestResolveProjectBindingBeatsEnclosingPack(t *testing.T) {
	paths := testPaths(t)
	ref := installFixturePack(t, paths, "firm", "0123456789abcdef0123456789abcdef01234567")

	root := t.TempDir()
	writePackCheckout(t, root, "other")
	project := filepath.Join(root, "matters", "meier")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteProject(project, ref); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(filepath.Join(project, "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "project-profile" {
		t.Fatalf("Source = %q, want project-profile (%+v)", got.Source, got)
	}
}

// The pack you are working in is a more specific answer than the one you
// installed globally.
func TestResolvePackCheckoutBeatsUserDefault(t *testing.T) {
	paths := testPaths(t)
	ref := installFixturePack(t, paths, "default", "fedcba9876543210fedcba9876543210fedcba98")
	if err := SetDefault(paths, ref); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writePackCheckout(t, root, "firm")

	got, err := Resolve(filepath.Join(root, "schemas"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "pack-checkout" {
		t.Fatalf("Source = %q, want pack-checkout (%+v)", got.Source, got)
	}
}

// A manifest that says "this is a pack" and then fails to load must not be
// walked past: resolving some other configuration instead is how a document
// gets checked against schemas nobody chose.
func TestFindPackReportsABrokenManifest(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, manifestName), "format: 1\nid: firm\nschemas: schemas\n")
	_, err := FindPack(filepath.Join(root, "documents"))
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("FindPack error = %v, want a load failure", err)
	}
}

// DOCC_PROFILE is for the case neither walking up nor the working directory
// can answer: a host carrying a pack beside itself, compiling a document that
// lives wherever the agent happens to be.
func TestResolveEnvProfile(t *testing.T) {
	paths := testPaths(t)
	ref := installFixturePack(t, paths, "default", "fedcba9876543210fedcba9876543210fedcba98")
	if err := SetDefault(paths, ref); err != nil {
		t.Fatal(err)
	}
	packRoot := t.TempDir()
	writePackCheckout(t, packRoot, "firm")
	t.Setenv(EnvProfile, packRoot)

	// A project binding right beside the document must not win: the
	// environment is the more deliberate act.
	work := t.TempDir()
	if err := WriteProject(work, ref); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(filepath.Join(work, "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "env-profile" || got.SchemaDir != filepath.Join(packRoot, "schemas") {
		t.Fatalf("Resolve = %+v, want the pack named by %s", got, EnvProfile)
	}
	if got.PackID != "firm" {
		t.Errorf("PackID = %q, want firm", got.PackID)
	}
}

// A DOCC_PROFILE that names nothing usable must say so. Falling back would
// compile the document against schemas nobody chose.
func TestResolveEnvProfileRejectsABadDirectory(t *testing.T) {
	t.Setenv(EnvProfile, filepath.Join(t.TempDir(), "nowhere"))
	_, err := Resolve(t.TempDir(), testPaths(t))
	if err == nil || !strings.Contains(err.Error(), EnvProfile) {
		t.Fatalf("Resolve error = %v, want one naming %s", err, EnvProfile)
	}
}

// Nothing configured anywhere resolves the starter pack embedded in the
// binary: docc works out of the box, and the materialized copy is reused.
func TestResolveFallsBackToBuiltin(t *testing.T) {
	t.Setenv(EnvProfile, "")
	paths := testPaths(t)

	got, err := Resolve(filepath.Join(t.TempDir(), "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "builtin" || got.PackID != "starter" {
		t.Fatalf("Resolve = %+v, want builtin starter", got)
	}
	if _, err := os.Stat(got.SchemaDir); err != nil {
		t.Fatalf("materialized schema dir: %v", err)
	}

	// The second resolution must reuse the extraction, not fail over it.
	again, err := Resolve(filepath.Join(t.TempDir(), "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if again.SchemaDir != got.SchemaDir {
		t.Errorf("second Resolve = %q, want the same materialized pack %q", again.SchemaDir, got.SchemaDir)
	}
}

// A user default still beats the builtin fallback.
func TestResolveDefaultBeatsBuiltin(t *testing.T) {
	t.Setenv(EnvProfile, "")
	paths := testPaths(t)
	ref := installFixturePack(t, paths, "default", "fedcba9876543210fedcba9876543210fedcba98")
	if err := SetDefault(paths, ref); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(filepath.Join(t.TempDir(), "letter.md"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "user-default" {
		t.Fatalf("Source = %q, want user-default", got.Source)
	}
}

// The legacy layout — bare schemas under .docc, no manifest — fails loudly
// rather than silently compiling against the builtin starter pack.
func TestResolveRejectsLegacyLayout(t *testing.T) {
	t.Setenv(EnvProfile, "")
	paths := testPaths(t)
	root := t.TempDir()
	write(t, filepath.Join(root, ".docc", "schemas", "memo.yaml"), "type: memo\ndescription: A memo.\n")

	_, err := Resolve(filepath.Join(root, "memo.md"), paths)
	if err == nil || !strings.Contains(err.Error(), "legacy layout") {
		t.Fatalf("Resolve error = %v, want legacy-layout refusal", err)
	}
}
