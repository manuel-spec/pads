package downloader_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/internal/model"
	"pads/internal/state"
)

// newIgnoredRangeServer serves the whole body for every GET, Range header or
// not, and never advertises range support. Plenty of real servers behave this
// way behind a rewriting proxy.
func newIgnoredRangeServer(t *testing.T, content []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)
	return server
}

// A resume whose Range header is ignored must restart the transfer rather than
// append a second copy of the file onto the partial one.
func TestResumeWhenServerIgnoresRange(t *testing.T) {
	content := make([]byte, 64*1024)
	for i := range content {
		content[i] = byte(i % 251)
	}
	server := newIgnoredRangeServer(t, content)

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000
	if err := config.EnsureStateDirs(cfg); err != nil {
		t.Fatal(err)
	}

	// Stand in for an interrupted single-connection download: a quarter of the
	// bytes are already on disk and recorded in state.
	const partial = 16 * 1024
	tempDir := cfg.StatePath("tmp", "resume-ignored-range")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tempPath := filepath.Join(tempDir, "seg-0.part")
	if err := os.WriteFile(tempPath, content[:partial], 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "file.bin")
	st := &model.DownloadState{
		Version:   model.StateVersion,
		ID:        "resume-ignored-range",
		URL:       server.URL,
		Output:    output,
		TotalSize: int64(len(content)),
		TempDir:   tempDir,
		Segmented: false,
		CreatedAt: time.Now(),
		Segments: []model.Segment{{
			ID:              "seg-0",
			ByteStart:       0,
			ByteEnd:         int64(len(content)) - 1,
			BytesDownloaded: partial,
			TempPath:        tempPath,
			Status:          model.SegmentPending,
		}},
	}

	store := state.NewStore(cfg)
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}

	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{Resume: st, Store: store}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(content) {
		t.Fatalf("output is %d bytes, want %d (a second copy was appended)", len(got), len(content))
	}
	if !bytes.Equal(got, content) {
		t.Fatal("output does not match served content")
	}
}
