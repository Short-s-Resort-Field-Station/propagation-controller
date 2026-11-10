package automation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/irrigation"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
)

func TestScheduleFires(t *testing.T) {
	chicago := "America/Chicago"
	loc, _ := time.LoadLocation(chicago)
	at := func(hhmm string) time.Time {
		tm, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-07-08 "+hhmm, loc) // Wednesday
		return tm
	}
	rule := ScheduleRule{ZoneKey: "zone_1", StartTime: "06:30", DurationSeconds: 60}
	weekdayRule := ScheduleRule{ZoneKey: "zone_1", StartTime: "06:30", DurationSeconds: 60,
		Days: []string{"mon", "fri"}}

	cases := []struct {
		name      string
		rule      ScheduleRule
		last, now time.Time
		want      bool
	}{
		{"boundary inside window", rule, at("06:29:50"), at("06:30:10"), true},
		{"window before start", rule, at("06:28:00"), at("06:29:50"), false},
		{"window after start", rule, at("06:30:10"), at("06:31:10"), false},
		{"long gap covers start (catch within sweep)", rule, at("06:00:00"), at("07:00:00"), true},
		{"day filter excludes wednesday", weekdayRule, at("06:29:50"), at("06:30:10"), false},
	}
	for _, c := range cases {
		if got := fires(c.rule, chicago, c.last, c.now); got != c.want {
			t.Errorf("%s: fires = %v, want %v", c.name, got, c.want)
		}
	}
}

type fixture struct {
	svc      *Service
	engine   *irrigation.Engine
	cycles   *storage.CycleRepo
	zones    *storage.ZoneRepo
	rules    *storage.MoistureRuleRepo
	readings *storage.ReadingRepo
	events   *storage.EventRepo
	resID    int64
	now      time.Time
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
	eventRepo := storage.NewEventRepo(db)
	eventLog := logging.NewEventLogger(zerolog.Nop(), eventRepo)
	deviceRepo := storage.NewDeviceRepo(db)
	readings := storage.NewReadingRepo(db)
	settings := storage.NewSettingsRepo(db)

	mgr := devices.NewManager(relay.NewMock(), "mock", storage.NewRelayActionRepo(db), deviceRepo, eventLog, true)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	deviceRepo.Upsert("reservoir_level", "Reservoir", "reservoir_sensor", "generic_modbus", "{}", "simulated")
	dev, _ := deviceRepo.GetByKey("reservoir_level")

	cfg := safety.DefaultConfig()
	cfg.SensorStaleAfterSeconds = 24 * 3600
	body, _ := json.Marshal(cfg)
	settings.Set("safety", string(body))
	sup := safety.NewSupervisor(mgr, readings, settings, eventLog, zerolog.Nop(), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup.Start(ctx)

	zones := storage.NewZoneRepo(db)
	cycles := storage.NewCycleRepo(db)
	engine := irrigation.NewEngine(mgr, sup, zones, cycles, readings, eventLog, zerolog.Nop(), "reservoir_level_level")
	if err := engine.SeedDefaults(); err != nil {
		t.Fatalf("seed zones: %v", err)
	}
	// Zero the ebb-and-flow default drain so automation cycles complete within
	// the test window (the drain phase is covered in the irrigation package).
	for _, zk := range []string{"zone_1", "zone_2", "zone_3"} {
		z, err := zones.GetByKey(zk)
		if err != nil || z == nil {
			continue
		}
		var zc irrigation.ZoneConfig
		_ = json.Unmarshal([]byte(z.ConfigJSON), &zc)
		zc.DrainSeconds = 0
		b, _ := json.Marshal(zc)
		z.ConfigJSON = string(b)
		if err := zones.Update(*z); err != nil {
			t.Fatalf("reset zone drain: %v", err)
		}
	}
	engine.Start(ctx)

	f := &fixture{
		engine: engine, cycles: cycles, zones: zones,
		rules: storage.NewMoistureRuleRepo(db), readings: readings, events: eventRepo,
		resID: dev.ID, now: time.Now(),
	}
	// Automation is off until switched on explicitly (fail-safe, audit 2.9).
	if err := settings.Set("automation", `{"enabled":true}`); err != nil {
		t.Fatalf("seed automation setting: %v", err)
	}
	f.svc = NewService(engine, zones, cycles, f.rules, readings, storage.NewScheduleRepo(db),
		settings, eventLog, zerolog.Nop(), time.Hour /* loop not started; Tick() driven */)
	f.svc.SetNowFunc(func() time.Time { return f.now })
	f.svc.mu.Lock()
	f.svc.lastCheck = f.now
	f.svc.mu.Unlock()

	// Healthy reservoir so safety never interferes.
	f.insertReading(t, "reservoir_level_level", 80, f.now)
	return f
}

func (f *fixture) insertReading(t *testing.T, key string, value float64, at time.Time) {
	t.Helper()
	err := f.readings.InsertBatch(f.resID, []storage.ReadingInsert{
		{SensorKey: key, Value: value, Unit: "%", Quality: "good", ReadAt: at},
	})
	if err != nil {
		t.Fatalf("insert reading: %v", err)
	}
}

func (f *fixture) seedRule(t *testing.T, mutate func(*storage.MoistureRule)) {
	t.Helper()
	zone, _ := f.zones.GetByKey("zone_1")
	rule := storage.MoistureRule{
		ZoneID: zone.ID, Enabled: true, SensorKey: "soil_zone_1_moisture",
		TriggerBelow: 28, RunDurationSeconds: 1, MinimumIntervalMinutes: 0,
		SensorOfflineBehavior: "lockout",
	}
	if mutate != nil {
		mutate(&rule)
	}
	if err := f.rules.Upsert(rule); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
}

func (f *fixture) waitCycles(t *testing.T, zoneKey string, wantCount int, timeout time.Duration) []storage.Cycle {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cycles, _ := f.cycles.List(zoneKey, 50)
		done := 0
		for _, c := range cycles {
			if c.Result == irrigation.ResultComplete {
				done++
			}
		}
		if done >= wantCount {
			return cycles
		}
		time.Sleep(25 * time.Millisecond)
	}
	cycles, _ := f.cycles.List(zoneKey, 50)
	t.Fatalf("zone %s: wanted %d complete cycles, have %+v", zoneKey, wantCount, len(cycles))
	return nil
}

func TestMoistureTriggerEnqueuesCycle(t *testing.T) {
	f := newFixture(t)
	f.seedRule(t, nil)
	f.insertReading(t, "soil_zone_1_moisture", 22, f.now) // below 28

	f.svc.Tick()
	cycles := f.waitCycles(t, "zone_1", 1, 5*time.Second)
	if cycles[0].TriggerType != "moisture_threshold" {
		t.Errorf("trigger_type = %q", cycles[0].TriggerType)
	}
	if cycles[0].TriggerSource == nil || *cycles[0].TriggerSource != "soil_zone_1_moisture" {
		t.Errorf("trigger_source = %v", cycles[0].TriggerSource)
	}
}

// Maintenance mode must suspend moisture-triggered watering (pulling probes /
// moving trays shouldn't fire pumps) and resume cleanly when switched off.
func TestMaintenanceModeSuspendsAutomation(t *testing.T) {
	f := newFixture(t)
	f.seedRule(t, nil)
	f.insertReading(t, "soil_zone_1_moisture", 22, f.now) // below trigger (28)

	if err := f.svc.settings.Set("maintenance", `{"active":true}`); err != nil {
		t.Fatalf("set maintenance: %v", err)
	}
	f.svc.Tick()
	time.Sleep(300 * time.Millisecond)
	if cycles, _ := f.cycles.List("zone_1", 50); len(cycles) != 0 {
		t.Fatalf("maintenance mode watered anyway: %d cycles", len(cycles))
	}

	// Exit maintenance: the same below-threshold reading now waters.
	if err := f.svc.settings.Set("maintenance", `{"active":false}`); err != nil {
		t.Fatalf("clear maintenance: %v", err)
	}
	f.svc.Tick()
	f.waitCycles(t, "zone_1", 1, 5*time.Second)
}

// A disabled shelf (assignment cleared, or manually disabled) must never be
// watered by automation, and must water again once re-enabled (re-assigned).
func TestDisabledZoneNotWateredByAutomation(t *testing.T) {
	f := newFixture(t)
	f.seedRule(t, nil)
	f.insertReading(t, "soil_zone_1_moisture", 22, f.now) // below trigger (28)

	zone, _ := f.zones.GetByKey("zone_1")
	zone.Enabled = false
	if err := f.zones.Update(*zone); err != nil {
		t.Fatalf("disable zone: %v", err)
	}
	f.svc.Tick()
	time.Sleep(300 * time.Millisecond)
	if cycles, _ := f.cycles.List("zone_1", 50); len(cycles) != 0 {
		t.Fatalf("disabled shelf watered anyway: %d cycles", len(cycles))
	}

	zone.Enabled = true
	if err := f.zones.Update(*zone); err != nil {
		t.Fatalf("re-enable zone: %v", err)
	}
	f.svc.Tick()
	f.waitCycles(t, "zone_1", 1, 5*time.Second)
}

func TestHysteresisPreventsRetrigger(t *testing.T) {
	f := newFixture(t)
	recovered := 34.0
	f.seedRule(t, func(r *storage.MoistureRule) { r.RecoveredAbove = &recovered })

	f.insertReading(t, "soil_zone_1_moisture", 22, f.now)
	f.svc.Tick()
	f.waitCycles(t, "zone_1", 1, 5*time.Second)

	// Still below recovery threshold (30 < 34): must NOT retrigger even
	// though it's below trigger... wait, 30 > 28 trigger. Use 25: below both.
	f.insertReading(t, "soil_zone_1_moisture", 25, f.now.Add(time.Second))
	f.svc.Tick()
	f.svc.Tick()
	time.Sleep(300 * time.Millisecond)
	cycles, _ := f.cycles.List("zone_1", 50)
	if len(cycles) != 1 {
		t.Fatalf("retriggered while awaiting recovery: %d cycles", len(cycles))
	}

	// Recover above 34, then drop again → eligible again.
	f.insertReading(t, "soil_zone_1_moisture", 40, f.now.Add(2*time.Second))
	f.svc.Tick() // clears awaiting-recovery
	f.insertReading(t, "soil_zone_1_moisture", 20, f.now.Add(3*time.Second))
	f.svc.Tick()
	f.waitCycles(t, "zone_1", 2, 5*time.Second)
}

func TestMinimumIntervalBlocks(t *testing.T) {
	f := newFixture(t)
	f.seedRule(t, func(r *storage.MoistureRule) {
		r.MinimumIntervalMinutes = 240
		r.RecoveredAbove = nil
	})
	f.insertReading(t, "soil_zone_1_moisture", 22, f.now)
	f.svc.Tick()
	f.waitCycles(t, "zone_1", 1, 5*time.Second)

	// No hysteresis configured: recovery point equals the trigger, and the
	// reading is still below it, so the zone stays awaiting-recovery... to
	// exercise min-interval specifically, simulate recovery then re-drop.
	f.insertReading(t, "soil_zone_1_moisture", 30, f.now.Add(time.Second))
	f.svc.Tick()
	f.insertReading(t, "soil_zone_1_moisture", 22, f.now.Add(2*time.Second))
	f.svc.Tick()
	time.Sleep(300 * time.Millisecond)
	cycles, _ := f.cycles.List("zone_1", 50)
	if len(cycles) != 1 {
		t.Fatalf("min interval not enforced: %d cycles", len(cycles))
	}
	// Jump the clock past the interval → allowed.
	f.now = f.now.Add(5 * time.Hour)
	f.insertReading(t, "reservoir_level_level", 80, f.now)
	f.insertReading(t, "soil_zone_1_moisture", 22, f.now)
	f.svc.Tick()
	f.waitCycles(t, "zone_1", 2, 5*time.Second)
}

func TestSensorOfflineLockout(t *testing.T) {
	f := newFixture(t)
	f.seedRule(t, nil)
	// Reading is 2 hours old → stale → lockout behavior blocks, one event.
	f.insertReading(t, "soil_zone_1_moisture", 10, f.now.Add(-2*time.Hour))
	f.svc.Tick()
	f.svc.Tick()
	time.Sleep(200 * time.Millisecond)
	cycles, _ := f.cycles.List("zone_1", 50)
	if len(cycles) != 0 {
		t.Fatalf("stale sensor should not trigger irrigation")
	}
	evs, _ := f.events.List(storage.EventFilter{EventType: "moisture_trigger_blocked"})
	if len(evs) != 1 {
		t.Errorf("expected exactly 1 blocked event (deduped), got %d", len(evs))
	}
}

func TestScheduledIrrigationEndToEnd(t *testing.T) {
	f := newFixture(t)
	rule, _ := json.Marshal(ScheduleRule{ZoneKey: "zone_2", StartTime: "06:30", DurationSeconds: 1})
	loc, _ := time.LoadLocation("America/Chicago")
	if err := f.svc.schedules.Upsert("sched_zone2", "Zone 2 morning", "irrigation",
		true, "America/Chicago", string(rule), "skip"); err != nil {
		t.Fatalf("upsert schedule: %v", err)
	}
	// Advance the injected clock across 06:30 Chicago.
	base, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-07-08 06:29:50", loc)
	f.svc.mu.Lock()
	f.svc.lastCheck = base
	f.svc.mu.Unlock()
	f.now = base.Add(20 * time.Second) // 06:30:10
	f.insertReading(t, "reservoir_level_level", 80, time.Now())
	f.svc.Tick()

	f.waitCycles(t, "zone_2", 1, 5*time.Second)
	cycles, _ := f.cycles.List("zone_2", 5)
	if cycles[0].TriggerType != "schedule" {
		t.Errorf("trigger_type = %q, want schedule", cycles[0].TriggerType)
	}
}

// TestEnabledFailsSafe (audit 2.9): only an explicit {"enabled": true} turns
// automation on; missing or malformed values read as off.
func TestEnabledFailsSafe(t *testing.T) {
	f := newFixture(t)
	settings := f.svc.settings
	cases := []struct {
		raw  string
		want bool
	}{
		{`{"enabled":true}`, true},
		{`{"enabled":false}`, false},
		{`{"enabled":"true"}`, false},
		{`{}`, false},
		{`garbage`, false},
	}
	for _, c := range cases {
		if err := settings.Set("automation", c.raw); err != nil {
			t.Fatalf("set: %v", err)
		}
		if got := f.svc.Enabled(); got != c.want {
			t.Errorf("Enabled() with %s = %v, want %v", c.raw, got, c.want)
		}
	}
}

// TestLargeClockGapSkipsSchedules (audit 2.10): a jump bigger than two ticks
// (NTP sync after a cold boot) must not fire every schedule it crossed.
func TestLargeClockGapSkipsSchedules(t *testing.T) {
	f := newFixture(t)
	rule, _ := json.Marshal(ScheduleRule{ZoneKey: "zone_2", StartTime: "06:30", DurationSeconds: 1})
	loc, _ := time.LoadLocation("America/Chicago")
	if err := f.svc.schedules.Upsert("sched_zone2", "Zone 2 morning", "irrigation",
		true, "America/Chicago", string(rule), "skip"); err != nil {
		t.Fatalf("upsert schedule: %v", err)
	}
	// Fixture interval is 1 h, so the cap is 2 h: a 5 h gap over 06:30 is a
	// missed run.
	base, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-07-08 04:00:00", loc)
	f.svc.mu.Lock()
	f.svc.lastCheck = base
	f.svc.mu.Unlock()
	f.now = base.Add(5 * time.Hour) // 09:00
	f.insertReading(t, "reservoir_level_level", 80, time.Now())
	f.svc.Tick()

	time.Sleep(200 * time.Millisecond)
	if cycles, _ := f.cycles.List("zone_2", 5); len(cycles) != 0 {
		t.Fatalf("schedule fired across a clock jump: %+v", cycles)
	}
	evs, _ := f.events.List(storage.EventFilter{EventType: "schedule_catchup_skipped", Limit: 5})
	if len(evs) != 1 {
		t.Errorf("want one schedule_catchup_skipped event, got %d", len(evs))
	}
	// And the window after the cap still works: 06:29:50 → 06:30:10.
	f.svc.mu.Lock()
	f.svc.lastCheck = base.Add(2*time.Hour + 29*time.Minute + 50*time.Second)
	f.svc.mu.Unlock()
	f.now = base.Add(2*time.Hour + 30*time.Minute + 10*time.Second)
	f.svc.Tick()
	f.waitCycles(t, "zone_2", 1, 5*time.Second)
}
