package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultValidate(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestSetAndValidate(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("max_connections", "4"); err != nil {
		t.Fatalf("set max_connections: %v", err)
	}
	if cfg.MaxConnections != 4 {
		t.Fatalf("expected max_connections 4, got %d", cfg.MaxConnections)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cfg := Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.MaxConnections = 6

	if err := Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.MaxConnections != 6 {
		t.Fatalf("expected max_connections 6, got %d", loaded.MaxConnections)
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	_, err := os.Stat(filepath.Join(dir, ".pads", "config.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected missing config file, got err=%v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if cfg.MaxConnections != Default().MaxConnections {
		t.Fatalf("expected default max_connections")
	}
}
