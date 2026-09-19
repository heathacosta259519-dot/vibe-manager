package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRaw 是测试用的小工具。
func writeRaw(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestDefaultThemeIsLight(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if got := Load().Theme; got != ThemeLight {
		t.Fatalf("default theme = %q, want %q", got, ThemeLight)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	cfg := Load()
	cfg.Theme = ThemeDark
	cfg.ProjectRoot = `X:\some\where`
	if err := Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := Load()
	if got.Theme != ThemeDark {
		t.Fatalf("theme = %q, want %q", got.Theme, ThemeDark)
	}
	if got.ProjectRoot != `X:\some\where` {
		t.Fatalf("projectRoot = %q", got.ProjectRoot)
	}
	if filepath.Base(FilePath()) != "config.json" {
		t.Fatalf("unexpected config path: %s", FilePath())
	}
}

func TestUnknownThemeFallsBackToLight(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	cfg := Load()
	cfg.Theme = "neon"
	if err := Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := Load().Theme; got != ThemeLight {
		t.Fatalf("theme = %q, want %q", got, ThemeLight)
	}
}
