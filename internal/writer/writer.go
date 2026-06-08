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

// RemoveTemp removes a temp file if it exists.
func RemoveTemp(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove temp file: %w", err)
	}
	return nil
}
