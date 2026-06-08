package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"pads/internal/config"
)

// EntryStatus describes queue entry lifecycle.
type EntryStatus string

const (
	EntryPending  EntryStatus = "pending"
	EntryRunning  EntryStatus = "running"
	EntryComplete EntryStatus = "complete"
	EntryFailed   EntryStatus = "failed"
)

// Entry is one queued download.
type Entry struct {
	ID      string      `json:"id"`
	URL     string      `json:"url"`
	Output  string      `json:"output,omitempty"`
	Status  EntryStatus `json:"status"`
	Error   string      `json:"error,omitempty"`
	AddedAt time.Time   `json:"added_at"`
}

// File is the persisted queue document.
type File struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Store persists the download queue.
type Store struct {
	path string
}

const queueVersion = 1

// NewStore creates a queue store.
func NewStore(cfg *config.Config) *Store {
	return &Store{path: cfg.StatePath("queue", "queue.json")}
}

// Load reads the queue file.
func (s *Store) Load() (*File, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Version: queueVersion, Entries: []Entry{}}, nil
		}
		return nil, fmt.Errorf("read queue: %w", err)
	}

	file := &File{}
	if err := json.Unmarshal(data, file); err != nil {
		return nil, fmt.Errorf("parse queue: %w", err)
	}
	if file.Entries == nil {
		file.Entries = []Entry{}
	}
	return file, nil
}

// Save writes the queue atomically.
func (s *Store) Save(file *File) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create queue directory: %w", err)
	}

	file.Version = queueVersion
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal queue: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write queue: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit queue: %w", err)
	}
	return nil
}

// Add appends a pending queue entry.
func (s *Store) Add(url, output string) (Entry, error) {
	file, err := s.Load()
	if err != nil {
		return Entry{}, err
	}

	entry := Entry{
		ID:      uuid.NewString(),
		URL:     url,
		Output:  output,
		Status:  EntryPending,
		AddedAt: time.Now(),
	}
	file.Entries = append(file.Entries, entry)
	if err := s.Save(file); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Remove deletes an entry by ID.
func (s *Store) Remove(id string) error {
	file, err := s.Load()
	if err != nil {
		return err
	}

	filtered := make([]Entry, 0, len(file.Entries))
	found := false
	for _, entry := range file.Entries {
		if entry.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !found {
		return fmt.Errorf("queue entry not found: %s", id)
	}
	file.Entries = filtered
	return s.Save(file)
}

// Clear removes all entries.
func (s *Store) Clear() error {
	return s.Save(&File{Version: queueVersion, Entries: []Entry{}})
}

// UpdateEntryStatus updates one entry and saves.
func (s *Store) UpdateEntryStatus(id string, status EntryStatus, errMsg string) error {
	file, err := s.Load()
	if err != nil {
		return err
	}
	for i := range file.Entries {
		if file.Entries[i].ID != id {
			continue
		}
		file.Entries[i].Status = status
		file.Entries[i].Error = errMsg
		return s.Save(file)
	}
	return fmt.Errorf("queue entry not found: %s", id)
}
