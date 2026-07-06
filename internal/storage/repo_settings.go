package storage

import (
	"database/sql"
	"errors"
)

type SettingsRepo struct{ db *sql.DB }

func NewSettingsRepo(db *sql.DB) *SettingsRepo { return &SettingsRepo{db: db} }

// Get returns the raw JSON value for key, or ok=false if unset.
func (r *SettingsRepo) Get(key string) (string, bool, error) {
	var value string
	err := r.db.QueryRow(`SELECT value_json FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (r *SettingsRepo) Set(key, valueJSON string) error {
	_, err := r.db.Exec(
		`INSERT INTO settings (key, value_json) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = CURRENT_TIMESTAMP`,
		key, valueJSON,
	)
	return err
}

func (r *SettingsRepo) All() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT key, value_json FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
