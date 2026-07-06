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
		if err := s.settingsRepo.Set(key, string(raw)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.eventLog.Info("settings_updated", "settings updated", "api", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
