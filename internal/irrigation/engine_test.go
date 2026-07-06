package irrigation

import (
	"context"
	"encoding/json"
	"path/filepath"
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
	eventLog := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))

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
	engine.Start(ctx)

	f := &fixture{engine: engine, mgr: mgr, sup: sup, zones: zones, cycles: cycles,
		readings: readings, resID: dev.ID, ctx: ctx}
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

	// Watch valve channels 3 and 4 continuously until both cycles finish.
	done := make(chan struct{})
	violations := 0
	go func() {
		defer close(done)
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			s3 := f.relayState(t, 3)
			s4 := f.relayState(t, 4)
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
