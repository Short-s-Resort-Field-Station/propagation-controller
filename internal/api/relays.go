package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/devices/relay"
)

var allowedActors = map[string]bool{
	"api":          true,
	"manual_web":   true,
	"manual_touch": true,
}

func (s *Server) handleListRelays(w http.ResponseWriter, r *http.Request) {
	infos, err := s.mgr.Snapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "relay controller unavailable: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"relays":     infos,
		"controller": s.mgr.ControllerHealth(r.Context()),
	})
}

func (s *Server) handleGetRelay(w http.ResponseWriter, r *http.Request) {
	channel, ok := parseChannel(w, r)
	if !ok {
		return
	}
	info, err := s.mgr.GetRelay(r.Context(), channel)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type setRelayRequest struct {
	State          string `json:"state"`
	Reason         string `json:"reason"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Actor          string `json:"actor"`
}

// handleSetRelay implements POST /api/v1/relays/{channel}/set (spec §15.4).
// timeout_seconds is enforced server-side: the relay turns itself off even if
// the client disappears.
func (s *Server) handleSetRelay(w http.ResponseWriter, r *http.Request) {
	channel, ok := parseChannel(w, r)
	if !ok {
		return
	}
	var req setRelayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	state := relay.State(req.State)
	if state != relay.On && state != relay.Off {
		writeError(w, http.StatusBadRequest, `state must be "on" or "off"`)
		return
	}
	if req.TimeoutSeconds < 0 {
		writeError(w, http.StatusBadRequest, "timeout_seconds must be >= 0")
		return
	}
	actor := req.Actor
	if actor == "" {
		actor = "api"
	}
	if !allowedActors[actor] {
		writeError(w, http.StatusBadRequest, "actor must be api, manual_web, or manual_touch")
		return
	}

	// The safety supervisor gates every relay-on command (spec §12); off is
	// always allowed.
	if err := s.safety.Authorize(channel, state); err != nil {
		writeError(w, http.StatusLocked, err.Error())
		return
	}

	previous, err := s.mgr.SetRelay(r.Context(), channel, state, actor, req.Reason, req.TimeoutSeconds)
	if err != nil {
		writeError(w, http.StatusBadGateway, "relay command failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"channel":         channel,
		"previous_state":  previous,
		"state":           state,
		"timeout_seconds": req.TimeoutSeconds,
	})
}

func (s *Server) handleAllOff(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
		Actor  string `json:"actor"`
	}
	// Body is optional for all-off; it must work from a bare curl.
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	reason := req.Reason
	if reason == "" {
		reason = "all-off requested"
	}
	// Stop irrigation first so the state machine doesn't fight the all-off.
	s.irrigation.StopAll(actor)
	if err := s.mgr.AllOff(r.Context(), actor, reason); err != nil {
		writeError(w, http.StatusBadGateway, "all-off completed with errors: "+err.Error())
		return
	}
	s.eventLog.Info("relays_all_off", "all relays turned off", actor, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.deviceRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}

func parseChannel(w http.ResponseWriter, r *http.Request) (int, bool) {
	channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
	if err != nil || channel < 1 || channel > relay.NumChannels {
		writeError(w, http.StatusBadRequest, "channel must be an integer 1..8")
		return 0, false
	}
	return channel, true
}
