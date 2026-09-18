package writer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SegmentWriter writes downloaded bytes to a temp file for one segment.
type SegmentWriter struct {
	path string
	file *os.File
}

// OpenSegmentTemp creates or opens a segment temp file for writing.
func OpenSegmentTemp(tempDir, segmentID string) (*SegmentWriter, error) {
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}

	path := filepath.Join(tempDir, segmentID+".part")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open segment temp file: %w", err)
	}
	return &SegmentWriter{path: path, file: file}, nil
}

// ResumeSegmentTemp opens a segment temp file for append/resume.
func ResumeSegmentTemp(path string) (*SegmentWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("resume segment temp file: %w", err)
	}
	return &SegmentWriter{path: path, file: file}, nil
}

// ResumeSegmentTempAt opens a segment temp file for append and reconciles it
// with the caller's recorded offset. Progress is persisted after the bytes are
// written, so a file longer than the offset means the process died mid-write or
// the segment was re-planned while a worker was still running; those trailing
// bytes are unaccounted for and are discarded. A shorter file wins over the
// recorded offset, and the usable offset is returned.
func ResumeSegmentTempAt(path string, offset int64) (*SegmentWriter, int64, error) {
	if offset < 0 {
		return nil, 0, fmt.Errorf("resume offset must not be negative")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("stat segment temp file: %w", err)
	}

	usable := offset
	switch {
	case info.Size() > offset:
		if err := os.Truncate(path, offset); err != nil {
			return nil, 0, fmt.Errorf("truncate segment temp file: %w", err)
		}
	case info.Size() < offset:
		usable = info.Size()
	}

	w, err := ResumeSegmentTemp(path)
	if err != nil {
		return nil, 0, err
	}
	return w, usable, nil
}

// Path returns the temp file path.
func (w *SegmentWriter) Path() string {
	return w.path
}

// Write appends bytes to the segment temp file.
func (w *SegmentWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if err != nil {
		return n, fmt.Errorf("write segment temp file: %w", err)
	}
	return n, nil
}

// Truncate discards file content past size and moves the write position to the
// new end. It is used when a server ignores a resume Range header and restarts
// the transfer from byte zero. Without the seek, a writer that was not opened
// in append mode would leave a hole of zero bytes behind.
func (w *SegmentWriter) Truncate(size int64) error {
	if w.file == nil {
		return fmt.Errorf("segment temp file is closed")
	}
	if err := w.file.Truncate(size); err != nil {
		return fmt.Errorf("truncate segment temp file: %w", err)
	}
	if _, err := w.file.Seek(size, io.SeekStart); err != nil {
		return fmt.Errorf("seek segment temp file: %w", err)
	}
	return nil
}

// Close closes the segment temp file.
func (w *SegmentWriter) Close() error {
	if w.file == nil {
		return nil
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close segment temp file: %w", err)
	}
	w.file = nil
	return nil
}

// MergeFile copies a single completed temp file to the final output atomically.
func MergeFile(tempPath, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	src, err := os.Open(tempPath)
	if err != nil {
		return fmt.Errorf("open temp file for merge: %w", err)
	}
	defer src.Close()

	tmpOut := outputPath + ".tmp"
	dst, err := os.OpenFile(tmpOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open output temp file: %w", err)
	}

	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("copy to output temp file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("close output temp file: %w", closeErr)
	}

	if err := os.Rename(tmpOut, outputPath); err != nil {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("commit output file: %w", err)
	}
	return nil
}

// MergeSegments concatenates completed segment temp files into the final output.
func MergeSegments(segmentPaths []string, outputPath string, expectedSize int64) error {
	if len(segmentPaths) == 0 {
		return fmt.Errorf("no segment files to merge")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	tmpOut := outputPath + ".tmp"
	dst, err := os.OpenFile(tmpOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open output temp file: %w", err)
	}

	var total int64
	for i, path := range segmentPaths {
		src, err := os.Open(path)
		if err != nil {
			dst.Close()
			_ = os.Remove(tmpOut)
			return fmt.Errorf("open segment %d for merge: %w", i, err)
		}

		n, copyErr := io.Copy(dst, src)
		closeErr := src.Close()
		if copyErr != nil {
			dst.Close()
			_ = os.Remove(tmpOut)
			return fmt.Errorf("copy segment %d: %w", i, copyErr)
		}
		if closeErr != nil {
			dst.Close()
			_ = os.Remove(tmpOut)
			return fmt.Errorf("close segment %d: %w", i, closeErr)
		}
		total += n
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("close output temp file: %w", err)
	}

	if expectedSize > 0 && total != expectedSize {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("merged size %d does not match expected %d", total, expectedSize)
	}

	if err := os.Rename(tmpOut, outputPath); err != nil {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("commit output file: %w", err)
	}
	return nil
}

// RemoveTemp removes a temp file if it exists.
func RemoveTemp(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove temp file: %w", err)
	}
	return nil
}
