package api

import (
	"encoding/json"
	"net/http"

	"propagation-controller/internal/lighting"
	"propagation-controller/internal/species"
)

func (s *Server) handleLightingStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.lighting.Status(r.Context()))
}

type lightingOverrideRequest struct {
	Actor          string `json:"actor"`
	ExpiresMinutes int    `json:"expires_minutes"`
	Mode           string `json:"mode"` // only used by /override
}

func (s *Server) lightingSet(w http.ResponseWriter, r *http.Request, mode string) {
	var req lightingOverrideRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	if mode == "" {
		mode = req.Mode
	}
	if err := s.lighting.SetOverride(r.Context(), mode, actor, req.ExpiresMinutes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.lighting.Status(r.Context()))
}

func (s *Server) handleLightingOn(w http.ResponseWriter, r *http.Request) {
	s.lightingSet(w, r, "on")
}

func (s *Server) handleLightingOff(w http.ResponseWriter, r *http.Request) {
	s.lightingSet(w, r, "off")
}

func (s *Server) handleLightingOverride(w http.ResponseWriter, r *http.Request) {
	s.lightingSet(w, r, "")
}

func (s *Server) handleGetLightingSchedule(w http.ResponseWriter, r *http.Request) {
	sch, err := s.lighting.GetSchedule()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sch == nil {
		writeError(w, http.StatusNotFound, "no lighting schedule configured")
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) handlePutLightingSchedule(w http.ResponseWriter, r *http.Request) {
	var sch lighting.Schedule
	if err := json.NewDecoder(r.Body).Decode(&sch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := s.lighting.PutSchedule(sch, "api"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) handleListSpecies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"species": species.Catalog})
}
