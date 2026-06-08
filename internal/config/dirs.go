package config

import (
	"fmt"
	"os"
)

// EnsureStateDirs creates state, temp, and queue directories.
func EnsureStateDirs(cfg *Config) error {
	dirs := []string{
		cfg.StateDir,
		cfg.StatePath("state"),
		cfg.StatePath("tmp"),
		cfg.StatePath("queue"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}
