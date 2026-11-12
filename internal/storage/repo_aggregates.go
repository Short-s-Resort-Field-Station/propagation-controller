package storage

import (
	"database/sql"
	"time"
)

// HourlyAggregate is one sensor_hourly_aggregates row (spec §14.2 — kept
// forever, feeding long-term charts after raw readings are pruned).
type HourlyAggregate struct {
	SensorKey   string    `json:"sensor_key"`
	HourStart   time.Time `json:"hour_start"`
	MinValue    *float64  `json:"min_value"`
	MaxValue    *float64  `json:"max_value"`
	AvgValue    *float64  `json:"avg_value"`
	SampleCount int       `json:"sample_count"`
	Unit        string    `json:"unit"`
}

type AggregateRepo struct{ db *sql.DB }

func NewAggregateRepo(db *sql.DB) *AggregateRepo { return &AggregateRepo{db: db} }

const hourFormat = "2006-01-02 15:04:05"

// AggregateRange (re)computes hourly aggregates for raw readings in
// [from, to). Idempotent: existing hours are updated in place, so re-running
// over a window that gained new samples refreshes it.
//
// Bucketing note: the sqlite driver stores time.Time in Go's String()
// format ("2026-07-06 10:05:00 +0000 UTC"), which SQLite's strftime cannot
// parse — so the hour bucket is cut by substring. All readings are written
// through ReadingRepo in UTC, so the format is uniform and chars 1–13 are
// exactly "YYYY-MM-DD HH".
func (r *AggregateRepo) AggregateRange(from, to time.Time) (int64, error) {
	res, err := r.db.Exec(`
		INSERT INTO sensor_hourly_aggregates
		  (sensor_key, hour_start, min_value, max_value, avg_value, sample_count, unit)
		SELECT sensor_key, substr(read_at, 1, 13) || ':00:00',
		       MIN(value), MAX(value), AVG(value), COUNT(*), unit
		FROM sensor_readings
		WHERE read_at >= ? AND read_at < ? AND quality != 'out_of_media'
		GROUP BY sensor_key, substr(read_at, 1, 13)
		ON CONFLICT(sensor_key, hour_start) DO UPDATE SET
		  min_value = excluded.min_value,
		  max_value = excluded.max_value,
		  avg_value = excluded.avg_value,
		  sample_count = excluded.sample_count,
		  unit = excluded.unit`,
		from.UTC(), to.UTC())
	if err != nil {
		return 0, err
	}
	// An hour whose every raw reading was taken with the probe out of its pot
	// has nothing to summarise: drop any aggregate left from before the rows
	// were flagged. Hours whose raw rows are already pruned are untouched.
	// Each probe walks only that hour's rows through the (sensor_key,
	// read_at) index — read_at sorts as text, and ';' is the character after
	// ':' — so the pass stays in milliseconds; this runs every few minutes on
	// the one DB connection the relay and safety paths share.
	if _, err := r.db.Exec(`
		DELETE FROM sensor_hourly_aggregates
		WHERE hour_start >= ? AND hour_start < ?
		  AND EXISTS (SELECT 1 FROM sensor_readings sr
		              WHERE sr.sensor_key = sensor_hourly_aggregates.sensor_key
		                AND sr.read_at >= substr(sensor_hourly_aggregates.hour_start, 1, 13)
		                AND sr.read_at <  substr(sensor_hourly_aggregates.hour_start, 1, 13) || ';')
		  AND NOT EXISTS (SELECT 1 FROM sensor_readings sr
		              WHERE sr.sensor_key = sensor_hourly_aggregates.sensor_key
		                AND sr.read_at >= substr(sensor_hourly_aggregates.hour_start, 1, 13)
		                AND sr.read_at <  substr(sensor_hourly_aggregates.hour_start, 1, 13) || ';'
		                AND sr.quality != 'out_of_media')`,
		from.UTC().Truncate(time.Hour).Format(hourFormat), to.UTC().Format(hourFormat)); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *AggregateRepo) List(sensorKey string, from, to *time.Time, limit int) ([]HourlyAggregate, error) {
	if limit <= 0 || limit > 8760 {
		limit = 720
	}
	query := `SELECT sensor_key, hour_start, min_value, max_value, avg_value, sample_count, unit
	          FROM sensor_hourly_aggregates WHERE sensor_key = ?`
	args := []any{sensorKey}
	if from != nil {
		query += " AND hour_start >= ?"
		args = append(args, from.UTC().Format(hourFormat))
	}
	if to != nil {
		query += " AND hour_start <= ?"
		args = append(args, to.UTC().Format(hourFormat))
	}
	query += " ORDER BY hour_start ASC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HourlyAggregate
	for rows.Next() {
		var a HourlyAggregate
		var hour any
		if err := rows.Scan(&a.SensorKey, &hour, &a.MinValue, &a.MaxValue, &a.AvgValue, &a.SampleCount, &a.Unit); err != nil {
			return nil, err
		}
		// hour_start is written without a zone; readings are stored UTC, so
		// the bucket is UTC. The driver may hand the DATETIME column back as
		// a time.Time or as text depending on its parse.
		switch v := hour.(type) {
		case time.Time:
			a.HourStart = v.UTC()
		case string:
			t, err := time.ParseInLocation(hourFormat, v, time.UTC)
			if err != nil {
				continue
			}
			a.HourStart = t
		case []byte:
			t, err := time.ParseInLocation(hourFormat, string(v), time.UTC)
			if err != nil {
				continue
			}
			a.HourStart = t
		default:
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteRawBefore prunes raw readings older than the cutoff (spec §14.1).
func (r *ReadingRepo) DeleteRawBefore(cutoff time.Time) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM sensor_readings WHERE read_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
