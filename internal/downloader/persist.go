package downloader

import (
	"time"

	"pads/internal/model"
	"pads/internal/scheduler"
	"pads/internal/state"
)

func newDownloadState(
	id, url, output, tempDir string,
	totalSize int64,
	segments []model.Segment,
	segmented bool,
) *model.DownloadState {
	now := time.Now()
	return &model.DownloadState{
		Version:   model.StateVersion,
		ID:        id,
		URL:       url,
		Output:    output,
		TotalSize: totalSize,
		Segments:  segments,
		TempDir:   tempDir,
		Segmented: segmented,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func snapshotState(
	base *model.DownloadState,
	manager *scheduler.SegmentManager,
	paused bool,
) *model.DownloadState {
	copyState := *base
	copyState.Segments = manager.Segments()
	if base.Headers != nil {
		copyHeaders := make(map[string]string, len(base.Headers))
		for k, v := range base.Headers {
			copyHeaders[k] = v
		}
		copyState.Headers = copyHeaders
	}
	copyState.Paused = paused
	copyState.UpdatedAt = time.Now()
	return &copyState
}

func saveSnapshot(store *state.Store, base *model.DownloadState, manager *scheduler.SegmentManager, paused bool) error {
	if store == nil {
		return nil
	}
	return store.Save(snapshotState(base, manager, paused))
}

func prepareSegmentsForResume(segments []model.Segment) []model.Segment {
	out := make([]model.Segment, len(segments))
	copy(out, segments)
	for i := range out {
		if out[i].Status == model.SegmentComplete {
			continue
		}
		out[i].Status = model.SegmentPending
	}
	return out
}
