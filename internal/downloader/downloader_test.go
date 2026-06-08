package downloader_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/testutil"
)

func TestDownloadWithRangeSupport(t *testing.T) {
	content := []byte("pads downloader integration payload")
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
	})
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000

	output := filepath.Join(dir, "file.bin")
	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{
		URL:    server.URL,
		Output: output,
	}); err != nil {
		t.Fatalf("download failed: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", string(got), string(content))
	}
}

func TestDownloadSegmentedParallel(t *testing.T) {
	content := make([]byte, 256*1024)
	for i := range content {
		content[i] = byte(i % 251)
	}

	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
	})
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000
	cfg.MinSegmentSize = 64 * 1024
	cfg.InitialConnections = 4
	cfg.MaxConnections = 4

	output := filepath.Join(dir, "large.bin")
	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{
		URL:    server.URL,
		Output: output,
	}); err != nil {
		t.Fatalf("segmented download failed: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(got) != len(content) {
		t.Fatalf("size mismatch: got %d want %d", len(got), len(content))
	}
	for i := range content {
		if got[i] != content[i] {
			t.Fatalf("byte mismatch at %d", i)
		}
	}
}

func TestDownloadWithoutRangeFallsBack(t *testing.T) {
	content := []byte("no-range-fallback-payload")
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: false,
	})
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000
	cfg.InitialConnections = 4

	output := filepath.Join(dir, "file.bin")
	dl := downloader.New(cfg)
	if err := dl.Run(context.Background(), downloader.Options{
		URL:    server.URL,
		Output: output,
	}); err != nil {
		t.Fatalf("download failed: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", string(got), string(content))
	}
}
