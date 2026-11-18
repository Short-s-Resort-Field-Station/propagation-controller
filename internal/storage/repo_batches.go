package storage

import (
	"database/sql"
	"errors"
	"time"
)

// Batch is one propagation batch: a row of the "Batches" sheet.
type Batch struct {
	ID           int64     `json:"id"`
	BatchCode    string    `json:"batch_code"`
	Species      string    `json:"species"`
	SpeciesCode  string    `json:"species_code"`
	Source       string    `json:"source"`
	StartedOn    string    `json:"started_on"`
	Method       string    `json:"method"`
	Container    string    `json:"container"`
	Medium       string    `json:"medium"`
	CountStarted *int      `json:"count_started"`
	CountCurrent *int      `json:"count_current"`
	Status       string    `json:"status"`
	ZoneKey      string    `json:"zone_key"`
	Notes        string    `json:"notes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BatchLogEntry is one dated event: a row of the "Log" sheet.
type BatchLogEntry struct {
	ID         int64     `json:"id"`
	BatchID    int64     `json:"batch_id"`
	OccurredOn string    `json:"date"`
	Event      string    `json:"event"`
	Count      *int      `json:"count"`
	ZoneKey    string    `json:"zone_key"`
	Note       string    `json:"note"`
	Actor      string    `json:"actor"`
	CreatedAt  time.Time `json:"created_at"`
}

type BatchRepo struct{ db *sql.DB }

func NewBatchRepo(db *sql.DB) *BatchRepo { return &BatchRepo{db: db} }

const batchCols = `id, batch_code, species, species_code, source, started_on, method, container,
	medium, count_started, count_current, status, zone_key, notes, created_at, updated_at`

func scanBatch(sc interface{ Scan(...any) error }) (*Batch, error) {
	var b Batch
	var cs, cc sql.NullInt64
	if err := sc.Scan(&b.ID, &b.BatchCode, &b.Species, &b.SpeciesCode, &b.Source, &b.StartedOn,
		&b.Method, &b.Container, &b.Medium, &cs, &cc, &b.Status, &b.ZoneKey, &b.Notes,
		&b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	b.CountStarted, b.CountCurrent = nullInt(cs), nullInt(cc)
	return &b, nil
}

func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// List returns every batch, the ones still growing first, newest first.
func (r *BatchRepo) List() ([]Batch, error) {
	rows, err := r.db.Query(`SELECT ` + batchCols + ` FROM batches
		ORDER BY CASE WHEN status IN ('planted_out','failed','archived') THEN 1 ELSE 0 END,
		         started_on DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Batch{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *BatchRepo) Get(id int64) (*Batch, error) {
	b, err := scanBatch(r.db.QueryRow(`SELECT `+batchCols+` FROM batches WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func (r *BatchRepo) GetByCode(code string) (*Batch, error) {
	b, err := scanBatch(r.db.QueryRow(`SELECT `+batchCols+` FROM batches WHERE batch_code = ?`, code))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

// Insert creates the batch and its opening log rows in one transaction, so a
// batch never exists without the "started" line that dates it.
func (r *BatchRepo) Insert(b Batch, opening []BatchLogEntry) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO batches (batch_code, species, species_code, source, started_on,
		method, container, medium, count_started, count_current, status, zone_key, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.BatchCode, b.Species, b.SpeciesCode, b.Source, b.StartedOn, b.Method, b.Container,
		b.Medium, b.CountStarted, b.CountCurrent, b.Status, b.ZoneKey, b.Notes)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, e := range opening {
		if _, err := tx.Exec(`INSERT INTO batch_log (batch_id, occurred_on, event, count, zone_key, note, actor)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, e.OccurredOn, e.Event, e.Count, e.ZoneKey, e.Note, e.Actor); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// Update writes the batch's editable fields and, in the same transaction, any
// log rows the change implies (a move, a new status).
func (r *BatchRepo) Update(b Batch, entries []BatchLogEntry) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE batches SET batch_code = ?, species = ?, species_code = ?, source = ?,
		started_on = ?, method = ?, container = ?, medium = ?, count_started = ?, count_current = ?,
		status = ?, zone_key = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		b.BatchCode, b.Species, b.SpeciesCode, b.Source, b.StartedOn, b.Method, b.Container, b.Medium,
		b.CountStarted, b.CountCurrent, b.Status, b.ZoneKey, b.Notes, b.ID); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := tx.Exec(`INSERT INTO batch_log (batch_id, occurred_on, event, count, zone_key, note, actor)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, b.ID, e.OccurredOn, e.Event, e.Count, e.ZoneKey, e.Note, e.Actor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *BatchRepo) Delete(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM batch_log WHERE batch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM batches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Log returns a batch's entries oldest first; batchID 0 returns every batch's.
func (r *BatchRepo) Log(batchID int64) ([]BatchLogEntry, error) {
	q := `SELECT id, batch_id, occurred_on, event, count, zone_key, note, actor, created_at FROM batch_log`
	args := []any{}
	if batchID > 0 {
		q += ` WHERE batch_id = ?`
		args = append(args, batchID)
	}
	q += ` ORDER BY batch_id, occurred_on, id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BatchLogEntry{}
	for rows.Next() {
		var e BatchLogEntry
		var c sql.NullInt64
		if err := rows.Scan(&e.ID, &e.BatchID, &e.OccurredOn, &e.Event, &c, &e.ZoneKey, &e.Note, &e.Actor, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Count = nullInt(c)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ShelfConditions is what the rack measured for one shelf over a span of days:
// the optional "measured" columns of the Log sheet.
type ShelfConditions struct {
	Waterings    int
	MoistureLow  *float64
	MoistureHigh *float64
}

// Conditions reads waterings and the moisture range for a shelf between two
// instants. Drains, purges and refused starts are not waterings.
func (r *BatchRepo) Conditions(zoneKey string, from, to time.Time) (ShelfConditions, error) {
	var c ShelfConditions
	// Stored instants are UTC text; compare in UTC or a local-zone bound sorts wrong.
	from, to = from.UTC(), to.UTC()
	var zoneID int64
	var sensorKey sql.NullString
	err := r.db.QueryRow(`SELECT id, soil_sensor_key FROM irrigation_zones WHERE zone_key = ?`, zoneKey).Scan(&zoneID, &sensorKey)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM irrigation_cycles
		WHERE zone_id = ? AND started_at >= ? AND started_at < ?
		  AND COALESCE(actual_duration_seconds, 0) > 0
		  AND COALESCE(trigger_source, '') NOT IN ('drain', 'purge')`, zoneID, from, to).Scan(&c.Waterings); err != nil {
		return c, err
	}
	if sensorKey.Valid && sensorKey.String != "" {
		var lo, hi sql.NullFloat64
		if err := r.db.QueryRow(`SELECT MIN(min_value), MAX(max_value) FROM sensor_hourly_aggregates
			WHERE sensor_key = ? AND hour_start >= ? AND hour_start < ?`, sensorKey.String, from, to).Scan(&lo, &hi); err != nil {
			return c, err
		}
		if lo.Valid {
			c.MoistureLow = &lo.Float64
		}
		if hi.Valid {
			c.MoistureHigh = &hi.Float64
		}
	}
	return c, nil
}
