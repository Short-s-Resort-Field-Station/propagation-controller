package lighting

import (
	"context"
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

	svc := NewService(mgr, sup, storage.NewScheduleRepo(db), eventLog, zerolog.Nop(), 20*time.Millisecond)
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
