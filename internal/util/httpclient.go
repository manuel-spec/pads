package util

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const defaultMaxRedirects = 10

// NewHTTPClient returns an HTTP client with bounded redirects and timeouts.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= defaultMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", defaultMaxRedirects)
			}
			return nil
		},
	}
}

// DoRequest executes an HTTP request with context cancellation.
func DoRequest(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	return resp, nil
}
