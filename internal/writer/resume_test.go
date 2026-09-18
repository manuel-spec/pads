package writer_test

import (
	"os"
	"path/filepath"
	"testing"

	"pads/internal/writer"
)

func TestResumeSegmentTempAtDiscardsUnaccountedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seg-0.part")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Only six bytes were ever persisted as progress; a steal shortened the
	// segment or the process died mid-write, so the tail must go.
	w, usable, err := writer.ResumeSegmentTempAt(path, 6)
	if err != nil {
		t.Fatalf("resume at offset: %v", err)
	}
	if usable != 6 {
		t.Fatalf("usable offset = %d, want 6", usable)
	}
	if _, err := w.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "012345ab" {
		t.Fatalf("file = %q, want %q", got, "012345ab")
	}
}

func TestResumeSegmentTempAtReportsShortFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seg-0.part")
	if err := os.WriteFile(path, []byte("012"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Recorded progress runs ahead of the file; the file is the truth.
	w, usable, err := writer.ResumeSegmentTempAt(path, 9)
	if err != nil {
		t.Fatalf("resume at offset: %v", err)
	}
	defer w.Close()
	if usable != 3 {
		t.Fatalf("usable offset = %d, want 3", usable)
	}
}

func TestSegmentWriterTruncate(t *testing.T) {
	dir := t.TempDir()
	w, err := writer.OpenSegmentTemp(dir, "seg-0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	if err := w.Truncate(0); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := w.Write([]byte("whole")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "whole" {
		t.Fatalf("file = %q, want %q", got, "whole")
	}
}
