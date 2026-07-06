package retention

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

func setup(t *testing.T) (*Service, *storage.ReadingRepo, *storage.AggregateRepo, int64) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	devices := storage.NewDeviceRepo(db)
	devices.Upsert("soil_zone_1", "Soil", "soil_sensor", "generic_modbus", "{}", "simulated")
	dev, _ := devices.GetByKey("soil_zone_1")

	readings := storage.NewReadingRepo(db)
	aggregates := storage.NewAggregateRepo(db)
	settings := storage.NewSettingsRepo(db)
	events := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))
	svc := NewService(aggregates, readings, settings, events, zerolog.Nop())
	return svc, readings, aggregates, dev.ID
}

func insert(t *testing.T, readings *storage.ReadingRepo, devID int64, key string, value float64, at time.Time) {
	t.Helper()
	err := readings.InsertBatch(devID, []storage.ReadingInsert{
		{SensorKey: key, Value: value, Unit: "%", Quality: "good", ReadAt: at},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestAggregationBucketsAndStats(t *testing.T) {
	svc, readings, aggregates, devID := setup(t)
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

	// Three samples in hour 10, one in hour 11.
	insert(t, readings, devID, "m", 30, base.Add(5*time.Minute))
	insert(t, readings, devID, "m", 40, base.Add(25*time.Minute))
	insert(t, readings, devID, "m", 35, base.Add(45*time.Minute))
	insert(t, readings, devID, "m", 50, base.Add(70*time.Minute))

	svc.aggregate() // 48h window covers only recent times... use direct range:
	if _, err := aggregates.AggregateRange(base.Add(-time.Hour), base.Add(3*time.Hour)); err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	aggs, err := aggregates.List("m", nil, nil, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(aggs) != 2 {
		t.Fatalf("got %d buckets, want 2: %+v", len(aggs), aggs)
	}
	h10 := aggs[0]
	if h10.HourStart != base {
		t.Errorf("bucket start = %v, want %v", h10.HourStart, base)
	}
	if h10.SampleCount != 3 || *h10.MinValue != 30 || *h10.MaxValue != 40 || *h10.AvgValue != 35 {
		t.Errorf("hour-10 stats wrong: %+v", h10)
	}
	if aggs[1].SampleCount != 1 || *aggs[1].AvgValue != 50 {
		t.Errorf("hour-11 stats wrong: %+v", aggs[1])
	}

	// Idempotent + refreshes when new samples land in an existing bucket.
	insert(t, readings, devID, "m", 60, base.Add(50*time.Minute))
	if _, err := aggregates.AggregateRange(base.Add(-time.Hour), base.Add(3*time.Hour)); err != nil {
		t.Fatalf("re-aggregate: %v", err)
	}
	aggs, _ = aggregates.List("m", nil, nil, 100)
	if len(aggs) != 2 || aggs[0].SampleCount != 4 || *aggs[0].MaxValue != 60 {
		t.Errorf("bucket not refreshed: %+v", aggs[0])
	}
}

func TestPruneAggregatesBeforeDeleting(t *testing.T) {
	svc, readings, aggregates, devID := setup(t)

	// Beyond the 30-day window; truncated to the hour so both samples land
	// in one bucket regardless of what time of day the test runs.
	old := time.Now().UTC().AddDate(0, 0, -40).Truncate(time.Hour)
	insert(t, readings, devID, "m", 42, old.Add(5*time.Minute))
	insert(t, readings, devID, "m", 44, old.Add(15*time.Minute))
	fresh := time.Now().UTC().Add(-time.Hour)
	insert(t, readings, devID, "m", 55, fresh)

	svc.prune()

	// Old raw rows gone, fresh survives.
	if latest, _ := readings.Latest("m"); latest == nil || latest.Value != 55 {
		t.Fatalf("fresh reading lost: %+v", latest)
	}
	rows, err := readings.List("m", nil, nil, 100)
	if err != nil {
		t.Fatalf("list raw: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only the fresh raw row, got %d", len(rows))
	}

	// But the old data lives on as an aggregate.
	aggs, _ := aggregates.List("m", nil, nil, 100)
	found := false
	for _, a := range aggs {
		if a.SampleCount == 2 && *a.AvgValue == 43 {
			found = true
		}
	}
	if !found {
		t.Errorf("pre-delete aggregation missing old bucket: %+v", aggs)
	}
}
