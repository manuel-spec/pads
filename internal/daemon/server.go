package daemon

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server exposes an authenticated loopback HTTP API for the daemon.
type Server struct {
	manager      *Manager
	token        string
	http         *http.Server
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// NewServer builds the daemon API server around a job manager.
func NewServer(manager *Manager, token string) *Server {
	s := &Server{manager: manager, token: token, shutdown: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/downloads", s.handleStart)
	mux.HandleFunc("GET /api/v1/downloads", s.handleList)
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.handleGet)
	mux.HandleFunc("POST /api/v1/downloads/{id}/pause", s.handlePause)
	mux.HandleFunc("POST /api/v1/downloads/{id}/resume", s.handleResume)
	mux.HandleFunc("POST /api/v1/queue/start", s.handleQueueStart)
	mux.HandleFunc("POST /api/v1/shutdown", s.handleShutdown)
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.http = &http.Server{
		Handler:           s.auth(mux.ServeHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	return s
}

// ShutdownRequested is closed when a client asks the daemon to stop.
func (s *Server) ShutdownRequested() <-chan struct{} {
	return s.shutdown
}

func (s *Server) signalShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdown) })
}

// Listen binds the loopback address and returns the bound listener.
func (s *Server) Listen(addr string) (net.Listener, error) {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	if err := validateAddress(addr); err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", addr)
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
		if !isLoopback(r) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "remote access forbidden"})
			return
		}
		for key := range r.Header {
			if strings.EqualFold(key, "Origin") {
				writeJSON(w, http.StatusForbidden, errorResponse{Error: "browser origin forbidden"})
				return
			}
		}
		token := bearer(r)
		got, want := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(s.token))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 || token == "" || s.token == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid bearer token"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		next(w, r)
	}
}

func bearer(r *http.Request) string {
	if len(r.Header.Values("Authorization")) != 1 {
		return ""
	}
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

const maxRequestBody = 64 << 10

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxRequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if err := dec.Decode(new(interface{})); err != io.EOF {
		if err == nil {
			return errors.New("request must contain exactly one JSON value")
		}
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func validateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse addr: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("daemon address must use a literal loopback IP")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return errors.New("daemon address must use a numeric port from 0 to 65535")
	}
	return nil
}
