package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/internal/model"
	"pads/internal/queue"
	"pads/internal/state"
	"pads/internal/util"
)

// App wires configuration and core services for CLI commands.
type App struct {
	Config   *config.Config
	state    *state.Store
	queue    *queue.Store
	registry *Registry
}

// DownloadState re-exports the persisted download model.
type DownloadState = model.DownloadState

// New creates an application instance with loaded configuration.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &App{
		Config:   cfg,
		state:    state.NewStore(cfg),
		queue:    queue.NewStore(cfg),
		registry: NewRegistry(),
	}, nil
}

// EnsureDirs creates required state directories.
func (a *App) EnsureDirs() error {
	return config.EnsureStateDirs(a.Config)
}

// LoadState returns persisted download state by ID.
func (a *App) LoadState(id string) (*model.DownloadState, error) {
	return a.state.Load(id)
}

// StateStore exposes the persisted-state store for daemon operations.
func (a *App) StateStore() *state.Store {
	return a.state
}

// ActiveIDs lists in-process active download IDs.
func (a *App) ActiveIDs() []string {
	return a.registry.ActiveIDs()
}

// IsDownloadActive reports whether a download runs in this process.
func (a *App) IsDownloadActive(id string) bool {
	return a.registry.IsActive(id)
}

// FilenameForURL derives a safe filename from a URL.
func FilenameForURL(url string) (string, error) {
	return util.FilenameFromURL(url)
}

// Download runs a single-file download with persistence.
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

	downloadID := uuid.NewString()
	ctx, cancel := context.WithCancel(ctx)
	a.registry.Register(downloadID, cancel)
	defer a.registry.Unregister(downloadID)

	dl := downloader.New(a.Config)
	return dl.Run(ctx, downloader.Options{
		URL:        url,
		Output:     output,
		DownloadID: downloadID,
		Store:      a.state,
	})
}

// Resume continues an interrupted download.
func (a *App) Resume(ctx context.Context, id string) error {
	st, err := a.state.Load(id)
	if err != nil {
		return err
	}
	if st.Complete {
		return fmt.Errorf("download %s is already complete", id)
	}

	state.NormalizeForResume(st)
	if err := a.state.Save(st); err != nil {
		return err
	}
	if err := a.EnsureDirs(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	a.registry.Register(id, cancel)
	defer a.registry.Unregister(id)

	dl := downloader.New(a.Config)
	return dl.Run(ctx, downloader.Options{
		Resume:     st,
		DownloadID: id,
		Store:      a.state,
	})
}

// Pause stops an active download or marks a saved state as paused.
func (a *App) Pause(id string) error {
	if a.registry.IsActive(id) {
		return a.registry.Pause(id)
	}

	st, err := a.state.Load(id)
	if err != nil {
		return err
	}
	st.Paused = true
	return a.state.Save(st)
}

// Status prints persisted and active download information.
func (a *App) Status(w io.Writer) error {
	states, err := a.state.List()
	if err != nil {
		return err
	}

	type statusEntry struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		Output   string `json:"output"`
		Paused   bool   `json:"paused"`
		Complete bool   `json:"complete"`
		Active   bool   `json:"active"`
		Bytes    int64  `json:"bytes_done"`
		Total    int64  `json:"total_size"`
	}

	entries := make([]statusEntry, 0, len(states))
	for _, st := range states {
		var bytesDone int64
		for _, seg := range st.Segments {
			bytesDone += seg.BytesDownloaded
		}
		entries = append(entries, statusEntry{
			ID:       st.ID,
			URL:      st.URL,
			Output:   st.Output,
			Paused:   st.Paused,
			Complete: st.Complete,
			Active:   a.registry.IsActive(st.ID),
			Bytes:    bytesDone,
			Total:    st.TotalSize,
		})
	}

	for _, id := range a.registry.ActiveIDs() {
		found := false
		for _, entry := range entries {
			if entry.ID == id {
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, statusEntry{ID: id, Active: true})
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// QueueAdd enqueues a download URL.
func (a *App) QueueAdd(url, output string) (queue.Entry, error) {
	if _, err := util.ValidateURL(url); err != nil {
		return queue.Entry{}, err
	}
	if err := a.EnsureDirs(); err != nil {
		return queue.Entry{}, err
	}
	return a.queue.Add(url, output)
}

// QueueList returns all queue entries.
func (a *App) QueueList() ([]queue.Entry, error) {
	if err := a.EnsureDirs(); err != nil {
		return nil, err
	}
	file, err := a.queue.Load()
	if err != nil {
		return nil, err
	}
	return file.Entries, nil
}

// QueueStart processes pending queue entries sequentially.
func (a *App) QueueStart(ctx context.Context) error {
	if err := a.EnsureDirs(); err != nil {
		return err
	}

	file, err := a.queue.Load()
	if err != nil {
		return err
	}

	for _, entry := range file.Entries {
		if entry.Status != queue.EntryPending {
			continue
		}
		if err := a.queue.UpdateEntryStatus(entry.ID, queue.EntryRunning, ""); err != nil {
			return err
		}
		if err := a.Download(ctx, entry.URL, entry.Output); err != nil {
			_ = a.queue.UpdateEntryStatus(entry.ID, queue.EntryFailed, err.Error())
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if err := a.queue.UpdateEntryStatus(entry.ID, queue.EntryComplete, ""); err != nil {
			return err
		}
	}
	return nil
}

// QueueClear removes all queue entries.
func (a *App) QueueClear() error {
	if err := a.EnsureDirs(); err != nil {
		return err
	}
	return a.queue.Clear()
}

// QueueRemove deletes one queue entry.
func (a *App) QueueRemove(id string) error {
	if err := a.EnsureDirs(); err != nil {
		return err
	}
	return a.queue.Remove(id)
}

// QueueUpdateStatus sets one queue entry's status.
func (a *App) QueueUpdateStatus(id string, status queue.EntryStatus, errText string) error {
	return a.queue.UpdateEntryStatus(id, status, errText)
}
