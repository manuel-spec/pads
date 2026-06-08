package scheduler

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"pads/internal/model"
)

// SegmentManager owns concurrency-safe segment state.
type SegmentManager struct {
	mu             sync.RWMutex
	segments       []model.Segment
	totalSize      int64
	minSegmentSize int64
	nextID         int
}

// NewSegmentManager creates a manager for the planned segments.
func NewSegmentManager(segments []model.Segment, totalSize, minSegmentSize int64) *SegmentManager {
	nextID := len(segments)
	return &SegmentManager{
		segments:       append([]model.Segment(nil), segments...),
		totalSize:      totalSize,
		minSegmentSize: minSegmentSize,
		nextID:         nextID,
	}
}

// Segments returns a snapshot sorted by byte start.
func (m *SegmentManager) Segments() []model.Segment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sortedCopyLocked()
}

// ClaimPending marks the next pending segment active.
func (m *SegmentManager) ClaimPending() (*model.Segment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].Status != model.SegmentPending {
			continue
		}
		m.segments[i].Status = model.SegmentActive
		m.segments[i].StartedAt = time.Now()
		m.segments[i].UpdatedAt = time.Now()
		seg := m.segments[i]
		return &seg, true
	}
	return nil, false
}

// Get returns a segment by ID.
func (m *SegmentManager) Get(id string) (*model.Segment, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.segments {
		if m.segments[i].ID == id {
			seg := m.segments[i]
			return &seg, true
		}
	}
	return nil, false
}

// SetTempPath records the temp file path for a segment.
func (m *SegmentManager) SetTempPath(id, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].ID == id {
			m.segments[i].TempPath = path
			return
		}
	}
}

// UpdateProgress records in-flight progress for a segment.
func (m *SegmentManager) UpdateProgress(id string, bytesDelta int64, speed float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].ID != id {
			continue
		}
		m.segments[i].BytesDownloaded += bytesDelta
		m.segments[i].SpeedBPS = speed
		m.segments[i].UpdatedAt = time.Now()
		remaining := m.segments[i].RemainingBytes()
		if speed > 0 {
			m.segments[i].ETASeconds = float64(remaining) / speed
		}
		return
	}
}

// Complete marks a segment finished.
func (m *SegmentManager) Complete(id, tempPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].ID != id {
			continue
		}
		m.segments[i].Status = model.SegmentComplete
		m.segments[i].TempPath = tempPath
		m.segments[i].UpdatedAt = time.Now()
		return
	}
}

// Fail marks a segment failed.
func (m *SegmentManager) Fail(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].ID != id {
			continue
		}
		m.segments[i].Status = model.SegmentFailed
		m.segments[i].UpdatedAt = time.Now()
		return
	}
}

// Activate marks a pending segment active.
func (m *SegmentManager) Activate(id string) (*model.Segment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].ID != id || m.segments[i].Status != model.SegmentPending {
			continue
		}
		m.segments[i].Status = model.SegmentActive
		m.segments[i].UpdatedAt = time.Now()
		seg := m.segments[i]
		return &seg, true
	}
	return nil, false
}

// RequeueAllActive moves every active segment back to pending.
func (m *SegmentManager) RequeueAllActive() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].Status == model.SegmentActive {
			m.segments[i].Status = model.SegmentPending
			m.segments[i].UpdatedAt = time.Now()
		}
	}
}

// RequeueActive returns an interrupted active segment to pending.
func (m *SegmentManager) RequeueActive(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.segments {
		if m.segments[i].ID != id {
			continue
		}
		if m.segments[i].Status != model.SegmentActive {
			return
		}
		m.segments[i].Status = model.SegmentPending
		m.segments[i].UpdatedAt = time.Now()
		return
	}
}

// ActiveCount returns the number of active segments.
func (m *SegmentManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, seg := range m.segments {
		if seg.Status == model.SegmentActive {
			count++
		}
	}
	return count
}

// PendingCount returns pending segments.
func (m *SegmentManager) PendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, seg := range m.segments {
		if seg.Status == model.SegmentPending {
			count++
		}
	}
	return count
}

// RemainingBytes returns total bytes left across all segments.
func (m *SegmentManager) RemainingBytes() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var remaining int64
	for _, seg := range m.segments {
		switch seg.Status {
		case model.SegmentComplete:
			continue
		default:
			remaining += seg.RemainingBytes()
		}
	}
	return remaining
}

// AverageActiveSpeed returns mean speed of active segments.
func (m *SegmentManager) AverageActiveSpeed() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total float64
	var count int
	for _, seg := range m.segments {
		if seg.Status != model.SegmentActive || seg.SpeedBPS <= 0 {
			continue
		}
		total += seg.SpeedBPS
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// AllComplete reports whether every segment finished successfully.
func (m *SegmentManager) AllComplete() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, seg := range m.segments {
		if seg.Status != model.SegmentComplete {
			return false
		}
	}
	return len(m.segments) > 0
}

// StealSlowest splits the slowest active segment when it qualifies.
func (m *SegmentManager) StealSlowest(avgSpeed float64) (stolenID string, newSeg *model.Segment, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	slowest := -1
	slowestSpeed := avgSpeed
	for i := range m.segments {
		seg := m.segments[i]
		if seg.Status != model.SegmentActive {
			continue
		}
		remaining := seg.RemainingBytes()
		if remaining < m.minSegmentSize*2 {
			continue
		}
		if avgSpeed > 0 && seg.SpeedBPS >= avgSpeed*stealSpeedRatio {
			continue
		}
		if slowest < 0 || seg.SpeedBPS < slowestSpeed {
			slowest = i
			slowestSpeed = seg.SpeedBPS
		}
	}
	if slowest < 0 {
		return "", nil, false
	}

	seg := &m.segments[slowest]
	absolutePos := seg.ByteStart + seg.BytesDownloaded
	remaining := seg.ByteEnd - absolutePos + 1
	splitPoint := absolutePos + remaining/2

	origEnd := seg.ByteEnd
	seg.ByteEnd = splitPoint - 1
	seg.UpdatedAt = time.Now()

	if err := validateSegmentLocked(*seg); err != nil {
		seg.ByteEnd = origEnd
		return "", nil, false
	}

	newSegment := model.Segment{
		ID:        fmt.Sprintf("seg-%d", m.nextID),
		ByteStart: splitPoint,
		ByteEnd:   origEnd,
		Status:    model.SegmentPending,
	}
	m.nextID++

	if err := validateSegmentLocked(newSegment); err != nil {
		seg.ByteEnd = origEnd
		return "", nil, false
	}

	m.segments = append(m.segments, newSegment)
	return seg.ID, &newSegment, true
}

func (m *SegmentManager) sortedCopyLocked() []model.Segment {
	out := append([]model.Segment(nil), m.segments...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ByteStart < out[j].ByteStart
	})
	return out
}

func validateSegmentLocked(seg model.Segment) error {
	if seg.ByteEnd < seg.ByteStart {
		return fmt.Errorf("invalid segment range")
	}
	if seg.ByteLength() <= 0 {
		return fmt.Errorf("invalid segment length")
	}
	return nil
}

const stealSpeedRatio = 0.6
