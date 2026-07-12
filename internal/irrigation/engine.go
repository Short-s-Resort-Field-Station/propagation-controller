// Package irrigation implements the four-zone irrigation engine (spec §9).
//
// Hard rule (Austin, 2026-07-05): only ONE zone may receive water at a time.
// All requests enter a FIFO queue and a single worker drains it, so two
// shelves can never flood simultaneously.
//
// Plumbing model: reservoir → pump → shared manifold → one solenoid per
// shelf. The valve opens just before the pump starts (never pump against a
// closed manifold) and the pump stops just before the valve closes.
package irrigation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
)

// Cycle states (spec §9.2).
const (
	StatePrecheck     = "precheck"
	StateOpeningValve = "opening_valve"
	StateStartingPump = "starting_pump"
	StateFlooding     = "flooding"
	StateStoppingPump = "stopping_pump"
	StateDraining     = "draining"
	StateClosingValve = "closing_valve"

	ResultComplete  = "complete"
	ResultAborted   = "aborted"
	ResultFailed    = "failed"
	ResultLockedOut = "locked_out"
)

// ZoneConfig is the per-zone config_json payload.
type ZoneConfig struct {
	DefaultRunSeconds int `json:"default_run_seconds"`
	// DrainSeconds keeps the shelf's valve open after the pump stops so the
	// tray drains back through the (now off) pump — the ebb-and-flow model
	// (Austin swapped to a drain-back pump, 2026-07-09). The next shelf does
	// not start until this drain completes and the valve closes. Default 300 s
	// (5 min); adjustable per zone.
	DrainSeconds int `json:"drain_seconds"`
	// TargetMoisture ends flooding early once the zone's soil sensor reads
	// at or above this value. The run duration remains the hard cap.
	TargetMoisture *float64 `json:"target_moisture,omitempty"`
	// Specimen assignment (species catalog or free text). A shelf may house
	// two species at once (Austin, 2026-07-06).
	Species       string `json:"species,omitempty"`
	SpeciesLatin  string `json:"species_latin,omitempty"`
	Species2      string `json:"species2,omitempty"`
	Species2Latin string `json:"species2_latin,omitempty"`

	// change 3 (species profiles): monitoring-only target RANGES — the unit
	// senses EC/temperature but does not control them. These drive the gauge
	// "happy band" and the Environmental time-in-range tracking; nil = no band.
	// EC is substrate conductivity in µS/cm (ZTS-3000 probe, register 2).
	ECMin    *float64 `json:"ec_min,omitempty"`
	ECMax    *float64 `json:"ec_max,omitempty"`
	TempMinF *float64 `json:"temp_min_f,omitempty"`
	TempMaxF *float64 `json:"temp_max_f,omitempty"`
	// Which saved profile is applied to this shelf (for display + "update the
	// profile from the current shelf"). SpeciesID matches the species catalog.
	SpeciesID   string `json:"species_id,omitempty"`
	ProfileID   int64  `json:"profile_id,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
	Stage       string `json:"stage,omitempty"`
}

type Request struct {
	ZoneKey         string    `json:"zone_key"`
	DurationSeconds int       `json:"duration_seconds"`
	TriggerType     string    `json:"trigger_type"`
	TriggerSource   string    `json:"trigger_source,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	EnqueuedAt      time.Time `json:"enqueued_at"`
}

type ActiveCycle struct {
	CycleID         int64     `json:"cycle_id"`
	ZoneKey         string    `json:"zone_key"`
	State           string    `json:"state"`
	StartedAt       time.Time `json:"started_at"`
	DurationSeconds int       `json:"duration_seconds"`
	TriggerType     string    `json:"trigger_type"`
}

type Engine struct {
	mgr      *devices.Manager
	safety   *safety.Supervisor
	zones    *storage.ZoneRepo
	cycles   *storage.CycleRepo
	readings *storage.ReadingRepo
	events   *logging.EventLogger
	log      zerolog.Logger

	reservoirSensorKey string
	hub                *bus.Hub

	mu         sync.Mutex
	rootCtx    context.Context
	queue      []*Request
	active     *ActiveCycle
	stopReason string
	cancelRun  context.CancelFunc
	wake       chan struct{}
}

func NewEngine(mgr *devices.Manager, sup *safety.Supervisor, zones *storage.ZoneRepo,
	cycles *storage.CycleRepo, readings *storage.ReadingRepo, events *logging.EventLogger,
	log zerolog.Logger, reservoirSensorKey string) *Engine {
	return &Engine{
		mgr:                mgr,
		safety:             sup,
		zones:              zones,
		cycles:             cycles,
		readings:           readings,
		events:             events,
		log:                log,
		reservoirSensorKey: reservoirSensorKey,
		wake:               make(chan struct{}, 1),
	}
}

// SeedDefaults creates the shelf zones on first boot: three active shelves
// (valves 3–5) plus a disabled fourth for expansion.
func (e *Engine) SeedDefaults() error {
	existing, err := e.zones.List()
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	// Flood caps sized for the largest pots: soaks can run up to 30 minutes
	// (Austin, 2026-07-06).
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: 300})
	soil1 := "soil_zone_1_moisture"
	seeds := []storage.Zone{
		{ZoneKey: "zone_1", Name: "Shelf 1", ValveRelayChannel: 3, SoilSensorKey: &soil1,
			Enabled: true, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
		{ZoneKey: "zone_2", Name: "Shelf 2", ValveRelayChannel: 4,
			Enabled: true, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
		{ZoneKey: "zone_3", Name: "Shelf 3", ValveRelayChannel: 5,
			Enabled: true, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
		{ZoneKey: "zone_4", Name: "Shelf 4 (future)", ValveRelayChannel: 6,
			Enabled: false, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
	}
	for _, z := range seeds {
		if err := e.zones.Insert(z); err != nil {
			return err
		}
	}
	e.events.Info("irrigation_zones_seeded", "default irrigation zones created (3 shelves + 1 disabled)", "startup", nil)
	return nil
}

func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	e.rootCtx = ctx
	e.mu.Unlock()
	go e.worker(ctx)
}

func (e *Engine) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		}
		for {
			e.mu.Lock()
			if len(e.queue) == 0 {
				e.mu.Unlock()
				break
			}
			req := e.queue[0]
			e.queue = e.queue[1:]
			runCtx, cancel := context.WithCancel(ctx)
			e.cancelRun = cancel
			e.stopReason = ""
			// Mark active in the same critical section as the pop, so a zone
			// is never momentarily neither queued nor active (Enqueue's
			// duplicate check depends on this).
			e.active = &ActiveCycle{
				ZoneKey: req.ZoneKey, State: StatePrecheck,
				StartedAt: time.Now().UTC(), DurationSeconds: req.DurationSeconds,
				TriggerType: req.TriggerType,
			}
			e.mu.Unlock()

			e.runCycle(runCtx, req)

			e.mu.Lock()
			e.cancelRun = nil
			e.active = nil
			e.mu.Unlock()
			cancel()
		}
	}
}

// Enqueue validates and queues a run request. Duplicate requests for a zone
// that is already active or queued are rejected — one shelf, one slot.
func (e *Engine) Enqueue(req Request) (position int, err error) {
	zone, err := e.zones.GetByKey(req.ZoneKey)
	if err != nil {
		return 0, err
	}
	if zone == nil {
		return 0, fmt.Errorf("unknown zone %q", req.ZoneKey)
	}
	if !zone.Enabled {
		return 0, fmt.Errorf("zone %q is disabled", req.ZoneKey)
	}
	if req.DurationSeconds <= 0 {
		return 0, fmt.Errorf("duration_seconds must be > 0")
	}
	if req.DurationSeconds > zone.MaxFloodSeconds {
		return 0, fmt.Errorf("duration_seconds %d exceeds zone max flood duration %d",
			req.DurationSeconds, zone.MaxFloodSeconds)
	}
	// Fail fast on conditions that would only fail precheck later anyway.
	if err := e.safety.Authorize(devices.PumpChannel, relay.On); err != nil {
		return 0, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active != nil && e.active.ZoneKey == req.ZoneKey {
		return 0, fmt.Errorf("zone %q is already running", req.ZoneKey)
	}
	for _, q := range e.queue {
		if q.ZoneKey == req.ZoneKey {
			return 0, fmt.Errorf("zone %q is already queued", req.ZoneKey)
		}
	}
	req.EnqueuedAt = time.Now().UTC()
	e.queue = append(e.queue, &req)
	// Position counts the active cycle: position 1 = runs immediately.
	position = len(e.queue)
	if e.active != nil {
		position++
	}
	if position > 1 {
		e.events.Info("irrigation_queued",
			fmt.Sprintf("zone %s queued for irrigation (position %d)", req.ZoneKey, position),
			req.TriggerType, map[string]any{"zone": req.ZoneKey, "position": position})
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
	return position, nil
}

// Stop aborts the active cycle for a zone or removes it from the queue.
func (e *Engine) Stop(zoneKey, actor string) error {
	e.mu.Lock()
	if e.active != nil && e.active.ZoneKey == zoneKey {
		e.stopReason = fmt.Sprintf("stopped by %s", actor)
		cancel := e.cancelRun
		e.mu.Unlock()
		cancel()
		return nil
	}
	for i, q := range e.queue {
		if q.ZoneKey == zoneKey {
			e.queue = append(e.queue[:i], e.queue[i+1:]...)
			e.mu.Unlock()
			e.events.Info("irrigation_dequeued",
				fmt.Sprintf("zone %s removed from irrigation queue", zoneKey), actor, nil)
			return nil
		}
	}
	e.mu.Unlock()
	return fmt.Errorf("zone %q is not running or queued", zoneKey)
}

// StopAll clears the queue and aborts any active cycle.
func (e *Engine) StopAll(actor string) {
	e.mu.Lock()
	cleared := len(e.queue)
	e.queue = nil
	var cancel context.CancelFunc
	if e.active != nil {
		e.stopReason = fmt.Sprintf("stop all by %s", actor)
		cancel = e.cancelRun
	}
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cleared > 0 || cancel != nil {
		e.events.Info("irrigation_stop_all", "irrigation queue cleared and active cycle aborted", actor,
			map[string]any{"queue_cleared": cleared})
	}
}

type EngineStatus struct {
	Active *ActiveCycle `json:"active_cycle"`
	Queue  []Request    `json:"queue"`
}

func (e *Engine) Status() EngineStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := EngineStatus{Queue: []Request{}}
	if e.active != nil {
		a := *e.active
		st.Active = &a
	}
	for _, q := range e.queue {
		st.Queue = append(st.Queue, *q)
	}
	return st
}

// ZoneState reports where a zone stands right now.
func (e *Engine) ZoneState(zoneKey string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active != nil && e.active.ZoneKey == zoneKey {
		return "running"
	}
	for _, q := range e.queue {
		if q.ZoneKey == zoneKey {
			return "queued"
		}
	}
	return "ready"
}

// runCycle drives one zone through the state machine.
func (e *Engine) runCycle(ctx context.Context, req *Request) {
	zone, err := e.zones.GetByKey(req.ZoneKey)
	if err != nil || zone == nil || !zone.Enabled {
		e.events.Warn("irrigation_cycle_skipped",
			fmt.Sprintf("zone %s no longer available; request dropped", req.ZoneKey),
			"system", nil)
		return
	}
	var zcfg ZoneConfig
	json.Unmarshal([]byte(zone.ConfigJSON), &zcfg)

	duration := min(req.DurationSeconds, zone.MaxFloodSeconds, zone.MaxPumpSeconds)
	var source *string
	if req.TriggerSource != "" {
		source = &req.TriggerSource
	}
	cycleID, err := e.cycles.InsertStart(zone.ID, duration, req.TriggerType, source,
		e.latestValue(e.reservoirSensorKey), e.latestValue(deref(zone.SoilSensorKey)))
	if err != nil {
		e.log.Error().Err(err).Msg("failed to record irrigation cycle start")
		return
	}

	e.setActive(&ActiveCycle{
		CycleID: cycleID, ZoneKey: zone.ZoneKey, State: StatePrecheck,
		StartedAt: time.Now().UTC(), DurationSeconds: duration, TriggerType: req.TriggerType,
	})
	e.events.Info("irrigation_cycle_started",
		fmt.Sprintf("zone %s (%s) irrigation cycle started, %ds", zone.ZoneKey, zone.Name, duration),
		req.TriggerType, map[string]any{"cycle_id": cycleID, "zone": zone.ZoneKey})

	// --- precheck (spec §9.3) ---
	if err := e.precheck(zone); err != nil {
		e.finish(cycleID, zone, nil, ResultLockedOut, err.Error(), "")
		return
	}

	valveCh := zone.ValveRelayChannel

	// --- open valve, then pump: never pump against a closed manifold ---
	e.setState(StateOpeningValve)
	// Valve backstop timeout: even if this process dies mid-cycle, the relay
	// timer closes the valve.
	valveBackstop := duration + zcfg.DrainSeconds + 30
	if err := e.relayOnChecked(ctx, valveCh, valveBackstop); err != nil {
		e.finish(cycleID, zone, nil, ResultFailed, "", "valve open failed: "+err.Error())
		return
	}

	e.setState(StateStartingPump)
	pumpBackstop := duration + 15
	if err := e.relayOnChecked(ctx, devices.PumpChannel, pumpBackstop); err != nil {
		e.relayOff(valveCh, "cycle failed before pump start")
		e.finish(cycleID, zone, nil, ResultFailed, "", "pump start failed: "+err.Error())
		return
	}

	// --- flooding ---
	e.setState(StateFlooding)
	floodStart := time.Now()
	floodErr := e.flood(ctx, zone, zcfg, time.Duration(duration)*time.Second)
	actual := int(time.Since(floodStart).Seconds())

	// --- stop pump first, then close valve (Austin: effectively together) ---
	e.setState(StateStoppingPump)
	pumpOffErr := e.relayOff(devices.PumpChannel, "irrigation cycle ending")

	if floodErr != nil {
		e.relayOff(valveCh, "irrigation cycle aborted")
		reason := floodErr.Error()
		e.mu.Lock()
		if e.stopReason != "" {
			reason = e.stopReason
		}
		e.mu.Unlock()
		e.finish(cycleID, zone, &actual, ResultAborted, reason, errText(pumpOffErr))
		return
	}

	if zcfg.DrainSeconds > 0 {
		e.setState(StateDraining)
		// Valve stays open to drain; only an abort or e-stop cuts it short.
		e.waitInterruptible(ctx, time.Duration(zcfg.DrainSeconds)*time.Second)
	}

	e.setState(StateClosingValve)
	valveOffErr := e.relayOff(valveCh, "irrigation cycle complete")

	if pumpOffErr != nil || valveOffErr != nil {
		e.finish(cycleID, zone, &actual, ResultFailed, "",
			"relay off failed: "+errText(pumpOffErr)+errText(valveOffErr))
		return
	}
	e.finish(cycleID, zone, &actual, ResultComplete, "", "")
}

// flood waits out the flood duration while watching for aborts, safety
// lockouts/e-stop, and the optional moisture target.
func (e *Engine) flood(ctx context.Context, zone *storage.Zone, zcfg ZoneConfig, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("cycle stopped")
		case <-timer.C:
			return nil
		case <-tick.C:
			if err := e.safety.Authorize(devices.PumpChannel, relay.On); err != nil {
				return fmt.Errorf("safety abort: %w", err)
			}
			if zcfg.TargetMoisture != nil && zone.SoilSensorKey != nil {
				if v := e.latestValue(*zone.SoilSensorKey); v != nil && *v >= *zcfg.TargetMoisture {
					e.events.Info("irrigation_moisture_target",
						fmt.Sprintf("zone %s reached moisture target %.1f (reading %.1f); ending flood early",
							zone.ZoneKey, *zcfg.TargetMoisture, *v),
						"system", map[string]any{"zone": zone.ZoneKey})
					return nil
				}
			}
		}
	}
}

func (e *Engine) waitInterruptible(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// precheck implements spec §9.3. The safety supervisor covers e-stop, global
// reservoir, and pump lockouts; the zone-specific minimum is checked here.
func (e *Engine) precheck(zone *storage.Zone) error {
	if err := e.safety.Authorize(devices.PumpChannel, relay.On); err != nil {
		return err
	}
	if zone.MinReservoirLevel != nil {
		level := e.latestValue(e.reservoirSensorKey)
		if level == nil {
			return fmt.Errorf("zone %s requires reservoir level >= %.1f but no reading is available",
				zone.ZoneKey, *zone.MinReservoirLevel)
		}
		if *level < *zone.MinReservoirLevel {
			return fmt.Errorf("reservoir %.1f below zone minimum %.1f", *level, *zone.MinReservoirLevel)
		}
	}
	return nil
}

func (e *Engine) relayOnChecked(ctx context.Context, channel, timeoutSeconds int) error {
	if err := e.safety.Authorize(channel, relay.On); err != nil {
		return err
	}
	_, err := e.mgr.SetRelay(ctx, channel, relay.On, "system", "irrigation cycle", timeoutSeconds)
	return err
}

func (e *Engine) relayOff(channel int, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := e.mgr.SetRelay(ctx, channel, relay.Off, "system", reason, 0)
	if err != nil {
		e.events.Error("irrigation_relay_off_failed",
			fmt.Sprintf("relay %d failed to turn off during irrigation — check hardware", channel),
			"system", map[string]any{"channel": channel, "error": err.Error()})
	}
	return err
}

func (e *Engine) finish(cycleID int64, zone *storage.Zone, actualSeconds *int,
	result, abortReason, errorMessage string) {
	var abort, errMsg *string
	if abortReason != "" {
		abort = &abortReason
	}
	if errorMessage != "" {
		errMsg = &errorMessage
	}
	if err := e.cycles.Finish(cycleID, actualSeconds, result, abort, errMsg,
		e.latestValue(e.reservoirSensorKey), e.latestValue(deref(zone.SoilSensorKey))); err != nil {
		e.log.Error().Err(err).Int64("cycle", cycleID).Msg("failed to record cycle finish")
	}

	msg := fmt.Sprintf("zone %s irrigation %s", zone.ZoneKey, result)
	details := map[string]any{"cycle_id": cycleID, "zone": zone.ZoneKey, "result": result}
	if abortReason != "" {
		details["abort_reason"] = abortReason
	}
	if errorMessage != "" {
		details["error"] = errorMessage
	}
	switch result {
	case ResultComplete:
		e.events.Info("irrigation_cycle_complete", msg, "system", details)
	case ResultFailed:
		e.events.Error("irrigation_cycle_failed", msg, "system", details)
	default:
		e.events.Warn("irrigation_cycle_"+result, msg, "system", details)
	}
	e.hub.Publish("irrigation_cycle_update", map[string]any{
		"cycle_id": cycleID, "zone_key": zone.ZoneKey, "state": "ended", "result": result,
	})
}

func (e *Engine) setActive(a *ActiveCycle) {
	e.mu.Lock()
	e.active = a
	e.mu.Unlock()
}

// AttachHub enables live irrigation_cycle_update events over SSE.
func (e *Engine) AttachHub(h *bus.Hub) { e.hub = h }

func (e *Engine) setState(state string) {
	e.mu.Lock()
	var snapshot *ActiveCycle
	if e.active != nil {
		e.active.State = state
		a := *e.active
		snapshot = &a
	}
	e.mu.Unlock()
	if snapshot != nil {
		e.hub.Publish("irrigation_cycle_update", snapshot)
	}
}

func (e *Engine) latestValue(sensorKey string) *float64 {
	if sensorKey == "" {
		return nil
	}
	latest, err := e.readings.Latest(sensorKey)
	if err != nil || latest == nil {
		return nil
	}
	v := latest.Value
	return &v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
