package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestBucketedSummarisesRawReadings(t *testing.T) {
	db := openTestDB(t)
	repo := NewReadingRepo(db)
	devices := NewDeviceRepo(db)
	if err := devices.Upsert("probe", "Probe", "soil_sensor", "generic_modbus", "{}", "simulated"); err != nil {
		t.Fatalf("device: %v", err)
	}
	dev, _ := devices.GetByKey("probe")
	base := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	var ins []ReadingInsert
	for i := 0; i < 60; i++ { // one reading a minute for an hour
		ins = append(ins, ReadingInsert{SensorKey: "k", Value: float64(i), Unit: "%", Quality: "good", ReadAt: base.Add(time.Duration(i) * time.Minute)})
	}
	if err := repo.InsertBatch(dev.ID, ins); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := repo.Bucketed("k", base, base.Add(time.Hour), 15*time.Minute)
	if err != nil {
		t.Fatalf("bucketed: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("buckets = %d, want 4", len(got))
	}
	b := got[1]
	if !b.HourStart.Equal(base.Add(15*time.Minute)) || b.SampleCount != 15 || *b.MinValue != 15 || *b.MaxValue != 29 || *b.AvgValue != 22 {
		t.Errorf("bucket 2 = start %v n %d min %v max %v avg %v", b.HourStart, b.SampleCount, *b.MinValue, *b.MaxValue, *b.AvgValue)
	}
}

// Readings taken with a probe in air are flagged (with the probe's other
// channels and a settle window) and left out of buckets and hourly averages.
func TestOutOfMediaReadingsExcluded(t *testing.T) {
	db := openTestDB(t)
	repo := NewReadingRepo(db)
	aggs := NewAggregateRepo(db)
	devices := NewDeviceRepo(db)
	devices.Upsert("soil_zone_1", "Probe", "soil_sensor", "generic_modbus", "{}", "simulated")
	dev, _ := devices.GetByKey("soil_zone_1")
	base := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	var ins []ReadingInsert
	for i := 0; i < 30; i++ { // minutes 10–14 in air, the rest at 20 %
		m, ec := 20.0, 400.0
		if i >= 10 && i < 15 {
			m, ec = 0, 0
		}
		at := base.Add(time.Duration(i) * time.Minute)
		ins = append(ins,
			ReadingInsert{SensorKey: "soil_zone_1_moisture", Value: m, Unit: "%", Quality: "good", ReadAt: at},
			ReadingInsert{SensorKey: "soil_zone_1_ec", Value: ec, Unit: "uS/cm", Quality: "good", ReadAt: at.Add(100 * time.Millisecond)})
	}
	if err := repo.InsertBatch(dev.ID, ins); err != nil {
		t.Fatalf("insert: %v", err)
	}
	n, err := repo.FlagOutOfMediaHistory(2 * time.Minute)
	if err != nil {
		t.Fatalf("flag: %v", err)
	}
	if n != 14 { // 5 air + 2 settle minutes, both channels
		t.Errorf("flagged %d rows, want 14", n)
	}
	if n2, _ := repo.FlagOutOfMediaHistory(2 * time.Minute); n2 != 0 {
		t.Errorf("second pass flagged %d rows, want 0", n2)
	}
	if _, err := aggs.AggregateRange(base, base.Add(time.Hour)); err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	for key, want := range map[string]float64{"soil_zone_1_moisture": 20, "soil_zone_1_ec": 400} {
		list, _ := aggs.List(key, nil, nil, 10)
		if len(list) != 1 || *list[0].AvgValue != want || *list[0].MinValue != want || list[0].SampleCount != 23 {
			t.Errorf("%s hourly = %+v", key, list)
		}
		b, _ := repo.Bucketed(key, base, base.Add(time.Hour), 5*time.Minute)
		for _, x := range b {
			if *x.MinValue != want {
				t.Errorf("%s bucket %v min %v", key, x.HourStart, *x.MinValue)
			}
		}
	}
}
