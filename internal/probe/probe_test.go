package probe_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"pads/internal/probe"
	"pads/testutil"
)

func TestProbeRangeSupport(t *testing.T) {
	content := []byte("hello pads probe test data")
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      content,
		SupportRange: true,
	})
	defer server.Close()

	profile, err := probe.Probe(context.Background(), server.Client(), server.URL, probe.Options{
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if !profile.RangeSupported {
		t.Fatal("expected range support")
	}
	if profile.ContentLength != int64(len(content)) {
		t.Fatalf("expected content length %d, got %d", len(content), profile.ContentLength)
	}
}

func TestProbeWithoutRange(t *testing.T) {
	server := testutil.NewFileServer(testutil.FileServerOptions{
		Content:      []byte("no range"),
		SupportRange: false,
	})
	defer server.Close()

	profile, err := probe.Probe(context.Background(), server.Client(), server.URL, probe.Options{
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if profile.RangeSupported {
		t.Fatal("expected range to be unsupported")
	}
	if profile.RecommendedConns != 1 {
		t.Fatalf("expected 1 recommended connection, got %d", profile.RecommendedConns)
	}
}

func TestProbeInvalidURL(t *testing.T) {
	_, err := probe.Probe(context.Background(), http.DefaultClient, "not-a-url", probe.Options{
		Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("expected probe to fail for invalid url")
	}
}
