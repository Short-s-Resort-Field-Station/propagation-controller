// Package lighting implements rack lighting control (spec §10): one relay
// for the whole rack (all-or-nothing, confirmed by Austin — per-shelf cuts
// happen at the physical switch), a daily photoperiod schedule, manual
// overrides with optional expiry, and restore-expected-state after reboot.
//
// The control loop is edge-triggered: it applies the scheduled state at
// startup and at schedule transitions, but otherwise leaves the relay alone,
// so a raw relay command from diagnostics isn't silently fought.
package lighting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
)

const scheduleKey = "lighting"

// overrideKey is the settings row that carries a manual override across a
// restart: with the schedule disabled and the lights on by hand, a power
// blip must not leave the rack dark until a human notices (audit
// 2026-11-10, 2.11). Cleared overrides are stored as JSON null.
const overrideKey = "lighting_override"

// Rule is the schedules-table rule_json payload.
type Rule struct {
	OnTime  string   `json:"on_time"`        // "06:00"
	OffTime string   `json:"off_time"`       // "19:30"
	Days    []string `json:"days,omitempty"` // lowercase mon..sun; empty = daily
}

type Schedule struct {
	Enabled  bool   `json:"enabled"`
	Timezone string `json:"timezone"`
	Rule
}

func (s Schedule) Validate() error {
	if _, err := parseHHMM(s.OnTime); err != nil {
		return fmt.Errorf("on_time: %w", err)
	}
	if _, err := parseHHMM(s.OffTime); err != nil {
		return fmt.Errorf("off_time: %w", err)
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("unknown timezone %q", s.Timezone)
	}
	valid := map[string]bool{"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true}
	for _, d := range s.Days {
		if !valid[strings.ToLower(d)] {
			return fmt.Errorf("invalid day %q (use mon..sun)", d)
		}
	}
	return nil
}

type Override struct {
	Mode  string     `json:"mode"` // on | off
	Until *time.Time `json:"until,omitempty"`
	Actor string     `json:"actor"`
	Since time.Time  `json:"since"`
}

type Service struct {
	mgr       *devices.Manager
	safety    *safety.Supervisor
	schedules *storage.ScheduleRepo
	settings  *storage.SettingsRepo
	events    *logging.EventLogger
	log       zerolog.Logger
	interval  time.Duration

	mu           sync.Mutex
	override     *Override
	lastExpected *bool
}

func NewService(mgr *devices.Manager, sup *safety.Supervisor, schedules *storage.ScheduleRepo,
	settings *storage.SettingsRepo, events *logging.EventLogger, log zerolog.Logger, interval time.Duration) *Service {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &Service{mgr: mgr, safety: sup, schedules: schedules, settings: settings,
		events: events, log: log, interval: interval}
}

// EnsureDefault seeds the photoperiod schedule from the design handoff
// (06:00–19:30 daily).
func (s *Service) EnsureDefault(timezone string) error {
	existing, err := s.schedules.GetByKey(scheduleKey)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	rule, _ := json.Marshal(Rule{OnTime: "06:00", OffTime: "19:30"})
	return s.schedules.Upsert(scheduleKey, "Rack lighting photoperiod", "lighting",
		true, timezone, string(rule), "restore_current_expected_state")
}

func (s *Service) Start(ctx context.Context) {
	s.restoreOverride(ctx)
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.tick(ctx) // restore expected state immediately at startup (spec §12.2)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

// restoreOverride reloads a persisted manual override and re-applies it.
// An override whose expiry passed while the daemon was down is dropped and
// the schedule takes over on the first tick.
func (s *Service) restoreOverride(ctx context.Context) {
	raw, ok, err := s.settings.Get(overrideKey)
	if err != nil {
		s.log.Error().Err(err).Msg("lighting: cannot read persisted override")
		return
	}
	if !ok {
		return
	}
	var o *Override
	if err := json.Unmarshal([]byte(raw), &o); err != nil || o == nil {
		return
	}
	if o.Mode != "on" && o.Mode != "off" {
		return
	}
	if o.Until != nil && time.Now().After(*o.Until) {
		s.persistOverride(nil)
		s.events.Info("lighting_override_expired", "lighting override expired during downtime; returning to schedule", o.Actor, nil)
		return
	}
	s.mu.Lock()
	s.override = o
	s.mu.Unlock()
	s.apply(ctx, o.Mode == "on", o.Actor, "manual lighting override restored after restart")
	s.events.Info("lighting_override_restored", fmt.Sprintf("lighting override (%s) restored after restart", o.Mode),
		o.Actor, map[string]any{"until": o.Until})
}

// persistOverride stores the current override (nil clears it).
func (s *Service) persistOverride(o *Override) {
	body, err := json.Marshal(o)
	if err == nil {
		err = s.settings.Set(overrideKey, string(body))
	}
	if err != nil {
		s.log.Error().Err(err).Msg("lighting: cannot persist override")
	}
}

func (s *Service) tick(ctx context.Context) {
	sch, err := s.loadSchedule()
	if err != nil {
		s.log.Error().Err(err).Msg("lighting: cannot load schedule")
		return
	}

	var expected *bool
	if sch != nil && sch.Enabled {
		v := evaluate(*sch, time.Now())
		expected = &v
	}

	s.mu.Lock()
	// Expire overrides.
	if s.override != nil && s.override.Until != nil && time.Now().After(*s.override.Until) {
		actor := s.override.Actor
		s.override = nil
		s.mu.Unlock()
		s.persistOverride(nil)
		s.events.Info("lighting_override_expired", "lighting override expired; returning to schedule", actor, nil)
		s.mu.Lock()
		if expected != nil {
			exp := *expected
			s.lastExpected = &exp
			s.mu.Unlock()
			s.apply(ctx, exp, "schedule", "override expired")
			return
		}
	}

	overrideActive := s.override != nil
	transition := expected != nil && (s.lastExpected == nil || *s.lastExpected != *expected)
	if expected != nil {
		exp := *expected
		s.lastExpected = &exp
	}
	s.mu.Unlock()

	if transition && !overrideActive {
		s.apply(ctx, *expected, "schedule", "scheduled transition")
	}
}

func (s *Service) apply(ctx context.Context, on bool, actor, reason string) {
	state := relay.Off
	if on {
		state = relay.On
	}
	if err := s.safety.Authorize(devices.LightsChannel, state); err != nil {
		s.log.Warn().Err(err).Msg("lighting: command blocked by safety")
		return
	}
	if _, err := s.mgr.SetRelay(ctx, devices.LightsChannel, state, actor, reason, 0); err != nil {
		s.events.Error("lighting_command_failed", "lighting relay command failed", actor,
			map[string]any{"error": err.Error()})
	}
}

// SetOverride handles manual on/off. mode "auto" clears the override and
// reapplies the scheduled state.
func (s *Service) SetOverride(ctx context.Context, mode, actor string, expiresMinutes int) error {
	switch mode {
	case "on", "off":
		var until *time.Time
		if expiresMinutes > 0 {
			t := time.Now().Add(time.Duration(expiresMinutes) * time.Minute)
			until = &t
		}
		o := &Override{Mode: mode, Until: until, Actor: actor, Since: time.Now().UTC()}
		s.mu.Lock()
		s.override = o
		s.mu.Unlock()
		s.persistOverride(o)
		s.apply(ctx, mode == "on", actor, "manual lighting override")
		s.events.Info("lighting_override", fmt.Sprintf("lighting manually forced %s", mode), actor,
			map[string]any{"expires_minutes": expiresMinutes})
		return nil
	case "auto":
		s.mu.Lock()
		had := s.override != nil
		s.override = nil
		expected := s.lastExpected
		s.mu.Unlock()
		if had {
			s.persistOverride(nil)
			s.events.Info("lighting_override_cleared", "lighting override cleared; returning to schedule", actor, nil)
		}
		if expected != nil {
			s.apply(ctx, *expected, "schedule", "override cleared")
		}
		return nil
	default:
		return fmt.Errorf(`mode must be "on", "off", or "auto"`)
	}
}

type StatusView struct {
	State      string    `json:"state"` // scheduled_on | off | manual_on | manual_off | disabled
	RelayState string    `json:"relay_state"`
	Override   *Override `json:"override,omitempty"`
	Schedule   *Schedule `json:"schedule,omitempty"`
}

func (s *Service) Status(ctx context.Context) StatusView {
	view := StatusView{}
	if info, err := s.mgr.GetRelay(ctx, devices.LightsChannel); err == nil {
		view.RelayState = string(info.State)
	} else {
		view.RelayState = "unknown"
	}
	sch, err := s.loadSchedule()
	if err == nil && sch != nil {
		view.Schedule = sch
	}

	s.mu.Lock()
	override := s.override
	s.mu.Unlock()

	switch {
	case override != nil:
		view.State = "manual_" + override.Mode
		o := *override
		view.Override = &o
	case sch == nil || !sch.Enabled:
		view.State = "disabled"
	case evaluate(*sch, time.Now()):
		view.State = "scheduled_on"
	default:
		view.State = "off"
	}
	return view
}

func (s *Service) GetSchedule() (*Schedule, error) { return s.loadSchedule() }

func (s *Service) PutSchedule(sch Schedule, actor string) error {
	if err := sch.Validate(); err != nil {
		return err
	}
	rule, err := json.Marshal(sch.Rule)
	if err != nil {
		return err
	}
	if err := s.schedules.Upsert(scheduleKey, "Rack lighting photoperiod", "lighting",
		sch.Enabled, sch.Timezone, string(rule), "restore_current_expected_state"); err != nil {
		return err
	}
	s.events.Info("lighting_schedule_updated", "lighting schedule updated", actor,
		map[string]any{"on": sch.OnTime, "off": sch.OffTime, "enabled": sch.Enabled})
	// Force re-evaluation on the next tick as a fresh transition.
	s.mu.Lock()
	s.lastExpected = nil
	s.mu.Unlock()
	return nil
}

func (s *Service) loadSchedule() (*Schedule, error) {
	row, err := s.schedules.GetByKey(scheduleKey)
	if err != nil || row == nil {
		return nil, err
	}
	var rule Rule
	if err := json.Unmarshal([]byte(row.RuleJSON), &rule); err != nil {
		return nil, fmt.Errorf("bad lighting rule: %w", err)
	}
	return &Schedule{Enabled: row.Enabled, Timezone: row.Timezone, Rule: rule}, nil
}

// evaluate reports whether the lights should be on at t under the schedule.
// Windows may span midnight (on 20:00, off 06:00). The day filter applies to
// the calendar day of t; for overnight photoperiods use daily (no day list).
func evaluate(sch Schedule, t time.Time) bool {
	loc, err := time.LoadLocation(sch.Timezone)
	if err != nil {
		loc = time.UTC
	}
	n := t.In(loc)
	if len(sch.Days) > 0 {
		day := strings.ToLower(n.Weekday().String()[:3])
		found := false
		for _, d := range sch.Days {
			if strings.ToLower(d) == day {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	on, err1 := parseHHMM(sch.OnTime)
	off, err2 := parseHHMM(sch.OffTime)
	if err1 != nil || err2 != nil || on == off {
		return false
	}
	cur := n.Hour()*60 + n.Minute()
	if on < off {
		return cur >= on && cur < off
	}
	return cur >= on || cur < off // spans midnight
}

func parseHHMM(v string) (int, error) {
	t, err := time.Parse("15:04", v)
	if err != nil {
		return 0, fmt.Errorf("time %q must be HH:MM (24h)", v)
	}
	return t.Hour()*60 + t.Minute(), nil
}
