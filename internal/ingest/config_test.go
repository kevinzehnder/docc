package ingest

import (
	"os"
	"path/filepath"
	"testing"
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

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ingest.yaml")
	writeFile(t, path, "modle: qwen3-vl\n") // typo, strict mode should catch it

	if _, err := LoadConfig(path); err == nil {
		t.Error("expected an error for an unknown field under strict YAML unmarshalling")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
