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
