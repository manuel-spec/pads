package scheduler

import (
	"fmt"
	"testing"
	"time"

	"pads/internal/model"
)

func BenchmarkSegmentManagerUpdateProgressParallel(b *testing.B) {
	const segCount = 64
	segs := make([]model.Segment, segCount)
	for i := range segs {
		segs[i] = model.Segment{
			ID:        fmt.Sprintf("seg-%d", i),
			ByteStart: int64(i * 1024 * 1024),
			ByteEnd:   int64((i+1)*1024*1024 - 1),
			Status:    model.SegmentActive,
		}
	}

	mgr := NewSegmentManager(segs, int64(segCount*1024*1024), 1024*1024)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			segID := fmt.Sprintf("seg-%d", i%segCount)
			mgr.UpdateProgress(segID, 32*1024, 5*1024*1024)
			i++
		}
	})
}

func BenchmarkBandwidthMonitorRecordParallel(b *testing.B) {
	mon := NewBandwidthMonitor(5 * time.Second)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mon.Record(32 * 1024)
		}
	})
}

func BenchmarkBandwidthMonitorSnapshotParallel(b *testing.B) {
	mon := NewBandwidthMonitor(5 * time.Second)
	for i := 0; i < 100; i++ {
		mon.Record(32 * 1024)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mon.Snapshot(10 * 1024 * 1024)
		}
	})
}
