package probe

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pads/internal/model"
	"pads/internal/util"
)

// Options configures server probing behavior.
type Options struct {
	Timeout time.Duration
}

// Probe inspects a remote server and returns a scheduling profile.
func Probe(ctx context.Context, client *http.Client, url string, opts Options) (*model.ServerProfile, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	if client == nil {
		client = util.NewHTTPClient(opts.Timeout)
	}

	start := time.Now()
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create head request: %w", err)
	}

	headResp, err := util.DoRequest(ctx, client, headReq)
	if err != nil {
		return nil, fmt.Errorf("head request: %w", err)
	}
	defer headResp.Body.Close()

	latencyMS := float64(time.Since(start).Milliseconds())
	profile := &model.ServerProfile{
		ContentType:      headResp.Header.Get("Content-Type"),
		HTTPVersion:      headResp.Proto,
		LatencyMS:        latencyMS,
		AcceptRanges:     headResp.Header.Get("Accept-Ranges"),
		RangeSupported:   supportsRanges(headResp),
		RecommendedConns: 1,
	}

	if length, ok := parseContentLength(headResp); ok {
		profile.ContentLength = length
		profile.ContentLengthTrusted = headResp.StatusCode == http.StatusOK
	}

	if profile.RangeSupported && profile.ContentLength > 0 {
		profile.RecommendedConns = recommendConnections(profile.ContentLength, profile.LatencyMS)
	}

	if err := verifyRangeSupport(ctx, client, url, profile); err != nil {
		profile.RangeSupported = false
		profile.RecommendedConns = 1
	}

	return profile, nil
}

func supportsRanges(resp *http.Response) bool {
	accept := strings.ToLower(resp.Header.Get("Accept-Ranges"))
	if accept == "bytes" {
		return true
	}
	if resp.Header.Get("Content-Range") != "" {
		return true
	}
	return false
}

func parseContentLength(resp *http.Response) (int64, bool) {
	raw := resp.Header.Get("Content-Length")
	if raw == "" {
		return 0, false
	}
	length, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || length < 0 {
		return 0, false
	}
	return length, true
}

func verifyRangeSupport(ctx context.Context, client *http.Client, url string, profile *model.ServerProfile) error {
	if !profile.RangeSupported {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create range request: %w", err)
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := util.DoRequest(ctx, client, req)
	if err != nil {
		return fmt.Errorf("range request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server returned %d for range request", resp.StatusCode)
	}

	if profile.ContentLength == 0 {
		if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
			profile.ContentLength = total
			profile.ContentLengthTrusted = true
		}
	}
	return nil
}

func parseContentRangeTotal(header string) (int64, bool) {
	if header == "" {
		return 0, false
	}
	parts := strings.Split(header, "/")
	if len(parts) != 2 {
		return 0, false
	}
	total, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || total <= 0 {
		return 0, false
	}
	return total, true
}

func recommendConnections(size int64, latencyMS float64) int {
	switch {
	case size < 5*1024*1024:
		return 2
	case latencyMS > 300:
		return 2
	case size < 50*1024*1024:
		return 4
	default:
		return 6
	}
}
