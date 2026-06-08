package util

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ValidateURL checks that a URL is well-formed and uses HTTP or HTTPS.
func ValidateURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q: only http and https are allowed", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("url host is required")
	}
	return parsed, nil
}

// SafeOutputPath resolves and validates an output path against traversal.
func SafeOutputPath(requested, baseDir string) (string, error) {
	if requested == "" {
		return "", fmt.Errorf("output path is required")
	}

	clean := filepath.Clean(requested)
	if filepath.IsAbs(clean) {
		return clean, nil
	}

	if baseDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		baseDir = wd
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base directory: %w", err)
	}
	absPath, err := filepath.Abs(filepath.Join(absBase, clean))
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return "", fmt.Errorf("check output path: %w", err)
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("output path escapes base directory")
	}
	return absPath, nil
}

// FilenameFromURL derives a filename from a URL when no output path is given.
func FilenameFromURL(raw string) (string, error) {
	parsed, err := ValidateURL(raw)
	if err != nil {
		return "", err
	}
	path := filepath.Base(parsed.Path)
	if path == "" || path == "." || path == "/" {
		return "download", nil
	}
	return path, nil
}
