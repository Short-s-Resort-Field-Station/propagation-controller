// Package automation runs the two automatic irrigation triggers (spec §9,
// §11, and the sensor-triggered irrigation addendum):
//
//   - Moisture rules: when a zone's soil sensor drops below its trigger
//     threshold, request a cycle — with hysteresis, minimum interval,
//     max-cycles-per-day, and sensor-offline lockout.
//   - Irrigation schedules: time-of-day rules from the schedules table.
//
// Both paths only ever Enqueue into the irrigation engine, so the one-zone-
// at-a-time rule holds no matter who asks for water. Missed schedules are
// skipped (spec §11.2 default): on startup evaluation begins at "now".
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/irrigation"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

// ScheduleRule is the rule_json payload for type=irrigation schedules.
type ScheduleRule struct {
	ZoneKey         string   `json:"zone_key"`
	StartTime       string   `json:"start_time"` // "HH:MM"
	DurationSeconds int      `json:"duration_seconds"`
	Days            []string `json:"days,omitempty"` // lowercase mon..sun; empty = daily
}

func (r ScheduleRule) Validate() error {
	if r.ZoneKey == "" {
		return fmt.Errorf("zone_key is required")
	}
	if _, err := parseHHMM(r.StartTime); err != nil {
		return err
	}
	if r.DurationSeconds <= 0 {
		return fmt.Errorf("duration_seconds must be > 0")
	}
	valid := map[string]bool{"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true}
	for _, d := range r.Days {
		if !valid[strings.ToLower(d)] {
			return fmt.Errorf("invalid day %q (use mon..sun)", d)
		}
	}
	return nil
}

type Service struct {
	engine    *irrigation.Engine
	zones     *storage.ZoneRepo
	cycles    *storage.CycleRepo
	rules     *storage.MoistureRuleRepo
	readings  *storage.ReadingRepo
	schedules *storage.ScheduleRepo
	settings  *storage.SettingsRepo
	events    *logging.EventLogger
	log       zerolog.Logger

	interval time.Duration
	nowFn    func() time.Time

	mu sync.Mutex
	// lastCheck bounds the schedule sweep: minute boundaries in
	// (lastCheck, now] fire. Initialized to Start time = missed runs skip.
	lastCheck time.Time
	// awaitingRecovery implements hysteresis: once a zone triggers, it stays
	// ineligible until the reading rises above recovered_above.
	awaitingRecovery map[string]bool
	// lastBlock de-duplicates "blocked" events per zone.
	lastBlock map[string]string
}

func NewService(engine *irrigation.Engine, zones *storage.ZoneRepo, cycles *storage.CycleRepo,
	rules *storage.MoistureRuleRepo, readings *storage.ReadingRepo, schedules *storage.ScheduleRepo,
	settings *storage.SettingsRepo, events *logging.EventLogger, log zerolog.Logger,
	interval time.Duration) *Service {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Service{
		engine: engine, zones: zones, cycles: cycles, rules: rules, readings: readings,
		schedules: schedules, settings: settings, events: events, log: log,
		interval: interval, nowFn: time.Now,
		awaitingRecovery: map[string]bool{}, lastBlock: map[string]string{},
	}
}

// SetNowFunc injects a clock for tests.
func (s *Service) SetNowFunc(f func() time.Time) { s.nowFn = f }

func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.lastCheck = s.nowFn()
	s.mu.Unlock()
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Tick()
			}
		}
	}()
}

// Enabled reports the global automation switch (settings key "automation").
func (s *Service) Enabled() bool {
	raw, ok, err := s.settings.Get("automation")
	if err != nil || !ok {
		return true // default on
	}
	var v struct {
		Enabled *bool `json:"enabled"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil || v.Enabled == nil {
		return true
	}
	return *v.Enabled
}

// Tick runs one evaluation pass (exported for tests).
func (s *Service) Tick() {
	now := s.nowFn()
	s.mu.Lock()
	last := s.lastCheck
	s.lastCheck = now
	s.mu.Unlock()

	if !s.Enabled() {
		return
	}
	s.evaluateSchedules(last, now)
	s.evaluateMoistureRules(now)
}

// --- scheduled irrigation ---

func (s *Service) evaluateSchedules(last, now time.Time) {
	rows, err := s.schedules.ListByType("irrigation")
	if err != nil {
		s.log.Error().Err(err).Msg("automation: cannot list irrigation schedules")
		return
	}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		var rule ScheduleRule
		if err := json.Unmarshal([]byte(row.RuleJSON), &rule); err != nil {
			s.log.Error().Err(err).Str("schedule", row.ScheduleKey).Msg("automation: bad schedule rule")
			continue
		}
		if !fires(rule, row.Timezone, last, now) {
			continue
		}
		pos, err := s.engine.Enqueue(irrigation.Request{
			ZoneKey:         rule.ZoneKey,
			DurationSeconds: rule.DurationSeconds,
			TriggerType:     "schedule",
			TriggerSource:   row.ScheduleKey,
			Reason:          "scheduled irrigation " + row.ScheduleKey,
		})
		if err != nil {
			s.events.Warn("schedule_run_blocked",
				fmt.Sprintf("schedule %s could not run: %s", row.ScheduleKey, err.Error()),
				"schedule", map[string]any{"schedule": row.ScheduleKey, "zone": rule.ZoneKey})
			continue
		}
		s.events.Info("schedule_run",
			fmt.Sprintf("schedule %s queued zone %s for %ds (position %d)",
				row.ScheduleKey, rule.ZoneKey, rule.DurationSeconds, pos),
			"schedule", map[string]any{"schedule": row.ScheduleKey, "zone": rule.ZoneKey})
	}
}

// fires reports whether any minute boundary in (last, now] matches the rule.
func fires(rule ScheduleRule, timezone string, last, now time.Time) bool {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	target, err := parseHHMM(rule.StartTime)
	if err != nil {
		return false
	}
	// Walk minute boundaries from the first after `last` through `now`.
	m := last.In(loc).Truncate(time.Minute).Add(time.Minute)
	for ; !m.After(now.In(loc)); m = m.Add(time.Minute) {
		if m.Hour()*60+m.Minute() != target {
			continue
		}
		if len(rule.Days) > 0 {
			day := strings.ToLower(m.Weekday().String()[:3])
			found := false
			for _, d := range rule.Days {
				if strings.ToLower(d) == day {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		return true
	}
	return false
}

// --- moisture-triggered irrigation ---

func (s *Service) evaluateMoistureRules(now time.Time) {
	rules, err := s.rules.ListEnabled()
	if err != nil {
		s.log.Error().Err(err).Msg("automation: cannot list moisture rules")
		return
	}
	for _, rule := range rules {
		s.evaluateMoistureRule(rule, now)
	}
}

func (s *Service) evaluateMoistureRule(rule storage.MoistureRule, now time.Time) {
	latest, err := s.readings.Latest(rule.SensorKey)
	if err != nil {
		return
	}

	// Sensor offline behavior (spec addendum: "if sensor offline: do not
	// irrigate" when behavior is lockout).
	const staleAfter = 5 * time.Minute
	fresh := latest != nil && now.Sub(latest.ReadAt) <= staleAfter
	if !fresh {
		if rule.SensorOfflineBehavior == "lockout" {
			s.blockOnce(rule.ZoneKey, "sensor_offline",
				fmt.Sprintf("zone %s moisture automation locked out: sensor %s has no recent reading",
					rule.ZoneKey, rule.SensorKey))
		}
		return
	}

	// Hysteresis: after a trigger, wait for recovery above recovered_above
	// before the zone can trigger again.
	s.mu.Lock()
	awaiting := s.awaitingRecovery[rule.ZoneKey]
	s.mu.Unlock()
	if awaiting {
		recoverAt := rule.TriggerBelow
		if rule.RecoveredAbove != nil {
			recoverAt = *rule.RecoveredAbove
		}
		if latest.Value >= recoverAt {
			s.mu.Lock()
			s.awaitingRecovery[rule.ZoneKey] = false
			s.mu.Unlock()
			s.clearBlock(rule.ZoneKey)
		}
		return
	}

	if latest.Value >= rule.TriggerBelow {
		s.clearBlock(rule.ZoneKey)
		return
	}

	// Below threshold — check the rule's own guards. (Reservoir level,
	// e-stop, and pump lockouts are enforced by the safety supervisor and
	// the engine's precheck; they don't need re-implementing here.)
	if rule.MinimumIntervalMinutes > 0 {
		lastCycle, err := s.cycles.LastWateredForZone(rule.ZoneID)
		if err == nil && lastCycle != nil {
			nextAllowed := lastCycle.StartedAt.Add(time.Duration(rule.MinimumIntervalMinutes) * time.Minute)
			if now.Before(nextAllowed) {
				s.blockOnce(rule.ZoneKey, "min_interval",
					fmt.Sprintf("zone %s below moisture threshold but minimum interval not elapsed (next allowed %s)",
						rule.ZoneKey, nextAllowed.Format(time.RFC3339)))
				return
			}
		}
	}
	if rule.MaxCyclesPerDay != nil {
		// Rolling 24h window; counts only moisture-triggered cycles —
		// manual runs are the grower's business.
		n, err := s.cycles.CountCyclesSince(rule.ZoneID, "moisture_threshold", now.Add(-24*time.Hour))
		if err == nil && n >= *rule.MaxCyclesPerDay {
			s.blockOnce(rule.ZoneKey, "max_cycles",
				fmt.Sprintf("zone %s below moisture threshold but at its daily limit (%d cycles/24h)",
					rule.ZoneKey, *rule.MaxCyclesPerDay))
			return
		}
	}

	pos, err := s.engine.Enqueue(irrigation.Request{
		ZoneKey:         rule.ZoneKey,
		DurationSeconds: rule.RunDurationSeconds,
		TriggerType:     "moisture_threshold",
		TriggerSource:   rule.SensorKey,
		Reason: fmt.Sprintf("moisture %.1f below threshold %.1f", latest.Value, rule.TriggerBelow),
	})
	if err != nil {
		// Already queued/running, safety lockout, etc. — logged once.
		s.blockOnce(rule.ZoneKey, "enqueue:"+err.Error(),
			fmt.Sprintf("zone %s moisture trigger blocked: %s", rule.ZoneKey, err.Error()))
		return
	}
	s.mu.Lock()
	s.awaitingRecovery[rule.ZoneKey] = true
	s.mu.Unlock()
	s.clearBlock(rule.ZoneKey)
	s.events.Info("moisture_trigger",
		fmt.Sprintf("zone %s moisture %.1f fell below %.1f; irrigation queued (%ds, position %d)",
			rule.ZoneKey, latest.Value, rule.TriggerBelow, rule.RunDurationSeconds, pos),
		"system", map[string]any{
			"zone": rule.ZoneKey, "sensor": rule.SensorKey, "value": latest.Value,
			"threshold": rule.TriggerBelow, "duration_seconds": rule.RunDurationSeconds,
		})
}

// blockOnce emits a warning event only when the block reason changes, so a
// persistent condition doesn't spam the event log every tick.
func (s *Service) blockOnce(zoneKey, reason, message string) {
	s.mu.Lock()
	changed := s.lastBlock[zoneKey] != reason
	s.lastBlock[zoneKey] = reason
	s.mu.Unlock()
	if changed {
		s.events.Warn("moisture_trigger_blocked", message, "system", map[string]any{"zone": zoneKey})
	}
}

func (s *Service) clearBlock(zoneKey string) {
	s.mu.Lock()
	delete(s.lastBlock, zoneKey)
	s.mu.Unlock()
}

func parseHHMM(v string) (int, error) {
	t, err := time.Parse("15:04", v)
	if err != nil {
		return 0, fmt.Errorf("time %q must be HH:MM (24h)", v)
	}
	return t.Hour()*60 + t.Minute(), nil
}
