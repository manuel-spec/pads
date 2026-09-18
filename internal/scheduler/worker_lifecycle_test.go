package scheduler

import (
	"context"
	"testing"
	"time"

	"pads/internal/config"
	"pads/internal/model"
)

// blockingWorker runs until its context is cancelled, standing in for a worker
// that is mid-transfer when the scheduler steals its segment.
type blockingWorker struct {
	started chan struct{}
}

func (w *blockingWorker) Download(ctx context.Context, seg *model.Segment) error {
	select {
	case w.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func testScheduler(manager *SegmentManager, worker SegmentWorker) *Scheduler {
	return &Scheduler{
		opts: Options{
			Config:      config.Default(),
			Manager:     manager,
			Monitor:     NewBandwidthMonitor(0),
			Worker:      worker,
			TotalSize:   2048,
			MaxConns:    4,
			InitialConn: 1,
		},
		workerCancels: make(map[string]context.CancelFunc),
	}
}

// A cancelled worker must hand its segment back. Leaving it active stranded the
// segment: nothing re-claims an active segment, so the run ended with
// "scheduler finished with incomplete segments".
func TestCancelledWorkerRequeuesItsSegment(t *testing.T) {
	segments := []model.Segment{
		{ID: "seg-0", ByteStart: 0, ByteEnd: 1023, Status: model.SegmentPending},
		{ID: "seg-1", ByteStart: 1024, ByteEnd: 2047, Status: model.SegmentPending},
	}
	manager := NewSegmentManager(segments, 2048, 256)
	worker := &blockingWorker{started: make(chan struct{}, 1)}
	s := testScheduler(manager, worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !s.startNextWorker(ctx) {
		t.Fatal("expected a worker to start")
	}
	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never ran")
	}

	s.cancelWorker("seg-0")
	s.wg.Wait()

	seg, ok := manager.Get("seg-0")
	if !ok {
		t.Fatal("segment disappeared")
	}
	if seg.Status != model.SegmentPending {
		t.Fatalf("segment status = %s, want %s", seg.Status, model.SegmentPending)
	}
	if manager.PendingCount() != 2 {
		t.Fatalf("pending count = %d, want 2", manager.PendingCount())
	}
}

// Work must never sit pending while no connection is running, whatever the
// phase's scaling rules would otherwise say.
func TestApplyScalingStartsWorkerWhenIdle(t *testing.T) {
	segments := []model.Segment{
		{ID: "seg-0", ByteStart: 0, ByteEnd: 2047, Status: model.SegmentPending},
	}
	manager := NewSegmentManager(segments, 2048, 256)
	worker := &blockingWorker{started: make(chan struct{}, 1)}
	s := testScheduler(manager, worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tail normally neither scales up nor steals.
	s.applyScaling(ctx, model.PhaseTail)

	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("idle scheduler left a pending segment unclaimed")
	}
	if manager.ActiveCount() != 1 {
		t.Fatalf("active count = %d, want 1", manager.ActiveCount())
	}

	cancel()
	s.wg.Wait()
}

// A steal must not relaunch the stolen segment itself: the old worker is still
// registered, so the relaunch was silently dropped and the segment stayed
// active with nothing downloading it.
func TestStealLeavesStolenSegmentRecoverable(t *testing.T) {
	segments := []model.Segment{
		{ID: "seg-0", ByteStart: 0, ByteEnd: 4095, Status: model.SegmentPending},
		{ID: "seg-1", ByteStart: 4096, ByteEnd: 8191, Status: model.SegmentPending},
	}
	manager := NewSegmentManager(segments, 8192, 256)
	worker := &blockingWorker{started: make(chan struct{}, 4)}
	s := testScheduler(manager, worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 2; i++ {
		if !s.startNextWorker(ctx) {
			t.Fatalf("expected worker %d to start", i)
		}
		select {
		case <-worker.started:
		case <-time.After(2 * time.Second):
			t.Fatalf("worker %d never ran", i)
		}
	}

	// seg-0 crawls while seg-1 races, which is what makes seg-0 stealable.
	manager.UpdateProgress("seg-0", 256, 1)
	manager.UpdateProgress("seg-1", 2048, 100)

	s.trySteal(ctx)

	// The replacement worker for the new tail segment keeps running, so wait on
	// the stolen segment alone rather than on the whole worker group.
	stolen, ok := waitForStatus(manager, "seg-0", model.SegmentPending)
	if !ok {
		t.Fatalf("stolen segment status = %s, want %s", stolen.Status, model.SegmentPending)
	}
	if err := model.ValidateSegmentCoverage(manager.Segments(), 8192); err != nil {
		t.Fatalf("steal broke byte coverage: %v", err)
	}
	if len(manager.Segments()) != 3 {
		t.Fatalf("segment count = %d, want 3", len(manager.Segments()))
	}

	cancel()
	s.wg.Wait()
}

// waitForStatus polls a segment until it reaches want or the deadline passes.
func waitForStatus(manager *SegmentManager, id string, want model.SegmentStatus) (model.Segment, bool) {
	deadline := time.Now().Add(2 * time.Second)
	var seg model.Segment
	for time.Now().Before(deadline) {
		current, ok := manager.Get(id)
		if !ok {
			return model.Segment{}, false
		}
		seg = *current
		if seg.Status == want {
			return seg, true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return seg, false
}
