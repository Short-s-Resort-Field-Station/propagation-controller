package irrigation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
)

type fixture struct {
	engine   *Engine
	mgr      *devices.Manager
	sup      *safety.Supervisor
	zones    *storage.ZoneRepo
	cycles   *storage.CycleRepo
	readings *storage.ReadingRepo
	events   *storage.EventRepo
	resID    int64
	ctx      context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	deviceRepo := storage.NewDeviceRepo(db)
	readings := storage.NewReadingRepo(db)
	settings := storage.NewSettingsRepo(db)
	eventRepo := storage.NewEventRepo(db)
	eventLog := logging.NewEventLogger(zerolog.Nop(), eventRepo)

	mgr := devices.NewManager(relay.NewMock(), "mock", storage.NewRelayActionRepo(db), deviceRepo, eventLog, true)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}

	deviceRepo.Upsert("reservoir_level", "Reservoir", "reservoir_sensor", "generic_modbus", "{}", "simulated")
	dev, _ := deviceRepo.GetByKey("reservoir_level")

	cfg := safety.DefaultConfig()
	cfg.SensorStaleAfterSeconds = 3600
	body, _ := json.Marshal(cfg)
	settings.Set("safety", string(body))

	sup := safety.NewSupervisor(mgr, readings, settings, eventLog, zerolog.Nop(), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("supervisor start: %v", err)
	}

	zones := storage.NewZoneRepo(db)
	cycles := storage.NewCycleRepo(db)
	engine := NewEngine(mgr, sup, zones, cycles, readings, eventLog, zerolog.Nop(), "reservoir_level_level")
	if err := engine.SeedDefaults(); err != nil {
		t.Fatalf("seed zones: %v", err)
	}
	// The ebb-and-flow default drain (5 min) would make every generic cycle
	// test wait out the drain phase. Reset seeded zones to zero drain here;
	// the drain phase itself is exercised by TestDrainPhaseHoldsValveOpen.
	for _, zk := range []string{"zone_1", "zone_2", "zone_3"} {
		z, err := zones.GetByKey(zk)
		if err != nil || z == nil {
			continue
		}
		var zc ZoneConfig
		_ = json.Unmarshal([]byte(z.ConfigJSON), &zc)
		zc.DrainSeconds = 0
		b, _ := json.Marshal(zc)
		z.ConfigJSON = string(b)
		if err := zones.Update(*z); err != nil {
			t.Fatalf("reset zone drain: %v", err)
		}
	}
	// Same wiring as cmd/propagationd: e-stop aborts the engine (audit 2.6).
	sup.OnEmergencyStop(func() { engine.StopAll("safety_supervisor") })
	engine.Start(ctx)

	f := &fixture{engine: engine, mgr: mgr, sup: sup, zones: zones, cycles: cycles,
		readings: readings, events: eventRepo, resID: dev.ID, ctx: ctx}
	f.insertLevel(t, 80)
	return f
}

func (f *fixture) insertLevel(t *testing.T, value float64) {
	t.Helper()
	err := f.readings.InsertBatch(f.resID, []storage.ReadingInsert{
		{SensorKey: "reservoir_level_level", Value: value, Unit: "%", Quality: "good", ReadAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
}

func (f *fixture) waitCycleResult(t *testing.T, zoneKey, want string, timeout time.Duration) storage.Cycle {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cycles, err := f.cycles.List(zoneKey, 1)
		if err != nil {
			t.Fatalf("list cycles: %v", err)
		}
		if len(cycles) > 0 && cycles[0].Result == want {
			return cycles[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	cycles, _ := f.cycles.List(zoneKey, 1)
	if len(cycles) > 0 {
		t.Fatalf("zone %s: last cycle result %q, want %q", zoneKey, cycles[0].Result, want)
	}
	t.Fatalf("zone %s: no cycle recorded", zoneKey)
	return storage.Cycle{}
}

func (f *fixture) setDrain(t *testing.T, zoneKey string, drainSeconds int) {
	t.Helper()
	z, _ := f.zones.GetByKey(zoneKey)
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: drainSeconds})
	z.ConfigJSON = string(cfg)
	if err := f.zones.Update(*z); err != nil {
		t.Fatalf("update zone: %v", err)
	}
}

func (f *fixture) waitState(t *testing.T, zoneKey, state string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st := f.engine.Status(); st.Active != nil && st.Active.ZoneKey == zoneKey && st.Active.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("zone %s never reached state %q", zoneKey, state)
}

func (f *fixture) hasEvent(t *testing.T, eventType string) bool {
	t.Helper()
	list, err := f.events.List(storage.EventFilter{EventType: eventType, Limit: 10})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return len(list) > 0
}

func (f *fixture) relayState(t *testing.T, ch int) relay.State {
	t.Helper()
	info, err := f.mgr.GetRelay(context.Background(), ch)
	if err != nil {
		t.Fatalf("get relay %d: %v", ch, err)
	}
	return info.State
}

func TestManualRunCompletesAndLogsCycle(t *testing.T) {
	f := newFixture(t)
	pos, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 1, TriggerType: "manual_web"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if pos != 1 {
		t.Errorf("position = %d, want 1", pos)
	}
	cycle := f.waitCycleResult(t, "zone_1", ResultComplete, 5*time.Second)
	if cycle.ReservoirLevelStart == nil || *cycle.ReservoirLevelStart != 80 {
		t.Errorf("reservoir start not recorded: %+v", cycle.ReservoirLevelStart)
	}
	if f.relayState(t, devices.PumpChannel) != relay.Off {
		t.Error("pump still on after cycle")
	}
	if f.relayState(t, 3) != relay.Off {
		t.Error("valve still open after cycle")
	}
}

// The core requirement: two shelves must NEVER flood at the same time.
func TestQueueNeverRunsTwoZonesAtOnce(t *testing.T) {
	f := newFixture(t)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 1, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue zone_1: %v", err)
	}
	pos, err := f.engine.Enqueue(Request{ZoneKey: "zone_2", DurationSeconds: 1, TriggerType: "manual_web"})
	if err != nil {
		t.Fatalf("enqueue zone_2: %v", err)
	}
	if pos < 1 {
		t.Errorf("zone_2 queue position = %d", pos)
	}

	// Watch valve channels 1 and 2 continuously until both cycles finish.
	done := make(chan struct{})
	violations := 0
	go func() {
		defer close(done)
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			s3 := f.relayState(t, 1)
			s4 := f.relayState(t, 2)
			if s3 == relay.On && s4 == relay.On {
				violations++
			}
			cycles, _ := f.cycles.List("zone_2", 1)
			if len(cycles) > 0 && cycles[0].Result == ResultComplete {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	<-done

	if violations > 0 {
		t.Fatalf("both valves were open simultaneously %d times", violations)
	}
	f.waitCycleResult(t, "zone_1", ResultComplete, time.Second)
	c2 := f.waitCycleResult(t, "zone_2", ResultComplete, time.Second)
	c1 := f.waitCycleResult(t, "zone_1", ResultComplete, time.Second)
	if c1.EndedAt == nil || c2.StartedAt.Before(*c1.EndedAt) {
		t.Errorf("zone_2 started (%v) before zone_1 ended (%v)", c2.StartedAt, c1.EndedAt)
	}
}

func TestDuplicateEnqueueRejected(t *testing.T) {
	f := newFixture(t)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 2, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 2, TriggerType: "manual_web"}); err == nil {
		t.Error("duplicate enqueue should be rejected")
	}
	f.engine.StopAll("test")
}

func TestStopAbortsActiveCycle(t *testing.T) {
	f := newFixture(t)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Wait until it is actually flooding.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := f.engine.Status(); st.Active != nil && st.Active.State == StateFlooding {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := f.engine.Stop("zone_1", "manual_touch"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 5*time.Second)
	if cycle.AbortReason == nil || *cycle.AbortReason == "" {
		t.Error("abort reason missing")
	}
	if f.relayState(t, devices.PumpChannel) != relay.Off || f.relayState(t, 3) != relay.Off {
		t.Error("pump/valve not off after abort")
	}
}

func TestEstopAbortsMidFlood(t *testing.T) {
	f := newFixture(t)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := f.engine.Status(); st.Active != nil && st.Active.State == StateFlooding {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.sup.EmergencyStop(context.Background(), "manual_touch", "test")
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 5*time.Second)
	if cycle.AbortReason == nil {
		t.Fatal("abort reason missing")
	}
	if f.relayState(t, devices.PumpChannel) != relay.Off || f.relayState(t, 3) != relay.Off {
		t.Error("pump/valve not off after estop abort")
	}
	f.sup.Reset("test")
}

func TestEnqueueRejectedDuringEstop(t *testing.T) {
	f := newFixture(t)
	f.sup.EmergencyStop(context.Background(), "manual_web", "test")
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 5, TriggerType: "manual_web"}); err == nil {
		t.Error("enqueue should fail during estop")
	}
	f.sup.Reset("test")
}

func TestMoistureTargetEndsFloodEarly(t *testing.T) {
	f := newFixture(t)
	// Configure zone_1 with a moisture target and give it a soil reading
	// already above the target: the flood should end almost immediately.
	zone, _ := f.zones.GetByKey("zone_1")
	target := 35.0
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: 0, TargetMoisture: &target})
	zone.ConfigJSON = string(cfg)
	if err := f.zones.Update(*zone); err != nil {
		t.Fatalf("update zone: %v", err)
	}
	err := f.readings.InsertBatch(f.resID, []storage.ReadingInsert{
		{SensorKey: "soil_zone_1_moisture", Value: 40, Unit: "%", Quality: "good", ReadAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("insert moisture: %v", err)
	}

	start := time.Now()
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 30, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	f.waitCycleResult(t, "zone_1", ResultComplete, 5*time.Second)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("cycle took %v; moisture target should have ended it early", elapsed)
	}
}

// TestDrainPhaseHoldsValveOpen verifies the ebb-and-flow drain-back: after the
// pump stops the shelf's valve stays OPEN for the drain window, the pump is
// off, and the next shelf cannot start until that valve finally closes.
func TestDrainPhaseHoldsValveOpen(t *testing.T) {
	f := newFixture(t)
	z, _ := f.zones.GetByKey("zone_1")
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: 3})
	z.ConfigJSON = string(cfg)
	if err := f.zones.Update(*z); err != nil {
		t.Fatalf("update zone: %v", err)
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 1, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue zone_1: %v", err)
	}

	// Wait for the drain phase: pump OFF while zone_1's valve (ch 1) stays OPEN.
	drainSeen := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if f.relayState(t, devices.PumpChannel) == relay.Off && f.relayState(t, 1) == relay.On {
			drainSeen = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !drainSeen {
		t.Fatal("never observed drain phase (pump off, valve open)")
	}

	// Request zone_2 during zone_1's drain: its valve (ch 2) must stay closed
	// and zone_1's valve must still be open — one shelf at a time, drain included.
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_2", DurationSeconds: 1, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue zone_2: %v", err)
	}
	if f.relayState(t, 2) != relay.Off {
		t.Error("zone_2 valve opened while zone_1 was still draining")
	}
	if f.relayState(t, 1) != relay.On {
		t.Error("zone_1 valve closed before its drain completed")
	}

	// zone_1 completes and its valve closes; only then does zone_2 run.
	f.waitCycleResult(t, "zone_1", ResultComplete, 8*time.Second)
	if f.relayState(t, 1) != relay.Off {
		t.Error("zone_1 valve still open after cycle complete")
	}
	f.waitCycleResult(t, "zone_2", ResultComplete, 8*time.Second)
}

// TestDrainOnlyOpensValveWithoutPump: the touch-screen "Drain" action opens
// the shelf's valve for the drain window and never touches the pump.
func TestDrainOnlyOpensValveWithoutPump(t *testing.T) {
	f := newFixture(t)
	z, _ := f.zones.GetByKey("zone_1")
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: 2})
	z.ConfigJSON = string(cfg)
	if err := f.zones.Update(*z); err != nil {
		t.Fatalf("update zone: %v", err)
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DrainOnly: true, TriggerType: "manual_touch"}); err != nil {
		t.Fatalf("enqueue drain: %v", err)
	}
	sawValveOpen := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if f.relayState(t, devices.PumpChannel) == relay.On {
			t.Fatal("pump turned on during a drain-only request")
		}
		if f.relayState(t, 1) == relay.On {
			sawValveOpen = true
		}
		if sawValveOpen && f.relayState(t, 1) == relay.Off {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sawValveOpen {
		t.Fatal("valve never opened for drain")
	}
	cycle := f.waitCycleResult(t, "zone_1", ResultComplete, 5*time.Second)
	if cycle.TriggerSource == nil || *cycle.TriggerSource != "drain" {
		t.Errorf("drain cycle not tagged as drain: %+v", cycle.TriggerSource)
	}
	if f.relayState(t, 1) != relay.Off {
		t.Error("valve still open after drain complete")
	}
}

// TestAbortedFloodStillDrains: a safety abort mid-flood (reservoir drops below
// the minimum) stops the pump but holds the valve open for the drain window
// before closing — the tray must never be left full (2026-11-10).
func TestAbortedFloodStillDrains(t *testing.T) {
	f := newFixture(t)
	z, _ := f.zones.GetByKey("zone_1")
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: 3})
	z.ConfigJSON = string(cfg)
	if err := f.zones.Update(*z); err != nil {
		t.Fatalf("update zone: %v", err)
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 10, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Wait for flooding, then drop the reservoir below the safety minimum.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && f.relayState(t, devices.PumpChannel) != relay.On {
		time.Sleep(25 * time.Millisecond)
	}
	if f.relayState(t, devices.PumpChannel) != relay.On {
		t.Fatal("pump never started")
	}
	f.insertLevel(t, 5)
	// Pump must drop; valve must remain open (draining) rather than slam shut.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && f.relayState(t, devices.PumpChannel) == relay.On {
		time.Sleep(25 * time.Millisecond)
	}
	if f.relayState(t, devices.PumpChannel) == relay.On {
		t.Fatal("pump still on after reservoir-low")
	}
	time.Sleep(300 * time.Millisecond)
	if f.relayState(t, 1) != relay.On {
		t.Error("valve closed immediately on abort; expected drain-back with valve open")
	}
	if st := f.engine.Status(); st.Active == nil || st.Active.State != StateDraining {
		t.Errorf("engine state during abort-drain = %+v, want draining", st.Active)
	}
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 8*time.Second)
	if cycle.AbortReason == nil || !strings.Contains(*cycle.AbortReason, "safety abort") {
		t.Errorf("abort reason = %v", cycle.AbortReason)
	}
	if f.relayState(t, 1) != relay.Off {
		t.Error("valve still open after abort-drain finished")
	}
}

// TestDrainAllowedUnderReservoirLockout: a reservoir lockout blocks watering
// but must NOT block a drain — that is exactly when the tray needs emptying.
func TestDrainAllowedUnderReservoirLockout(t *testing.T) {
	f := newFixture(t)
	z, _ := f.zones.GetByKey("zone_1")
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: 1})
	z.ConfigJSON = string(cfg)
	f.zones.Update(*z)
	f.insertLevel(t, 5) // below the 20 % minimum → reservoir_low lockout
	time.Sleep(150 * time.Millisecond)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 5, TriggerType: "manual_web"}); err == nil {
		t.Fatal("watering should be refused under reservoir lockout")
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DrainOnly: true, TriggerType: "manual_touch"}); err != nil {
		t.Fatalf("drain refused under reservoir lockout: %v", err)
	}
	f.waitCycleResult(t, "zone_1", ResultComplete, 5*time.Second)
	if f.relayState(t, devices.PumpChannel) == relay.On {
		t.Fatal("pump ran during drain")
	}
}

// TestFloodAbortsWhenValveClosedUnderneath (audit 2.4): a manual valve-off
// during a flood would dead-head the pump; the engine must notice within
// about a second and stop the pump.
func TestFloodAbortsWhenValveClosedUnderneath(t *testing.T) {
	f := newFixture(t)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	f.waitState(t, "zone_1", StateFlooding, 3*time.Second)
	f.mgr.SetRelay(context.Background(), 1, relay.Off, "manual_web", "bench script", 0)
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 4*time.Second)
	if cycle.AbortReason == nil || !strings.Contains(*cycle.AbortReason, "during flood") {
		t.Errorf("abort reason = %v", cycle.AbortReason)
	}
	if f.relayState(t, devices.PumpChannel) != relay.Off {
		t.Error("pump left on after valve closed underneath it")
	}
	if !f.hasEvent(t, "irrigation_abort_no_drain") {
		t.Error("expected irrigation_abort_no_drain (valve already closed, nothing to drain)")
	}
}

// TestEstopAbortsWithoutFakeDrain (audit 2.6): e-stop aborts the cycle via
// the supervisor hook and the engine does not sit "draining" for the drain
// window with the valve already forced closed.
func TestEstopAbortsWithoutFakeDrain(t *testing.T) {
	f := newFixture(t)
	f.setDrain(t, "zone_1", 10)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_2", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue zone_2: %v", err)
	}
	f.waitState(t, "zone_1", StateFlooding, 3*time.Second)
	start := time.Now()
	f.sup.EmergencyStop(context.Background(), "manual_touch", "test")
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 4*time.Second)
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("abort took %v; should not wait out the 10 s drain", took)
	}
	if cycle.AbortReason == nil || !strings.Contains(*cycle.AbortReason, "safety_supervisor") {
		t.Errorf("abort reason = %v, want the e-stop's stop-all", cycle.AbortReason)
	}
	if !f.hasEvent(t, "irrigation_abort_no_drain") {
		t.Error("expected irrigation_abort_no_drain event")
	}
	if st := f.engine.Status(); st.Active != nil || len(st.Queue) != 0 {
		t.Errorf("engine not idle after e-stop: %+v", st)
	}
	f.sup.Reset("test")
}

// TestStopDuringDrainRecordsAborted (audit 2.13 F14): a Stop that cuts the
// drain-back short is an abort, not a complete.
func TestStopDuringDrainRecordsAborted(t *testing.T) {
	f := newFixture(t)
	f.setDrain(t, "zone_1", 5)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 1, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	f.waitState(t, "zone_1", StateDraining, 5*time.Second)
	if err := f.engine.Stop("zone_1", "manual_touch"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 3*time.Second)
	if cycle.AbortReason == nil || *cycle.AbortReason != "stopped by manual_touch" {
		t.Errorf("abort reason = %v", cycle.AbortReason)
	}
	if f.relayState(t, 1) != relay.Off {
		t.Error("valve still open after stop")
	}
}

// TestDrainOnlyDurationCapped (audit 2.7): a drain request cannot hold a
// valve open (and block every other shelf) for days.
func TestDrainOnlyDurationCapped(t *testing.T) {
	f := newFixture(t)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DrainOnly: true, DurationSeconds: 999999, TriggerType: "manual_touch"}); err == nil {
		t.Fatal("oversized drain accepted")
	}
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DrainOnly: true, DurationSeconds: -5, TriggerType: "manual_touch"}); err == nil {
		t.Fatal("negative drain accepted")
	}
}

// TestAllOffDuringFloodDoesNotFakeDrain (audit 2.6): the all-off handler
// stops the engine and then forces every relay off; the engine must not sit
// "draining" for the drain window with the valve already closed.
func TestAllOffDuringFloodDoesNotFakeDrain(t *testing.T) {
	f := newFixture(t)
	f.setDrain(t, "zone_1", 10)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	f.waitState(t, "zone_1", StateFlooding, 3*time.Second)
	start := time.Now()
	f.engine.StopAll("manual_web")
	f.mgr.AllOff(context.Background(), "manual_web", "all-off requested")
	cycle := f.waitCycleResult(t, "zone_1", ResultAborted, 4*time.Second)
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("abort took %v; should not wait out the 10 s drain with the valve closed", took)
	}
	if cycle.AbortReason == nil || !strings.Contains(*cycle.AbortReason, "stop all") {
		t.Errorf("abort reason = %v", cycle.AbortReason)
	}
	if st := f.engine.Status(); st.Active != nil {
		t.Errorf("engine still active after all-off: %+v", st.Active)
	}
}

func (f *fixture) setPurge(t *testing.T, zoneKey string, drainSeconds, purgeDelay, purgeSeconds int) {
	t.Helper()
	z, _ := f.zones.GetByKey(zoneKey)
	cfg, _ := json.Marshal(ZoneConfig{DefaultRunSeconds: 60, DrainSeconds: drainSeconds,
		PurgeDelaySeconds: purgeDelay, PurgeSeconds: purgeSeconds})
	z.ConfigJSON = string(cfg)
	if err := f.zones.Update(*z); err != nil {
		t.Fatalf("update zone: %v", err)
	}
}

// TestPostDrainPurgeReopensValve (Austin, 2026-11-11): after a flood's
// drain-back closes the valve, the same valve re-opens once more for the
// purge time with the pump off, as its own "purge" cycle.
func TestPostDrainPurgeReopensValve(t *testing.T) {
	f := newFixture(t)
	f.setPurge(t, "zone_1", 1, 1, 1)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 1, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	flood := f.waitCycleResult(t, "zone_1", ResultComplete, 6*time.Second)
	if !f.hasEvent(t, "irrigation_purge_scheduled") {
		t.Fatal("purge not scheduled after flood drain-back")
	}
	st := f.engine.Status()
	if len(st.PendingPurges) != 1 || st.PendingPurges[0].ZoneKey != "zone_1" {
		t.Fatalf("pending purges = %+v", st.PendingPurges)
	}
	// Wait for the purge cycle: valve opens with the pump off, then closes.
	sawValveOpen := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if f.relayState(t, 1) == relay.On {
			sawValveOpen = true
			if f.relayState(t, devices.PumpChannel) == relay.On {
				t.Fatal("pump ran during purge")
			}
		}
		cycles, _ := f.cycles.List("zone_1", 1)
		if sawValveOpen && len(cycles) > 0 && cycles[0].ID != flood.ID && cycles[0].Result == ResultComplete {
			if cycles[0].TriggerSource == nil || *cycles[0].TriggerSource != "purge" {
				t.Fatalf("purge cycle tagged %v", cycles[0].TriggerSource)
			}
			if f.relayState(t, 1) != relay.Off {
				t.Fatal("valve left open after purge")
			}
			if len(f.engine.Status().PendingPurges) != 0 {
				t.Fatal("purge still listed as pending after it ran")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("purge cycle never ran (valve opened: %v)", sawValveOpen)
}

// TestStopCancelsPendingPurge: Stop on an idle shelf with a purge armed
// cancels the purge instead of erroring.
func TestStopCancelsPendingPurge(t *testing.T) {
	f := newFixture(t)
	f.setPurge(t, "zone_1", 1, 3600, 60)
	if _, err := f.engine.Enqueue(Request{ZoneKey: "zone_1", DurationSeconds: 1, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	f.waitCycleResult(t, "zone_1", ResultComplete, 6*time.Second)
	if len(f.engine.Status().PendingPurges) != 1 {
		t.Fatal("expected one pending purge")
	}
	if err := f.engine.Stop("zone_1", "manual_web"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(f.engine.Status().PendingPurges) != 0 {
		t.Fatal("purge still pending after Stop")
	}
	if !f.hasEvent(t, "irrigation_purge_cancelled") {
		t.Error("expected irrigation_purge_cancelled event")
	}
	if err := f.engine.Stop("zone_1", "manual_web"); err == nil {
		t.Error("second Stop should report nothing to stop")
	}
}

// TestRestartDrainsInterruptedShelf (Austin, 2026-11-11): a cycle left
// "running" by a crash or power loss means a tray that may be full. On start
// the engine closes the record and drains that shelf — valve only, no pump.
func TestRestartDrainsInterruptedShelf(t *testing.T) {
	f := newFixture(t)
	f.setDrain(t, "zone_2", 1)
	z, _ := f.zones.GetByKey("zone_2")
	if _, err := f.cycles.InsertStart(z.ID, 420, "moisture", nil, nil, nil); err != nil {
		t.Fatalf("insert running cycle: %v", err)
	}
	// A second engine on the same store stands in for the restarted daemon.
	restarted := NewEngine(f.mgr, f.sup, f.zones, f.cycles, f.readings,
		logging.NewEventLogger(zerolog.Nop(), f.events), zerolog.Nop(), "reservoir_level_level")
	restarted.Start(f.ctx)

	sawValve := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if f.relayState(t, devices.PumpChannel) == relay.On {
			t.Fatal("pump ran during the recovery drain")
		}
		if f.relayState(t, 2) == relay.On {
			sawValve = true
		}
		cycles, _ := f.cycles.List("zone_2", 2)
		if sawValve && len(cycles) == 2 && cycles[0].Result == ResultComplete {
			if cycles[0].TriggerSource == nil || *cycles[0].TriggerSource != "drain" {
				t.Errorf("recovery cycle tagged %v", cycles[0].TriggerSource)
			}
			if cycles[1].Result != ResultAborted {
				t.Errorf("interrupted cycle result = %q", cycles[1].Result)
			}
			if !f.hasEvent(t, "irrigation_recovery_drain") {
				t.Error("expected irrigation_recovery_drain event")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("recovery drain never completed (valve opened: %v)", sawValve)
}
