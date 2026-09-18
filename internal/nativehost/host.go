package nativehost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"pads/internal/config"
	"pads/internal/daemon"
	"pads/internal/util"
)

// dialer opens a daemon client. It is a field so tests can supply their own and
// so each request re-reads the control file: the daemon may have restarted on a
// different port since the browser launched this host.
type dialer func() (*daemon.Client, error)

// Host answers extension requests by calling the daemon.
type Host struct {
	cfg  *config.Config
	dial dialer
}

// New builds a host bound to the given configuration.
func New(cfg *config.Config) *Host {
	return &Host{
		cfg:  cfg,
		dial: func() (*daemon.Client, error) { return daemon.Dial(cfg.StateDir) },
	}
}

// Serve reads framed requests until the stream closes. The browser closes it
// when the extension disconnects or the browser exits.
func (h *Host) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	for {
		payload, err := readMessage(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			// A framing error desynchronises the stream, so there is nothing
			// left to read; report it and let the browser restart the host.
			return err
		}

		resp := h.handle(ctx, payload)
		if err := writeResponse(out, resp); err != nil {
			return err
		}
	}
}

func (h *Host) handle(ctx context.Context, payload []byte) Response {
	var req Request
	if err := decodeRequest(payload, &req); err != nil {
		return errorResponse(err)
	}

	switch req.Type {
	case "health":
		return h.health()
	case "start":
		return h.start(ctx, req)
	case "jobs":
		return h.jobs(ctx)
	case "pause":
		return h.pause(ctx, req)
	case "resume":
		return h.resume(ctx, req)
	default:
		return errorResponse(fmt.Errorf("unknown request type %q", req.Type))
	}
}

// health reports whether the daemon is reachable. A missing daemon is an
// ordinary answer rather than an error: the extension uses it to decide whether
// to capture downloads at all.
func (h *Host) health() Response {
	client, err := h.dial()
	if err != nil {
		return Response{OK: true, Running: false}
	}
	if _, err := client.Health(); err != nil {
		return Response{OK: true, Running: false, Addr: client.Addr()}
	}
	return Response{OK: true, Running: true, Addr: client.Addr()}
}

func (h *Host) start(ctx context.Context, req Request) Response {
	if _, err := util.ValidateURL(req.URL); err != nil {
		return errorResponse(err)
	}

	output, err := h.outputPath(req.URL, req.Filename)
	if err != nil {
		return errorResponse(err)
	}

	client, err := h.dial()
	if err != nil {
		return errorResponse(errNoDaemon(err))
	}
	id, err := client.Start(ctx, req.URL, output)
	if err != nil {
		return errorResponse(err)
	}
	return Response{OK: true, Running: true, ID: id, Output: output}
}

func (h *Host) jobs(ctx context.Context) Response {
	client, err := h.dial()
	if err != nil {
		return Response{OK: true, Running: false}
	}
	jobs, err := client.Jobs(ctx)
	if err != nil {
		return Response{OK: true, Running: false}
	}

	out := make([]Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, Job{
			ID:     job.ID,
			URL:    job.URL,
			Output: job.Output,
			Status: string(job.Status),
			Error:  job.Error,
			Bytes:  job.Bytes,
			Total:  job.Total,
		})
	}
	return Response{OK: true, Running: true, Jobs: out}
}

func (h *Host) pause(ctx context.Context, req Request) Response {
	client, err := h.dial()
	if err != nil {
		return errorResponse(errNoDaemon(err))
	}
	if err := client.Pause(ctx, req.ID); err != nil {
		return errorResponse(err)
	}
	return Response{OK: true, Running: true, ID: req.ID}
}

func (h *Host) resume(ctx context.Context, req Request) Response {
	client, err := h.dial()
	if err != nil {
		return errorResponse(errNoDaemon(err))
	}
	if err := client.Resume(ctx, req.ID); err != nil {
		return errorResponse(err)
	}
	return Response{OK: true, Running: true, ID: req.ID}
}

func errNoDaemon(err error) error {
	if errors.Is(err, daemon.ErrNoDaemon) {
		return errors.New("no PADS daemon is running; start one with 'pads daemon run'")
	}
	return err
}

// outputPath decides where a browser-initiated download lands. The suggested
// filename comes from the page, so it is untrusted: only its base name is kept
// and the result must stay inside the configured download directory.
func (h *Host) outputPath(rawURL, suggested string) (string, error) {
	name := cleanFilename(suggested)
	if name == "" {
		derived, err := util.FilenameFromURL(rawURL)
		if err != nil {
			return "", err
		}
		name = cleanFilename(derived)
	}
	if name == "" {
		name = "download"
	}

	if err := os.MkdirAll(h.cfg.DownloadDir, 0o755); err != nil {
		return "", fmt.Errorf("create download directory: %w", err)
	}

	path, err := util.SafeOutputPath(name, h.cfg.DownloadDir)
	if err != nil {
		return "", err
	}
	return uniquePath(path), nil
}

// cleanFilename reduces a browser-supplied name to a single path element.
func cleanFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Browsers hand back native separators; normalise both so a Windows-style
	// name cannot smuggle a directory past filepath.Base on Linux.
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)

	switch name {
	case ".", "..", "/", "":
		return ""
	}
	return name
}

// uniquePath avoids overwriting a file already in the download directory, the
// way a browser's own downloader would.
func uniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return path
}
