package api

import (
	"encoding/json"
	"net/http"

	"propagation-controller/internal/safety"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.settingsRepo.All()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]json.RawMessage{}
	for k, v := range all {
		out[k] = json.RawMessage(v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": out})
}

// handlePutSettings accepts a partial map of settings. The "safety" key is
// validated and hot-applied to the running supervisor; other keys are stored
// as-is for their owning modules.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "no settings provided")
		return
	}
	for key, raw := range body {
		if key == "safety" {
			var cfg safety.Config
			if err := json.Unmarshal(raw, &cfg); err != nil {
				writeError(w, http.StatusBadRequest, "invalid safety config: "+err.Error())
				return
			}
			if err := s.safety.UpdateConfig(cfg); err != nil {
				writeError(w, http.StatusBadRequest, "invalid safety config: "+err.Error())
				return
			}
			continue
		}
		if key == "maintenance" {
			// Validate shape and log the transition so the event log records
			// who suspended/resumed automation. Storage is the source of truth
			// the automation loop reads each tick (Service.MaintenanceActive).
			var v struct {
				Active bool `json:"active"`
			}
			if err := json.Unmarshal(raw, &v); err != nil {
				writeError(w, http.StatusBadRequest, "invalid maintenance config: "+err.Error())
				return
			}
			prev := false
			if cur, ok, _ := s.settingsRepo.Get("maintenance"); ok {
				var pv struct {
					Active bool `json:"active"`
				}
				_ = json.Unmarshal([]byte(cur), &pv)
				prev = pv.Active
			}
			if err := s.settingsRepo.Set(key, string(raw)); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if v.Active != prev {
				if v.Active {
					s.eventLog.Warn("maintenance_entered",
						"maintenance mode ON — irrigation automation suspended; lighting unaffected", "api", nil)
				} else {
					s.eventLog.Info("maintenance_exited",
						"maintenance mode OFF — irrigation automation resumed", "api", nil)
				}
			}
			continue
		}
		if key == "automation" {
			// Same shape check as maintenance: a typo'd value must not be
			// stored, because the automation loop reads this key every tick
			// and treats anything unreadable as disabled (audit 2.9).
			var v struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.Unmarshal(raw, &v); err != nil {
				writeError(w, http.StatusBadRequest, "invalid automation config: "+err.Error())
				return
			}
			prev := false
			if cur, ok, _ := s.settingsRepo.Get("automation"); ok {
				var pv struct {
					Enabled bool `json:"enabled"`
				}
				_ = json.Unmarshal([]byte(cur), &pv)
				prev = pv.Enabled
			}
			body, _ := json.Marshal(v)
			if err := s.settingsRepo.Set(key, string(body)); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if v.Enabled != prev {
				if v.Enabled {
					s.eventLog.Info("automation_enabled", "irrigation automation enabled", "api", nil)
				} else {
					s.eventLog.Warn("automation_disabled", "irrigation automation disabled", "api", nil)
				}
			}
			continue
		}
		if err := s.settingsRepo.Set(key, string(raw)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.eventLog.Info("settings_updated", "settings updated", "api", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
