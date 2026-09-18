package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAuthTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	manager, _ := newTestManager(t)
	const token = "correct-horse-battery-staple"
	return NewServer(manager, token), token
}

func TestAuthRejectsBadCredentials(t *testing.T) {
	api, token := newAuthTestServer(t)
	httpServer := httptest.NewServer(api.http.Handler)
	defer httpServer.Close()

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{name: "no header", header: "", want: http.StatusUnauthorized},
		{name: "wrong token", header: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "not bearer", header: "Basic " + token, want: http.StatusUnauthorized},
		{name: "correct token", header: "Bearer " + token, want: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/api/v1/health", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := httpServer.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

// A browser can reach loopback with a valid token only if a page already has
// it, but an Origin header still marks the request as browser-driven and is
// refused outright.
func TestAuthRejectsBrowserOrigin(t *testing.T) {
	api, token := newAuthTestServer(t)
	httpServer := httptest.NewServer(api.http.Handler)
	defer httpServer.Close()

	req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://example.com")

	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestAuthRejectsNonLoopbackPeer(t *testing.T) {
	api, token := newAuthTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "203.0.113.7:54321"

	rec := httptest.NewRecorder()
	api.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestPathIDMustBeCanonicalUUID(t *testing.T) {
	api, token := newAuthTestServer(t)
	httpServer := httptest.NewServer(api.http.Handler)
	defer httpServer.Close()

	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/downloads/not-a-uuid/pause", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestValidateAddressRequiresLoopback(t *testing.T) {
	valid := []string{"127.0.0.1:0", "127.0.0.1:8080", "[::1]:9000"}
	for _, addr := range valid {
		if err := validateAddress(addr); err != nil {
			t.Errorf("validateAddress(%q) = %v, want nil", addr, err)
		}
	}

	invalid := []string{"0.0.0.0:8080", "example.com:80", "127.0.0.1:99999", "127.0.0.1", ""}
	for _, addr := range invalid {
		if err := validateAddress(addr); err == nil {
			t.Errorf("validateAddress(%q) = nil, want an error", addr)
		}
	}
}

func TestShutdownEndpointSignalsOnce(t *testing.T) {
	api, token := newAuthTestServer(t)
	httpServer := httptest.NewServer(api.http.Handler)
	defer httpServer.Close()

	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/shutdown", strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
		}
	}

	select {
	case <-api.ShutdownRequested():
	default:
		t.Fatal("shutdown was not signalled")
	}
}
