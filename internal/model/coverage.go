package model

import "fmt"

// ValidateSegmentCoverage ensures segments fully cover [0, totalSize).
func ValidateSegmentCoverage(segments []Segment, totalSize int64) error {
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
