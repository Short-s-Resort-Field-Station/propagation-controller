package api

import (
	"net/http"
	"time"

	"propagation-controller/internal/version"
)

// handleHealth implements GET /api/v1/health (spec §15.1). Subsystems that
// don't exist yet in Phase 0 report "not_started" rather than lying with "ok".
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := s.db.PingContext(r.Context()); err != nil {
		dbStatus = "error"
	}
	relayHealth := s.mgr.ControllerHealth(r.Context())
	busHealth := s.sensors.BusHealth(r.Context())

	healthy := func(status string) bool { return status == "online" || status == "simulated" }
	status := "ok"
	if dbStatus != "ok" || !healthy(relayHealth.Status) || !healthy(busHealth.Status) {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":            status,
		"version":           version.Version,
		"uptime_seconds":    int(time.Since(s.startedAt).Seconds()),
		"database":          dbStatus,
		"relay_controller":  relayHealth.Status,
		"modbus_bus":        busHealth.Status,
		"scheduler":         "running",
		"safety_supervisor": s.safety.HealthStatus(),
		"simulation":        s.simulation,
	})
}
