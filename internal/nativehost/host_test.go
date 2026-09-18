package nativehost

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pads/internal/config"
	"pads/internal/daemon"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = filepath.Join(dir, "state")
	cfg.DownloadDir = filepath.Join(dir, "downloads")
	return cfg
}

// fakeDaemon stands in for a running daemon: it publishes a control file the
// host can dial and records the requests it receives.
func fakeDaemon(t *testing.T, cfg *config.Config, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	info := map[string]interface{}{
		"pid":   os.Getpid(),
		"addr":  server.Listener.Addr().String(),
		"token": "test-token",
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "daemon.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return server
}

func TestStartForwardsToDaemon(t *testing.T) {
	cfg := testConfig(t)

	var gotBody map[string]string
	fakeDaemon(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer token: %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "job-1"})
	})

	host := New(cfg)
	resp := host.handle(context.Background(), []byte(`{"type":"start","url":"http://example.test/file.bin","filename":"file.bin"}`))

	if !resp.OK {
		t.Fatalf("response not ok: %s", resp.Error)
	}
	if resp.ID != "job-1" {
		t.Fatalf("id = %q, want job-1", resp.ID)
	}
	if gotBody["url"] != "http://example.test/file.bin" {
		t.Fatalf("forwarded url = %q", gotBody["url"])
	}
	want := filepath.Join(cfg.DownloadDir, "file.bin")
	if gotBody["output"] != want {
		t.Fatalf("forwarded output = %q, want %q", gotBody["output"], want)
	}
}

// A filename comes from the page and is therefore untrusted. It must never
// place a file outside the download directory.
func TestOutputPathRejectsTraversal(t *testing.T) {
	cfg := testConfig(t)
	host := New(cfg)

	cases := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "posix traversal", filename: "../../etc/passwd", want: "passwd"},
		{name: "absolute", filename: "/etc/shadow", want: "shadow"},
		{name: "windows traversal", filename: `..\..\windows\system32\evil.dll`, want: "evil.dll"},
		{name: "dot", filename: ".", want: "file.bin"},
		{name: "empty falls back to url", filename: "", want: "file.bin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := host.outputPath("http://example.test/file.bin", tc.filename)
			if err != nil {
				t.Fatalf("outputPath: %v", err)
			}
			if filepath.Dir(got) != cfg.DownloadDir {
				t.Fatalf("path %q escaped %q", got, cfg.DownloadDir)
			}
			if filepath.Base(got) != tc.want {
				t.Fatalf("name = %q, want %q", filepath.Base(got), tc.want)
			}
		})
	}
}

func TestOutputPathDoesNotOverwrite(t *testing.T) {
	cfg := testConfig(t)
	host := New(cfg)

	first, err := host.outputPath("http://example.test/file.bin", "file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := host.outputPath("http://example.test/file.bin", "file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("second download would overwrite %q", first)
	}
	if filepath.Base(second) != "file (1).bin" {
		t.Fatalf("name = %q, want %q", filepath.Base(second), "file (1).bin")
	}
}

func TestStartRejectsUnsupportedScheme(t *testing.T) {
	host := New(testConfig(t))
	resp := host.handle(context.Background(), []byte(`{"type":"start","url":"file:///etc/passwd"}`))
	if resp.OK {
		t.Fatal("expected a file:// URL to be rejected")
	}
	if !strings.Contains(resp.Error, "scheme") {
		t.Fatalf("error = %q, want a scheme complaint", resp.Error)
	}
}

// Without a daemon the host still answers; the extension needs to distinguish
// "no daemon" from "the host is broken".
func TestHealthWithoutDaemon(t *testing.T) {
	host := New(testConfig(t))
	resp := host.handle(context.Background(), []byte(`{"type":"health"}`))
	if !resp.OK {
		t.Fatalf("health failed: %s", resp.Error)
	}
	if resp.Running {
		t.Fatal("reported a running daemon when none exists")
	}
}

func TestStartWithoutDaemonExplainsHowToFix(t *testing.T) {
	host := New(testConfig(t))
	resp := host.handle(context.Background(), []byte(`{"type":"start","url":"http://example.test/f.bin"}`))
	if resp.OK {
		t.Fatal("expected start to fail without a daemon")
	}
	if !strings.Contains(resp.Error, "pads daemon run") {
		t.Fatalf("error = %q, want it to name the fix", resp.Error)
	}
}

func TestUnknownRequestType(t *testing.T) {
	host := New(testConfig(t))
	resp := host.handle(context.Background(), []byte(`{"type":"explode"}`))
	if resp.OK {
		t.Fatal("expected an unknown type to fail")
	}
}

// Serve must answer each framed request in order and stop cleanly at EOF.
func TestServeAnswersFramedRequests(t *testing.T) {
	host := New(testConfig(t))

	var in bytes.Buffer
	in.Write(framed(`{"type":"health"}`))
	in.Write(framed(`{"type":"explode"}`))

	var out bytes.Buffer
	if err := host.Serve(context.Background(), &in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	for i, wantOK := range []bool{true, false} {
		payload, err := readMessage(&out)
		if err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
		var resp Response
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
		if resp.OK != wantOK {
			t.Fatalf("reply %d ok = %v, want %v", i, resp.OK, wantOK)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("%d unread bytes after the final reply", out.Len())
	}
}

func TestJobsMapsDaemonFields(t *testing.T) {
	cfg := testConfig(t)
	fakeDaemon(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]daemon.Job{{
			ID:     "job-1",
			URL:    "http://example.test/f.bin",
			Output: "/tmp/f.bin",
			Status: daemon.JobRunning,
			Bytes:  512,
			Total:  2048,
		}})
	})

	host := New(cfg)
	resp := host.handle(context.Background(), []byte(`{"type":"jobs"}`))
	if !resp.OK || !resp.Running {
		t.Fatalf("jobs failed: ok=%v running=%v err=%s", resp.OK, resp.Running, resp.Error)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(resp.Jobs))
	}
	job := resp.Jobs[0]
	if job.ID != "job-1" || job.Status != "running" || job.Bytes != 512 || job.Total != 2048 {
		t.Fatalf("job mapped incorrectly: %+v", job)
	}
}
