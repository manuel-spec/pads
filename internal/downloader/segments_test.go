package downloader

import (
	"testing"
)

func TestBuildSegmentsEvenSplit(t *testing.T) {
	segments, err := BuildSegments(100, 4, 10)
	if err != nil {
		t.Fatalf("build segments: %v", err)
	}
	if len(segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segments))
	}
	if segments[0].ByteStart != 0 || segments[0].ByteEnd != 24 {
		t.Fatalf("unexpected first segment: %+v", segments[0])
	}
	if segments[3].ByteStart != 75 || segments[3].ByteEnd != 99 {
		t.Fatalf("unexpected last segment: %+v", segments[3])
	}
}

func TestBuildSegmentsSmallFileCollapses(t *testing.T) {
	segments, err := BuildSegments(512*1024, 8, 1024*1024)
	if err != nil {
		t.Fatalf("build segments: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment for small file, got %d", len(segments))
	}
}

func TestBuildSegmentsRemainderInLast(t *testing.T) {
	segments, err := BuildSegments(10, 3, 1)
	if err != nil {
		t.Fatalf("build segments: %v", err)
	}
	if segments[2].ByteEnd != 9 {
		t.Fatalf("last segment should end at 9, got %d", segments[2].ByteEnd)
	}
	if err := ValidateCoverage(segments, 10); err != nil {
		t.Fatalf("coverage invalid: %v", err)
	}
}

func TestValidateCoverageRejectsGap(t *testing.T) {
	segs, err := BuildSegments(10, 2, 1)
	if err != nil {
		t.Fatalf("build segments: %v", err)
	}
	segs[1].ByteStart = 6
	if err := ValidateCoverage(segs, 10); err == nil {
		t.Fatal("expected gap to be rejected")
	}
}
