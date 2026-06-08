package util

import (
	"path/filepath"
	"testing"
)

func TestValidateURL(t *testing.T) {
	_, err := ValidateURL("ftp://example.com/file")
	if err == nil {
		t.Fatal("expected ftp scheme to be rejected")
	}

	u, err := ValidateURL("https://example.com/file.bin")
	if err != nil {
		t.Fatalf("valid url rejected: %v", err)
	}
	if u.Host != "example.com" {
		t.Fatalf("unexpected host: %s", u.Host)
	}
}

func TestSafeOutputPathRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	_, err := SafeOutputPath("../escape.txt", base)
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestSafeOutputPathAllowsRelative(t *testing.T) {
	base := t.TempDir()
	got, err := SafeOutputPath("file.bin", base)
	if err != nil {
		t.Fatalf("relative path rejected: %v", err)
	}
	want := filepath.Join(base, "file.bin")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestFilenameFromURL(t *testing.T) {
	name, err := FilenameFromURL("https://example.com/path/archive.zip")
	if err != nil {
		t.Fatalf("filename from url: %v", err)
	}
	if name != "archive.zip" {
		t.Fatalf("expected archive.zip, got %s", name)
	}
}
