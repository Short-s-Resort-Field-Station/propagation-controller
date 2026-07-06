package storage

import (
	"database/sql"
	"errors"
	"time"
)

type Device struct {
	ID         int64      `json:"id"`
	DeviceKey  string     `json:"device_key"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Driver     string     `json:"driver"`
	Enabled    bool       `json:"enabled"`
	Status     string     `json:"status"`
	ConfigJSON string     `json:"config_json"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	LastError  *string    `json:"last_error,omitempty"`
}

type DeviceRepo struct{ db *sql.DB }

func NewDeviceRepo(db *sql.DB) *DeviceRepo { return &DeviceRepo{db: db} }

func (r *DeviceRepo) Upsert(deviceKey, name, deviceType, driver, configJSON, status string) error {
	_, err := r.db.Exec(
		`INSERT INTO devices (device_key, name, type, driver, config_json, status)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(device_key) DO UPDATE SET
		   name = excluded.name,
		   type = excluded.type,
		   driver = excluded.driver,
		   status = excluded.status,
		   updated_at = CURRENT_TIMESTAMP`,
		deviceKey, name, deviceType, driver, configJSON, status,
	)
	return err
}

func (r *DeviceRepo) SetStatus(deviceKey, status string, lastError *string) error {
	_, err := r.db.Exec(
		`UPDATE devices SET status = ?, last_error = ?, last_seen_at = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE device_key = ?`,
		status, lastError, time.Now().UTC(), deviceKey,
	)
	return err
}

func (r *DeviceRepo) GetByKey(deviceKey string) (*Device, error) {
	var d Device
	err := r.db.QueryRow(
		`SELECT id, device_key, name, type, driver, enabled, status, config_json, last_seen_at, last_error
		 FROM devices WHERE device_key = ?`, deviceKey).
		Scan(&d.ID, &d.DeviceKey, &d.Name, &d.Type, &d.Driver, &d.Enabled,
			&d.Status, &d.ConfigJSON, &d.LastSeenAt, &d.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeviceRepo) UpdateConfig(deviceKey, configJSON string) error {
	_, err := r.db.Exec(
		`UPDATE devices SET config_json = ?, updated_at = CURRENT_TIMESTAMP WHERE device_key = ?`,
		configJSON, deviceKey,
	)
	return err
}

func (r *DeviceRepo) ListByType(deviceType string) ([]Device, error) {
	rows, err := r.db.Query(
		`SELECT id, device_key, name, type, driver, enabled, status, config_json, last_seen_at, last_error
		 FROM devices WHERE type = ? ORDER BY device_key`, deviceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.DeviceKey, &d.Name, &d.Type, &d.Driver, &d.Enabled,
			&d.Status, &d.ConfigJSON, &d.LastSeenAt, &d.LastError); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (r *DeviceRepo) List() ([]Device, error) {
	rows, err := r.db.Query(
		`SELECT id, device_key, name, type, driver, enabled, status, config_json, last_seen_at, last_error
		 FROM devices ORDER BY device_key`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.DeviceKey, &d.Name, &d.Type, &d.Driver, &d.Enabled,
			&d.Status, &d.ConfigJSON, &d.LastSeenAt, &d.LastError); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
