package storage

import (
	"path/filepath"
	"testing"
)

func TestMigrateAndRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Idempotent.
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	events := NewEventRepo(db)
	actor := "test"
	if err := events.Insert("info", "unit_test", "hello", nil, &actor); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	got, err := events.List(EventFilter{EventType: "unit_test"})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(got) != 1 || got[0].Message != "hello" {
		t.Fatalf("unexpected events: %+v", got)
	}

	settings := NewSettingsRepo(db)
	if err := settings.Set("timezone", `"America/Chicago"`); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	val, ok, err := settings.Get("timezone")
	if err != nil || !ok || val != `"America/Chicago"` {
		t.Fatalf("get setting = %q ok=%v err=%v", val, ok, err)
	}

	devices := NewDeviceRepo(db)
	if err := devices.Upsert("relay_controller", "Numato", "relay_controller", "mock", "{}", "simulated"); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	// Upsert again must update, not duplicate.
	if err := devices.Upsert("relay_controller", "Numato 8ch", "relay_controller", "mock", "{}", "online"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	list, err := devices.List()
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Numato 8ch" || list[0].Status != "online" {
		t.Fatalf("unexpected devices: %+v", list)
	}
}
