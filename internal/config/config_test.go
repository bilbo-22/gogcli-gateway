package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path := NewConfigStore(testSystemLayout(t, PathKindConfig)).Path()

	if filepath.Base(path) != "config.json" {
		t.Fatalf("unexpected config file: %q", filepath.Base(path))
	}

	if filepath.Base(filepath.Dir(path)) != AppName {
		t.Fatalf("unexpected config dir: %q", filepath.Dir(path))
	}
}

func TestReadConfig_Missing(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.KeyringBackend != "" {
		t.Fatalf("expected empty config, got %q", cfg.KeyringBackend)
	}
}

func TestReadConfig_JSON5(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	path := store.Path()
	data := `{
  // allow comments + trailing commas
  keyring_backend: "file",
}`

	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got := strings.TrimSpace(cfg.KeyringBackend); got != "file" {
		t.Fatalf("expected keyring_backend=file, got %q", got)
	}
}

func TestConfigStoreWriteAndUpdate(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	if err := store.Write(File{KeyringBackend: "file"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := store.Update(func(cfg *File) error {
		cfg.DefaultTimezone = "UTC"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.KeyringBackend != "file" || cfg.DefaultTimezone != "UTC" {
		t.Fatalf("config = %#v", cfg)
	}
}
