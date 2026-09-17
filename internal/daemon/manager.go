package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"pads/internal/app"
	"pads/internal/queue"
)

// JobStatus describes the lifecycle state of a background job.
type JobStatus string

const (
	JobRunning  JobStatus = "running"
	JobPaused   JobStatus = "paused"
	JobComplete JobStatus = "complete"
	JobFailed   JobStatus = "failed"
)

// Job is one background download managed by the daemon.
type Job struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Output    string    `json:"output"`
	Status    JobStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	Bytes     int64     `json:"bytes_done"`
	Total     int64     `json:"total_size"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Manager runs background downloads and reports their status.
type Manager struct {
	app     *app.App
	mu      sync.Mutex
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc
	queueID string
}

// NewManager creates an empty job manager.
func NewManager(application *app.App) *Manager {
	return &Manager{
		app:     application,
		jobs:    make(map[string]*Job),
		cancels: make(map[string]context.CancelFunc),
	}
}

// Start launches a download in the background and returns its job ID.
func (m *Manager) Start(ctx context.Context, url, output string) (string, error) {
	if output == "" {
		name, err := app.FilenameForURL(url)
		if err != nil {
			return "", err
		}
		output = name
	}
	id := uuid.NewString()
	m.register(id, url, output, 0, 0)
	jobCtx, cancel := context.WithCancel(ctx)
	m.setCancel(id, cancel)
	go func() {
		defer m.unregisterCancel(id)
		m.finish(id, m.app.Download(jobCtx, url, output), jobCtx.Err())
	}()
	return id, nil
}

// ResumeJob resumes a saved download state in the background.
func (m *Manager) ResumeJob(ctx context.Context, id string) error {
	st, err := m.app.LoadState(id)
	if err != nil {
		return err
	}
	if m.isKnown(st.ID) {
		return fmt.Errorf("job %s already exists", st.ID)
	}
	m.register(st.ID, st.URL, st.Output, downloadedOf(st), st.TotalSize)
	jobCtx, cancel := context.WithCancel(ctx)
	m.setCancel(st.ID, cancel)
	go func() {
		defer m.unregisterCancel(st.ID)
		m.finish(st.ID, m.app.Resume(jobCtx, st.ID), jobCtx.Err())
	}()
	return nil
}

func downloadedOf(st *app.DownloadState) int64 {
	var done int64
	for _, seg := range st.Segments {
		done += seg.BytesDownloaded
	}
	return done
}

// Pause cancels a running job; progress stays resumable on disk.
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	cancel, running := m.cancels[id]
	job, known := m.jobs[id]
	m.mu.Unlock()

	if !running {
		return fmt.Errorf("job %s is not running", id)
	}
	cancel()
	if known {
		m.mu.Lock()
		job.Status = JobPaused
		job.UpdatedAt = time.Now()
		m.mu.Unlock()
	}
	return nil
}

// List returns a snapshot of all jobs.
func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		out = append(out, *job)
	}
	return out
}

// Get returns one job by ID.
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *job, true
}

// WaitJob blocks until a job leaves the running state.
func (m *Manager) WaitJob(id string, timeout time.Duration) (Job, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, ok := m.Get(id)
		if ok && job.Status != JobRunning {
			return job, true
		}
		time.Sleep(20 * time.Millisecond)
	}
	job, ok := m.Get(id)
	return job, ok
}

// StartQueue processes pending queue entries sequentially in the background.
func (m *Manager) StartQueue(ctx context.Context) (string, error) {
	m.mu.Lock()
	if m.queueID != "" {
		if job, ok := m.jobs[m.queueID]; ok && job.Status == JobRunning {
			m.mu.Unlock()
			return "", fmt.Errorf("queue worker is already running")
		}
	}
	id := uuid.NewString()
	m.queueID = id
	m.mu.Unlock()

	go m.runQueue(ctx, id)
	return id, nil
}

func (m *Manager) runQueue(ctx context.Context, jobID string) {
	defer func() {
		m.mu.Lock()
		delete(m.cancels, jobID)
		m.mu.Unlock()
	}()

	entries, err := m.app.QueueList()
	if err != nil {
		m.mu.Lock()
		m.jobs[jobID] = &Job{ID: jobID, Status: JobFailed, Error: err.Error(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
		m.mu.Unlock()
		return
	}

	for _, entry := range entries {
		if entry.Status != queue.EntryPending || ctx.Err() != nil {
			continue
		}
		if err := m.runQueueEntry(ctx, jobID, entry); err != nil {
			if ctx.Err() != nil {
				return
			}
		}
	}

	m.mu.Lock()
	if job, ok := m.jobs[jobID]; ok {
		if ctx.Err() != nil {
			job.Status = JobPaused
		} else {
			job.Status = JobComplete
		}
		job.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
}

func (m *Manager) runQueueEntry(ctx context.Context, jobID string, entry queue.Entry) error {
	if err := m.app.QueueUpdateStatus(entry.ID, queue.EntryRunning, ""); err != nil {
		return err
	}

	m.mu.Lock()
	m.jobs[jobID] = &Job{
		ID:        jobID,
		URL:       entry.URL,
		Output:    entry.Output,
		Status:    JobRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.mu.Unlock()

	err := m.app.Download(ctx, entry.URL, entry.Output)
	switch {
	case ctx.Err() != nil:
		_ = m.app.QueueUpdateStatus(entry.ID, queue.EntryPending, "")
		return ctx.Err()
	case err != nil:
		return m.app.QueueUpdateStatus(entry.ID, queue.EntryFailed, err.Error())
	default:
		return m.app.QueueUpdateStatus(entry.ID, queue.EntryComplete, "")
	}
}

func (m *Manager) register(id, url, output string, bytesDone, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[id] = &Job{
		ID:        id,
		URL:       url,
		Output:    output,
		Status:    JobRunning,
		Bytes:     bytesDone,
		Total:     total,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (m *Manager) setCancel(id string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancels[id] = cancel
}

func (m *Manager) unregisterCancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, id)
}

func (m *Manager) isKnown(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.jobs[id]
	return ok
}

func (m *Manager) finish(id string, err error, ctxErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return
	}
	job.UpdatedAt = time.Now()
	switch {
	case ctxErr != nil:
		job.Status = JobPaused
	case err != nil:
		job.Status = JobFailed
		job.Error = err.Error()
	default:
		job.Status = JobComplete
	}
}
