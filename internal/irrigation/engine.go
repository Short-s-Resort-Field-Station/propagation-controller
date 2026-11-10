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

// MaxDrainSeconds bounds a zone's drain_seconds and a drain-only request.
const MaxDrainSeconds = 3600

type Request struct {
	ZoneKey         string `json:"zone_key"`
	DurationSeconds int    `json:"duration_seconds"`
	// DrainOnly opens the shelf's valve for its drain-back time with the pump
	// locked off — "Drain" on the touch screen (Austin, 2026-11-10), for a tray
	// left full by an interrupted cycle. Runs through the same one-shelf queue.
	DrainOnly     bool      `json:"drain_only,omitempty"`
	TriggerType   string    `json:"trigger_type"`
	TriggerSource string    `json:"trigger_source,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	EnqueuedAt    time.Time `json:"enqueued_at"`
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
		{ZoneKey: "zone_1", Name: "Shelf 1", ValveRelayChannel: 1, SoilSensorKey: &soil1,
			Enabled: true, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
		{ZoneKey: "zone_2", Name: "Shelf 2", ValveRelayChannel: 2,
			Enabled: true, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
		{ZoneKey: "zone_3", Name: "Shelf 3", ValveRelayChannel: 3,
			Enabled: true, MaxFloodSeconds: 1800, MaxPumpSeconds: 1800, ConfigJSON: string(cfg)},
	}
	for _, z := range seeds {
		if err := e.zones.Insert(z); err != nil {
			return err
		}
	}
	e.events.Info("irrigation_zones_seeded", "default irrigation zones created (3 shelves)", "startup", nil)
	return nil
}

func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	e.rootCtx = ctx
	e.mu.Unlock()
	// A cycle still "running" in the DB was interrupted by a crash or power
	// loss (2026-11-10: a reboot mid-drain left a phantom "Watering"). The
	// startup safe-state already closed its valve; close the record too.
	if n, err := e.cycles.AbortInterrupted("interrupted by controller restart"); err != nil {
		e.log.Error().Err(err).Msg("failed to abort interrupted cycles")
	} else if n > 0 {
		e.events.Warn("irrigation_cycles_interrupted",
			fmt.Sprintf("%d irrigation cycle(s) were interrupted by a restart and marked aborted; tray may still hold water — use Drain", n),
			"startup", map[string]any{"count": n})
	}
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
	if req.DurationSeconds <= 0 && !req.DrainOnly {
		return 0, fmt.Errorf("duration_seconds must be > 0")
	}
	if !req.DrainOnly && req.DurationSeconds > zone.MaxFloodSeconds {
		return 0, fmt.Errorf("duration_seconds %d exceeds zone max flood duration %d",
			req.DurationSeconds, zone.MaxFloodSeconds)
	}
	if req.DrainOnly {
		// A drain holds the valve open and blocks every other shelf for its
		// whole duration, so it is capped like a flood is (audit 2026-11-10,
		// 2.7): the zone's own drain time or an hour, whichever is longer.
		if req.DurationSeconds < 0 {
			return 0, fmt.Errorf("duration_seconds must be >= 0")
		}
		var zcfg ZoneConfig
		json.Unmarshal([]byte(zone.ConfigJSON), &zcfg)
		if limit := max(zcfg.DrainSeconds, MaxDrainSeconds); req.DurationSeconds > limit {
			return 0, fmt.Errorf("duration_seconds %d exceeds the drain limit of %d", req.DurationSeconds, limit)
		}
	}
	// Fail fast on conditions that would only fail precheck later anyway.
	// A drain never runs the pump, so a pump lockout (reservoir low/offline —
	// the very faults that leave a tray full) must not refuse it. The valve
	// path still honours e-stop via relayOnChecked (audit 2026-11-10, 2.1).
	if !req.DrainOnly {
		if err := e.safety.Authorize(devices.PumpChannel, relay.On); err != nil {
			return 0, err
		}
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
	if req.DrainOnly {
		// Drain-only cycles are tagged so history and the UI can tell a
		// drain from a watering (requested duration 0, no pump time).
		drain := "drain"
		source = &drain
	} else if req.TriggerSource != "" {
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

	valveCh := zone.ValveRelayChannel

	// --- drain only: valve open, pump never touched, then close ---
	if req.DrainOnly {
		drainFor := zcfg.DrainSeconds
		if req.DurationSeconds > 0 {
			drainFor = req.DurationSeconds
		}
		if drainFor <= 0 {
			drainFor = 300
		}
		e.setState(StateOpeningValve)
		if err := e.relayOnChecked(ctx, valveCh, drainFor+30); err != nil {
			e.finish(cycleID, zone, nil, ResultFailed, "", "valve open failed: "+err.Error())
			return
		}
		zero := 0
		interrupted, err := e.drainAndClose(ctx, zone, valveCh, drainFor)
		switch {
		case err != nil:
			e.finish(cycleID, zone, &zero, ResultFailed, "", "valve close failed: "+err.Error())
		case interrupted:
			e.finish(cycleID, zone, &zero, ResultAborted, e.takeStopReason("drain stopped"), "")
		default:
			e.finish(cycleID, zone, &zero, ResultComplete, "", "")
		}
		return
	}

	// --- precheck (spec §9.3) ---
	if err := e.precheck(zone); err != nil {
		e.finish(cycleID, zone, nil, ResultLockedOut, err.Error(), "")
		return
	}

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
	// Tell the supervisor the pump is ours from here until we report it
	// stopped: a pump found on without this claim (e.g. after a relay
	// outage) is an orphan it may switch off (audit 2026-11-10, 2.2c).
	e.safety.SetPumpExpected(true)
	if err := e.relayOnChecked(ctx, devices.PumpChannel, pumpBackstop); err != nil {
		e.safety.SetPumpExpected(false)
		e.relayOff(valveCh, "cycle failed before pump start")
		e.finish(cycleID, zone, nil, ResultFailed, "", "pump start failed: "+err.Error())
		return
	}

	// --- flooding ---
	e.setState(StateFlooding)
	floodStart := time.Now()
	floodErr := e.flood(ctx, zone, valveCh, zcfg, time.Duration(duration)*time.Second)
	actual := int(time.Since(floodStart).Seconds())

	// --- stop pump first, then close valve (Austin: effectively together) ---
	e.setState(StateStoppingPump)
	pumpOffErr := e.relayOff(devices.PumpChannel, "irrigation cycle ending")
	e.safety.SetPumpExpected(false)

	if floodErr != nil {
		// Ebb-and-flow: an aborted flood still drains back. The pump is
		// already off; holding the valve open returns the tray to the
		// reservoir — which is exactly what a reservoir-low abort needs
		// (2026-11-10: slamming the valve left Shelf 2 full). A second Stop,
		// or e-stop, cuts the drain short.
		reason := e.takeStopReason(floodErr.Error())
		if e.canDrain(valveCh) {
			e.events.Warn("irrigation_abort_draining",
				fmt.Sprintf("zone %s flood aborted (%s); draining back for %ds before closing valve", zone.ZoneKey, reason, zcfg.DrainSeconds),
				"system", map[string]any{"zone": zone.ZoneKey, "cycle_id": cycleID})
			e.drainAndClose(e.rootCtx, zone, valveCh, zcfg.DrainSeconds)
		} else {
			// E-stop, all-off, or a manual valve command already closed the
			// valve (or forbids opening it): waiting out a "drain" with the
			// valve shut is dishonest, so close now and say so (audit
			// 2026-11-10, 2.6).
			e.events.Warn("irrigation_abort_no_drain",
				fmt.Sprintf("zone %s flood aborted (%s); valve closed without drain-back — tray may hold water — use Drain", zone.ZoneKey, reason),
				"system", map[string]any{"zone": zone.ZoneKey, "cycle_id": cycleID})
			e.setState(StateClosingValve)
			e.relayOff(valveCh, "irrigation aborted; no drain")
		}
		e.finish(cycleID, zone, &actual, ResultAborted, reason, errText(pumpOffErr))
		return
	}

	interrupted, valveOffErr := e.drainAndClose(ctx, zone, valveCh, zcfg.DrainSeconds)

	if pumpOffErr != nil || valveOffErr != nil {
		e.finish(cycleID, zone, &actual, ResultFailed, "",
			"relay off failed: "+errText(pumpOffErr)+errText(valveOffErr))
		return
	}
	if interrupted {
		// The flood itself finished; a Stop during drain-back is still an
		// abort — the tray was not given its full drain (audit 2.13 F14).
		e.finish(cycleID, zone, &actual, ResultAborted, e.takeStopReason("drain stopped"), "")
		return
	}
	e.finish(cycleID, zone, &actual, ResultComplete, "", "")
}

// takeStopReason returns the Stop/StopAll reason recorded for the active
// cycle, or fallback if the cycle ended for another reason.
func (e *Engine) takeStopReason(fallback string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopReason != "" {
		return e.stopReason
	}
	return fallback
}

// canDrain reports whether an abort-drain makes sense: the valve must still
// be open (nothing else closed it) and safety must allow it to be open.
func (e *Engine) canDrain(valveCh int) bool {
	if err := e.safety.Authorize(valveCh, relay.On); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := e.mgr.GetRelay(ctx, valveCh)
	return err == nil && info.State == relay.On
}

// drainAndClose holds the valve open for drainSeconds (interruptible by a
// Stop/e-stop through a fresh cancel installed as the active cycle's), then
// closes it. Reports whether the wait was cut short, and the valve-off
// error, if any.
func (e *Engine) drainAndClose(parent context.Context, zone *storage.Zone, valveCh, drainSeconds int) (interrupted bool, err error) {
	if drainSeconds > 0 {
		e.setState(StateDraining)
		drainCtx, cancel := context.WithCancel(parent)
		e.mu.Lock()
		e.cancelRun = cancel
		e.mu.Unlock()
		interrupted = e.holdValveOpen(drainCtx, zone, valveCh, time.Duration(drainSeconds)*time.Second)
		cancel()
	}
	e.setState(StateClosingValve)
	return interrupted, e.relayOff(valveCh, "irrigation cycle complete")
}

// holdValveOpen waits out the drain window. It ends early (returning true)
// on cancellation, or when the valve reads off — all-off, a relay timeout,
// or a manual command closed it, and "draining" with a shut valve is a lie
// that would also block the queue (audit 2026-11-10, 2.6).
func (e *Engine) holdValveOpen(ctx context.Context, zone *storage.Zone, valveCh int, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-timer.C:
			return false
		case <-tick.C:
			info, err := e.mgr.GetRelay(ctx, valveCh)
			if err != nil || info.State == relay.On {
				continue
			}
			e.mu.Lock()
			if e.stopReason == "" {
				e.stopReason = "valve closed during drain-back"
			}
			e.mu.Unlock()
			e.events.Warn("irrigation_drain_cut_short",
				fmt.Sprintf("zone %s valve closed during drain-back — tray may hold water — use Drain", zone.ZoneKey),
				"system", map[string]any{"zone": zone.ZoneKey})
			return true
		}
	}
}

// flood waits out the flood duration while watching for aborts, safety
// lockouts/e-stop, the optional moisture target, and — about once a second —
// the valve and pump relays themselves: a manual command or a timeout that
// closes the valve under a running pump dead-heads it against the manifold,
// and a pump that reads off is not flooding (audit 2026-11-10, 2.4).
func (e *Engine) flood(ctx context.Context, zone *storage.Zone, valveCh int, zcfg ZoneConfig, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	lastRelayCheck := time.Now()
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
			if time.Since(lastRelayCheck) >= time.Second {
				lastRelayCheck = time.Now()
				if err := e.checkFloodRelays(ctx, valveCh); err != nil {
					return err
				}
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

// checkFloodRelays aborts the flood if the valve or pump no longer reads on.
// A read error is not an abort: the supervisor owns the unreachable-board
// case and the relays hold their last state.
func (e *Engine) checkFloodRelays(ctx context.Context, valveCh int) error {
	for _, ch := range []int{valveCh, devices.PumpChannel} {
		info, err := e.mgr.GetRelay(ctx, ch)
		if err != nil {
			return nil
		}
		if info.State != relay.On {
			return fmt.Errorf("relay %d (%s) read %s during flood", ch, devices.DefaultAssignments[ch], info.State)
		}
	}
	return nil
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
