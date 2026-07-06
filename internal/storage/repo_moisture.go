package storage

import (
	"database/sql"
	"errors"
	"time"
)

// MoistureRule is one row of moisture_control_rules (spec addendum:
// sensor-triggered irrigation).
type MoistureRule struct {
	ID                     int64    `json:"id"`
	ZoneID                 int64    `json:"zone_id"`
	ZoneKey                string   `json:"zone_key"`
	Enabled                bool     `json:"enabled"`
	SensorKey              string   `json:"sensor_key"`
	TriggerBelow           float64  `json:"trigger_below"`
	RecoveredAbove         *float64 `json:"recovered_above,omitempty"`
	RunDurationSeconds     int      `json:"run_duration_seconds"`
	MinimumIntervalMinutes int      `json:"minimum_interval_minutes"`
	MaxCyclesPerDay        *int     `json:"max_cycles_per_day,omitempty"`
	SensorOfflineBehavior  string   `json:"sensor_offline_behavior"` // lockout | ignore
}

type MoistureRuleRepo struct{ db *sql.DB }

func NewMoistureRuleRepo(db *sql.DB) *MoistureRuleRepo { return &MoistureRuleRepo{db: db} }

const moistureColumns = `m.id, m.zone_id, z.zone_key, m.enabled, m.sensor_key, m.trigger_below,
	m.recovered_above, m.run_duration_seconds, m.minimum_interval_minutes,
	m.max_cycles_per_day, m.sensor_offline_behavior`

func scanMoistureRule(row interface{ Scan(...any) error }) (MoistureRule, error) {
	var m MoistureRule
	err := row.Scan(&m.ID, &m.ZoneID, &m.ZoneKey, &m.Enabled, &m.SensorKey, &m.TriggerBelow,
		&m.RecoveredAbove, &m.RunDurationSeconds, &m.MinimumIntervalMinutes,
		&m.MaxCyclesPerDay, &m.SensorOfflineBehavior)
	return m, err
}

func (r *MoistureRuleRepo) GetByZoneID(zoneID int64) (*MoistureRule, error) {
	m, err := scanMoistureRule(r.db.QueryRow(
		`SELECT `+moistureColumns+` FROM moisture_control_rules m
		 JOIN irrigation_zones z ON z.id = m.zone_id WHERE m.zone_id = ?`, zoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MoistureRuleRepo) ListEnabled() ([]MoistureRule, error) {
	rows, err := r.db.Query(
		`SELECT ` + moistureColumns + ` FROM moisture_control_rules m
		 JOIN irrigation_zones z ON z.id = m.zone_id
		 WHERE m.enabled = 1 AND z.enabled = 1 ORDER BY z.zone_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MoistureRule
	for rows.Next() {
		m, err := scanMoistureRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MoistureRuleRepo) Upsert(m MoistureRule) error {
	// One rule per zone: replace on conflict via delete+insert inside a tx
	// (the table has no unique constraint on zone_id in the spec schema).
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM moisture_control_rules WHERE zone_id = ?`, m.ZoneID); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO moisture_control_rules
		 (zone_id, enabled, sensor_key, trigger_below, recovered_above, run_duration_seconds,
		  minimum_interval_minutes, max_cycles_per_day, sensor_offline_behavior)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ZoneID, m.Enabled, m.SensorKey, m.TriggerBelow, m.RecoveredAbove, m.RunDurationSeconds,
		m.MinimumIntervalMinutes, m.MaxCyclesPerDay, m.SensorOfflineBehavior); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// CountCyclesSince counts cycles for a zone with the given trigger type that
// started after the cutoff. Used for max-cycles-per-day enforcement.
func (r *CycleRepo) CountCyclesSince(zoneID int64, triggerType string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM irrigation_cycles
		 WHERE zone_id = ? AND trigger_type = ? AND started_at >= ? AND result != 'locked_out'`,
		zoneID, triggerType, since.UTC()).Scan(&n)
	return n, err
}

// LastWateredForZone returns the most recent cycle that actually moved water
// (anything but locked_out). Used for minimum-interval enforcement.
func (r *CycleRepo) LastWateredForZone(zoneID int64) (*Cycle, error) {
	c, err := scanCycle(r.db.QueryRow(
		`SELECT `+cycleColumns+` FROM irrigation_cycles c
		 JOIN irrigation_zones z ON z.id = c.zone_id
		 WHERE c.zone_id = ? AND c.result != 'locked_out'
		 ORDER BY c.started_at DESC, c.id DESC LIMIT 1`, zoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
