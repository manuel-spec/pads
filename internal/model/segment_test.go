package model

import "testing"

func TestSegmentByteLength(t *testing.T) {
	seg := Segment{ByteStart: 0, ByteEnd: 1023}
	if got := seg.ByteLength(); got != 1024 {
		t.Fatalf("expected 1024 bytes, got %d", got)
	}
}

func TestSegmentRemainingBytes(t *testing.T) {
	seg := Segment{ByteStart: 0, ByteEnd: 99, BytesDownloaded: 40}
	if got := seg.RemainingBytes(); got != 60 {
		t.Fatalf("expected 60 remaining bytes, got %d", got)
	}
}
