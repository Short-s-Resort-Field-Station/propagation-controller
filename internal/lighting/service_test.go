package lighting

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

func TestEvaluate(t *testing.T) {
	chicago, _ := time.LoadLocation("America/Chicago")
	at := func(hhmm string) time.Time {
		tm, _ := time.ParseInLocation("2006-01-02 15:04", "2026-07-08 "+hhmm, chicago) // a Wednesday
		return tm
	}
	daytime := Schedule{Enabled: true, Timezone: "America/Chicago", Rule: Rule{OnTime: "06:00", OffTime: "19:30"}}
	overnight := Schedule{Enabled: true, Timezone: "America/Chicago", Rule: Rule{OnTime: "20:00", OffTime: "06:00"}}
	weekdays := Schedule{Enabled: true, Timezone: "America/Chicago",
		Rule: Rule{OnTime: "06:00", OffTime: "19:30", Days: []string{"mon", "tue", "thu"}}}

	cases := []struct {
		name string
		sch  Schedule
		at   time.Time
		want bool
	}{
		{"daytime mid-window", daytime, at("12:00"), true},
		{"daytime before on", daytime, at("05:59"), false},
		{"daytime exactly on", daytime, at("06:00"), true},
		{"daytime exactly off", daytime, at("19:30"), false},
		{"overnight late evening", overnight, at("22:00"), true},
		{"overnight early morning", overnight, at("03:00"), true},
		{"overnight midday", overnight, at("12:00"), false},
		{"day filter excludes wednesday", weekdays, at("12:00"), false},
	}
	for _, c := range cases {
		if got := evaluate(c.sch, c.at); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScheduleValidate(t *testing.T) {
	good := Schedule{Enabled: true, Timezone: "America/Chicago", Rule: Rule{OnTime: "06:00", OffTime: "19:30"}}
	if err := good.Validate(); err != nil {
		t.Errorf("valid schedule rejected: %v", err)
	}
	bad := []Schedule{
		{Timezone: "America/Chicago", Rule: Rule{OnTime: "6am", OffTime: "19:30"}},
		{Timezone: "Mars/Olympus", Rule: Rule{OnTime: "06:00", OffTime: "19:30"}},
		{Timezone: "America/Chicago", Rule: Rule{OnTime: "06:00", OffTime: "19:30", Days: []string{"funday"}}},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("bad schedule %d accepted", i)
		}
	}
}

func newTestService(t *testing.T) (*Service, *devices.Manager, context.Context) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	eventLog := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))
	mgr := devices.NewManager(relay.NewMock(), "mock", storage.NewRelayActionRepo(db),
		storage.NewDeviceRepo(db), eventLog, true)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	sup := safety.NewSupervisor(mgr, storage.NewReadingRepo(db), storage.NewSettingsRepo(db),
		eventLog, zerolog.Nop(), time.Hour) // effectively inert for these tests
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup.Start(ctx)

	svc := NewService(mgr, sup, storage.NewScheduleRepo(db), storage.NewSettingsRepo(db),
		eventLog, zerolog.Nop(), 20*time.Millisecond)
	return svc, mgr, ctx
}

func waitLights(t *testing.T, mgr *devices.Manager, want relay.State, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := mgr.GetRelay(context.Background(), devices.LightsChannel)
		if err == nil && info.State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("lights never reached %q", want)
}

func TestRestoreScheduledStateAtStartupAndOverride(t *testing.T) {
	svc, mgr, ctx := newTestService(t)

	// Schedule that is ON right now (24h window minus one minute around now).
	now := time.Now()
	on := now.Add(-1 * time.Hour).Format("15:04")
	off := now.Add(1 * time.Hour).Format("15:04")
	if err := svc.PutSchedule(Schedule{Enabled: true, Timezone: "Local",
		Rule: Rule{OnTime: on, OffTime: off}}, "test"); err != nil {
		t.Fatalf("put schedule: %v", err)
	}

	svc.Start(ctx)
	// Startup restore: lights come on because the schedule says so.
	waitLights(t, mgr, relay.On, 3*time.Second)
	if st := svc.Status(ctx); st.State != "scheduled_on" {
		t.Errorf("state = %q, want scheduled_on", st.State)
	}

	// Manual off override wins over the schedule.
	if err := svc.SetOverride(ctx, "off", "manual_touch", 0); err != nil {
		t.Fatalf("override: %v", err)
	}
	waitLights(t, mgr, relay.Off, 3*time.Second)
	if st := svc.Status(ctx); st.State != "manual_off" {
		t.Errorf("state = %q, want manual_off", st.State)
	}
	// Loop must not fight the override back to scheduled state.
	time.Sleep(100 * time.Millisecond)
	info, _ := mgr.GetRelay(ctx, devices.LightsChannel)
	if info.State != relay.Off {
		t.Error("override was overridden by the schedule loop")
	}

	// Clearing returns to the scheduled state immediately.
	if err := svc.SetOverride(ctx, "auto", "manual_touch", 0); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	waitLights(t, mgr, relay.On, 3*time.Second)
}

// TestOverrideSurvivesRestart (audit 2.11): with the schedule disabled and
// the lights on by hand, a new service over the same DB re-applies the
// override at Start; an override that expired while down is dropped.
func TestOverrideSurvivesRestart(t *testing.T) {
	svc, mgr, ctx := newTestService(t)
	if err := svc.PutSchedule(Schedule{Enabled: false, Timezone: "Local",
		Rule: Rule{OnTime: "06:00", OffTime: "19:30"}}, "test"); err != nil {
		t.Fatalf("put schedule: %v", err)
	}
	svc.Start(ctx)
	if err := svc.SetOverride(ctx, "on", "manual_touch", 0); err != nil {
		t.Fatalf("override: %v", err)
	}
	waitLights(t, mgr, relay.On, 3*time.Second)

	// "Power loss": the board comes up all-off and a fresh service starts.
	mgr.SetRelay(ctx, devices.LightsChannel, relay.Off, "system", "power loss", 0)
	svc2 := NewService(mgr, svc.safety, svc.schedules, svc.settings, svc.events, zerolog.Nop(), 20*time.Millisecond)
	svc2.Start(ctx)
	waitLights(t, mgr, relay.On, 3*time.Second)
	if st := svc2.Status(ctx); st.State != "manual_on" || st.Override == nil {
		t.Errorf("state after restart = %+v, want manual_on with override", st)
	}

	// Clearing persists too.
	if err := svc2.SetOverride(ctx, "auto", "manual_touch", 0); err != nil {
		t.Fatalf("clear: %v", err)
	}
	svc3 := NewService(mgr, svc.safety, svc.schedules, svc.settings, svc.events, zerolog.Nop(), 20*time.Millisecond)
	svc3.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	if st := svc3.Status(ctx); st.Override != nil {
		t.Errorf("cleared override came back: %+v", st.Override)
	}

	// An override that expired during downtime is dropped on load.
	past := time.Now().Add(-time.Minute)
	body, _ := json.Marshal(Override{Mode: "on", Until: &past, Actor: "manual_web", Since: past.Add(-time.Hour)})
	svc.settings.Set(overrideKey, string(body))
	mgr.SetRelay(ctx, devices.LightsChannel, relay.Off, "system", "power loss", 0)
	svc4 := NewService(mgr, svc.safety, svc.schedules, svc.settings, svc.events, zerolog.Nop(), 20*time.Millisecond)
	svc4.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	if st := svc4.Status(ctx); st.Override != nil {
		t.Errorf("expired override restored: %+v", st.Override)
	}
	if info, _ := mgr.GetRelay(ctx, devices.LightsChannel); info.State != relay.Off {
		t.Error("expired override switched the lights on")
	}
}

// The fan instance must drive the fan relay and keep its own schedule and
// override rows, never touching the lights' (Austin 2026-11-10: fans on a
// schedule like the lights).
func TestFanServiceUsesOwnRelayAndRows(t *testing.T) {
	svc, _, ctx := newTestService(t)
	fans := NewFanService(svc.mgr, svc.safety, svc.schedules, svc.settings, svc.events, zerolog.Nop(), 20*time.Millisecond)
	if err := fans.EnsureDefault("America/Chicago"); err != nil {
		t.Fatalf("seed fan schedule: %v", err)
	}
	row, _ := svc.schedules.GetByKey(FanScheduleKey)
	if row == nil || row.Type != "fans" {
		t.Fatalf("fan schedule row missing or wrong type: %+v", row)
	}
	if err := fans.SetOverride(ctx, "on", "api", 0); err != nil {
		t.Fatalf("fan override: %v", err)
	}
	if info, _ := svc.mgr.GetRelay(ctx, devices.FansChannel); info.State != relay.On {
		t.Fatalf("fan relay = %s, want on", info.State)
	}
	if info, _ := svc.mgr.GetRelay(ctx, devices.LightsChannel); info.State == relay.On {
		t.Fatalf("lights relay turned on by the fan service")
	}
	if _, ok, _ := svc.settings.Get(FanOverrideKey); !ok {
		t.Fatalf("fan override not persisted under %s", FanOverrideKey)
	}
	if raw, ok, _ := svc.settings.Get(overrideKey); ok && raw != "null" {
		t.Fatalf("lighting override row touched by the fan service: %s", raw)
	}
}
