package downloader

import (
	"fmt"

	"pads/internal/model"
)

// BuildSegments splits content into evenly sized inclusive byte ranges.
func BuildSegments(totalSize int64, connectionCount int, minSegmentSize int64) ([]model.Segment, error) {
	if totalSize <= 0 {
		return nil, fmt.Errorf("total size must be positive")
	}
	if connectionCount < 1 {
		return nil, fmt.Errorf("connection count must be at least 1")
	}
	if minSegmentSize < 1 {
		return nil, fmt.Errorf("min segment size must be positive")
	}

	segmentCount := connectionCount
	if totalSize < minSegmentSize {
		segmentCount = 1
	} else {
		for segmentCount > 1 && totalSize/int64(segmentCount) < minSegmentSize {
			segmentCount--
		}
	}

	baseSize := totalSize / int64(segmentCount)
	remainder := totalSize % int64(segmentCount)

	segments := make([]model.Segment, 0, segmentCount)
	start := int64(0)
	for i := 0; i < segmentCount; i++ {
		size := baseSize
		if i == segmentCount-1 {
			size += remainder
		}
		end := start + size - 1
		segments = append(segments, model.Segment{
			ID:        fmt.Sprintf("seg-%d", i),
			ByteStart: start,
			ByteEnd:   end,
			Status:    model.SegmentPending,
		})
		start = end + 1
	}

	if err := ValidateCoverage(segments, totalSize); err != nil {
		return nil, err
	}
	return segments, nil
}

// ValidateCoverage ensures segments fully cover [0, totalSize) without gaps or overlap.
func ValidateCoverage(segments []model.Segment, totalSize int64) error {
	if len(segments) == 0 {
		return fmt.Errorf("segments cannot be empty")
	}

	expectedStart := int64(0)
	for i, seg := range segments {
		if seg.ByteStart != expectedStart {
			return fmt.Errorf("segment %d starts at %d, expected %d", i, seg.ByteStart, expectedStart)
		}
		if seg.ByteEnd < seg.ByteStart {
			return fmt.Errorf("segment %d has invalid range [%d, %d]", i, seg.ByteStart, seg.ByteEnd)
		}
		length := seg.ByteEnd - seg.ByteStart + 1
		if length <= 0 {
			return fmt.Errorf("segment %d has non-positive length", i)
		}
		expectedStart = seg.ByteEnd + 1
	}

	if expectedStart != totalSize {
		return fmt.Errorf("segments cover %d bytes, expected %d", expectedStart, totalSize)
	}
	return nil
}
