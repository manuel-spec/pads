package daemon

import "net/http"

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
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "url is required"})
		return
	}
	id, err := s.manager.Start(r.Context(), req.URL, req.Output)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, startResponse{ID: id})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.List())
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	job, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Pause(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.ResumeJob(r.Context(), id); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, startResponse{ID: id})
}

func (s *Server) handleQueueStart(w http.ResponseWriter, r *http.Request) {
	id, err := s.manager.StartQueue(r.Context())
	if err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, startResponse{ID: id})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Jobs: len(s.manager.List())})
}
