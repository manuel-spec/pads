package daemon

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pads/internal/app"
	"pads/testutil"
)

// newTestManager builds a manager over a throwaway home directory so the test
// never touches the developer's real ~/.pads.
func newTestManager(t *testing.T) (*Manager, *app.App) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	application, err := app.New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if err := application.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	manager := NewManager(application)
	t.Cleanup(manager.Close)
	return manager, application
}

func waitForJob(t *testing.T, manager *Manager, id string, want JobStatus) Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var job Job
	for time.Now().Before(deadline) {
		current, ok := manager.Get(id)
		if ok {
			job = current
			if job.Status == want {
				return job
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s status = %s (%s), want %s", id, job.Status, job.Error, want)
	return job
}

// A job must outlive the HTTP request that started it. Deriving job contexts
// from the request context cancelled every download the instant the handler
// wrote its 202.
func TestStartedJobOutlivesRequestContext(t *testing.T) {
	content := bytes.Repeat([]byte("pads"), 32*1024)
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
		ChunkSize:    4096,
		ReadDelay:    time.Millisecond,
	})
	defer server.Close()

	manager, _ := newTestManager(t)
	token := "test-token"
	api := NewServer(manager, token)
	httpServer := httptest.NewServer(api.http.Handler)
	defer httpServer.Close()

	output := filepath.Join(t.TempDir(), "file.bin")
	client := &Client{addr: httpServer.Listener.Addr().String(), token: token, http: httpServer.Client()}

	id, err := client.Start(context.Background(), server.URL, output)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	job := waitForJob(t, manager, id, JobComplete)
	if job.ID != id {
		t.Fatalf("job id = %s, want %s", job.ID, id)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("downloaded file does not match served content")
	}
}

// Close must stop running work and wait for it, so the daemon does not exit
// while a download is still writing.
func TestCloseCancelsRunningJobs(t *testing.T) {
	content := bytes.Repeat([]byte("slow"), 256*1024)
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
		ChunkSize:    512,
		ReadDelay:    20 * time.Millisecond,
	})
	defer server.Close()

	manager, _ := newTestManager(t)
	output := filepath.Join(t.TempDir(), "file.bin")

	id, err := manager.Start(server.URL, output)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForJob(t, manager, id, JobRunning)

	done := make(chan struct{})
	go func() {
		manager.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Close did not return")
	}

	job, ok := manager.Get(id)
	if !ok {
		t.Fatal("job disappeared")
	}
	if job.Status != JobPaused {
		t.Fatalf("job status = %s, want %s", job.Status, JobPaused)
	}

	if _, err := manager.Start(server.URL, output); err != ErrShuttingDown {
		t.Fatalf("start after close = %v, want %v", err, ErrShuttingDown)
	}
}

// A job's byte counters must track the download, not stay frozen at whatever
// was true when the job was registered. The extension popup renders them.
func TestJobProgressTracksDownload(t *testing.T) {
	content := bytes.Repeat([]byte("pads"), 128*1024)
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
		ChunkSize:    1024,
		ReadDelay:    5 * time.Millisecond,
	})
	defer server.Close()

	manager, _ := newTestManager(t)
	output := filepath.Join(t.TempDir(), "file.bin")

	id, err := manager.Start(server.URL, output)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Catch it mid-flight: bytes and total must both be populated from state.
	deadline := time.Now().Add(10 * time.Second)
	sawProgress := false
	for time.Now().Before(deadline) {
		job, ok := manager.Get(id)
		if ok && job.Bytes > 0 && job.Total == int64(len(content)) {
			sawProgress = true
			break
		}
		if ok && job.Status != JobRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawProgress {
		t.Fatal("job never reported in-flight progress")
	}

	final := waitForJob(t, manager, id, JobComplete)
	if final.Bytes != int64(len(content)) || final.Total != int64(len(content)) {
		t.Fatalf("completed job = %d/%d bytes, want %d/%d",
			final.Bytes, final.Total, len(content), len(content))
	}
}
