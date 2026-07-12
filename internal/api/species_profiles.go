package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/storage"
)

// Species profiles (change 3): per-species, per-stage saved settings a grower
// dials in once and re-applies to a shelf. settings holds the moisture-control
// settings (active control) plus EC/temperature target ranges (monitoring only).

type profileRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// profileSettings is the validated shape of a profile's settings object. Every
// field is optional so partial presets are allowed; the shape is stored (and
// returned) verbatim so it can grow without a migration.
type profileSettings struct {
	Moisture *struct {
		TriggerBelow           *float64 `json:"trigger_below"`
		RecoveredAbove         *float64 `json:"recovered_above"`
		RunDurationSeconds     *int     `json:"run_duration_seconds"`
		DrainSeconds           *int     `json:"drain_seconds"`
		MinimumIntervalMinutes *int     `json:"minimum_interval_minutes"`
		MaxCyclesPerDay        *int     `json:"max_cycles_per_day"`
	} `json:"moisture"`
	EC   *profileRange `json:"ec"`
	Temp *profileRange `json:"temp"`
}

func validateProfileSettings(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("settings is required")
	}
	var ps profileSettings
	if err := json.Unmarshal(raw, &ps); err != nil {
		return fmt.Errorf("invalid settings: %w", err)
	}
	if ps.EC != nil && ps.EC.Min >= ps.EC.Max {
		return fmt.Errorf("ec.min must be below ec.max")
	}
	if ps.Temp != nil && ps.Temp.Min >= ps.Temp.Max {
		return fmt.Errorf("temp.min must be below temp.max")
	}
	if ps.Moisture != nil && ps.Moisture.TriggerBelow != nil && ps.Moisture.RecoveredAbove != nil &&
		*ps.Moisture.RecoveredAbove <= *ps.Moisture.TriggerBelow {
		return fmt.Errorf("moisture.recovered_above must exceed moisture.trigger_below")
	}
	return nil
}

func profileView(p storage.SpeciesProfile) map[string]any {
	return map[string]any{
		"id": p.ID, "species_id": p.SpeciesID, "stage": p.Stage, "name": p.Name,
		"settings":   json.RawMessage(p.SettingsJSON),
		"created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
	}
}

type profileBody struct {
	SpeciesID string          `json:"species_id"`
	Stage     string          `json:"stage"`
	Name      string          `json:"name"`
	Settings  json.RawMessage `json:"settings"`
}

func (s *Server) handleListSpeciesProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.profiles.List(r.URL.Query().Get("species"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]map[string]any, 0, len(list))
	for _, p := range list {
		views = append(views, profileView(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": views})
}

func (s *Server) handleCreateSpeciesProfile(w http.ResponseWriter, r *http.Request) {
	var b profileBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	b.SpeciesID = strings.TrimSpace(b.SpeciesID)
	b.Name = strings.TrimSpace(b.Name)
	if b.SpeciesID == "" || b.Name == "" {
		writeError(w, http.StatusBadRequest, "species_id and name are required")
		return
	}
	if err := validateProfileSettings(b.Settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.profiles.Insert(storage.SpeciesProfile{
		SpeciesID: b.SpeciesID, Stage: strings.TrimSpace(b.Stage), Name: b.Name,
		SettingsJSON: string(b.Settings),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, _ := s.profiles.Get(id)
	s.eventLog.Info("species_profile_saved",
		fmt.Sprintf("saved profile %q for %s", b.Name, b.SpeciesID), "api", nil)
	writeJSON(w, http.StatusCreated, profileView(*p))
}

func (s *Server) handleGetSpeciesProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	p, err := s.profiles.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "unknown profile")
		return
	}
	writeJSON(w, http.StatusOK, profileView(*p))
}

func (s *Server) handlePutSpeciesProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	existing, err := s.profiles.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "unknown profile")
		return
	}
	var b profileBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	// species_id is immutable once created; name/stage/settings may change.
	if strings.TrimSpace(b.Name) != "" {
		existing.Name = strings.TrimSpace(b.Name)
	}
	existing.Stage = strings.TrimSpace(b.Stage)
	if len(b.Settings) > 0 {
		if err := validateProfileSettings(b.Settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing.SettingsJSON = string(b.Settings)
	}
	if err := s.profiles.Update(*existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, _ := s.profiles.Get(id)
	s.eventLog.Info("species_profile_updated",
		fmt.Sprintf("updated profile %q", p.Name), "api", nil)
	writeJSON(w, http.StatusOK, profileView(*p))
}

func (s *Server) handleDeleteSpeciesProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	if err := s.profiles.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
