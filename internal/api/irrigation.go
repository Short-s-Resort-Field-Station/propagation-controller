package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/irrigation"
	"propagation-controller/internal/storage"
)

type zoneView struct {
	storage.Zone
	State        string          `json:"state"` // running | queued | ready | disabled
	LastCycle    *storage.Cycle  `json:"last_cycle,omitempty"`
	LatestValues map[string]any  `json:"latest,omitempty"`
	Config       json.RawMessage `json:"config"`
}

func (s *Server) zoneView(z storage.Zone) zoneView {
	v := zoneView{Zone: z, State: "disabled", Config: json.RawMessage(z.ConfigJSON)}
	v.ConfigJSON = "" // avoid duplicating raw JSON as a string field
	if z.Enabled {
		v.State = s.irrigation.ZoneState(z.ZoneKey)
	}
	if last, err := s.cycles.LastForZone(z.ID); err == nil && last != nil {
		v.LastCycle = last
	}
	if z.SoilSensorKey != nil {
		if latest, err := s.readings.Latest(*z.SoilSensorKey); err == nil && latest != nil {
			v.LatestValues = map[string]any{"moisture": latest.Value, "read_at": latest.ReadAt}
		}
	}
	return v
}

func (s *Server) handleListZones(w http.ResponseWriter, r *http.Request) {
	zones, err := s.zones.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]zoneView, 0, len(zones))
	for _, z := range zones {
		views = append(views, s.zoneView(z))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"zones":  views,
		"engine": s.irrigation.Status(),
	})
}

func (s *Server) handleGetZone(w http.ResponseWriter, r *http.Request) {
	zone, err := s.zones.GetByKey(chi.URLParam(r, "zone_key"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if zone == nil {
		writeError(w, http.StatusNotFound, "unknown zone")
		return
	}
	writeJSON(w, http.StatusOK, s.zoneView(*zone))
}

type zonePatch struct {
	Name              *string  `json:"name"`
	Enabled           *bool    `json:"enabled"`
	SoilSensorKey     *string  `json:"soil_sensor_key"`
	MaxFloodSeconds   *int     `json:"max_flood_seconds"`
	MaxPumpSeconds    *int     `json:"max_pump_seconds"`
	MinReservoirLevel *float64 `json:"min_reservoir_level"`
	DefaultRunSeconds *int     `json:"default_run_seconds"`
	DrainSeconds      *int     `json:"drain_seconds"`
	TargetMoisture    *float64 `json:"target_moisture"`
	Species           *string  `json:"species"`
	SpeciesLatin      *string  `json:"species_latin"`
	Species2          *string  `json:"species2"`
	Species2Latin     *string  `json:"species2_latin"`
}

func (s *Server) handlePatchZone(w http.ResponseWriter, r *http.Request) {
	zone, err := s.zones.GetByKey(chi.URLParam(r, "zone_key"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if zone == nil {
		writeError(w, http.StatusNotFound, "unknown zone")
		return
	}
	var p zonePatch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if p.Name != nil {
		zone.Name = *p.Name
	}
	if p.Enabled != nil {
		zone.Enabled = *p.Enabled
	}
	if p.SoilSensorKey != nil {
		if *p.SoilSensorKey == "" {
			zone.SoilSensorKey = nil
		} else {
			zone.SoilSensorKey = p.SoilSensorKey
		}
	}
	if p.MaxFloodSeconds != nil {
		if *p.MaxFloodSeconds < 5 {
			writeError(w, http.StatusBadRequest, "max_flood_seconds must be >= 5")
			return
		}
		zone.MaxFloodSeconds = *p.MaxFloodSeconds
	}
	if p.MaxPumpSeconds != nil {
		if *p.MaxPumpSeconds < 5 {
			writeError(w, http.StatusBadRequest, "max_pump_seconds must be >= 5")
			return
		}
		zone.MaxPumpSeconds = *p.MaxPumpSeconds
	}
	if p.MinReservoirLevel != nil {
		zone.MinReservoirLevel = p.MinReservoirLevel
	}

	var cfg irrigation.ZoneConfig
	json.Unmarshal([]byte(zone.ConfigJSON), &cfg)
	if p.DefaultRunSeconds != nil {
		cfg.DefaultRunSeconds = *p.DefaultRunSeconds
	}
	if p.DrainSeconds != nil {
		cfg.DrainSeconds = *p.DrainSeconds
	}
	if p.TargetMoisture != nil {
		if *p.TargetMoisture <= 0 {
			cfg.TargetMoisture = nil // 0 or negative clears the target
		} else {
			cfg.TargetMoisture = p.TargetMoisture
		}
	}
	if p.Species != nil {
		cfg.Species = *p.Species
	}
	if p.SpeciesLatin != nil {
		cfg.SpeciesLatin = *p.SpeciesLatin
	}
	if p.Species2 != nil {
		cfg.Species2 = *p.Species2 // empty string clears the second specimen
	}
	if p.Species2Latin != nil {
		cfg.Species2Latin = *p.Species2Latin
	}
	body, _ := json.Marshal(cfg)
	zone.ConfigJSON = string(body)

	if err := s.zones.Update(*zone); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventLog.Info("zone_config_updated", "irrigation zone "+zone.ZoneKey+" updated", "api", nil)
	writeJSON(w, http.StatusOK, s.zoneView(*zone))
}

func (s *Server) handleRunZone(w http.ResponseWriter, r *http.Request) {
	zoneKey := chi.URLParam(r, "zone_key")
	var req struct {
		DurationSeconds int    `json:"duration_seconds"`
		Reason          string `json:"reason"`
		Actor           string `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
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
	if req.DurationSeconds <= 0 {
		// Fall back to the zone's configured default run time.
		if zone, err := s.zones.GetByKey(zoneKey); err == nil && zone != nil {
			var cfg irrigation.ZoneConfig
			json.Unmarshal([]byte(zone.ConfigJSON), &cfg)
			req.DurationSeconds = cfg.DefaultRunSeconds
		}
		if req.DurationSeconds <= 0 {
			req.DurationSeconds = 60
		}
	}

	pos, err := s.irrigation.Enqueue(irrigation.Request{
		ZoneKey:         zoneKey,
		DurationSeconds: req.DurationSeconds,
		TriggerType:     actor,
		TriggerSource:   actor,
		Reason:          req.Reason,
	})
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	status := "started"
	if pos > 1 {
		status = "queued"
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":           status,
		"queue_position":   pos,
		"zone_key":         zoneKey,
		"duration_seconds": req.DurationSeconds,
	})
}

func (s *Server) handleStopZone(w http.ResponseWriter, r *http.Request) {
	zoneKey := chi.URLParam(r, "zone_key")
	var req struct {
		Actor string `json:"actor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" || !allowedActors[actor] {
		actor = "api"
	}
	if err := s.irrigation.Stop(zoneKey, actor); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (s *Server) handleListCycles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cycles, err := s.cycles.List(q.Get("zone"), parseIntOr(q.Get("limit"), 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cycles == nil {
		cycles = []storage.Cycle{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cycles": cycles})
}

func (s *Server) handleGetCycle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "cycle_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cycle_id must be an integer")
		return
	}
	cycle, err := s.cycles.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cycle == nil {
		writeError(w, http.StatusNotFound, "unknown cycle")
		return
	}
	writeJSON(w, http.StatusOK, cycle)
}
