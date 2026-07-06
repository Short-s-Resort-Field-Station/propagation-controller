package storage

import (
	"database/sql"
	"errors"
)

type Zone struct {
	ID                int64    `json:"id"`
	ZoneKey           string   `json:"zone_key"`
	Name              string   `json:"name"`
	ValveRelayChannel int      `json:"valve_relay_channel"`
	SoilSensorKey     *string  `json:"soil_sensor_key,omitempty"`
	Enabled           bool     `json:"enabled"`
	MaxFloodSeconds   int      `json:"max_flood_seconds"`
	MaxPumpSeconds    int      `json:"max_pump_seconds"`
	MinReservoirLevel *float64 `json:"min_reservoir_level,omitempty"`
	ConfigJSON        string   `json:"config_json"`
}

type ZoneRepo struct{ db *sql.DB }

func NewZoneRepo(db *sql.DB) *ZoneRepo { return &ZoneRepo{db: db} }

const zoneColumns = `id, zone_key, name, valve_relay_channel, soil_sensor_key, enabled,
	max_flood_seconds, max_pump_seconds, min_reservoir_level, config_json`

func scanZone(row interface{ Scan(...any) error }) (Zone, error) {
	var z Zone
	err := row.Scan(&z.ID, &z.ZoneKey, &z.Name, &z.ValveRelayChannel, &z.SoilSensorKey,
		&z.Enabled, &z.MaxFloodSeconds, &z.MaxPumpSeconds, &z.MinReservoirLevel, &z.ConfigJSON)
	return z, err
}

func (r *ZoneRepo) Insert(z Zone) error {
	_, err := r.db.Exec(
		`INSERT INTO irrigation_zones
		 (zone_key, name, valve_relay_channel, soil_sensor_key, enabled,
		  max_flood_seconds, max_pump_seconds, min_reservoir_level, config_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		z.ZoneKey, z.Name, z.ValveRelayChannel, z.SoilSensorKey, z.Enabled,
		z.MaxFloodSeconds, z.MaxPumpSeconds, z.MinReservoirLevel, z.ConfigJSON,
	)
	return err
}

func (r *ZoneRepo) Update(z Zone) error {
	_, err := r.db.Exec(
		`UPDATE irrigation_zones SET
		 name = ?, valve_relay_channel = ?, soil_sensor_key = ?, enabled = ?,
		 max_flood_seconds = ?, max_pump_seconds = ?, min_reservoir_level = ?, config_json = ?,
		 updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		z.Name, z.ValveRelayChannel, z.SoilSensorKey, z.Enabled,
		z.MaxFloodSeconds, z.MaxPumpSeconds, z.MinReservoirLevel, z.ConfigJSON, z.ID,
	)
	return err
}

func (r *ZoneRepo) GetByKey(zoneKey string) (*Zone, error) {
	z, err := scanZone(r.db.QueryRow(
		`SELECT `+zoneColumns+` FROM irrigation_zones WHERE zone_key = ?`, zoneKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &z, nil
}

func (r *ZoneRepo) List() ([]Zone, error) {
	rows, err := r.db.Query(`SELECT ` + zoneColumns + ` FROM irrigation_zones ORDER BY zone_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []Zone
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, err
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}
