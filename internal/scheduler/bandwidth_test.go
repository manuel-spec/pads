package scheduler

import (
	"testing"
	"time"
)

func TestBandwidthCurrentSpeed(t *testing.T) {
	monitor := NewBandwidthMonitor(time.Second)
	monitor.Record(1000)
	time.Sleep(50 * time.Millisecond)
	monitor.Record(1000)

	speed := monitor.CurrentSpeed()
	if speed <= 0 {
		t.Fatalf("expected positive current speed, got %f", speed)
	}
}

func TestBandwidthETA(t *testing.T) {
	monitor := NewBandwidthMonitor(time.Second)
	monitor.Record(1000)
	time.Sleep(50 * time.Millisecond)
	monitor.Record(1000)
	eta := monitor.ETA(1000)
	if eta <= 0 {
		t.Fatalf("expected positive eta, got %f", eta)
	}
}

func TestBandwidthVelocity(t *testing.T) {
	monitor := NewBandwidthMonitor(2 * time.Second)
	monitor.Record(500)
	monitor.Record(500)
	velocity := monitor.Velocity()
	if velocity < -1 || velocity > 1 {
		t.Fatalf("unexpected velocity: %f", velocity)
	}
}

func TestBandwidthSnapshot(t *testing.T) {
	monitor := NewBandwidthMonitor(time.Second)
	monitor.Record(1000)
	time.Sleep(50 * time.Millisecond)
	monitor.Record(1000)

	snap := monitor.Snapshot(2000)
	if snap.CurrentSpeed <= 0 {
		t.Fatalf("expected positive CurrentSpeed, got %f", snap.CurrentSpeed)
	}
	if snap.AverageSpeed <= 0 {
		t.Fatalf("expected positive AverageSpeed, got %f", snap.AverageSpeed)
	}
	if snap.ETA <= 0 {
		t.Fatalf("expected positive ETA, got %f", snap.ETA)
	}
}

func TestBandwidthSnapshotConcurrent(t *testing.T) {
	monitor := NewBandwidthMonitor(500 * time.Millisecond)
	done := make(chan struct{})

	go func() {
		for i := 0; i < 50; i++ {
			monitor.Record(100)
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()

	for {
		select {
		case <-done:
			return
		default:
			snap := monitor.Snapshot(1000)
			if snap.AverageSpeed < 0 || snap.CurrentSpeed < 0 {
				t.Fatalf("inconsistent speeds: %+v", snap)
			}
			time.Sleep(time.Millisecond)
		}
	}
}
