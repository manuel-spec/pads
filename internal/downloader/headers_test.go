package downloader_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/internal/model"
	"pads/internal/state"
)

// authServerOptions configures the authenticated test server.
type authServerOptions struct {
	content []byte
	cookie  string
	// chunk and delay throttle the body so a test can interrupt a transfer
	// rather than race it.
	chunk    int
	delay    time.Duration
	requests *int64
}

// newAuthenticatedServer serves content only to requests carrying the expected
// cookie, and answers 401 otherwise. Every request is counted so a test can
// confirm the header reached the segment workers, not just the probe.
func newAuthenticatedServer(t *testing.T, opts authServerOptions) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(opts.requests, 1)
		if r.Header.Get("Cookie") != opts.cookie {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")

		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(opts.content)))
			w.WriteHeader(http.StatusOK)
			return
		}

		body := opts.content
		status := http.StatusOK
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			var start, end int
			if _, err := fmtSscan(rangeHeader, &start, &end); err != nil {
				http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			if start < 0 || end >= len(opts.content) || end < start {
				http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			body = opts.content[start : end+1]
			status = http.StatusPartialContent
			w.Header().Set("Content-Range",
				"bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(len(opts.content)))
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(status)
		writeBody(w, body, opts.chunk, opts.delay)
	}))
	t.Cleanup(server.Close)
	return server
}

func writeBody(w http.ResponseWriter, body []byte, chunk int, delay time.Duration) {
	if chunk <= 0 {
		chunk = len(body)
	}
	for offset := 0; offset < len(body); offset += chunk {
		end := offset + chunk
		if end > len(body) {
			end = len(body)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		if _, err := w.Write(body[offset:end]); err != nil {
			return
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func fmtSscan(header string, start, end *int) (int, error) {
	trimmed := strings.TrimPrefix(header, "bytes=")
	parts := strings.SplitN(trimmed, "-", 2)
	if len(parts) != 2 {
		return 0, os.ErrInvalid
	}
	s, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	e, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	*start, *end = s, e
	return 2, nil
}

func headerTestConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000
	cfg.MinSegmentSize = 16 * 1024
	cfg.InitialConnections = 2
	cfg.MaxConnections = 4
	return cfg, dir
}

func TestDownloadForwardsHeadersToEveryRequest(t *testing.T) {
	content := make([]byte, 256*1024)
	for i := range content {
		content[i] = byte(i % 253)
	}

	var requests int64
	const cookie = "session=secret"
	server := newAuthenticatedServer(t, authServerOptions{
		content: content, cookie: cookie, requests: &requests,
		chunk: 2048, delay: 3 * time.Millisecond,
	})

	cfg, dir := headerTestConfig(t)
	output := filepath.Join(dir, "file.bin")

	dl := downloader.New(cfg)
	err := dl.Run(context.Background(), downloader.Options{
		URL:     server.URL,
		Output:  output,
		Store:   state.NewStore(cfg),
		Headers: map[string]string{"Cookie": cookie},
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("downloaded content does not match")
	}

	// The probe alone is two requests; a segmented download makes more. Every
	// one of them had to carry the cookie or the server would have refused it.
	if atomic.LoadInt64(&requests) < 3 {
		t.Fatalf("only %d requests reached the server", requests)
	}
}

func TestDownloadWithoutHeadersIsRejected(t *testing.T) {
	content := make([]byte, 64*1024)
	var requests int64
	server := newAuthenticatedServer(t, authServerOptions{
		content: content, cookie: "session=secret", requests: &requests,
	})

	cfg, dir := headerTestConfig(t)
	dl := downloader.New(cfg)
	err := dl.Run(context.Background(), downloader.Options{
		URL:    server.URL,
		Output: filepath.Join(dir, "file.bin"),
		Store:  state.NewStore(cfg),
	})
	if err == nil {
		t.Fatal("expected the download to fail without the session cookie")
	}
}

// A resumed download has to authenticate the same way the original did, so the
// headers must survive in the state file and be used again on resume.
func TestResumeCarriesSavedHeaders(t *testing.T) {
	content := make([]byte, 256*1024)
	for i := range content {
		content[i] = byte(i % 251)
	}

	var requests int64
	const cookie = "session=secret"
	server := newAuthenticatedServer(t, authServerOptions{
		content: content, cookie: cookie, requests: &requests,
		chunk: 2048, delay: 3 * time.Millisecond,
	})

	cfg, dir := headerTestConfig(t)
	store := state.NewStore(cfg)
	output := filepath.Join(dir, "file.bin")

	// Start a download and stop it once its state has been written, rather than
	// racing a sleep against the transfer.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		dl := downloader.New(cfg)
		done <- dl.Run(ctx, downloader.Options{
			URL:     server.URL,
			Output:  output,
			Store:   store,
			Headers: map[string]string{"Cookie": cookie},
		})
	}()

	saved := waitForSavedState(t, store)
	cancel()
	<-done

	if saved.Headers["Cookie"] != cookie {
		t.Fatalf("saved headers = %v, want the session cookie", saved.Headers)
	}

	// Reload from disk so the resume runs off persisted bytes, not the struct
	// the first run happened to leave behind.
	reloaded, err := store.Load(saved.ID)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if reloaded.Headers["Cookie"] != cookie {
		t.Fatalf("reloaded headers = %v, want the session cookie", reloaded.Headers)
	}

	state.NormalizeForResume(reloaded)
	if err := store.Save(reloaded); err != nil {
		t.Fatal(err)
	}

	// Resume passes no headers of its own; the only way this can authenticate
	// is by using the ones it loaded.
	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{Resume: reloaded, Store: store}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("resumed content does not match")
	}
}

// waitForSavedState polls until the downloader has written its initial state.
func waitForSavedState(t *testing.T, store *state.Store) *model.DownloadState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		states, err := store.List()
		if err == nil && len(states) > 0 {
			return states[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("download never wrote its state")
	return nil
}

// State can hold session cookies, so it must not be world readable.
func TestStateFileIsOwnerOnly(t *testing.T) {
	content := make([]byte, 128*1024)
	var requests int64
	const cookie = "session=secret"
	server := newAuthenticatedServer(t, authServerOptions{
		content: content, cookie: cookie, requests: &requests,
		chunk: 2048, delay: 3 * time.Millisecond,
	})

	cfg, dir := headerTestConfig(t)
	store := state.NewStore(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		dl := downloader.New(cfg)
		done <- dl.Run(ctx, downloader.Options{
			URL:     server.URL,
			Output:  filepath.Join(dir, "file.bin"),
			Store:   store,
			Headers: map[string]string{"Cookie": cookie},
		})
	}()

	saved := waitForSavedState(t, store)
	cancel()
	<-done

	info, err := os.Stat(store.Path(saved.ID))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %o, want 600", perm)
	}
}
