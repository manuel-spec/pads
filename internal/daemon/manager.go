package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// ErrShuttingDown is returned when the daemon can no longer accept work.
var ErrShuttingDown = errors.New("daemon is shutting down")

// Manager runs background downloads and reports their status.
type Manager struct {
	app        *app.App
	baseCtx    context.Context
	baseCancel context.CancelFunc
	wg         sync.WaitGroup

	mu      sync.Mutex
	closed  bool
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc
	queueID string
}

// NewManager creates an empty job manager.
func NewManager(application *app.App) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		app:        application,
		baseCtx:    ctx,
		baseCancel: cancel,
		jobs:       make(map[string]*Job),
		cancels:    make(map[string]context.CancelFunc),
	}
}

// Close cancels every running job and waits for it to stop. Partial progress
// stays on disk and remains resumable. Close is safe to call more than once.
func (m *Manager) Close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.baseCancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// claimJobLocked registers a job and returns its context. Jobs descend from the
// manager's own context, never from a request context, so a download outlives
// the HTTP call that started it.
func (m *Manager) claimJobLocked(job *Job) context.Context {
	ctx, cancel := context.WithCancel(m.baseCtx)
	m.cancels[job.ID] = cancel
	m.jobs[job.ID] = job
	m.wg.Add(1)
	return ctx
}

// endJob releases a finished job's cancel function and its wait-group slot.
func (m *Manager) endJob(id string) {
	m.mu.Lock()
	delete(m.cancels, id)
	m.mu.Unlock()
	m.wg.Done()
}

// Start launches a download in the background and returns its job ID.
func (m *Manager) Start(url, output string) (string, error) {
	if output == "" {
		name, err := app.FilenameForURL(url)
		if err != nil {
			return "", err
		}
		output = name
	}
	id := uuid.NewString()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", ErrShuttingDown
	}
	jobCtx := m.claimJobLocked(newJob(id, url, output, 0, 0))
	m.mu.Unlock()

	go func() {
		defer m.endJob(id)
		m.finish(id, m.app.DownloadWithID(jobCtx, id, url, output), jobCtx.Err())
	}()
	return id, nil
}

// ResumeJob resumes a saved download state in the background.
func (m *Manager) ResumeJob(id string) error {
	st, err := m.app.LoadState(id)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrShuttingDown
	}
	if job, ok := m.jobs[st.ID]; ok && job.Status == JobRunning {
		m.mu.Unlock()
		return fmt.Errorf("job %s is already running", st.ID)
	}
	jobCtx := m.claimJobLocked(newJob(st.ID, st.URL, st.Output, downloadedOf(st), st.TotalSize))
	m.mu.Unlock()

	go func() {
		defer m.endJob(st.ID)
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

// List returns a snapshot of all jobs with their byte counters refreshed.
func (m *Manager) List() []Job {
	m.mu.Lock()
	ids := make([]string, 0, len(m.jobs))
	for id := range m.jobs {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	progress := m.readProgress(ids)

	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		snap, ok := progress[job.ID]
		applyProgress(job, snap, ok)
		out = append(out, *job)
	}
	return out
}

// Get returns one job by ID with its byte counters refreshed.
func (m *Manager) Get(id string) (Job, bool) {
	progress := m.readProgress([]string{id})

	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	snap, found := progress[id]
	applyProgress(job, snap, found)
	return *job, true
}

// progressSnapshot is one download's byte counters as persisted on disk.
type progressSnapshot struct {
	bytes int64
	total int64
}

// readProgress reads persisted counters without holding the lock. A job record
// only knows what was true when it was created; the download's state file is
// what the downloader keeps current, so progress has to come from there.
func (m *Manager) readProgress(ids []string) map[string]progressSnapshot {
	store := m.app.StateStore()
	out := make(map[string]progressSnapshot, len(ids))

	for _, id := range ids {
		st, err := store.Load(id)
		if err != nil {
			// No state file: the download finished and cleaned up, or this is
			// the queue worker, whose ID is not a download ID.
			continue
		}
		var done int64
		for _, seg := range st.Segments {
			done += seg.BytesDownloaded
		}
		out[id] = progressSnapshot{bytes: done, total: st.TotalSize}
	}
	return out
}

// applyProgress folds a snapshot into a job. State files are removed once a
// download completes, so a missing snapshot keeps the last reading rather than
// resetting the job to zero.
func applyProgress(job *Job, snap progressSnapshot, found bool) {
	if !found {
		return
	}
	job.Bytes = snap.bytes
	if snap.total > 0 {
		job.Total = snap.total
	}
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
func (m *Manager) StartQueue() (string, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", ErrShuttingDown
	}
	if job, ok := m.jobs[m.queueID]; ok && job.Status == JobRunning {
		m.mu.Unlock()
		return "", fmt.Errorf("queue worker is already running")
	}
	id := uuid.NewString()
	m.queueID = id
	jobCtx := m.claimJobLocked(newJob(id, "", "", 0, 0))
	m.mu.Unlock()

	go func() {
		defer m.endJob(id)
		m.runQueue(jobCtx, id)
	}()
	return id, nil
}

func (m *Manager) runQueue(ctx context.Context, jobID string) {
	entries, err := m.app.QueueList()
	if err != nil {
		m.finish(jobID, fmt.Errorf("load queue: %w", err), nil)
		return
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}
		if entry.Status != queue.EntryPending {
			continue
		}
		// A failed entry is recorded on the entry itself; the worker moves on.
		// Only bookkeeping errors stop the run.
		if err := m.runQueueEntry(ctx, jobID, entry); err != nil && ctx.Err() == nil {
			m.finish(jobID, fmt.Errorf("queue entry %s: %w", entry.ID, err), nil)
			return
		}
	}
	m.finish(jobID, nil, ctx.Err())
}

func (m *Manager) runQueueEntry(ctx context.Context, jobID string, entry queue.Entry) error {
	if err := m.app.QueueUpdateStatus(entry.ID, queue.EntryRunning, ""); err != nil {
		return err
	}
	m.setJobTarget(jobID, entry.URL, entry.Output)

	err := m.app.Download(ctx, entry.URL, entry.Output)
	switch {
	case ctx.Err() != nil:
		// Leave the entry pending so a later queue start picks it up again.
		return m.app.QueueUpdateStatus(entry.ID, queue.EntryPending, "")
	case err != nil:
		return m.app.QueueUpdateStatus(entry.ID, queue.EntryFailed, err.Error())
	default:
		return m.app.QueueUpdateStatus(entry.ID, queue.EntryComplete, "")
	}
}

func newJob(id, url, output string, bytesDone, total int64) *Job {
	now := time.Now()
	return &Job{
		ID:        id,
		URL:       url,
		Output:    output,
		Status:    JobRunning,
		Bytes:     bytesDone,
		Total:     total,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// setJobTarget points the queue worker's job at the entry it is downloading.
func (m *Manager) setJobTarget(id, url, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return
	}
	job.URL = url
	job.Output = output
	job.UpdatedAt = time.Now()
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
		if info, statErr := os.Stat(job.Output); statErr == nil {
			job.Bytes = info.Size()
			job.Total = info.Size()
		}
	}
}
