package scheduler

import (
	"fmt"
	"testing"

	"pads/internal/model"
)

func testSegments(count int, size int64) []model.Segment {
	base := size / int64(count)
	remainder := size % int64(count)
	segments := make([]model.Segment, 0, count)
	start := int64(0)
	for i := 0; i < count; i++ {
		segSize := base
		if i == count-1 {
			segSize += remainder
		}
		end := start + segSize - 1
		segments = append(segments, model.Segment{
			ID:        fmt.Sprintf("seg-%d", i),
			ByteStart: start,
			ByteEnd:   end,
			Status:    model.SegmentPending,
		})
		start = end + 1
	}
	return segments
}

func TestSegmentManagerClaimAndComplete(t *testing.T) {
	segments := []model.Segment{
		{ID: "seg-0", ByteStart: 0, ByteEnd: 249, Status: model.SegmentPending},
		{ID: "seg-1", ByteStart: 250, ByteEnd: 499, Status: model.SegmentPending},
		{ID: "seg-2", ByteStart: 500, ByteEnd: 749, Status: model.SegmentPending},
		{ID: "seg-3", ByteStart: 750, ByteEnd: 999, Status: model.SegmentPending},
	}
	mgr := NewSegmentManager(segments, 1000, 100)

	seg, ok := mgr.ClaimPending()
	if !ok {
		t.Fatal("expected pending segment")
	}
	if seg.Status != model.SegmentActive {
		t.Fatalf("expected active, got %s", seg.Status)
	}

	mgr.Complete(seg.ID, "/tmp/seg.part")
	if mgr.PendingCount() != 3 {
		t.Fatalf("expected 3 pending, got %d", mgr.PendingCount())
	}
}

func TestSegmentManagerStealSlowest(t *testing.T) {
	segments := testSegments(2, 10*1024*1024)
	mgr := NewSegmentManager(segments, 10*1024*1024, 1024*1024)

	seg, ok := mgr.ClaimPending()
	if !ok {
		t.Fatal("expected segment")
	}
	mgr.UpdateProgress(seg.ID, 1024*1024, 1000)

	stolenID, newSeg, stole := mgr.StealSlowest(5000)
	if !stole {
		t.Fatal("expected steal to occur")
	}
	if stolenID != seg.ID {
		t.Fatalf("expected steal on %s, got %s", seg.ID, stolenID)
	}
	if newSeg.ByteStart <= seg.ByteStart {
		t.Fatalf("expected new segment after split point")
	}
}
