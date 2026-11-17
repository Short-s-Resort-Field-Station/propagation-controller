package storage

import "testing"

// TestLastWateringSkipsDrainsAndRefusals: "Last Water" is the last cycle that
// ran the pump, not a later drain, purge or refused attempt.
func TestLastWateringSkipsDrainsAndRefusals(t *testing.T) {
	db := openTestDB(t)
	zones := NewZoneRepo(db)
	if err := zones.Insert(Zone{ZoneKey: "zone_1", Name: "Shelf 1", ValveRelayChannel: 1, Enabled: true}); err != nil {
		t.Fatalf("zone: %v", err)
	}
	z, _ := zones.GetByKey("zone_1")
	cycles := NewCycleRepo(db)
	secs := func(n int) *int { return &n }
	str := func(v string) *string { return &v }

	soak, _ := cycles.InsertStart(z.ID, 900, "moisture", nil, nil, nil)
	cycles.Finish(soak, secs(900), "complete", nil, nil, nil, nil)
	drain, _ := cycles.InsertStart(z.ID, 0, "system", str("drain"), nil, nil)
	cycles.Finish(drain, secs(0), "complete", nil, nil, nil, nil)
	purge, _ := cycles.InsertStart(z.ID, 0, "system", str("purge"), nil, nil)
	cycles.Finish(purge, secs(0), "complete", nil, nil, nil, nil)
	refused, _ := cycles.InsertStart(z.ID, 900, "manual_web", nil, nil, nil)
	cycles.Finish(refused, nil, "aborted", str("pump is locked out"), nil, nil, nil)

	last, err := cycles.LastForZone(z.ID)
	if err != nil || last == nil || last.ID != refused {
		t.Fatalf("LastForZone = %+v, %v; want the refused attempt", last, err)
	}
	lw, err := cycles.LastWateringForZone(z.ID)
	if err != nil || lw == nil || lw.ID != soak {
		t.Fatalf("LastWateringForZone = %+v, %v; want the soak (id %d)", lw, err, soak)
	}
}
