package devices

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"

	"github.com/rs/zerolog"
)

func newTestManager(t *testing.T) (*Manager, *storage.RelayActionRepo) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	actions := storage.NewRelayActionRepo(db)
	eventLog := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))
	mgr := NewManager(relay.NewMock(), "mock", actions, storage.NewDeviceRepo(db), eventLog, true)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	return mgr, actions
}

func TestSetRelayLogsAction(t *testing.T) {
	mgr, actions := newTestManager(t)
	ctx := context.Background()

	if _, err := mgr.SetRelay(ctx, 3, relay.On, "manual_web", "test run", 0); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	info, err := mgr.GetRelay(ctx, 3)
	if err != nil || info.State != relay.On {
		t.Fatalf("relay 3 state = %v (err %v), want on", info.State, err)
	}

	logged, err := actions.List(nil, nil, 50)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	found := false
	for _, a := range logged {
		if a.RelayChannel == 3 && a.RequestedState == "on" && a.Actor == "manual_web" && a.Success {
			found = true
		}
	}
	if !found {
		t.Errorf("expected logged action for relay 3 on by manual_web, got %+v", logged)
	}
}

func TestStartForcesPumpAndValvesOff(t *testing.T) {
	mgr, actions := newTestManager(t)

	logged, err := actions.List(nil, nil, 50)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	offByStartup := map[int]bool{}
	for _, a := range logged {
		if a.Actor == "startup" && a.RequestedState == "off" && a.Success {
			offByStartup[a.RelayChannel] = true
		}
	}
	for _, ch := range []int{2, 3, 4, 5, 6} {
		if !offByStartup[ch] {
			t.Errorf("relay %d not forced off at startup", ch)
		}
	}

	info, _ := mgr.GetRelay(context.Background(), 2)
	if info.State != relay.Off {
		t.Errorf("pump state after startup = %v, want off", info.State)
	}
}

func TestRelayTimeoutTurnsOff(t *testing.T) {
	mgr, actions := newTestManager(t)
	ctx := context.Background()

	if _, err := mgr.SetRelay(ctx, 2, relay.On, "manual_touch", "pump 1s test", 1); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	info, _ := mgr.GetRelay(ctx, 2)
	if info.State != relay.On {
		t.Fatalf("pump should be on immediately, got %v", info.State)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, _ = mgr.GetRelay(ctx, 2)
		if info.State == relay.Off {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if info.State != relay.Off {
		t.Fatalf("pump still on after timeout")
	}

	logged, _ := actions.List(nil, nil, 50)
	found := false
	for _, a := range logged {
		if a.RelayChannel == 2 && a.Actor == "system" && a.Reason != nil && *a.Reason == "timeout expired after 1s" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected system timeout action in log")
	}
}

func TestNewCommandCancelsPendingTimeout(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	if _, err := mgr.SetRelay(ctx, 4, relay.On, "manual_web", "first", 1); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	// Re-command with no timeout; the 1s timer must not fire afterwards.
	if _, err := mgr.SetRelay(ctx, 4, relay.On, "manual_web", "second, no timeout", 0); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	info, _ := mgr.GetRelay(ctx, 4)
	if info.State != relay.On {
		t.Errorf("relay 4 = %v, want on (stale timeout fired)", info.State)
	}
}
