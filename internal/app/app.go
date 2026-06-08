package app

import (
	"context"
	"fmt"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/internal/util"
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

// Download runs a single-file download.
func (a *App) Download(ctx context.Context, url, output string) error {
	if _, err := util.ValidateURL(url); err != nil {
		return err
	}
	if output == "" {
		name, err := util.FilenameFromURL(url)
		if err != nil {
			return err
		}
		output = name
	}
	if err := a.EnsureDirs(); err != nil {
		return err
	}
	dl := downloader.New(a.Config)
	return dl.Run(ctx, downloader.Options{URL: url, Output: output})
}
