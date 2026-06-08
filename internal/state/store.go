package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pads/internal/config"
	"pads/internal/model"
)

// ErrNotFound indicates that a download state file does not exist.
var ErrNotFound = errors.New("download state not found")

// ErrCorrupt indicates that a state file failed validation.
var ErrCorrupt = errors.New("download state is corrupt")

// Store persists versioned download state.
type Store struct {
	dir string
}

// NewStore creates a state store under the configured state directory.
func NewStore(cfg *config.Config) *Store {
	return &Store{dir: cfg.StatePath("state")}
}

// Path returns the file path for a download ID.
func (s *Store) Path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// Save writes download state atomically.
func (s *Store) Save(st *model.DownloadState) error {
	if err := Validate(st); err != nil {
		return err
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	st.Version = model.StateVersion
	st.UpdatedAt = time.Now()
	if st.CreatedAt.IsZero() {
		st.CreatedAt = st.UpdatedAt
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := s.Path(st.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, s.Path(st.ID)); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}

// Load reads and validates download state.
func (s *Store) Load(id string) (*model.DownloadState, error) {
	data, err := os.ReadFile(s.Path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	st := &model.DownloadState{}
	if err := json.Unmarshal(data, st); err != nil {
		_ = s.quarantine(id, data)
		return nil, fmt.Errorf("%w: parse state: %v", ErrCorrupt, err)
	}

	if err := Validate(st); err != nil {
		_ = s.quarantine(id, data)
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	return st, nil
}

// Delete removes persisted state for a download.
func (s *Store) Delete(id string) error {
	err := os.Remove(s.Path(id))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete state: %w", err)
	}
	return nil
}

// List returns all non-complete download states.
func (s *Store) List() ([]*model.DownloadState, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state directory: %w", err)
	}

	var states []*model.DownloadState
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		st, err := s.Load(id)
		if err != nil {
			continue
		}
		states = append(states, st)
	}
	return states, nil
}

// Validate checks a download state for consistency.
func Validate(st *model.DownloadState) error {
	if st == nil {
		return fmt.Errorf("state is nil")
	}
	if st.ID == "" {
		return fmt.Errorf("download id is required")
	}
	if st.URL == "" {
		return fmt.Errorf("url is required")
	}
	if st.Output == "" {
		return fmt.Errorf("output path is required")
	}
	if st.Version > model.StateVersion {
		return fmt.Errorf("unsupported state version %d", st.Version)
	}
	if st.Complete {
		return nil
	}
	if st.TotalSize <= 0 {
		return fmt.Errorf("total size must be positive")
	}
	if len(st.Segments) == 0 {
		return fmt.Errorf("segments are required")
	}
	return model.ValidateSegmentCoverage(st.Segments, st.TotalSize)
}

// NormalizeForResume resets active segments to pending for restart.
func NormalizeForResume(st *model.DownloadState) {
	st.Paused = false
	st.Complete = false
	for i := range st.Segments {
		switch st.Segments[i].Status {
		case model.SegmentComplete:
			continue
		case model.SegmentFailed:
			st.Segments[i].Status = model.SegmentPending
		default:
			st.Segments[i].Status = model.SegmentPending
		}
	}
}

func (s *Store) quarantine(id string, data []byte) error {
	path := filepath.Join(s.dir, id+".corrupt")
	return os.WriteFile(path, data, 0o644)
}
