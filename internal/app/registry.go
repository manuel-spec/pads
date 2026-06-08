package app

import (
	"context"
	"fmt"
	"sync"
)

// Registry tracks in-memory active downloads for pause and status.
type Registry struct {
	mu      sync.Mutex
	active  map[string]context.CancelFunc
	pauseCh map[string]chan struct{}
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		active:  make(map[string]context.CancelFunc),
		pauseCh: make(map[string]chan struct{}),
	}
}

// Register associates a download ID with a cancel function.
func (r *Registry) Register(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[id] = cancel
}

// Unregister removes a download ID from the registry.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, id)
	delete(r.pauseCh, id)
}

// ActiveIDs returns currently registered download IDs.
func (r *Registry) ActiveIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.active))
	for id := range r.active {
		ids = append(ids, id)
	}
	return ids
}

// IsActive reports whether a download is currently running in memory.
func (r *Registry) IsActive(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.active[id]
	return ok
}

// Pause cancels an active download by ID.
func (r *Registry) Pause(id string) error {
	r.mu.Lock()
	cancel, ok := r.active[id]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("download %s is not active", id)
	}
	cancel()
	return nil
}
