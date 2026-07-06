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
