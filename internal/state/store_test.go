package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pads/internal/config"
	"pads/internal/model"
)

func sampleState(id string) *model.DownloadState {
	return &model.DownloadState{
		Version:   model.StateVersion,
		ID:        id,
		URL:       "https://example.com/file.bin",
		Output:    "/tmp/file.bin",
		TotalSize: 100,
		Segmented: true,
		TempDir:   "/tmp/pads/id",
		Segments: []model.Segment{
			{ID: "seg-0", ByteStart: 0, ByteEnd: 99, Status: model.SegmentPending},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	store := NewStore(cfg)

	st := sampleState("dl-1")
	if err := store.Save(st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := store.Load("dl-1")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loaded.URL != st.URL {
		t.Fatalf("url mismatch")
	}
}

func TestStoreLoadCorruptQuarantined(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	store := NewStore(cfg)

	if err := os.MkdirAll(cfg.StatePath("state"), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	path := store.Path("bad")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	_, err := store.Load("bad")
	if err == nil {
		t.Fatal("expected corrupt state error")
	}

	corruptPath := filepath.Join(cfg.StatePath("state"), "bad.corrupt")
	if _, err := os.Stat(corruptPath); err != nil {
		t.Fatalf("expected quarantined file: %v", err)
	}
}

func TestNormalizeForResume(t *testing.T) {
	st := sampleState("dl-2")
	st.Segments[0].Status = model.SegmentActive
	st.Paused = true
	NormalizeForResume(st)
	if st.Paused {
		t.Fatal("expected paused cleared")
	}
	if st.Segments[0].Status != model.SegmentPending {
		t.Fatalf("expected pending, got %s", st.Segments[0].Status)
	}
}
