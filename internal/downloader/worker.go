package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"pads/internal/model"
	"pads/internal/ui"
	"pads/internal/util"
	"pads/internal/writer"
)

// HTTPStatusError indicates a non-retryable or terminal HTTP response.
type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.Message)
}

func (d *Downloader) downloadSegment(
	ctx context.Context,
	url string,
	seg *model.Segment,
	tempDir string,
	bar *ui.Bar,
) error {
	seg.Status = model.SegmentActive
	return d.withRetries(ctx, func() error {
		return d.fetchSegmentRange(ctx, url, seg, tempDir, bar)
	})
}

func (d *Downloader) fetchSegmentRange(
	ctx context.Context,
	url string,
	seg *model.Segment,
	tempDir string,
	bar *ui.Bar,
) error {
	segWriter, err := writer.OpenSegmentTemp(tempDir, seg.ID)
	if err != nil {
		return err
	}
	defer segWriter.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create segment request: %w", err)
	}
	req.Header.Set("Range", formatRange(seg.ByteStart, seg.ByteEnd))

	resp, err := util.DoRequest(ctx, d.client, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkSegmentResponse(resp, seg); err != nil {
		return err
	}

	expected := seg.ByteLength()
	written, err := copyWithProgress(resp.Body, segWriter, bar, expected)
	if err != nil {
		return err
	}
	if written != expected {
		return fmt.Errorf("segment %s: expected %d bytes, got %d", seg.ID, expected, written)
	}

	if err := segWriter.Close(); err != nil {
		return err
	}

	seg.TempPath = segWriter.Path()
	seg.BytesDownloaded = written
	seg.Status = model.SegmentComplete
	seg.UpdatedAt = time.Now()
	return nil
}

func checkSegmentResponse(resp *http.Response, seg *model.Segment) error {
	switch resp.StatusCode {
	case http.StatusPartialContent:
		return nil
	case http.StatusOK:
		// Some servers ignore Range and return the full body for single-segment downloads.
		if seg.ByteStart == 0 {
			return nil
		}
		return &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("server ignored range for segment %s", seg.ID),
		}
	case http.StatusRequestedRangeNotSatisfiable:
		return &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("range not satisfiable for segment %s [%d-%d]", seg.ID, seg.ByteStart, seg.ByteEnd),
		}
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("server busy for segment %s", seg.ID),
		}
	default:
		return &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("unexpected response for segment %s", seg.ID),
		}
	}
}

func formatRange(start, end int64) string {
	return fmt.Sprintf("bytes=%d-%d", start, end)
}

func copyWithProgress(src io.Reader, dst *writer.SegmentWriter, bar *ui.Bar, limit int64) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64

	for written < limit {
		toRead := int64(len(buf))
		remaining := limit - written
		if remaining < toRead {
			toRead = remaining
		}

		n, readErr := src.Read(buf[:toRead])
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return written, err
			}
			written += int64(n)
			bar.Add(int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return written, fmt.Errorf("read segment body: %w", readErr)
		}
	}
	return written, nil
}

func (d *Downloader) withRetries(ctx context.Context, fn func() error) error {
	backoff := time.Duration(d.cfg.RetryBackoffMS) * time.Millisecond
	var lastErr error

	for attempt := 0; attempt <= d.cfg.RetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func isRetryable(err error) bool {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			return true
		case http.StatusRequestedRangeNotSatisfiable:
			return false
		default:
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	msg := err.Error()
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if strings.Contains(msg, "connection reset") || strings.Contains(msg, "timeout") || strings.Contains(msg, "temporary") {
		return true
	}
	return false
}
