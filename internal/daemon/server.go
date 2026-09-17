package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// Server exposes an authenticated loopback HTTP API for the daemon.
type Server struct {
	manager *Manager
	token   string
	http    *http.Server
}

// NewServer builds the daemon API server around a job manager.
func NewServer(manager *Manager, token string) *Server {
	s := &Server{manager: manager, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/downloads", s.auth(s.handleStart))
	mux.HandleFunc("GET /api/v1/downloads", s.auth(s.handleList))
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.auth(s.handleGet))
	mux.HandleFunc("POST /api/v1/downloads/{id}/pause", s.auth(s.handlePause))
	mux.HandleFunc("POST /api/v1/downloads/{id}/resume", s.auth(s.handleResume))
	mux.HandleFunc("POST /api/v1/queue/start", s.auth(s.handleQueueStart))
	mux.HandleFunc("GET /api/v1/health", s.auth(s.handleHealth))
	s.http = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Listen binds the loopback address and returns the bound listener.
func (s *Server) Listen(addr string) (net.Listener, error) {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse addr: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	l, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	return l, nil
}

// Serve accepts connections until the context is cancelled.
func (s *Server) Serve(l net.Listener) error {
	err := s.http.Serve(l)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully drains in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		if token != s.token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bearer token"})
			return
		}
		if !isLoopback(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "remote access forbidden"})
			return
		}
		next(w, r)
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	h = strings.TrimSpace(h)
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("daemon: write response: %v", err)
	}
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}
