package safety

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
	"propagation-controller/internal/storage"
)

type fixture struct {
	sup      *Supervisor
	mgr      *devices.Manager
	readings *storage.ReadingRepo
	devices  *storage.DeviceRepo
	resID    int64
}

func newFixture(t *testing.T, cfg Config) *fixture {
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
	eventLog := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))

	mgr := devices.NewManager(relay.NewMock(), "mock", storage.NewRelayActionRepo(db), deviceRepo, eventLog, true)
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
	return &fixture{sup: sup, mgr: mgr, readings: readings, devices: deviceRepo, resID: dev.ID}
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
