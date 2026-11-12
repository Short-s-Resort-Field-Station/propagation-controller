package storage

import (
	"database/sql"
	"errors"
	"time"
)

type StoredReading struct {
	ID        int64     `json:"id"`
	SensorKey string    `json:"sensor_key"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit"`
	Quality   string    `json:"quality"`
	ReadAt    time.Time `json:"read_at"`
}

type ReadingRepo struct{ db *sql.DB }

func NewReadingRepo(db *sql.DB) *ReadingRepo { return &ReadingRepo{db: db} }

type ReadingInsert struct {
	SensorKey string
	Value     float64
	Unit      string
	Quality   string
	ReadAt    time.Time
}

// InsertBatch stores one poll's readings in a single transaction — one fsync
// per poll instead of per measurement.
func (r *ReadingRepo) InsertBatch(deviceID int64, readings []ReadingInsert) error {
	if len(readings) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO sensor_readings (device_id, sensor_key, value, unit, quality, read_at)
		 VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, rd := range readings {
		if _, err := stmt.Exec(deviceID, rd.SensorKey, rd.Value, rd.Unit, rd.Quality, rd.ReadAt.UTC()); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Latest returns the most recent reading for a sensor key, or nil if none.
func (r *ReadingRepo) Latest(sensorKey string) (*StoredReading, error) {
	var rd StoredReading
	err := r.db.QueryRow(
		`SELECT id, sensor_key, value, unit, quality, read_at FROM sensor_readings
		 WHERE sensor_key = ? ORDER BY read_at DESC, id DESC LIMIT 1`, sensorKey).
		Scan(&rd.ID, &rd.SensorKey, &rd.Value, &rd.Unit, &rd.Quality, &rd.ReadAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rd, nil
}

func (r *ReadingRepo) List(sensorKey string, from, to *time.Time, limit int) ([]StoredReading, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	query := `SELECT id, sensor_key, value, unit, quality, read_at FROM sensor_readings WHERE sensor_key = ?`
	args := []any{sensorKey}
	if from != nil {
		query += " AND read_at >= ?"
		args = append(args, from.UTC())
	}
	if to != nil {
		query += " AND read_at <= ?"
		args = append(args, to.UTC())
	}
	query += " ORDER BY read_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StoredReading
	for rows.Next() {
		var rd StoredReading
		if err := rows.Scan(&rd.ID, &rd.SensorKey, &rd.Value, &rd.Unit, &rd.Quality, &rd.ReadAt); err != nil {
			return nil, err
		}
		out = append(out, rd)
	}
	return out, rows.Err()
}

// Bucketed summarises raw readings into fixed-width buckets (min / avg / max
// / count) — the sub-hour resolution the dashboard's 24-hour timeline wants
// (Austin, 2026-11-11: detailed charts with peaks and lows). It reuses the
// hourly aggregate's shape so the client reads one format; HourStart is the
// bucket start. Bucket boundaries are aligned to UTC multiples of the width.
func (r *ReadingRepo) Bucketed(sensorKey string, from, to time.Time, bucket time.Duration) ([]HourlyAggregate, error) {
	if bucket < time.Minute {
		bucket = time.Minute
	}
	rows, err := r.db.Query(
		`SELECT value, unit, read_at FROM sensor_readings
		 WHERE sensor_key = ? AND read_at >= ? AND read_at <= ? ORDER BY read_at ASC, id ASC`,
		sensorKey, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []HourlyAggregate{}
	var cur *HourlyAggregate
	var sum float64
	flush := func() {
		if cur != nil && cur.SampleCount > 0 {
			avg := sum / float64(cur.SampleCount)
			cur.AvgValue = &avg
			out = append(out, *cur)
		}
		cur, sum = nil, 0
	}
	for rows.Next() {
		var v float64
		var unit string
		var at time.Time
		if err := rows.Scan(&v, &unit, &at); err != nil {
			return nil, err
		}
		start := at.UTC().Truncate(bucket)
		if cur == nil || !cur.HourStart.Equal(start) {
			flush()
			mn, mx := v, v
			cur = &HourlyAggregate{SensorKey: sensorKey, HourStart: start, MinValue: &mn, MaxValue: &mx, Unit: unit}
		}
		if v < *cur.MinValue {
			*cur.MinValue = v
		}
		if v > *cur.MaxValue {
			*cur.MaxValue = v
		}
		cur.SampleCount++
		sum += v
	}
	flush()
	return out, rows.Err()
}
