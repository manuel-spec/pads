package queue

import (
	"testing"

	"pads/internal/config"
)

func TestQueueAddListRemove(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	store := NewStore(cfg)

	entry, err := store.Add("https://example.com/a.bin", "a.bin")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	file, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(file.Entries))
	}

	if err := store.Remove(entry.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	file, err = store.Load()
	if err != nil {
		t.Fatalf("load after remove: %v", err)
	}
	if len(file.Entries) != 0 {
		t.Fatalf("expected empty queue")
	}
}

func TestQueueClear(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	store := NewStore(cfg)

	if _, err := store.Add("https://example.com/a.bin", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	file, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Entries) != 0 {
		t.Fatalf("expected empty queue after clear")
	}
}
