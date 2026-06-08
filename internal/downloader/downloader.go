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
	"pads/internal/scheduler"
	"pads/internal/state"
	"pads/internal/ui"
	"pads/internal/util"
	"pads/internal/writer"
)

// Options configures a download run.
type Options struct {
	URL        string
	Output     string
	DownloadID string
	Resume     *model.DownloadState
	Store      *state.Store
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

// Run probes the server and downloads using segmented or single-connection mode.
func (d *Downloader) Run(ctx context.Context, opts Options) error {
	if opts.Resume != nil {
		return d.runFromState(ctx, opts.Resume, opts.Store)
	}

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

	if d.shouldUseSegments(profile) {
		return d.runSegmented(ctx, parsed.String(), output, profile, nil, opts)
	}
	return d.runSingle(ctx, parsed.String(), output, profile, nil, opts)
}

func (d *Downloader) runFromState(ctx context.Context, st *model.DownloadState, store *state.Store) error {
	if err := state.Validate(st); err != nil {
		return fmt.Errorf("invalid resume state: %w", err)
	}

	profile, err := probe.Probe(ctx, d.client, st.URL, probe.Options{
		Timeout: time.Duration(d.cfg.ProbeTimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return fmt.Errorf("probe server for resume: %w", err)
	}
	if profile.ContentLength > 0 && profile.ContentLength != st.TotalSize {
		return fmt.Errorf("server size %d does not match saved state %d", profile.ContentLength, st.TotalSize)
	}

	opts := Options{Resume: st, Store: store, DownloadID: st.ID}
	if st.Segmented {
		return d.runSegmented(ctx, st.URL, st.Output, profile, st, opts)
	}
	return d.runSingle(ctx, st.URL, st.Output, profile, st, opts)
}

func (d *Downloader) shouldUseSegments(profile *model.ServerProfile) bool {
	return profile.RangeSupported &&
		profile.ContentLength > 0 &&
		profile.ContentLengthTrusted
}

func (d *Downloader) runSegmented(
	ctx context.Context,
	url, output string,
	profile *model.ServerProfile,
	existing *model.DownloadState,
	opts Options,
) error {
	downloadID := opts.DownloadID
	if downloadID == "" {
		if existing != nil {
			downloadID = existing.ID
		} else {
			downloadID = uuid.NewString()
		}
	}

	tempDir := d.cfg.StatePath("tmp", downloadID)
	if existing != nil && existing.TempDir != "" {
		tempDir = existing.TempDir
	}

	maxConns := d.cfg.MaxConnections
	if profile.RecommendedConns > 0 && profile.RecommendedConns < maxConns {
		maxConns = profile.RecommendedConns
	}

	var segments []model.Segment
	var err error
	if existing != nil {
		segments = prepareSegmentsForResume(existing.Segments)
	} else {
		segments, err = BuildSegments(profile.ContentLength, maxConns, d.cfg.MinSegmentSize)
		if err != nil {
			return fmt.Errorf("plan segments: %w", err)
		}
	}

	downloadState := existing
	if downloadState == nil {
		downloadState = newDownloadState(downloadID, url, output, tempDir, profile.ContentLength, segments, true)
	}
	if opts.Store != nil {
		if err := opts.Store.Save(downloadState); err != nil {
			return fmt.Errorf("save initial state: %w", err)
		}
	}

	progress := ui.NewProgress(profile.ContentLength)
	bar := progress.AddBar("download")
	for _, seg := range segments {
		if seg.Status == model.SegmentComplete {
			bar.Add(seg.BytesDownloaded)
		}
	}

	manager := scheduler.NewSegmentManager(segments, profile.ContentLength, d.cfg.MinSegmentSize)
	monitor := scheduler.NewBandwidthMonitor(0)
	worker := &adaptiveWorker{
		downloader: d,
		url:        url,
		tempDir:    tempDir,
		bar:        bar,
		manager:    manager,
		monitor:    monitor,
	}

	runErr := scheduler.Run(ctx, scheduler.Options{
		Config:      d.cfg,
		Profile:     profile,
		Manager:     manager,
		Monitor:     monitor,
		Worker:      worker,
		TotalSize:   profile.ContentLength,
		MaxConns:    maxConns,
		InitialConn: d.cfg.InitialConnections,
		OnTick: func() error {
			return saveSnapshot(opts.Store, downloadState, manager, false)
		},
	})

	if runErr != nil || ctx.Err() != nil {
		manager.RequeueAllActive()
		if saveErr := saveSnapshot(opts.Store, downloadState, manager, true); saveErr != nil {
			return saveErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return runErr
	}

	mergedSegments := manager.Segments()
	if err := writer.MergeSegments(segmentPaths(mergedSegments), output, profile.ContentLength); err != nil {
		return fmt.Errorf("merge segments: %w", err)
	}

	for _, path := range segmentPaths(mergedSegments) {
		_ = writer.RemoveTemp(path)
	}

	downloadState.Complete = true
	downloadState.Paused = false
	downloadState.Segments = mergedSegments
	if opts.Store != nil {
		_ = opts.Store.Delete(downloadState.ID)
	}

	progress.Complete(profile.ContentLength)
	return nil
}

func segmentPaths(segments []model.Segment) []string {
	paths := make([]string, len(segments))
	for i, seg := range segments {
		paths[i] = seg.TempPath
	}
	return paths
}

func (d *Downloader) runSingle(
	ctx context.Context,
	url, output string,
	profile *model.ServerProfile,
	existing *model.DownloadState,
	opts Options,
) error {
	downloadID := opts.DownloadID
	if downloadID == "" {
		if existing != nil {
			downloadID = existing.ID
		} else {
			downloadID = uuid.NewString()
		}
	}

	tempDir := d.cfg.StatePath("tmp", downloadID)
	if existing != nil && existing.TempDir != "" {
		tempDir = existing.TempDir
	}

	segmentID := "seg-0"
	var segWriter *writer.SegmentWriter
	var err error
	var bytesDone int64

	if existing != nil && len(existing.Segments) > 0 {
		seg := existing.Segments[0]
		bytesDone = seg.BytesDownloaded
		if seg.TempPath != "" && bytesDone > 0 {
			segWriter, err = writer.ResumeSegmentTemp(seg.TempPath)
		} else {
			segWriter, err = writer.OpenSegmentTemp(tempDir, segmentID)
		}
	} else {
		segWriter, err = writer.OpenSegmentTemp(tempDir, segmentID)
	}
	if err != nil {
		return err
	}
	defer segWriter.Close()

	downloadState := existing
	if downloadState == nil {
		segments := []model.Segment{{ID: segmentID, ByteStart: 0, ByteEnd: profile.ContentLength - 1, Status: model.SegmentPending}}
		if profile.ContentLength <= 0 {
			segments[0].ByteEnd = 0
		}
		downloadState = newDownloadState(downloadID, url, output, tempDir, profile.ContentLength, segments, false)
	}
	if opts.Store != nil {
		if err := opts.Store.Save(downloadState); err != nil {
			return fmt.Errorf("save initial state: %w", err)
		}
	}

	progress := ui.NewProgress(profile.ContentLength)
	bar := progress.AddBar("download")

	total, err := d.downloadAll(ctx, url, segWriter, bar, profile.ContentLength, bytesDone, func(downloaded int64) {
		if len(downloadState.Segments) == 0 {
			return
		}
		downloadState.Segments[0].BytesDownloaded = downloaded
		downloadState.Segments[0].TempPath = segWriter.Path()
		downloadState.Segments[0].Status = model.SegmentActive
		_ = saveSnapshot(opts.Store, downloadState, scheduler.NewSegmentManager(downloadState.Segments, downloadState.TotalSize, d.cfg.MinSegmentSize), false)
	})
	if err != nil {
		if len(downloadState.Segments) > 0 {
			downloadState.Segments[0].BytesDownloaded = total
			downloadState.Segments[0].TempPath = segWriter.Path()
			downloadState.Segments[0].Status = model.SegmentPending
		}
		downloadState.Paused = ctx.Err() != nil
		if opts.Store != nil {
			if saveErr := opts.Store.Save(downloadState); saveErr != nil && ctx.Err() == nil {
				return saveErr
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	if err := segWriter.Close(); err != nil {
		return err
	}

	if err := writer.MergeFile(segWriter.Path(), output); err != nil {
		return fmt.Errorf("merge output: %w", err)
	}
	_ = writer.RemoveTemp(segWriter.Path())

	if opts.Store != nil {
		_ = opts.Store.Delete(downloadState.ID)
	}

	progress.Complete(total)
	return nil
}

func (d *Downloader) downloadAll(
	ctx context.Context,
	url string,
	segWriter *writer.SegmentWriter,
	bar *ui.Bar,
	expectedSize int64,
	offset int64,
	onProgress func(int64),
) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return offset, fmt.Errorf("create download request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", formatRange(offset, expectedSize-1))
	}

	resp, err := util.DoRequest(ctx, d.client, req)
	if err != nil {
		return offset, err
	}
	defer resp.Body.Close()

	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode != http.StatusOK {
			return offset, fmt.Errorf("unexpected status %d for resume", resp.StatusCode)
		}
	}
	if offset == 0 && resp.StatusCode != http.StatusOK {
		return offset, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	totalSize := expectedSize
	if totalSize <= 0 {
		totalSize = resp.ContentLength + offset
	}
	if totalSize > 0 {
		bar.SetTotal(totalSize)
		if offset > 0 {
			bar.Add(offset)
		}
	}

	buf := make([]byte, 32*1024)
	downloaded := offset

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
			if onProgress != nil {
				onProgress(downloaded)
			}
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
