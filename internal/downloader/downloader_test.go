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

func TestDownloadSingleConnection(t *testing.T) {
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
