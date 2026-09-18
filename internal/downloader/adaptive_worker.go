package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"pads/internal/model"
	"pads/internal/scheduler"
	"pads/internal/ui"
	"pads/internal/util"
	"pads/internal/writer"
)

type adaptiveWorker struct {
	downloader *Downloader
	url        string
	tempDir    string
	bar        *ui.Bar
	manager    *scheduler.SegmentManager
	monitor    *scheduler.BandwidthMonitor
}

func (w *adaptiveWorker) Download(ctx context.Context, seg *model.Segment) error {
	err := w.downloader.downloadSegmentAdaptive(ctx, w.url, seg, w.tempDir, w.bar, w.manager, w.monitor)
	if err != nil {
		return err
	}
	w.manager.Complete(seg.ID, seg.TempPath)
	return nil
}

func (d *Downloader) downloadSegmentAdaptive(
	ctx context.Context,
	url string,
	seg *model.Segment,
	tempDir string,
	bar *ui.Bar,
	manager *scheduler.SegmentManager,
	monitor *scheduler.BandwidthMonitor,
) error {
	seg.Status = model.SegmentActive
	return d.withRetries(ctx, func() error {
		return d.fetchSegmentAdaptive(ctx, url, seg, tempDir, bar, manager, monitor)
	})
}

func (d *Downloader) fetchSegmentAdaptive(
	ctx context.Context,
	url string,
	seg *model.Segment,
	tempDir string,
	bar *ui.Bar,
	manager *scheduler.SegmentManager,
	monitor *scheduler.BandwidthMonitor,
) error {
	var segWriter *writer.SegmentWriter
	var err error

	if seg.BytesDownloaded > 0 && seg.TempPath != "" {
		// A steal can shorten a segment while its worker is still writing, so
		// the recorded progress may run past the segment's new end. Clamp it,
		// then let the writer reconcile the file with what is accounted for.
		var usable int64
		segWriter, usable, err = writer.ResumeSegmentTempAt(seg.TempPath, min(seg.BytesDownloaded, seg.ByteLength()))
		if err == nil && usable != seg.BytesDownloaded {
			seg.BytesDownloaded = usable
			manager.SetProgress(seg.ID, usable)
		}
	} else {
		segWriter, err = writer.OpenSegmentTemp(tempDir, seg.ID)
	}
	if err != nil {
		return err
	}
	defer segWriter.Close()

	seg.TempPath = segWriter.Path()
	manager.SetTempPath(seg.ID, seg.TempPath)

	rangeStart := seg.ByteStart + seg.BytesDownloaded
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create segment request: %w", err)
	}
	req.Header.Set("Range", formatRange(rangeStart, seg.ByteEnd))

	resp, err := util.DoRequest(ctx, d.client, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkSegmentResponse(resp, seg); err != nil {
		return err
	}

	expected := seg.ByteEnd - rangeStart + 1
	lastReport := time.Now()
	var windowBytes int64

	written, err := copyWithProgressCallback(ctx, resp.Body, segWriter, expected, func(n int64) {
		bar.Add(n)
		monitor.Record(n)
		manager.UpdateProgress(seg.ID, n, segmentSpeed(&lastReport, &windowBytes, n))
	})
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
	seg.BytesDownloaded = rangeStart - seg.ByteStart + written
	seg.UpdatedAt = time.Now()
	return nil
}

func copyWithProgressCallback(
	ctx context.Context,
	src io.Reader,
	dst *writer.SegmentWriter,
	limit int64,
	onChunk func(int64),
) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64

	for written < limit {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

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
			onChunk(int64(n))
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

func segmentSpeed(lastReport *time.Time, windowBytes *int64, delta int64) float64 {
	*windowBytes += delta
	elapsed := time.Since(*lastReport).Seconds()
	if elapsed < 0.25 {
		return 0
	}
	speed := float64(*windowBytes) / elapsed
	*lastReport = time.Now()
	*windowBytes = 0
	return speed
}
