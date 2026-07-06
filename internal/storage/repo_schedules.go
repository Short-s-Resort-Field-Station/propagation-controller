package storage

import (
	"database/sql"
	"errors"
)

type ScheduleRow struct {
	ID              int64  `json:"id"`
	ScheduleKey     string `json:"schedule_key"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Enabled         bool   `json:"enabled"`
	Timezone        string `json:"timezone"`
	RuleJSON        string `json:"rule_json"`
	MissedRunPolicy string `json:"missed_run_policy"`
}

type ScheduleRepo struct{ db *sql.DB }

func NewScheduleRepo(db *sql.DB) *ScheduleRepo { return &ScheduleRepo{db: db} }

func (r *ScheduleRepo) GetByKey(key string) (*ScheduleRow, error) {
	var s ScheduleRow
	err := r.db.QueryRow(
		`SELECT id, schedule_key, name, type, enabled, timezone, rule_json, missed_run_policy
		 FROM schedules WHERE schedule_key = ?`, key).
		Scan(&s.ID, &s.ScheduleKey, &s.Name, &s.Type, &s.Enabled, &s.Timezone, &s.RuleJSON, &s.MissedRunPolicy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *ScheduleRepo) Upsert(key, name, scheduleType string, enabled bool,
	timezone, ruleJSON, missedRunPolicy string) error {
	_, err := r.db.Exec(
		`INSERT INTO schedules (schedule_key, name, type, enabled, timezone, rule_json, missed_run_policy)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(schedule_key) DO UPDATE SET
		   name = excluded.name, type = excluded.type, enabled = excluded.enabled,
		   timezone = excluded.timezone, rule_json = excluded.rule_json,
		   missed_run_policy = excluded.missed_run_policy, updated_at = CURRENT_TIMESTAMP`,
		key, name, scheduleType, enabled, timezone, ruleJSON, missedRunPolicy,
	)
	return err
}

func (r *ScheduleRepo) Delete(key string) error {
	_, err := r.db.Exec(`DELETE FROM schedules WHERE schedule_key = ?`, key)
	return err
}

func (r *ScheduleRepo) ListByType(scheduleType string) ([]ScheduleRow, error) {
	rows, err := r.db.Query(
		`SELECT id, schedule_key, name, type, enabled, timezone, rule_json, missed_run_policy
		 FROM schedules WHERE type = ? ORDER BY schedule_key`, scheduleType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduleRow
	for rows.Next() {
		var s ScheduleRow
		if err := rows.Scan(&s.ID, &s.ScheduleKey, &s.Name, &s.Type, &s.Enabled,
			&s.Timezone, &s.RuleJSON, &s.MissedRunPolicy); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
