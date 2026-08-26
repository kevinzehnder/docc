package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemeImagePathRejectsSymlinkEscape(t *testing.T) {
	themeDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("not theme data"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(themeDir, "logo.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := themeImagePath(themeDir, "logo.png")
	if err == nil || !strings.Contains(err.Error(), "escapes the theme directory") {
		t.Fatalf("themeImagePath error = %v, want escape error", err)
	}
}

func TestThemeImagePathAcceptsRegularFile(t *testing.T) {
	themeDir := t.TempDir()
	want := filepath.Join(themeDir, "logo.png")
	if err := os.WriteFile(want, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := themeImagePath(themeDir, "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	want, err = filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("themeImagePath = %q, want %q", got, want)
	}
}
