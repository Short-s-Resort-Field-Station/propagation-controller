package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/automation"
	"propagation-controller/internal/storage"
)

// --- moisture control (spec addendum) ---

type moistureControlBody struct {
	Enabled                bool     `json:"enabled"`
	SensorKey              string   `json:"sensor_key"`
	TriggerBelow           float64  `json:"trigger_below"`
	RecoveredAbove         *float64 `json:"recovered_above"`
	RunDurationSeconds     int      `json:"run_duration_seconds"`
	MinimumIntervalMinutes int      `json:"minimum_interval_minutes"`
	MaxCyclesPerDay        *int     `json:"max_cycles_per_day"`
	SensorOfflineBehavior  string   `json:"sensor_offline_behavior"`
}

func (s *Server) handleGetMoistureControl(w http.ResponseWriter, r *http.Request) {
	zone, err := s.zones.GetByKey(chi.URLParam(r, "zone_key"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if zone == nil {
		writeError(w, http.StatusNotFound, "unknown zone")
		return
	}
	rule, err := s.moisture.GetByZoneID(zone.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rule == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false, "zone_key": zone.ZoneKey})
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handlePutMoistureControl(w http.ResponseWriter, r *http.Request) {
	zone, err := s.zones.GetByKey(chi.URLParam(r, "zone_key"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if zone == nil {
		writeError(w, http.StatusNotFound, "unknown zone")
		return
	}
	var body moistureControlBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if body.SensorKey == "" {
		writeError(w, http.StatusBadRequest, "sensor_key is required")
		return
	}
	if body.TriggerBelow <= 0 || body.TriggerBelow >= 100 {
		writeError(w, http.StatusBadRequest, "trigger_below must be between 0 and 100")
		return
	}
	if body.RecoveredAbove != nil && *body.RecoveredAbove <= body.TriggerBelow {
		writeError(w, http.StatusBadRequest, "recovered_above must be greater than trigger_below")
		return
	}
	if body.RunDurationSeconds <= 0 || body.RunDurationSeconds > zone.MaxFloodSeconds {
		writeError(w, http.StatusBadRequest, "run_duration_seconds must be > 0 and within the zone's max flood duration")
		return
	}
	if body.MinimumIntervalMinutes < 0 {
		writeError(w, http.StatusBadRequest, "minimum_interval_minutes must be >= 0")
		return
	}
	if body.MaxCyclesPerDay != nil && *body.MaxCyclesPerDay < 1 {
		writeError(w, http.StatusBadRequest, "max_cycles_per_day must be >= 1")
		return
	}
	switch body.SensorOfflineBehavior {
	case "":
		body.SensorOfflineBehavior = "lockout"
	case "lockout", "ignore":
	default:
		writeError(w, http.StatusBadRequest, `sensor_offline_behavior must be "lockout" or "ignore"`)
		return
	}

	rule := storage.MoistureRule{
		ZoneID:                 zone.ID,
		Enabled:                body.Enabled,
		SensorKey:              body.SensorKey,
		TriggerBelow:           body.TriggerBelow,
		RecoveredAbove:         body.RecoveredAbove,
		RunDurationSeconds:     body.RunDurationSeconds,
		MinimumIntervalMinutes: body.MinimumIntervalMinutes,
		MaxCyclesPerDay:        body.MaxCyclesPerDay,
		SensorOfflineBehavior:  body.SensorOfflineBehavior,
	}
	if err := s.moisture.Upsert(rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventLog.Info("moisture_control_updated",
		"moisture control rule updated for zone "+zone.ZoneKey, "api",
		map[string]any{"zone": zone.ZoneKey, "enabled": body.Enabled, "trigger_below": body.TriggerBelow})
	saved, _ := s.moisture.GetByZoneID(zone.ID)
	writeJSON(w, http.StatusOK, saved)
}

// --- schedules CRUD (spec §15.7) ---

type scheduleBody struct {
	ScheduleKey string                  `json:"schedule_key"`
	Name        string                  `json:"name"`
	Type        string                  `json:"type"`
	Enabled     bool                    `json:"enabled"`
	Timezone    string                  `json:"timezone"`
	Rule        automation.ScheduleRule `json:"rule"`
}

func (s *Server) validateScheduleBody(b *scheduleBody) string {
	if b.ScheduleKey == "" {
		return "schedule_key is required"
	}
	if b.Type != "irrigation" {
		return `type must be "irrigation" (lighting uses /api/v1/lighting/schedule)`
	}
	if b.Timezone == "" {
		b.Timezone = "America/Chicago"
	}
	if err := b.Rule.Validate(); err != nil {
		return err.Error()
	}
	zone, err := s.zones.GetByKey(b.Rule.ZoneKey)
	if err != nil || zone == nil {
		return "rule.zone_key: unknown zone " + b.Rule.ZoneKey
	}
	if b.Rule.DurationSeconds > zone.MaxFloodSeconds {
		return "rule.duration_seconds exceeds the zone's max flood duration"
	}
	if b.Name == "" {
		b.Name = b.ScheduleKey
	}
	return ""
}

func (s *Server) upsertSchedule(w http.ResponseWriter, b scheduleBody) {
	rule, _ := json.Marshal(b.Rule)
	if err := s.schedules.Upsert(b.ScheduleKey, b.Name, b.Type, b.Enabled,
		b.Timezone, string(rule), "skip"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventLog.Info("schedule_updated", "schedule "+b.ScheduleKey+" saved", "api",
		map[string]any{"schedule": b.ScheduleKey, "zone": b.Rule.ZoneKey, "enabled": b.Enabled})
	saved, _ := s.schedules.GetByKey(b.ScheduleKey)
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.schedules.ListByType("irrigation")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []storage.ScheduleRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": rows})
}

func (s *Server) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	row, err := s.schedules.GetByKey(chi.URLParam(r, "schedule_key"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "unknown schedule")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var b scheduleBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if msg := s.validateScheduleBody(&b); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	existing, err := s.schedules.GetByKey(b.ScheduleKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "schedule "+b.ScheduleKey+" already exists (use PUT to update)")
		return
	}
	s.upsertSchedule(w, b)
}

func (s *Server) handlePutSchedule(w http.ResponseWriter, r *http.Request) {
	var b scheduleBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	b.ScheduleKey = chi.URLParam(r, "schedule_key")
	if msg := s.validateScheduleBody(&b); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.upsertSchedule(w, b)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "schedule_key")
	if key == "lighting" {
		writeError(w, http.StatusBadRequest, "the lighting schedule cannot be deleted")
		return
	}
	if err := s.schedules.Delete(key); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventLog.Info("schedule_deleted", "schedule "+key+" deleted", "api", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
