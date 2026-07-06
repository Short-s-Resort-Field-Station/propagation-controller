-- Propagation Unit 01 — initial schema (spec §13 + moisture_control_rules addendum)

CREATE TABLE devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    driver TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'unknown',
    config_json TEXT NOT NULL,
    last_seen_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sensor_readings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER NOT NULL,
    sensor_key TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT NOT NULL,
    quality TEXT NOT NULL DEFAULT 'good',
    read_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(device_id) REFERENCES devices(id)
);

CREATE INDEX idx_sensor_readings_sensor_time
ON sensor_readings(sensor_key, read_at);

CREATE TABLE sensor_hourly_aggregates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sensor_key TEXT NOT NULL,
    hour_start DATETIME NOT NULL,
    min_value REAL,
    max_value REAL,
    avg_value REAL,
    sample_count INTEGER NOT NULL,
    unit TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(sensor_key, hour_start)
);

CREATE TABLE relay_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    relay_channel INTEGER NOT NULL,
    device_key TEXT,
    previous_state TEXT,
    requested_state TEXT NOT NULL,
    actual_state TEXT,
    actor TEXT NOT NULL,
    reason TEXT,
    success BOOLEAN NOT NULL,
    error_message TEXT,
    occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_relay_actions_time ON relay_actions(occurred_at);

CREATE TABLE irrigation_zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    valve_relay_channel INTEGER NOT NULL,
    soil_sensor_key TEXT,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    max_flood_seconds INTEGER NOT NULL DEFAULT 300,
    max_pump_seconds INTEGER NOT NULL DEFAULT 300,
    min_reservoir_level REAL,
    config_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE irrigation_cycles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id INTEGER NOT NULL,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    requested_duration_seconds INTEGER,
    actual_duration_seconds INTEGER,
    trigger_type TEXT NOT NULL,
    trigger_source TEXT,
    reservoir_level_start REAL,
    reservoir_level_end REAL,
    soil_moisture_start REAL,
    soil_moisture_end REAL,
    result TEXT NOT NULL,
    abort_reason TEXT,
    error_message TEXT,
    FOREIGN KEY(zone_id) REFERENCES irrigation_zones(id)
);

CREATE TABLE schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    schedule_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    timezone TEXT NOT NULL DEFAULT 'America/Chicago',
    rule_json TEXT NOT NULL,
    missed_run_policy TEXT NOT NULL DEFAULT 'skip',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE system_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    details_json TEXT,
    actor TEXT,
    occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_system_events_time ON system_events(occurred_at);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE moisture_control_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 0,
    sensor_key TEXT NOT NULL,
    trigger_below REAL NOT NULL,
    recovered_above REAL,
    run_duration_seconds INTEGER NOT NULL,
    minimum_interval_minutes INTEGER NOT NULL DEFAULT 240,
    max_cycles_per_day INTEGER,
    sensor_offline_behavior TEXT NOT NULL DEFAULT 'lockout',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(zone_id) REFERENCES irrigation_zones(id)
);
