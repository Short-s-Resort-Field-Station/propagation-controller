package storage

import (
	"database/sql"
	"errors"
	"time"
)

type Cycle struct {
	ID                       int64      `json:"id"`
	ZoneID                   int64      `json:"zone_id"`
	ZoneKey                  string     `json:"zone_key"`
	StartedAt                time.Time  `json:"started_at"`
	EndedAt                  *time.Time `json:"ended_at,omitempty"`
	RequestedDurationSeconds *int       `json:"requested_duration_seconds,omitempty"`
	ActualDurationSeconds    *int       `json:"actual_duration_seconds,omitempty"`
	TriggerType              string     `json:"trigger_type"`
	TriggerSource            *string    `json:"trigger_source,omitempty"`
	ReservoirLevelStart      *float64   `json:"reservoir_level_start,omitempty"`
	ReservoirLevelEnd        *float64   `json:"reservoir_level_end,omitempty"`
	SoilMoistureStart        *float64   `json:"soil_moisture_start,omitempty"`
	SoilMoistureEnd          *float64   `json:"soil_moisture_end,omitempty"`
	Result                   string     `json:"result"`
	AbortReason              *string    `json:"abort_reason,omitempty"`
	ErrorMessage             *string    `json:"error_message,omitempty"`
}

type CycleRepo struct{ db *sql.DB }

func NewCycleRepo(db *sql.DB) *CycleRepo { return &CycleRepo{db: db} }

// InsertStart records a cycle the moment it leaves the queue, so an active
// cycle is visible in history immediately (result = "running").
func (r *CycleRepo) InsertStart(zoneID int64, requestedSeconds int, triggerType string,
	triggerSource *string, reservoirStart, moistureStart *float64) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO irrigation_cycles
		 (zone_id, started_at, requested_duration_seconds, trigger_type, trigger_source,
		  reservoir_level_start, soil_moisture_start, result)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'running')`,
		zoneID, time.Now().UTC(), requestedSeconds, triggerType, triggerSource,
		reservoirStart, moistureStart,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *CycleRepo) Finish(id int64, actualSeconds *int, result string,
	abortReason, errorMessage *string, reservoirEnd, moistureEnd *float64) error {
	_, err := r.db.Exec(
		`UPDATE irrigation_cycles SET
		 ended_at = ?, actual_duration_seconds = ?, result = ?,
		 abort_reason = ?, error_message = ?, reservoir_level_end = ?, soil_moisture_end = ?
		 WHERE id = ?`,
		time.Now().UTC(), actualSeconds, result, abortReason, errorMessage,
		reservoirEnd, moistureEnd, id,
	)
	return err
}

const cycleColumns = `c.id, c.zone_id, z.zone_key, c.started_at, c.ended_at,
	c.requested_duration_seconds, c.actual_duration_seconds, c.trigger_type, c.trigger_source,
	c.reservoir_level_start, c.reservoir_level_end, c.soil_moisture_start, c.soil_moisture_end,
	c.result, c.abort_reason, c.error_message`

func scanCycle(row interface{ Scan(...any) error }) (Cycle, error) {
	var c Cycle
	err := row.Scan(&c.ID, &c.ZoneID, &c.ZoneKey, &c.StartedAt, &c.EndedAt,
		&c.RequestedDurationSeconds, &c.ActualDurationSeconds, &c.TriggerType, &c.TriggerSource,
		&c.ReservoirLevelStart, &c.ReservoirLevelEnd, &c.SoilMoistureStart, &c.SoilMoistureEnd,
		&c.Result, &c.AbortReason, &c.ErrorMessage)
	return c, err
}

func (r *CycleRepo) Get(id int64) (*Cycle, error) {
	c, err := scanCycle(r.db.QueryRow(
		`SELECT `+cycleColumns+` FROM irrigation_cycles c
		 JOIN irrigation_zones z ON z.id = c.zone_id WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CycleRepo) List(zoneKey string, limit int) ([]Cycle, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT ` + cycleColumns + ` FROM irrigation_cycles c
	          JOIN irrigation_zones z ON z.id = c.zone_id`
	args := []any{}
	if zoneKey != "" {
		query += ` WHERE z.zone_key = ?`
		args = append(args, zoneKey)
	}
	query += ` ORDER BY c.started_at DESC, c.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cycles []Cycle
	for rows.Next() {
		c, err := scanCycle(rows)
		if err != nil {
			return nil, err
		}
		cycles = append(cycles, c)
	}
	return cycles, rows.Err()
}

func (r *CycleRepo) LastForZone(zoneID int64) (*Cycle, error) {
	c, err := scanCycle(r.db.QueryRow(
		`SELECT `+cycleColumns+` FROM irrigation_cycles c
		 JOIN irrigation_zones z ON z.id = c.zone_id
		 WHERE c.zone_id = ? ORDER BY c.started_at DESC, c.id DESC LIMIT 1`, zoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
