package app

import (
	"context"
	"fmt"

	"pads/internal/config"
)

// App wires configuration and core services for CLI commands.
type App struct {
	Config *config.Config
}

// New creates an application instance with loaded configuration.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &App{Config: cfg}, nil
}

// EnsureDirs creates required state directories.
func (a *App) EnsureDirs() error {
	return config.EnsureStateDirs(a.Config)
}

// Download is a placeholder for the download orchestration entry point.
func (a *App) Download(ctx context.Context, url, output string) error {
	_ = ctx
	return fmt.Errorf("download not yet implemented: %s -> %s", url, output)
}
