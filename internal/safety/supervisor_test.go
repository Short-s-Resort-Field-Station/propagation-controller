package safety

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

type fixture struct {
	sup      *Supervisor
	mgr      *devices.Manager
	readings *storage.ReadingRepo
	devices  *storage.DeviceRepo
	settings *storage.SettingsRepo
	events   *storage.EventRepo
	eventLog *logging.EventLogger
	resID    int64
}

// flakyController wraps the mock so a test can make the board "vanish":
// while failing is set every read/write errors like a yanked USB cable.
type flakyController struct {
	relay.Controller
	failing atomic.Bool
}

func (f *flakyController) GetAll(ctx context.Context) (map[int]relay.State, error) {
	if f.failing.Load() {
		return nil, errors.New("input/output error")
	}
	return f.Controller.GetAll(ctx)
}

func (f *flakyController) SetChannel(ctx context.Context, ch int, st relay.State) error {
	if f.failing.Load() {
		return errors.New("input/output error")
	}
	return f.Controller.SetChannel(ctx, ch, st)
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	return newFixtureWith(t, cfg, relay.NewMock())
}

func newFixtureWith(t *testing.T, cfg Config, ctrl relay.Controller) *fixture {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	readings := storage.NewReadingRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)
	settings := storage.NewSettingsRepo(db)
	eventRepo := storage.NewEventRepo(db)
	eventLog := logging.NewEventLogger(zerolog.Nop(), eventRepo)

	mgr := devices.NewManager(ctrl, "mock", storage.NewRelayActionRepo(db), deviceRepo, eventLog, true)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}

	if err := deviceRepo.Upsert("reservoir_level", "Reservoir", "reservoir_sensor", "generic_modbus", "{}", "simulated"); err != nil {
		t.Fatalf("upsert reservoir device: %v", err)
	}
	dev, err := deviceRepo.GetByKey("reservoir_level")
	if err != nil || dev == nil {
		t.Fatalf("get reservoir device: %v", err)
	}

	body, _ := json.Marshal(cfg)
	if err := settings.Set("safety", string(body)); err != nil {
		t.Fatalf("seed safety config: %v", err)
	}

	sup := NewSupervisor(mgr, readings, settings, eventLog, zerolog.Nop(), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("supervisor start: %v", err)
	}
	return &fixture{sup: sup, mgr: mgr, readings: readings, devices: deviceRepo,
		settings: settings, events: eventRepo, eventLog: eventLog, resID: dev.ID}
}

// restart builds a fresh supervisor over the same DB, as a daemon restart
// would, and returns it started.
func (f *fixture) restart(t *testing.T) *Supervisor {
	t.Helper()
	sup := NewSupervisor(f.mgr, f.readings, f.settings, f.eventLog, zerolog.Nop(), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("supervisor restart: %v", err)
	}
	return sup
}

func (f *fixture) countEvents(t *testing.T, eventType string) int {
	t.Helper()
	list, err := f.events.List(storage.EventFilter{Limit: 500})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	n := 0
	for _, e := range list {
		if e.EventType == eventType {
			n++
		}
	}
	return n
}

func (f *fixture) waitEvent(t *testing.T, eventType string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f.countEvents(t, eventType) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event %q never logged", eventType)
}

func testConfig() Config {
	return Config{
		MaxPumpRuntimeSeconds:   1,
		MinReservoirLevel:       20,
		ReservoirSensorKey:      "reservoir_level_level",
		ReservoirOfflineLockout: false, // most tests control the reservoir explicitly
		SensorStaleAfterSeconds: 3600,
		EstopIncludesLights:     false,
	}
}

func (f *fixture) insertLevel(t *testing.T, value float64, at time.Time) {
	t.Helper()
	err := f.readings.InsertBatch(f.resID, []storage.ReadingInsert{
		{SensorKey: "reservoir_level_level", Value: value, Unit: "%", Quality: "good", ReadAt: at},
	})
	if err != nil {
		t.Fatalf("insert reading: %v", err)
	}
}

func (f *fixture) waitPumpState(t *testing.T, want relay.State, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := f.mgr.GetRelay(context.Background(), devices.PumpChannel)
		if err == nil && info.State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pump never reached state %q", want)
}

func (f *fixture) waitLockout(t *testing.T, key string, present bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := f.sup.Status(context.Background())
		found := false
		for _, lo := range st.Lockouts {
			if lo.Key == key {
				found = true
			}
		}
		if found == present {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("lockout %q present=%v never observed", key, present)
}

func TestEmergencyStopForcesOffAndLatches(t *testing.T) {
	f := newFixture(t, testConfig())
	ctx := context.Background()
	f.insertLevel(t, 80, time.Now())

	f.mgr.SetRelay(ctx, devices.LightsChannel, relay.On, "manual_web", "test", 0)
	f.mgr.SetRelay(ctx, 3, relay.On, "manual_web", "test", 0)

	f.sup.EmergencyStop(ctx, "manual_touch", "test estop")

	// Valves off; lights stay (estop_includes_lights=false).
	info, _ := f.mgr.GetRelay(ctx, 3)
	if info.State != relay.Off {
		t.Errorf("valve still on after estop")
	}
	lights, _ := f.mgr.GetRelay(ctx, devices.LightsChannel)
	if lights.State != relay.On {
		t.Errorf("lights should be untouched by estop with estop_includes_lights=false")
	}

	if err := f.sup.Authorize(3, relay.On); err == nil {
		t.Error("relay-on should be denied during estop")
	}
	if err := f.sup.Authorize(3, relay.Off); err != nil {
		t.Errorf("relay-off must always be allowed: %v", err)
	}
	if f.sup.HealthStatus() != "emergency_stop" {
		t.Errorf("health = %q", f.sup.HealthStatus())
	}

	// Latch enforcement: turn a valve on behind the supervisor's back and
	// verify the loop forces it back off.
	f.mgr.SetRelay(ctx, 4, relay.On, "manual_web", "sneaky", 0)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if info, _ := f.mgr.GetRelay(ctx, 4); info.State == relay.Off {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if info, _ := f.mgr.GetRelay(ctx, 4); info.State != relay.Off {
		t.Error("estop loop did not re-force valve off")
	}

	if err := f.sup.Reset("manual_web"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := f.sup.Authorize(3, relay.On); err != nil {
		t.Errorf("relay-on should be allowed after reset: %v", err)
	}
	if err := f.sup.Reset("manual_web"); err == nil {
		t.Error("second reset should fail when estop not active")
	}
}

func TestPumpMaxRuntimeEnforced(t *testing.T) {
	f := newFixture(t, testConfig())
	ctx := context.Background()
	f.insertLevel(t, 80, time.Now())

	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.On, "manual_web", "runtime test", 0)
	f.waitPumpState(t, relay.Off, 4*time.Second)
	f.waitLockout(t, LockoutPumpMaxRuntime, true, time.Second)

	if err := f.sup.Authorize(devices.PumpChannel, relay.On); err == nil {
		t.Error("pump-on should be denied while locked out")
	}
	// Manual-clear lockout: cannot auto-clear, must be cleared explicitly.
	if err := f.sup.ClearLockout(LockoutPumpMaxRuntime, "manual_web"); err != nil {
		t.Fatalf("clear lockout: %v", err)
	}
	if err := f.sup.Authorize(devices.PumpChannel, relay.On); err != nil {
		t.Errorf("pump-on should be allowed after clear: %v", err)
	}
}

func TestReservoirLowLockoutAndRecovery(t *testing.T) {
	f := newFixture(t, testConfig())
	ctx := context.Background()

	f.insertLevel(t, 10, time.Now()) // below min 20
	f.waitLockout(t, LockoutReservoirLow, true, 2*time.Second)
	if err := f.sup.Authorize(devices.PumpChannel, relay.On); err == nil {
		t.Error("pump-on should be denied while reservoir low")
	}
	// Auto lockout refuses manual clearing.
	if err := f.sup.ClearLockout(LockoutReservoirLow, "manual_web"); err == nil {
		t.Error("auto lockout should not be manually clearable")
	}
	// Pump running during lockout gets forced off.
	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.On, "manual_web", "bypass attempt", 0)
	f.waitPumpState(t, relay.Off, 2*time.Second)

	f.insertLevel(t, 50, time.Now()) // recovered
	f.waitLockout(t, LockoutReservoirLow, false, 2*time.Second)
	if err := f.sup.Authorize(devices.PumpChannel, relay.On); err != nil {
		t.Errorf("pump-on should be allowed after recovery: %v", err)
	}
}

func TestReservoirSensorOfflineLockout(t *testing.T) {
	cfg := testConfig()
	cfg.ReservoirOfflineLockout = true
	cfg.SensorStaleAfterSeconds = 1
	f := newFixture(t, cfg)

	// A stale reading (2 hours old) must trigger the offline lockout.
	f.insertLevel(t, 80, time.Now().Add(-2*time.Hour))
	f.waitLockout(t, LockoutReservoirOffline, true, 2*time.Second)

	// A fresh reading clears it.
	f.insertLevel(t, 80, time.Now())
	f.waitLockout(t, LockoutReservoirOffline, false, 2*time.Second)
}

// TestEstopAndManualLockoutSurviveRestart: the latch and the pump-max-runtime
// lockout come back after a daemon restart and the safe state is re-forced,
// until a human resets/clears them (audit 2026-11-10, 2.3).
func TestEstopAndManualLockoutSurviveRestart(t *testing.T) {
	f := newFixture(t, testConfig())
	ctx := context.Background()
	f.insertLevel(t, 80, time.Now())

	// Trip the manual lockout, then latch the e-stop.
	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.On, "manual_web", "runtime test", 0)
	f.waitLockout(t, LockoutPumpMaxRuntime, true, 4*time.Second)
	f.sup.EmergencyStop(ctx, "manual_touch", "bench")

	// Simulate the relays holding state through the restart: a valve on.
	f.mgr.SetRelay(ctx, 2, relay.On, "manual_web", "held by board", 0)

	sup2 := f.restart(t)
	st := sup2.Status(ctx)
	if !st.EmergencyStop.Active || st.EmergencyStop.Reason != "bench" {
		t.Fatalf("e-stop not restored: %+v", st.EmergencyStop)
	}
	if len(st.Lockouts) != 1 || st.Lockouts[0].Key != LockoutPumpMaxRuntime || st.Lockouts[0].Auto {
		t.Fatalf("manual lockout not restored: %+v", st.Lockouts)
	}
	if info, _ := f.mgr.GetRelay(ctx, 2); info.State != relay.Off {
		t.Error("safe state not re-forced on load")
	}
	if f.countEvents(t, "emergency_stop_restored") != 1 {
		t.Error("emergency_stop_restored not logged")
	}

	// Reset + clear persist too: a third start sees a clean slate.
	if err := sup2.Reset("manual_web"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := sup2.ClearLockout(LockoutPumpMaxRuntime, "manual_web"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	sup3 := f.restart(t)
	st = sup3.Status(ctx)
	if st.EmergencyStop.Active || len(st.Lockouts) != 0 {
		t.Fatalf("cleared state came back: %+v", st)
	}
}

// TestRelayUnreachableLoggedOnceAndPumpForcedOffOnRecovery (audit 2.2c):
// an outage logs one event, and a pump found on when the board returns is
// forced off unless the engine says a cycle owns it.
func TestRelayUnreachableLoggedOnceAndPumpForcedOffOnRecovery(t *testing.T) {
	ctrl := &flakyController{Controller: relay.NewMock()}
	f := newFixtureWith(t, testConfig(), ctrl)
	ctx := context.Background()
	f.insertLevel(t, 80, time.Now())

	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.On, "manual_web", "orphan", 0)
	ctrl.failing.Store(true)
	f.waitEvent(t, "relay_controller_unreachable", 2*time.Second)
	time.Sleep(100 * time.Millisecond) // several failing ticks
	if n := f.countEvents(t, "relay_controller_unreachable"); n != 1 {
		t.Fatalf("unreachable logged %d times, want once per outage", n)
	}
	ctrl.failing.Store(false)
	f.waitPumpState(t, relay.Off, 2*time.Second)

	// Same outage with the engine claiming the pump: left alone.
	f.sup.SetPumpExpected(true)
	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.On, "system", "irrigation cycle", 0)
	ctrl.failing.Store(true)
	time.Sleep(100 * time.Millisecond)
	ctrl.failing.Store(false)
	time.Sleep(100 * time.Millisecond)
	if info, _ := f.mgr.GetRelay(ctx, devices.PumpChannel); info.State != relay.On {
		t.Error("pump owned by a running cycle was forced off on recovery")
	}
	if n := f.countEvents(t, "relay_controller_unreachable"); n != 2 {
		t.Errorf("second outage should log a second event, got %d", n)
	}
	f.sup.SetPumpExpected(false)
	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.Off, "system", "done", 0)
}

// TestSetPumpExpectedFalseClosesRuntimeWindow (audit 2.13 F9): the engine's
// pump-stop report resets the runtime counter as soon as the relay reads off.
func TestSetPumpExpectedFalseClosesRuntimeWindow(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPumpRuntimeSeconds = 60
	f := newFixture(t, cfg)
	ctx := context.Background()
	f.insertLevel(t, 80, time.Now())

	f.sup.SetPumpExpected(true)
	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.On, "system", "cycle", 0)
	time.Sleep(60 * time.Millisecond)
	if !f.sup.Status(ctx).Pump.Running {
		t.Fatal("supervisor did not see the pump running")
	}
	f.mgr.SetRelay(ctx, devices.PumpChannel, relay.Off, "system", "cycle ending", 0)
	f.sup.SetPumpExpected(false)
	if st := f.sup.Status(ctx); st.Pump.Running {
		t.Error("runtime window still open after the engine reported pump stop")
	}
}
