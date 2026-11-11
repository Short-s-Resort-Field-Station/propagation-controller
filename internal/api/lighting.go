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

// --- circulation fans: the lighting scheduler bound to the fan relay ---

func (s *Server) fanSvc(w http.ResponseWriter) *lighting.Service {
	if s.fans == nil {
		writeError(w, http.StatusServiceUnavailable, "fan schedule service not configured")
	}
	return s.fans
}

func (s *Server) handleFanStatus(w http.ResponseWriter, r *http.Request) {
	if svc := s.fanSvc(w); svc != nil {
		writeJSON(w, http.StatusOK, svc.Status(r.Context()))
	}
}
func (s *Server) handleFanOn(w http.ResponseWriter, r *http.Request) {
	if svc := s.fanSvc(w); svc != nil {
		s.scheduledSet(w, r, svc, "on")
	}
}
func (s *Server) handleFanOff(w http.ResponseWriter, r *http.Request) {
	if svc := s.fanSvc(w); svc != nil {
		s.scheduledSet(w, r, svc, "off")
	}
}
func (s *Server) handleFanOverride(w http.ResponseWriter, r *http.Request) {
	if svc := s.fanSvc(w); svc != nil {
		s.scheduledSet(w, r, svc, "")
	}
}
func (s *Server) handleGetFanSchedule(w http.ResponseWriter, r *http.Request) {
	if svc := s.fanSvc(w); svc != nil {
		s.getSchedule(w, svc, "no fan schedule configured")
	}
}
func (s *Server) handlePutFanSchedule(w http.ResponseWriter, r *http.Request) {
	if svc := s.fanSvc(w); svc != nil {
		s.putSchedule(w, r, svc)
	}
}

type lightingOverrideRequest struct {
	Actor          string `json:"actor"`
	ExpiresMinutes int    `json:"expires_minutes"`
	Mode           string `json:"mode"` // only used by /override
}

func (s *Server) lightingSet(w http.ResponseWriter, r *http.Request, mode string) {
	s.scheduledSet(w, r, s.lighting, mode)
}

func (s *Server) scheduledSet(w http.ResponseWriter, r *http.Request, svc *lighting.Service, mode string) {
	var req lightingOverrideRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	if mode == "" {
		mode = req.Mode
	}
	if err := svc.SetOverride(r.Context(), mode, actor, req.ExpiresMinutes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, svc.Status(r.Context()))
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
	s.getSchedule(w, s.lighting, "no lighting schedule configured")
}

func (s *Server) handlePutLightingSchedule(w http.ResponseWriter, r *http.Request) {
	s.putSchedule(w, r, s.lighting)
}

func (s *Server) getSchedule(w http.ResponseWriter, svc *lighting.Service, missing string) {
	sch, err := svc.GetSchedule()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sch == nil {
		writeError(w, http.StatusNotFound, missing)
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) putSchedule(w http.ResponseWriter, r *http.Request, svc *lighting.Service) {
	var sch lighting.Schedule
	if err := json.NewDecoder(r.Body).Decode(&sch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := svc.PutSchedule(sch, "api"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) handleListSpecies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"species": species.Catalog})
}
