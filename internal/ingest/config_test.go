package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Endpoint == "" {
		t.Error("Defaults().Endpoint is empty")
	}
	if cfg.Temperature != 0.1 {
		t.Errorf("Defaults().Temperature = %v, want 0.1 — low by default, for a small precision-critical corpus", cfg.Temperature)
	}
	if !cfg.Anchor {
		t.Error("Defaults().Anchor = false, want true — anchoring is the main precision lever and should be on by default")
	}
	if cfg.MaxTokens <= 0 {
		t.Errorf("Defaults().MaxTokens = %d, want a positive default — an unset cap silently truncates dense pages", cfg.MaxTokens)
	}
}

func TestLoadConfigMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig on a missing file: %v", err)
	}
	if cfg != Defaults() {
		t.Errorf("LoadConfig on a missing file = %+v, want Defaults()", cfg)
	}
}

func TestLoadConfigOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ingest.yaml")
	writeFile(t, path, "model: qwen3-vl\ntemperature: 0.5\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Model != "qwen3-vl" {
		t.Errorf("Model = %q, want qwen3-vl", cfg.Model)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %v, want 0.5", cfg.Temperature)
	}
	// Fields not mentioned in the file keep their defaults.
	if cfg.DPI != Defaults().DPI {
		t.Errorf("DPI = %d, want the default %d", cfg.DPI, Defaults().DPI)
	}
}

func TestLoadConfigParsesStallTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, "stall_timeout: 5m\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.StallTimeout != 5*time.Minute {
		t.Errorf("StallTimeout = %v, want 5m — the key is the only escape hatch for slow hardware, and a silent parse failure would hand it the default", cfg.StallTimeout)
	}
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ingest.yaml")
	writeFile(t, path, "modle: qwen3-vl\n") // typo, strict mode should catch it

	if _, err := LoadConfig(path); err == nil {
		t.Error("expected an error for an unknown field under strict YAML unmarshalling")
	}
}

// profileFile is a config in the shape this project uses: shared server
// settings, top-level defaults, and two profiles that differ in the one way
// that matters — which protocol the model speaks.
const profileFile = `
endpoint: http://localhost:9000/v1/chat/completions
use: layout

dpi: 150
anchor: true

profiles:
  layout:
    model: mineru-pro-2605
    backend: mineru
    anchor: false
  qwen:
    model: qwen3.5-9b
    backend: chat
    dpi: 200
`

// The whole point: selecting a profile picks the model and the protocol it
// speaks in one move, so the pairing cannot be got wrong by editing one of them
// and forgetting the other.
func TestResolveTakesModelAndBackendTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, profileFile)

	f, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	for _, c := range []struct{ profile, model, backend string }{
		{"layout", "mineru-pro-2605", BackendMinerU},
		{"qwen", "qwen3.5-9b", BackendChat},
	} {
		cfg, err := f.Resolve(c.profile)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", c.profile, err)
		}
		if cfg.Model != c.model || cfg.Backend != c.backend {
			t.Errorf("Resolve(%q) = %s/%s, want %s/%s", c.profile, cfg.Model, cfg.Backend, c.model, c.backend)
		}
	}
}

// An empty name means the file's own choice, which is what every ordinary run
// takes.
func TestResolveDefaultsToUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, profileFile)

	f, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	cfg, err := f.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Model != "mineru-pro-2605" {
		t.Errorf("Model = %q, want the profile named by use:", cfg.Model)
	}
}

// Top-level settings are what a profile starts from, and the profile wins where
// the two disagree. Anchoring is the case this exists for: it is worth 0.116 of
// precision under chat and inert under mineru, so it belongs to the protocol
// rather than to the project.
func TestResolveLayersProfileOverFileOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, profileFile)

	f, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	layout, err := f.Resolve("layout")
	if err != nil {
		t.Fatalf("Resolve(layout): %v", err)
	}
	if layout.Anchor {
		t.Error("layout.Anchor = true, want the profile's false to beat the file's true")
	}
	if layout.DPI != 150 {
		t.Errorf("layout.DPI = %d, want the file's 150 inherited", layout.DPI)
	}
	if layout.MaxTokens != Defaults().MaxTokens {
		t.Errorf("layout.MaxTokens = %d, want the default %d", layout.MaxTokens, Defaults().MaxTokens)
	}

	qwen, err := f.Resolve("qwen")
	if err != nil {
		t.Fatalf("Resolve(qwen): %v", err)
	}
	if !qwen.Anchor {
		t.Error("qwen.Anchor = false, want the file's true inherited")
	}
	if qwen.DPI != 200 {
		t.Errorf("qwen.DPI = %d, want the profile's 200", qwen.DPI)
	}
	if qwen.Endpoint != "http://localhost:9000/v1/chat/completions" {
		t.Errorf("qwen.Endpoint = %q, want the shared endpoint", qwen.Endpoint)
	}
}

// A typo in --profile has to name the alternatives. The whole reason this
// project has profiles is that the wrong one is invisible in the output, so
// failing here beats transcribing forty pages with it.
func TestResolveUnknownProfileListsTheKnownOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, profileFile)

	f, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	_, err = f.Resolve("mineru")
	if err == nil {
		t.Fatal("Resolve of an undeclared profile succeeded")
	}
	for _, want := range []string{"layout", "qwen"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the declared profile %q", err, want)
		}
	}
}

// Declaring profiles and selecting none is the mistake that started this: the
// config named a MinerU model with the backend line commented out, which meant
// the default, which is chat. Now it is an error rather than a silent choice.
func TestResolveRequiresASelectionOnceProfilesExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, "profiles:\n  qwen:\n    model: qwen3.5-9b\n    backend: chat\n")

	f, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if _, err := f.Resolve(""); err == nil {
		t.Error("Resolve with profiles declared and none selected succeeded, want an error naming --profile")
	}
}

// The flat form is a whole configuration on its own, and stays one: a project
// with a single model never has to learn what a profile is.
func TestResolveWithoutProfilesUsesTheTopLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.yaml")
	writeFile(t, path, "model: qwen3.5-9b\nbackend: chat\ndpi: 300\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Model != "qwen3.5-9b" || cfg.Backend != BackendChat || cfg.DPI != 300 {
		t.Errorf("got %s/%s/%d, want qwen3.5-9b/chat/300", cfg.Model, cfg.Backend, cfg.DPI)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
