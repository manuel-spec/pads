package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNoDaemon indicates that no daemon control file could be read.
var ErrNoDaemon = errors.New("no running daemon")

const clientTimeout = 10 * time.Second

// Client calls the daemon API over loopback using the published bearer token.
type Client struct {
	addr  string
	token string
	http  *http.Client
}

// Dial reads the daemon control file and returns a client for that daemon.
// It does not confirm the daemon is alive; use Health for that.
func Dial(stateDir string) (*Client, error) {
	_, addr, token, err := ReadInfo(stateDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoDaemon, err)
	}
	return &Client{
		addr:  addr,
		token: token,
		http: &http.Client{
			// A proxy must never see loopback daemon traffic, token included.
			Transport: &http.Transport{Proxy: nil},
			Timeout:   clientTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Addr returns the daemon's loopback address.
func (c *Client) Addr() string {
	return c.addr
}

// Health reports whether the daemon answers on its published address.
func (c *Client) Health() (int, error) {
	var resp healthResponse
	if err := c.do(context.Background(), http.MethodGet, "/api/v1/health", nil, &resp); err != nil {
		return 0, err
	}
	return resp.Jobs, nil
}

// Jobs lists every job the daemon knows about.
func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	var jobs []Job
	if err := c.do(ctx, http.MethodGet, "/api/v1/downloads", nil, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Start hands a new download to the daemon and returns its job ID.
func (c *Client) Start(ctx context.Context, url, output string) (string, error) {
	var resp startResponse
	body := startRequest{URL: url, Output: output}
	if err := c.do(ctx, http.MethodPost, "/api/v1/downloads", body, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// Pause stops a running job, leaving its progress resumable.
func (c *Client) Pause(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/downloads/"+id+"/pause", nil, nil)
}

// Resume restarts a saved download inside the daemon.
func (c *Client) Resume(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/downloads/"+id+"/resume", nil, nil)
}

// StartQueue asks the daemon to work through pending queue entries.
func (c *Client) StartQueue(ctx context.Context) (string, error) {
	var resp startResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/queue/start", nil, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// Shutdown asks the daemon to stop accepting work and exit.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/shutdown", nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.addr+path, payload)
	if err != nil {
		return fmt.Errorf("create daemon request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("daemon: %s", apiError(resp))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxRequestBody))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(out); err != nil {
		return fmt.Errorf("decode daemon response: %w", err)
	}
	return nil
}

const maxResponseBody = 8 << 20

// apiError renders the daemon's error payload, falling back to the status line
// when the body is not the JSON shape the API promises.
func apiError(resp *http.Response) string {
	var payload errorResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRequestBody)).Decode(&payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	return resp.Status
}
