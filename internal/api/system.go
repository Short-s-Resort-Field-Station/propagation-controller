package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleSystemShutdown is the touch-screen power-down (hidden long-press on
// the masthead divider, Austin 2026-11-10): stop any cycle, force every relay
// off, record who asked, answer, then power the controller off cleanly so a
// shop power cut is never the way this unit goes dark with a valve open.
// Admin-only (the kiosk is loopback = implicit admin).
func (s *Server) handleSystemShutdown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Actor  string `json:"actor"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	reason := req.Reason
	if reason == "" {
		reason = "shutdown requested from the screen"
	}
	s.irrigation.StopAll(actor)
	if err := s.mgr.AllOff(r.Context(), actor, reason); err != nil {
		writeError(w, http.StatusBadGateway, "could not make the unit safe: "+err.Error())
		return
	}
	s.eventLog.Warn("system_shutdown_requested", "controller powering down: "+reason, actor,
		map[string]any{"poweroff": s.powerOff != nil})
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "shutting_down", "poweroff": s.powerOff != nil})
	if s.powerOff == nil {
		return
	}
	go func() {
		time.Sleep(1500 * time.Millisecond) // let the response and the event log land
		if err := s.powerOff(); err != nil {
			s.eventLog.Error("system_shutdown_failed", "poweroff command failed: "+err.Error(), "system", nil)
		}
	}()
}
