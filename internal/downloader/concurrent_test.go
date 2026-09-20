package downloader_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"pads/internal/config"
	"pads/internal/downloader"
	"pads/testutil"
)

// TestDownloaderConcurrentRuns verifies that a single *downloader.Downloader instance
// can be used concurrently by multiple goroutines downloading different URLs/headers without race conditions.
func TestDownloaderConcurrentRuns(t *testing.T) {
	const count = 5
	servers := make([]*httptest.Server, count)
	contents := make([][]byte, count)

	for i := 0; i < count; i++ {
		content := []byte(fmt.Sprintf("concurrent payload stream #%d", i))
		contents[i] = content
		servers[i] = testutil.NewFileServer(testutil.FileServerOptions{
			Content:      content,
			SupportRange: true,
		})
		defer servers[i].Close()
	}

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, ".pads")
	cfg.ProbeTimeoutMS = 2000

	dl := downloader.New(cfg)

	var wg sync.WaitGroup
	errCh := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			out := filepath.Join(dir, fmt.Sprintf("out-%d.bin", idx))
			headers := map[string]string{
				"X-Custom-Client": fmt.Sprintf("client-%d", idx),
			}
			err := dl.Run(context.Background(), downloader.Options{
				URL:     servers[idx].URL,
				Output:  out,
				Headers: headers,
			})
			if err != nil {
				errCh <- fmt.Errorf("worker %d failed: %w", idx, err)
				return
			}

			data, err := os.ReadFile(out)
			if err != nil {
				errCh <- fmt.Errorf("worker %d read failed: %w", idx, err)
				return
			}
			if string(data) != string(contents[idx]) {
				errCh <- fmt.Errorf("worker %d content mismatch: got %q, want %q", idx, string(data), string(contents[idx]))
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}
}
