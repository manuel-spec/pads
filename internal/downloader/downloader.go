package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"pads/internal/config"
	"pads/internal/model"
	"pads/internal/probe"
	"pads/internal/ui"
	"pads/internal/util"
	"pads/internal/writer"
)

// Options configures a download run.
type Options struct {
	URL    string
	Output string
}

// Downloader performs HTTP downloads.
type Downloader struct {
	cfg    *config.Config
	client *http.Client
}

// New creates a downloader with the given configuration.
func New(cfg *config.Config) *Downloader {
	timeout := time.Duration(cfg.ProbeTimeoutMS) * time.Millisecond * 4
	return &Downloader{
		cfg:    cfg,
		client: util.NewHTTPClient(timeout),
	}
}

// Run executes a download using single-connection mode for Phase B.
func (d *Downloader) Run(ctx context.Context, opts Options) error {
	parsed, err := util.ValidateURL(opts.URL)
	if err != nil {
		return err
	}

	output, err := util.SafeOutputPath(opts.Output, "")
	if err != nil {
		return err
	}

	profile, err := probe.Probe(ctx, d.client, parsed.String(), probe.Options{
		Timeout: time.Duration(d.cfg.ProbeTimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return fmt.Errorf("probe server: %w", err)
	}

	downloadID := uuid.NewString()
	tempDir := d.cfg.StatePath("tmp", downloadID)
	segmentID := "seg-0"
	segWriter, err := writer.OpenSegmentTemp(tempDir, segmentID)
	if err != nil {
		return err
	}
	defer segWriter.Close()

	progress := ui.NewProgress(profile.ContentLength)
	bar := progress.AddBar("download")
	defer progress.Wait()

	total, err := d.downloadAll(ctx, parsed.String(), segWriter, bar, profile.ContentLength)
	if err != nil {
		return err
	}

	if err := segWriter.Close(); err != nil {
		return err
	}

	if err := writer.MergeFile(segWriter.Path(), output); err != nil {
		return fmt.Errorf("merge output: %w", err)
	}
	_ = writer.RemoveTemp(segWriter.Path())

	progress.Complete(total)
	return nil
}

func (d *Downloader) downloadAll(
	ctx context.Context,
	url string,
	segWriter *writer.SegmentWriter,
	bar *ui.Bar,
	expectedSize int64,
) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create download request: %w", err)
	}

	resp, err := util.DoRequest(ctx, d.client, req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	totalSize := expectedSize
	if totalSize <= 0 {
		totalSize = resp.ContentLength
	}
	if totalSize > 0 {
		bar.SetTotal(totalSize)
	}

	buf := make([]byte, 32*1024)
	var downloaded int64

	for {
		select {
		case <-ctx.Done():
			return downloaded, ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := segWriter.Write(buf[:n]); err != nil {
				return downloaded, err
			}
			downloaded += int64(n)
			bar.Add(int64(n))
		}
		if readErr == io.EOF {
			return downloaded, nil
		}
		if readErr != nil {
			return downloaded, fmt.Errorf("read response body: %w", readErr)
		}
	}
}

// Profile is re-exported for callers that need probe data.
type Profile = model.ServerProfile
