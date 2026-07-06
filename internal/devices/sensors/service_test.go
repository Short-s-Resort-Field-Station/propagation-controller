package sensors

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/devices/modbus"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

func newTestService(t *testing.T) (*Service, *modbus.MockBus, *storage.ReadingRepo, *storage.DeviceRepo, *storage.EventRepo) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	bus := modbus.NewMockBus()
	bus.Open(context.Background())
	readings := storage.NewReadingRepo(db)
	devices := storage.NewDeviceRepo(db)
	events := storage.NewEventRepo(db)
	eventLog := logging.NewEventLogger(zerolog.Nop(), events)
	svc := NewService(bus, readings, devices, eventLog, zerolog.Nop(), true)
	return svc, bus, readings, devices, events
}

func TestSeedStartAndPoll(t *testing.T) {
	svc, _, readings, devices, _ := newTestService(t)
	if err := svc.EnsureDefaults(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Idempotent.
	if err := svc.EnsureDefaults(); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	// The first poll is immediate; wait for readings to land.
	deadline := time.Now().Add(3 * time.Second)
	var latest *storage.StoredReading
	for time.Now().Before(deadline) {
		var err error
		latest, err = readings.Latest("soil_zone_1_moisture")
		if err != nil {
			t.Fatalf("latest: %v", err)
		}
		if latest != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if latest == nil {
		t.Fatal("no moisture reading stored after startup poll")
	}
	if latest.Value < 25 || latest.Value > 42 {
		t.Errorf("moisture %v outside simulated range", latest.Value)
	}

	dev, err := devices.GetByKey("soil_zone_1")
	if err != nil || dev == nil {
		t.Fatalf("device lookup: %v", err)
	}
	if dev.Status != "simulated" {
		t.Errorf("device status = %q, want simulated", dev.Status)
	}

	list, err := svc.ListSensors(ctx)
	if err != nil {
		t.Fatalf("list sensors: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sensors, want 2", len(list))
	}
}

func TestPollNowStoresReadings(t *testing.T) {
	svc, _, readings, _, _ := newTestService(t)
	svc.EnsureDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	got, err := svc.PollNow(ctx, "reservoir_level")
	if err != nil {
		t.Fatalf("PollNow: %v", err)
	}
	if len(got) != 1 || got[0].SensorKey != "reservoir_level_level" {
		t.Fatalf("unexpected readings: %+v", got)
	}
	stored, err := readings.Latest("reservoir_level_level")
	if err != nil || stored == nil {
		t.Fatalf("reading not stored: %v", err)
	}
}

func TestOfflineDetectionAndRecovery(t *testing.T) {
	svc, bus, _, devices, events := newTestService(t)
	svc.EnsureDefaults()

	// Fast polling so the failure threshold trips quickly.
	dev, _ := devices.GetByKey("soil_zone_1")
	cfg := Config{
		ModbusAddress:        1,
		PollIntervalSeconds:  1,
		OfflineAfterFailures: 2,
		Registers:            []RegisterSpec{{Name: "moisture", Register: 0, DataType: "uint16", Scale: 0.1, Unit: "%"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)
	if err := svc.UpdateConfig("soil_zone_1", cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}
	_ = dev

	bus.SetFailing(true)
	waitForStatus(t, devices, "soil_zone_1", "offline", 5*time.Second)

	bus.SetFailing(false)
	waitForStatus(t, devices, "soil_zone_1", "simulated", 5*time.Second)

	evs, err := events.List(storage.EventFilter{EventType: "device_status_change"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) < 2 {
		t.Errorf("expected offline + recovery events, got %d", len(evs))
	}
}

func waitForStatus(t *testing.T, devices *storage.DeviceRepo, key, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dev, err := devices.GetByKey(key)
		if err != nil {
			t.Fatalf("get device: %v", err)
		}
		if dev != nil && dev.Status == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	dev, _ := devices.GetByKey(key)
	t.Fatalf("device %s never reached status %q (currently %q)", key, want, dev.Status)
}
