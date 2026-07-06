package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSafetyStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.safety.Status(r.Context()))
}

func (s *Server) handleEmergencyStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
		Actor  string `json:"actor"`
	}
	// Body optional: an emergency stop must work from a bare curl.
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	s.safety.EmergencyStop(r.Context(), actor, req.Reason)
	writeJSON(w, http.StatusOK, s.safety.Status(r.Context()))
}

func (s *Server) handleSafetyReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Actor string `json:"actor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	if err := s.safety.Reset(actor); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.safety.Status(r.Context()))
}

func (s *Server) handleListLockouts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lockouts": s.safety.Status(r.Context()).Lockouts})
}

func (s *Server) handleClearLockout(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "lockout_key")
	var req struct {
		Actor string `json:"actor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	if err := s.safety.ClearLockout(key, actor); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
