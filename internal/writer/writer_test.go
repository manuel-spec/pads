package writer_test

import (
	"os"
	"path/filepath"
	"testing"

	"pads/internal/writer"
)

func TestMergeFileAtomic(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, "segment.part")
	outputPath := filepath.Join(dir, "out.bin")

	if err := os.WriteFile(tempPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := writer.MergeFile(tempPath, outputPath); err != nil {
		t.Fatalf("merge file: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected output: %q", string(data))
	}
}

func TestMergeSegmentsOrdered(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "seg-0.part")
	second := filepath.Join(dir, "seg-1.part")
	outputPath := filepath.Join(dir, "merged.bin")

	if err := os.WriteFile(first, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("def"), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}

	if err := writer.MergeSegments([]string{first, second}, outputPath, 6); err != nil {
		t.Fatalf("merge segments: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read merged: %v", err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("unexpected merge result: %q", string(data))
	}
}

func TestOpenSegmentTemp(t *testing.T) {
	dir := t.TempDir()
	seg, err := writer.OpenSegmentTemp(dir, "seg-0")
	if err != nil {
		t.Fatalf("open segment temp: %v", err)
	}
	if _, err := seg.Write([]byte("abc")); err != nil {
		t.Fatalf("write segment: %v", err)
	}
	if err := seg.Close(); err != nil {
		t.Fatalf("close segment: %v", err)
	}
}
