package downloader

import (
	"errors"
	"net/http"
	"testing"

	"pads/internal/model"
)

func TestCheckSegmentResponsePartialContent(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusPartialContent}
	seg := model.Segment{ID: "seg-0", ByteStart: 10, ByteEnd: 20}
	if err := checkSegmentResponse(resp, &seg); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestCheckSegmentResponse416(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable}
	seg := model.Segment{ID: "seg-1", ByteStart: 0, ByteEnd: 9}
	err := checkSegmentResponse(resp, &seg)
	if err == nil {
		t.Fatal("expected 416 error")
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != 416 {
		t.Fatalf("expected HTTPStatusError 416, got %v", err)
	}
}

func TestCheckSegmentResponse503Retryable(t *testing.T) {
	err := &HTTPStatusError{StatusCode: http.StatusServiceUnavailable, Message: "busy"}
	if !isRetryable(err) {
		t.Fatal("expected 503 to be retryable")
	}
}

func TestCheckSegmentResponse416NotRetryable(t *testing.T) {
	err := &HTTPStatusError{StatusCode: http.StatusRequestedRangeNotSatisfiable, Message: "range"}
	if isRetryable(err) {
		t.Fatal("expected 416 to be non-retryable")
	}
}
