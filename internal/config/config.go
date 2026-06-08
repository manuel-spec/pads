package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const configVersion = 1

// Config holds user-adjustable PADS settings.
type Config struct {
	Version            int    `json:"version"`
	MaxConnections     int    `json:"max_connections"`
	MinSegmentSize     int64  `json:"min_segment_size"`
	InitialConnections int    `json:"initial_connections"`
	RetryAttempts      int    `json:"retry_attempts"`
	RetryBackoffMS     int    `json:"retry_backoff_ms"`
	ProbeTimeoutMS     int    `json:"probe_timeout_ms"`
	StateDir           string `json:"state_dir"`
}

// Default returns the default configuration.
func Default() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return &Config{
		Version:            configVersion,
		MaxConnections:     8,
		MinSegmentSize:     1024 * 1024,
		InitialConnections: 2,
		RetryAttempts:      3,
		RetryBackoffMS:     1000,
		ProbeTimeoutMS:     5000,
		StateDir:           filepath.Join(home, ".pads"),
	}
}

// Path returns the config file path.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".pads", "config.json"), nil
}

// Load reads configuration from disk, falling back to defaults.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes configuration to disk atomically.
func Save(cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	path, err := Path()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}
	return nil
}

// Validate checks configuration values.
func (c *Config) Validate() error {
	if c.MaxConnections < 1 {
		return fmt.Errorf("max_connections must be at least 1")
	}
	if c.InitialConnections < 1 {
		return fmt.Errorf("initial_connections must be at least 1")
	}
	if c.InitialConnections > c.MaxConnections {
		return fmt.Errorf("initial_connections cannot exceed max_connections")
	}
	if c.MinSegmentSize < 1 {
		return fmt.Errorf("min_segment_size must be positive")
	}
	if c.RetryAttempts < 0 {
		return fmt.Errorf("retry_attempts cannot be negative")
	}
	if c.ProbeTimeoutMS < 100 {
		return fmt.Errorf("probe_timeout_ms must be at least 100")
	}
	if c.StateDir == "" {
		return fmt.Errorf("state_dir cannot be empty")
	}
	return nil
}

// Set updates a configuration key.
func (c *Config) Set(key, value string) error {
	switch key {
	case "max_connections":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid max_connections: %w", err)
		}
		c.MaxConnections = v
	case "min_segment_size":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid min_segment_size: %w", err)
		}
		c.MinSegmentSize = v
	case "initial_connections":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid initial_connections: %w", err)
		}
		c.InitialConnections = v
	case "retry_attempts":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid retry_attempts: %w", err)
		}
		c.RetryAttempts = v
	case "retry_backoff_ms":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid retry_backoff_ms: %w", err)
		}
		c.RetryBackoffMS = v
	case "probe_timeout_ms":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid probe_timeout_ms: %w", err)
		}
		c.ProbeTimeoutMS = v
	case "state_dir":
		c.StateDir = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return c.Validate()
}

// String returns a human-readable configuration summary.
func (c *Config) String() string {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Sprintf("config: version=%d max_connections=%d", c.Version, c.MaxConnections)
	}
	return string(data)
}

// StatePath returns a path under the state directory.
func (c *Config) StatePath(parts ...string) string {
	all := append([]string{c.StateDir}, parts...)
	return filepath.Join(all...)
}
