package daemon

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"pads/internal/state"
)

type startRequest struct {
	URL    string `json:"url"`
	Output string `json:"output,omitempty"`
}

type startResponse struct {
	ID string `json:"id"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type healthResponse struct {
	Status string `json:"status"`
	Jobs   int    `json:"jobs"`
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := decodeJSON(r, &req); err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "url is required"})
		return
	}
	id, err := s.manager.Start(req.URL, req.Output)
	if err != nil {
		writeJSON(w, statusForJobError(err, http.StatusBadRequest), errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, startResponse{ID: id})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.List())
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id, valid := requestID(w, r)
	if !valid {
		return
	}
	job, ok := s.manager.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	id, valid := requestID(w, r)
	if !valid {
		return
	}
	if err := s.manager.Pause(id); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	id, valid := requestID(w, r)
	if !valid {
		return
	}
	if err := s.manager.ResumeJob(id); err != nil {
		writeJSON(w, statusForJobError(err, http.StatusConflict), errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, startResponse{ID: id})
}

func (s *Server) handleQueueStart(w http.ResponseWriter, r *http.Request) {
	id, err := s.manager.StartQueue()
	if err != nil {
		writeJSON(w, statusForJobError(err, http.StatusConflict), errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, startResponse{ID: id})
}

func requestID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "id must be a canonical UUID"})
		return "", false
	}
	return id, true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Jobs: len(s.manager.List())})
}

// handleShutdown asks the daemon to stop. The response is written before the
// signal so the caller sees an answer rather than a dropped connection.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	s.signalShutdown()
}

// statusForJobError maps a manager error to an HTTP status, defaulting to the
// caller's choice for ordinary rejections.
func statusForJobError(err error, fallback int) int {
	switch {
	case errors.Is(err, ErrShuttingDown):
		return http.StatusServiceUnavailable
	case errors.Is(err, state.ErrNotFound):
		return http.StatusNotFound
	default:
		return fallback
	}
}
