package profile

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// corpus is the repository's own golden profile, which every other test in the
// tree already treats as valid.
func corpus(t *testing.T) (schemaDir, themeDir string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "schemas"), filepath.Join(root, "themes")
}

func packageCorpus(t *testing.T, opts SkillOptions) *SkillResult {
	t.Helper()
	schemaDir, themeDir := corpus(t)
	opts.SchemaDir, opts.ThemeDir = schemaDir, themeDir
	if opts.Out == "" {
		opts.Out = filepath.Join(t.TempDir(), "kanzlei-skill")
	}
	result, err := PackageSkill(opts)
	if err != nil {
		t.Fatalf("PackageSkill: %v", err)
	}
	return result
}

func TestPackageSkillLayout(t *testing.T) {
	result := packageCorpus(t, SkillOptions{})

	for _, rel := range []string{
		"SKILL.md",
		"probe.sh",
		filepath.Join("config", "schemas"),
		filepath.Join("config", "themes"),
	} {
		if _, err := os.Stat(filepath.Join(result.Dir, rel)); err != nil {
			t.Errorf("packaged skill is missing %s: %v", rel, err)
		}
	}
	// Configuration only by default: a bundled binary pins a docc version
	// inside every consumer's copy, and it must be asked for.
	if _, err := os.Stat(filepath.Join(result.Dir, "bin")); !os.IsNotExist(err) {
		t.Errorf("bin/ exists without --with-binary (err = %v)", err)
	}
	if result.Binary != "" {
		t.Errorf("Binary = %q, want empty without --with-binary", result.Binary)
	}
	if len(result.Types) == 0 {
		t.Fatal("no renderable types reported")
	}
}

// Theme assets are the letterhead. A skill that copied the YAML but not the
// logo renders a document that is wrong in the one way anyone would notice.
func TestPackageSkillCopiesThemeAssets(t *testing.T) {
	_, themeDir := corpus(t)
	entries, err := os.ReadDir(themeDir)
	if err != nil {
		t.Fatal(err)
	}
	var assets []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml") {
			assets = append(assets, e.Name())
		}
	}
	if len(assets) == 0 {
		t.Skip("the corpus themes carry no non-YAML assets")
	}
	result := packageCorpus(t, SkillOptions{})
	for _, name := range assets {
		if _, err := os.Stat(filepath.Join(result.Dir, "config", "themes", name)); err != nil {
			t.Errorf("theme asset %s was not packaged: %v", name, err)
		}
	}
}

// The examples come from the schemas, so a type's shipped example cannot fall
// out of step with the type.
func TestPackageSkillWritesExamplesFromSchemas(t *testing.T) {
	result := packageCorpus(t, SkillOptions{})
	if len(result.Examples) == 0 {
		t.Fatal("no examples written; the corpus schemas declare them")
	}
	for _, name := range result.Examples {
		body, err := os.ReadFile(filepath.Join(result.Dir, "examples", name))
		if err != nil {
			t.Fatalf("read example %s: %v", name, err)
		}
		if len(body) == 0 {
			t.Errorf("example %s is empty", name)
		}
		typ := strings.TrimSuffix(name, ".md")
		if !slices.Contains(result.Types, typ) {
			t.Errorf("example %s does not correspond to a packaged type", name)
		}
	}
}

// The instructions are generated so they cannot drift from the contract. That
// is only true if they actually name what was packaged.
func TestSkillDocNamesEveryPackagedType(t *testing.T) {
	result := packageCorpus(t, SkillOptions{Name: "kanzlei"})
	doc, err := os.ReadFile(filepath.Join(result.Dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)
	if !strings.HasPrefix(text, "---\nname: kanzlei\n") {
		t.Errorf("SKILL.md does not open with the skill's frontmatter name:\n%.80s", text)
	}
	for _, typ := range result.Types {
		if !strings.Contains(text, "`"+typ+"`") {
			t.Errorf("SKILL.md never mentions the packaged type %q", typ)
		}
	}
	// Without a bundled binary the instructions must not tell the agent to run
	// one that is not there.
	if strings.Contains(text, "bin/") {
		t.Error("SKILL.md refers to bin/ in a configuration-only skill")
	}
}

func TestPackageSkillWithBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "docc-linux-amd64")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho docc test\n"), 0o755); err != nil { //nolint:gosec // a stand-in executable for the packaging test
		t.Fatal(err)
	}
	result := packageCorpus(t, SkillOptions{Out: filepath.Join(dir, "skill"), Binary: bin})
	if result.Binary != "docc-linux-amd64" {
		t.Fatalf("Binary = %q, want the bundled file name", result.Binary)
	}
	info, err := os.Stat(filepath.Join(result.Dir, "bin", result.Binary))
	if err != nil {
		t.Fatalf("bundled binary: %v", err)
	}
	// A skill that ships a binary it cannot execute is useless in the sandbox
	// it was built for.
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("bundled binary mode = %v, want it executable", info.Mode().Perm())
	}
	doc, err := os.ReadFile(filepath.Join(result.Dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "bin/docc-linux-amd64") {
		t.Error("SKILL.md does not tell the agent about the bundled binary")
	}
}

// A profile that cannot render must fail at packaging, on the machine that can
// still fix it, rather than inside whoever installs the skill.
func TestPackageSkillRejectsBrokenPair(t *testing.T) {
	schemaDir, themeDir := corpus(t)
	broken := t.TempDir()
	if err := copyTree(schemaDir, broken); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "broken.yaml"), []byte(
		"type: broken\ntheme: nonexistent-theme\ndescription: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := PackageSkill(SkillOptions{SchemaDir: broken, ThemeDir: themeDir, Out: filepath.Join(t.TempDir(), "skill")})
	if err == nil {
		t.Fatal("expected packaging to reject a schema naming an unknown theme")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error = %q, want it to name the offending schema", err)
	}
}

func TestZipSkillIsDeterministic(t *testing.T) {
	result := packageCorpus(t, SkillOptions{})
	dst := filepath.Join(t.TempDir(), "skill.zip")
	other := filepath.Join(t.TempDir(), "skill.zip")
	for _, path := range []string{dst, other} {
		if err := ZipSkill(result.Dir, path); err != nil {
			t.Fatalf("ZipSkill: %v", err)
		}
	}
	first, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("two archives of one skill differ; packaging is not reproducible")
	}

	// The skill directory must be nested at the archive root, which is what
	// the skill format requires.
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	base := filepath.Base(result.Dir)
	var sawDoc bool
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, base+"/") {
			t.Fatalf("archive entry %q is not nested under %q", f.Name, base)
		}
		if f.Name == base+"/SKILL.md" {
			sawDoc = true
		}
	}
	if !sawDoc {
		t.Error("archive does not contain SKILL.md")
	}
}
