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
