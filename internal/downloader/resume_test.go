package downloader_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/internal/model"
	"pads/internal/state"
	"pads/testutil"
)

func TestResumeSegmentedDownload(t *testing.T) {
	content := make([]byte, 512*1024)
	for i := range content {
		content[i] = byte(i % 200)
	}

	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
		ReadDelay:    5 * time.Millisecond,
		ChunkSize:    2048,
	})
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000
	cfg.MinSegmentSize = 64 * 1024
	cfg.InitialConnections = 2
	cfg.MaxConnections = 4

	store := state.NewStore(cfg)
	output := filepath.Join(dir, "file.bin")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		dl := downloader.New(cfg)
		done <- dl.Run(ctx, downloader.Options{
			URL:    server.URL,
			Output: output,
			Store:  store,
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancelled download")
	}

	states, err := store.List()
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("expected persisted state after cancel")
	}

	st := states[0]
	state.NormalizeForResume(st)
	if err := store.Save(st); err != nil {
		t.Fatalf("save resume state: %v", err)
	}

	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{
		Resume: st,
		Store:  store,
	}); err != nil {
		t.Fatalf("resume download failed: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(got) != len(content) {
		t.Fatalf("size mismatch: got %d want %d", len(got), len(content))
	}
}

func TestResumeFromSavedPartialState(t *testing.T) {
	content := []byte("left-half-right-half-payload-xx")
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
	})
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	store := state.NewStore(cfg)
	output := filepath.Join(dir, "file.bin")

	leftLen := int64(15)
	st := &model.DownloadState{
		Version:   model.StateVersion,
		ID:        "partial-1",
		URL:       server.URL,
		Output:    output,
		TotalSize: int64(len(content)),
		Segmented: true,
		TempDir:   filepath.Join(cfg.StatePath("tmp"), "partial-1"),
		Segments: []model.Segment{
			{
				ID:              "seg-0",
				ByteStart:       0,
				ByteEnd:         leftLen - 1,
				BytesDownloaded: leftLen,
				Status:          model.SegmentComplete,
				TempPath:        "",
			},
			{
				ID:        "seg-1",
				ByteStart: leftLen,
				ByteEnd:   int64(len(content)) - 1,
				Status:    model.SegmentPending,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := os.MkdirAll(st.TempDir, 0o755); err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	leftPath := filepath.Join(st.TempDir, "seg-0.part")
	if err := os.WriteFile(leftPath, content[:leftLen], 0o644); err != nil {
		t.Fatalf("write left segment: %v", err)
	}
	st.Segments[0].TempPath = leftPath

	if err := store.Save(st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{
		Resume: st,
		Store:  store,
	}); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: %q", string(got))
	}
}

func TestResumeValidatesSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	store := state.NewStore(cfg)

	st := &model.DownloadState{
		Version:   model.StateVersion,
		ID:        "bad-size",
		URL:       "https://example.com",
		Output:    filepath.Join(dir, "out.bin"),
		TotalSize: 9999,
		Segmented: true,
		TempDir:   filepath.Join(dir, "tmp"),
		Segments: []model.Segment{
			{ID: "seg-0", ByteStart: 0, ByteEnd: 9998, Status: model.SegmentPending},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Save(st); err != nil {
		t.Fatalf("save: %v", err)
	}

	dl := downloader.New(cfg)
	err := dl.Run(context.Background(), downloader.Options{Resume: st, Store: store})
	if err == nil {
		t.Fatal("expected size validation failure")
	}
}
