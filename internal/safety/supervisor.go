// Package safety implements the safety supervisor (spec §12). It runs a
// continuous enforcement loop inside propagationd and gates every relay-on
// command. Safety before convenience: rules here override every other actor.
package safety

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

// Config holds the operator-tunable safety limits, persisted in the settings
// table under the "safety" key and editable through the settings API.
type Config struct {
	MaxPumpRuntimeSeconds   int     `json:"max_pump_runtime_seconds"`
	MinReservoirLevel       float64 `json:"min_reservoir_level"`
	ReservoirSensorKey      string  `json:"reservoir_sensor_key"`
	ReservoirOfflineLockout bool    `json:"reservoir_offline_lockout"`
	SensorStaleAfterSeconds int     `json:"sensor_stale_after_seconds"`
	EstopIncludesLights     bool    `json:"estop_includes_lights"`
}

func DefaultConfig() Config {
	return Config{
		// Sized above the 30-minute max soak (largest pots) so the backstop
		// only fires when something is genuinely wrong.
		MaxPumpRuntimeSeconds:   1860,
		MinReservoirLevel:       20,
		ReservoirSensorKey:      "reservoir_level_level",
		ReservoirOfflineLockout: true,
		SensorStaleAfterSeconds: 60,
		EstopIncludesLights:     false,
	}
}

func (c Config) Validate() error {
	if c.MaxPumpRuntimeSeconds < 1 {
		return fmt.Errorf("max_pump_runtime_seconds must be >= 1")
	}
	if c.MinReservoirLevel < 0 {
		return fmt.Errorf("min_reservoir_level must be >= 0")
	}
	if c.SensorStaleAfterSeconds < 1 {
		return fmt.Errorf("sensor_stale_after_seconds must be >= 1")
	}
	if c.ReservoirSensorKey == "" {
		return fmt.Errorf("reservoir_sensor_key is required")
	}
	return nil
}

// Lockout keys.
const (
	LockoutReservoirLow     = "reservoir_low"
	LockoutReservoirOffline = "reservoir_sensor_offline"
	LockoutPumpMaxRuntime   = "pump_max_runtime"
)

type Lockout struct {
	Key    string    `json:"key"`
	Reason string    `json:"reason"`
	Since  time.Time `json:"since"`
	// Auto lockouts track a live condition and clear themselves when it
	// resolves; manual lockouts require an admin clear.
	Auto bool `json:"auto"`
}

type EstopState struct {
	Active bool      `json:"active"`
	Reason string    `json:"reason,omitempty"`
	Actor  string    `json:"actor,omitempty"`
	At     time.Time `json:"at,omitzero"`
}

type Status struct {
	EmergencyStop EstopState `json:"emergency_stop"`
	Lockouts      []Lockout  `json:"lockouts"`
	Pump          PumpStatus `json:"pump"`
	Reservoir     ResStatus  `json:"reservoir"`
	Config        Config     `json:"config"`
}

type PumpStatus struct {
	Running           bool `json:"running"`
	OnForSeconds      int  `json:"on_for_seconds"`
	MaxRuntimeSeconds int  `json:"max_runtime_seconds"`
}

type ResStatus struct {
	Level        *float64   `json:"level"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	MinLevel     float64    `json:"min_level"`
	SensorHealth string     `json:"sensor_health"` // ok | stale | no_data
}

type Supervisor struct {
	mgr      *devices.Manager
	readings *storage.ReadingRepo
	settings *storage.SettingsRepo
	events   *logging.EventLogger
	log      zerolog.Logger
	interval time.Duration

	mu          sync.Mutex
	cfg         Config
	estop       EstopState
	lockouts    map[string]*Lockout
	pumpOnSince *time.Time
	running     bool
}

func NewSupervisor(mgr *devices.Manager, readings *storage.ReadingRepo,
	settings *storage.SettingsRepo, events *logging.EventLogger, log zerolog.Logger,
	interval time.Duration) *Supervisor {
	if interval <= 0 {
		interval = time.Second
	}
	return &Supervisor{
		mgr:      mgr,
		readings: readings,
		settings: settings,
		events:   events,
		log:      log,
		interval: interval,
		cfg:      DefaultConfig(),
		lockouts: map[string]*Lockout{},
	}
}

// Start loads persisted config and launches the enforcement loop.
func (s *Supervisor) Start(ctx context.Context) error {
	raw, ok, err := s.settings.Get("safety")
	if err != nil {
		return err
	}
	if ok {
		var cfg Config
		if err := json.Unmarshal([]byte(raw), &cfg); err == nil && cfg.Validate() == nil {
			// Config migration: installs that stored the original 300 s
			// default get the new 30-minute-soak-compatible backstop.
			if cfg.MaxPumpRuntimeSeconds == 300 {
				cfg.MaxPumpRuntimeSeconds = DefaultConfig().MaxPumpRuntimeSeconds
				if body, err := json.Marshal(cfg); err == nil {
					s.settings.Set("safety", string(body))
				}
				s.events.Info("safety_config_migrated",
					"max pump runtime raised to cover 30-minute soak cycles", "startup", nil)
			}
			s.mu.Lock()
			s.cfg = cfg
			s.mu.Unlock()
		} else {
			s.events.Warn("safety_config_invalid", "stored safety config is invalid; using defaults", "startup", nil)
		}
	} else {
		body, _ := json.Marshal(DefaultConfig())
		s.settings.Set("safety", string(body))
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.mu.Lock()
				s.running = false
				s.mu.Unlock()
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
	s.events.Info("safety_supervisor_started", "safety supervisor running", "startup", nil)
	return nil
}

// UpdateConfig applies and persists new limits at runtime.
func (s *Supervisor) UpdateConfig(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.settings.Set("safety", string(body)); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	s.events.Info("safety_config_updated", "safety limits updated", "api", map[string]any{"config": cfg})
	return nil
}

// Authorize is consulted before any relay command is executed. Turning
// things OFF is always allowed — safety must never trap a relay on.
func (s *Supervisor) Authorize(channel int, state relay.State) error {
	if state == relay.Off {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.estop.Active {
		return fmt.Errorf("emergency stop is active (reason: %s) — reset required before any relay can be turned on", s.estop.Reason)
	}
	if channel == devices.PumpChannel && len(s.lockouts) > 0 {
		keys := make([]string, 0, len(s.lockouts))
		for k := range s.lockouts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return fmt.Errorf("pump is locked out: %s", strings.Join(keys, ", "))
	}
	return nil
}

// EmergencyStop forces the safe state and latches until Reset (spec §12.3).
func (s *Supervisor) EmergencyStop(ctx context.Context, actor, reason string) {
	if reason == "" {
		reason = "emergency stop"
	}
	s.mu.Lock()
	alreadyActive := s.estop.Active
	s.estop = EstopState{Active: true, Reason: reason, Actor: actor, At: time.Now().UTC()}
	includeLights := s.cfg.EstopIncludesLights
	s.mu.Unlock()

	s.forceSafeState(ctx, includeLights, "emergency stop")
	if !alreadyActive {
		s.events.Error("emergency_stop", "EMERGENCY STOP activated — manual reset required", actor,
			map[string]any{"reason": reason})
	}
}

// Reset clears the emergency stop latch. Lockouts are unaffected: a manual
// lockout still needs its own clear, and auto lockouts re-evaluate live.
func (s *Supervisor) Reset(actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.estop.Active {
		return fmt.Errorf("emergency stop is not active")
	}
	s.estop = EstopState{}
	s.events.Info("emergency_stop_reset", "emergency stop reset; automation may resume", actor, nil)
	return nil
}

func (s *Supervisor) ClearLockout(key, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lo, ok := s.lockouts[key]
	if !ok {
		return fmt.Errorf("no active lockout %q", key)
	}
	if lo.Auto {
		return fmt.Errorf("lockout %q tracks a live condition and clears itself when the condition resolves", key)
	}
	delete(s.lockouts, key)
	s.events.Info("safety_lockout_cleared", fmt.Sprintf("lockout %s cleared", key), actor,
		map[string]any{"lockout": key})
	return nil
}

func (s *Supervisor) Status(ctx context.Context) Status {
	s.mu.Lock()
	cfg := s.cfg
	estop := s.estop
	lockouts := make([]Lockout, 0, len(s.lockouts))
	for _, lo := range s.lockouts {
		lockouts = append(lockouts, *lo)
	}
	pumpSince := s.pumpOnSince
	s.mu.Unlock()

	sort.Slice(lockouts, func(i, j int) bool { return lockouts[i].Key < lockouts[j].Key })

	st := Status{
		EmergencyStop: estop,
		Lockouts:      lockouts,
		Pump:          PumpStatus{MaxRuntimeSeconds: cfg.MaxPumpRuntimeSeconds},
		Reservoir:     ResStatus{MinLevel: cfg.MinReservoirLevel, SensorHealth: "no_data"},
		Config:        cfg,
	}
	if pumpSince != nil {
		st.Pump.Running = true
		st.Pump.OnForSeconds = int(time.Since(*pumpSince).Seconds())
	}
	if latest, err := s.readings.Latest(cfg.ReservoirSensorKey); err == nil && latest != nil {
		st.Reservoir.Level = &latest.Value
		st.Reservoir.ReadAt = &latest.ReadAt
		if time.Since(latest.ReadAt) > time.Duration(cfg.SensorStaleAfterSeconds)*time.Second {
			st.Reservoir.SensorHealth = "stale"
		} else {
			st.Reservoir.SensorHealth = "ok"
		}
	}
	return st
}

// HealthStatus reports the supervisor state for the health endpoint.
func (s *Supervisor) HealthStatus() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.running:
		return "not_running"
	case s.estop.Active:
		return "emergency_stop"
	default:
		return "running"
	}
}

// tick is one pass of the enforcement loop.
func (s *Supervisor) tick(ctx context.Context) {
	s.mu.Lock()
	cfg := s.cfg
	estopActive := s.estop.Active
	includeLights := s.cfg.EstopIncludesLights
	s.mu.Unlock()

	// While e-stop is latched, keep re-forcing the safe state so nothing can
	// sneak a relay on between ticks.
	if estopActive {
		s.forceSafeState(ctx, includeLights, "emergency stop enforcement")
		return
	}

	snapshot, err := s.mgr.Snapshot(ctx)
	if err != nil {
		// Relay controller unreachable: nothing to enforce against; the
		// device manager and health endpoint surface this failure.
		return
	}
	pumpOn := false
	for _, r := range snapshot {
		if r.Channel == devices.PumpChannel {
			pumpOn = r.State == relay.On
		}
	}

	s.trackPumpRuntime(ctx, pumpOn, cfg)
	s.evaluateReservoir(ctx, cfg)

	// If any pump lockout is active while the pump is running, stop it.
	s.mu.Lock()
	lockedOut := len(s.lockouts) > 0
	s.mu.Unlock()
	if lockedOut && pumpOn {
		if _, err := s.mgr.SetRelay(ctx, devices.PumpChannel, relay.Off, "safety_supervisor", "pump lockout active", 0); err == nil {
			s.events.Warn("pump_forced_off", "pump forced off by active lockout", "safety_supervisor", nil)
		}
	}
}

func (s *Supervisor) trackPumpRuntime(ctx context.Context, pumpOn bool, cfg Config) {
	s.mu.Lock()
	if !pumpOn {
		s.pumpOnSince = nil
		s.mu.Unlock()
		return
	}
	if s.pumpOnSince == nil {
		now := time.Now()
		s.pumpOnSince = &now
		s.mu.Unlock()
		return
	}
	runtime := time.Since(*s.pumpOnSince)
	s.mu.Unlock()

	if runtime <= time.Duration(cfg.MaxPumpRuntimeSeconds)*time.Second {
		return
	}
	// Rule 1: pump exceeded max runtime. Force off and latch a manual-clear
	// lockout — something upstream failed to stop it, so a human looks first.
	if _, err := s.mgr.SetRelay(ctx, devices.PumpChannel, relay.Off, "safety_supervisor",
		fmt.Sprintf("max pump runtime %ds exceeded", cfg.MaxPumpRuntimeSeconds), 0); err != nil {
		s.events.Error("pump_stop_failed",
			"pump exceeded max runtime AND the off command failed — check hardware immediately",
			"safety_supervisor", map[string]any{"error": err.Error()})
	}
	s.setLockout(LockoutPumpMaxRuntime,
		fmt.Sprintf("pump ran longer than the configured maximum of %ds", cfg.MaxPumpRuntimeSeconds), false)
	s.mu.Lock()
	s.pumpOnSince = nil
	s.mu.Unlock()
}

func (s *Supervisor) evaluateReservoir(ctx context.Context, cfg Config) {
	latest, err := s.readings.Latest(cfg.ReservoirSensorKey)
	if err != nil {
		return
	}

	fresh := latest != nil &&
		time.Since(latest.ReadAt) <= time.Duration(cfg.SensorStaleAfterSeconds)*time.Second

	// Rule 4: reservoir sensor offline/stale locks out irrigation, unless
	// the operator explicitly disabled that behavior.
	if cfg.ReservoirOfflineLockout {
		if !fresh {
			s.setLockout(LockoutReservoirOffline,
				"reservoir sensor has no recent reading; irrigation locked out", true)
		} else {
			s.clearAutoLockout(LockoutReservoirOffline)
		}
	} else {
		s.clearAutoLockout(LockoutReservoirOffline)
	}

	// Rule 3: level below minimum locks the pump out. Auto-clears once the
	// level recovers above the minimum.
	if fresh {
		if latest.Value < cfg.MinReservoirLevel {
			s.setLockout(LockoutReservoirLow,
				fmt.Sprintf("reservoir level %.1f below minimum %.1f", latest.Value, cfg.MinReservoirLevel), true)
		} else {
			s.clearAutoLockout(LockoutReservoirLow)
		}
	}
}

func (s *Supervisor) forceSafeState(ctx context.Context, includeLights bool, reason string) {
	states, err := s.mgr.Snapshot(ctx)
	if err != nil {
		return
	}
	for _, r := range states {
		if r.Channel == devices.LightsChannel && !includeLights {
			continue
		}
		if r.State == relay.On {
			s.mgr.SetRelay(ctx, r.Channel, relay.Off, "safety_supervisor", reason, 0)
		}
	}
}

func (s *Supervisor) setLockout(key, reason string, auto bool) {
	s.mu.Lock()
	_, exists := s.lockouts[key]
	if !exists {
		s.lockouts[key] = &Lockout{Key: key, Reason: reason, Since: time.Now().UTC(), Auto: auto}
	}
	s.mu.Unlock()
	if !exists {
		s.events.Warn("safety_lockout", reason, "safety_supervisor", map[string]any{"lockout": key})
	}
}

func (s *Supervisor) clearAutoLockout(key string) {
	s.mu.Lock()
	lo, exists := s.lockouts[key]
	if exists && lo.Auto {
		delete(s.lockouts, key)
	} else {
		exists = false
	}
	s.mu.Unlock()
	if exists {
		s.events.Info("safety_lockout_cleared", fmt.Sprintf("lockout %s cleared (condition resolved)", key),
			"safety_supervisor", map[string]any{"lockout": key})
	}
}
