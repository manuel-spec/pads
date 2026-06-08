package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"
)

// FileServerOptions configures the test HTTP file server.
type FileServerOptions struct {
	Content      []byte
	ContentType  string
	SupportRange bool
	ReadDelay    time.Duration
	ChunkSize    int
}

// NewFileServer returns an httptest server serving static content.
func NewFileServer(opts FileServerOptions) *httptest.Server {
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", opts.ContentType)
			w.Header().Set("Content-Length", strconv.Itoa(len(opts.Content)))
			if opts.SupportRange {
				w.Header().Set("Accept-Ranges", "bytes")
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", opts.ContentType)
		if !opts.SupportRange {
			w.Header().Set("Content-Length", strconv.Itoa(len(opts.Content)))
			_, _ = w.Write(opts.Content)
			return
		}

		w.Header().Set("Accept-Ranges", "bytes")
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(opts.Content)))
			_, _ = w.Write(opts.Content)
			return
		}

		start, end, err := parseRange(rangeHeader, len(opts.Content))
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
			return
		}

		chunk := opts.Content[start : end+1]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(opts.Content)))
		w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
		w.WriteHeader(http.StatusPartialContent)
		writeThrottled(w, chunk, opts.ChunkSize, opts.ReadDelay)
	})

	return httptest.NewServer(handler)
}

func writeThrottled(w http.ResponseWriter, data []byte, chunkSize int, delay time.Duration) {
	if chunkSize <= 0 {
		chunkSize = len(data)
	}
	for offset := 0; offset < len(data); offset += chunkSize {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		_, _ = w.Write(data[offset:end])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func parseRange(header string, size int) (int, int, error) {
	if len(header) < 7 || header[:6] != "bytes=" {
		return 0, 0, fmt.Errorf("invalid range header")
	}
	var start, end int
	if _, err := fmt.Sscanf(header[6:], "%d-%d", &start, &end); err != nil {
		return 0, 0, fmt.Errorf("parse range: %w", err)
	}
	if start < 0 || end < start || end >= size {
		return 0, 0, fmt.Errorf("invalid byte range")
	}
	return start, end, nil
}
