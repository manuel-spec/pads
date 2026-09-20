package scheduler

import (
	"sync"
	"time"
)

const defaultWindow = 5 * time.Second

type sample struct {
	at    time.Time
	bytes int64
}

// BandwidthMonitor tracks throughput using a rolling byte window.
type BandwidthMonitor struct {
	mu      sync.Mutex
	window  time.Duration
	samples []sample
	total   int64
}

// NewBandwidthMonitor creates a monitor with the given rolling window.
func NewBandwidthMonitor(window time.Duration) *BandwidthMonitor {
	if window <= 0 {
		window = defaultWindow
	}
	return &BandwidthMonitor{window: window}
}

// Record adds downloaded bytes at the current time.
func (b *BandwidthMonitor) Record(bytes int64) {
	if bytes <= 0 {
		return
	}
	now := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.samples = append(b.samples, sample{at: now, bytes: bytes})
	b.total += bytes
	b.trimLocked(now)
}

// CurrentSpeed returns bytes per second over the rolling window.
func (b *BandwidthMonitor) CurrentSpeed() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.trimLocked(time.Now())
	return b.speedLocked(b.samples)
}

// AverageSpeed returns bytes per second since monitoring began.
func (b *BandwidthMonitor) AverageSpeed() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.samples) == 0 {
		return 0
	}
	span := time.Since(b.samples[0].at).Seconds()
	if span <= 0 {
		return 0
	}
	return float64(b.total) / span
}

// Metrics contains a point-in-time consistent snapshot of bandwidth telemetry.
type Metrics struct {
	CurrentSpeed float64
	AverageSpeed float64
	Velocity     float64
	ETA          float64
}

// Snapshot returns a point-in-time consistent snapshot of all bandwidth metrics.
func (b *BandwidthMonitor) Snapshot(remaining int64) Metrics {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.metricsLocked(time.Now(), remaining)
}

// Velocity returns the normalized trend between current and average speed.
func (b *BandwidthMonitor) Velocity() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.metricsLocked(time.Now(), 0).Velocity
}

// ETA estimates seconds to finish the given remaining bytes.
func (b *BandwidthMonitor) ETA(remaining int64) float64 {
	if remaining <= 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.metricsLocked(time.Now(), remaining).ETA
}

func (b *BandwidthMonitor) metricsLocked(now time.Time, remaining int64) Metrics {
	b.trimLocked(now)
	current := b.speedLocked(b.samples)
	avg := b.averageSpeedLocked(now)

	var velocity float64
	if avg > 0 {
		velocity = (current - avg) / avg
	}

	var eta float64
	if remaining > 0 {
		effectiveSpeed := current
		if effectiveSpeed <= 0 {
			effectiveSpeed = avg
		}
		if effectiveSpeed > 0 {
			eta = float64(remaining) / effectiveSpeed
		}
	}

	return Metrics{
		CurrentSpeed: current,
		AverageSpeed: avg,
		Velocity:     velocity,
		ETA:          eta,
	}
}

func (b *BandwidthMonitor) averageSpeedLocked(now time.Time) float64 {
	if len(b.samples) == 0 {
		return 0
	}
	span := now.Sub(b.samples[0].at).Seconds()
	if span <= 0 {
		return 0
	}
	return float64(b.total) / span
}

func (b *BandwidthMonitor) trimLocked(now time.Time) {
	cutoff := now.Add(-b.window)
	idx := 0
	for idx < len(b.samples) && b.samples[idx].at.Before(cutoff) {
		idx++
	}
	if idx > 0 {
		b.samples = append([]sample(nil), b.samples[idx:]...)
	}
}

func (b *BandwidthMonitor) speedLocked(samples []sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var bytes int64
	for _, s := range samples {
		bytes += s.bytes
	}
	span := time.Since(samples[0].at).Seconds()
	if span <= 0 {
		return 0
	}
	return float64(bytes) / span
}
