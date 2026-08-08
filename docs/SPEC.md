# Propagation Unit 01

# Software Architecture & Technical Specification

## Version 1.0

---

## 1. System Overview

Propagation Unit 01 is an embedded Linux-based industrial control appliance for automated plant propagation, irrigation, lighting, sensing, logging, and local/remote operation.

The system is designed as a reusable embedded automation platform, not a single-purpose Raspberry Pi script or Home Assistant integration.

The architecture should support:

* Deterministic device control
* Local-first operation
* No cloud dependency
* Web-based remote management
* Touchscreen operation at the unit
* Modular drivers
* Configurable automation logic
* Industrial-style safety limits
* Long-term reuse across future units

Target future deployments include:

* Propagation Unit 02
* Water Trailer
* Greenhouse
* Weather Station
* Nursery
* Seed drying cabinet

---

# 2. Recommended Technology Stack

## 2.1 Backend Language

Recommended backend language: **Go**

### Why Go

Go is the best fit for Version 1.0 because it provides:

* Excellent Linux support
* Strong concurrency model
* Simple static binaries
* Easy systemd deployment
* Good serial/USB support
* Good SQLite support
* Good REST API tooling
* Easier development velocity than Rust
* More production discipline than Python for long-running appliance software

Rust would also be a strong long-term choice, but Go is likely the best balance of reliability, speed of development, and maintainability.

Python should be avoided for the core control system unless rapid prototyping is needed first.

## 2.2 Backend Libraries

Recommended Go libraries:

* HTTP API: `net/http`, `chi`, or `gin`
* SQLite: `modernc.org/sqlite` or `mattn/go-sqlite3`
* Migrations: `golang-migrate`
* Serial: `go.bug.st/serial`
* Modbus RTU: `github.com/goburrow/modbus`
* Logging: `zap` or `zerolog`
* Config validation: `go-playground/validator`
* Background jobs: internal scheduler package
* Metrics: Prometheus-compatible endpoint

## 2.3 Frontend Stack

Recommended frontend:

* TypeScript
* React
* Vite
* TanStack Query
* Tailwind CSS
* Recharts or Apache ECharts
* Zustand or Redux Toolkit for local UI state

The frontend should be a single web application with responsive layouts, but the **touchscreen UI should be a distinct mode**, not simply the same desktop UI squeezed onto the screen.

## 2.4 Touchscreen UI

Recommended approach:

* Run a local kiosk browser on the Piconium Mini
* Load `http://localhost/ui/touch`
* Use the same backend REST API as the web app
* Launch automatically using systemd
* Lock into fullscreen kiosk mode

Possible kiosk tools:

* Chromium kiosk mode
* Cage + Chromium
* Weston + Chromium
* X11 + unclutter + Chromium

## 2.5 Database

Initial database:

* SQLite

Future-compatible with:

* PostgreSQL

Design rule:

The application should avoid SQLite-specific assumptions where possible. Use clean repository interfaces so the persistence layer can later be swapped.

---

# 3. High-Level Architecture

## 3.1 Major Components

The system should be divided into the following services/modules:

1. **API Server**

   * REST API
   * WebSocket/SSE event stream
   * Serves frontend static assets
   * Authentication and session management

2. **Device Manager**

   * Owns hardware drivers
   * Manages relay controller
   * Manages Modbus RTU bus
   * Tracks device status and health

3. **Automation Engine**

   * Runs irrigation schedules
   * Runs lighting schedules
   * Applies safety rules
   * Handles manual overrides

4. **Scheduler**

   * Time-based jobs
   * Polling intervals
   * Retention jobs
   * Aggregation jobs

5. **Safety Supervisor**

   * Pump timeout enforcement
   * Max flood duration enforcement
   * Low reservoir lockout
   * Sensor offline detection
   * Safe startup state
   * Emergency stop state

6. **Data Logger**

   * Sensor readings
   * Relay actions
   * Irrigation cycles
   * Errors
   * Warnings
   * System events

7. **Configuration Service**

   * Stores all user-editable settings
   * Provides validation
   * Tracks config revisions
   * Avoids manual config files

8. **UI Applications**

   * Desktop web UI
   * Touchscreen UI
   * Same backend API
   * Different interaction model

9. **Watchdog / Process Supervisor**

   * systemd
   * Optional independent watchdog process
   * Health endpoint monitoring
   * Auto-restart on failure

---

# 4. Runtime Process Model

Recommended process layout:

```text
propagationd
  ├── REST API server
  ├── scheduler
  ├── automation engine
  ├── device manager
  ├── safety supervisor
  ├── data logger
  └── embedded frontend assets

propagation-watchdog
  ├── monitors propagationd health endpoint
  ├── verifies scheduler heartbeat
  ├── verifies database write health
  └── can trigger service restart or safe relay state

chromium-kiosk
  └── launches touchscreen UI at localhost
```

The system should be installable as:

```text
/etc/systemd/system/propagationd.service
/etc/systemd/system/propagation-watchdog.service
/etc/systemd/system/propagation-kiosk.service
```

---

# 5. Directory Structure

```text
propagation-controller/
  README.md
  Makefile
  go.mod
  go.sum

  cmd/
    propagationd/
      main.go
    propagation-watchdog/
      main.go
    propagation-cli/
      main.go

  internal/
    api/
      router.go
      middleware.go
      handlers/
        health.go
        devices.go
        relays.go
        sensors.go
        irrigation.go
        lighting.go
        schedules.go
        events.go
        settings.go
        diagnostics.go
        auth.go

    app/
      app.go
      lifecycle.go
      dependencies.go

    automation/
      engine.go
      irrigation.go
      lighting.go
      manual_override.go
      rules.go

    scheduler/
      scheduler.go
      jobs.go
      retention.go
      aggregation.go

    safety/
      supervisor.go
      rules.go
      startup.go
      lockouts.go
      emergency_stop.go

    devices/
      manager.go
      registry.go
      health.go

      relay/
        relay.go
        numato_usb.go
        mock.go

      modbus/
        bus.go
        rtu.go
        registry.go
        mock.go

      sensors/
        sensor.go
        soil_moisture.go
        soil_temperature.go
        reservoir_level.go
        generic_modbus.go

    storage/
      db.go
      migrations.go
      repositories/
        readings.go
        relays.go
        irrigation.go
        schedules.go
        settings.go
        events.go
        devices.go

    config/
      schema.go
      validation.go
      defaults.go

    logging/
      logger.go
      event_logger.go

    metrics/
      prometheus.go
      heartbeat.go

    auth/
      users.go
      sessions.go
      permissions.go

  migrations/
    001_initial_schema.sql
    002_hourly_aggregates.sql
    003_device_registry.sql

  web/
    package.json
    vite.config.ts
    src/
      api/
      components/
      pages/
        desktop/
        touch/
      layouts/
      charts/
      state/
      styles/

  deploy/
    systemd/
      propagationd.service
      propagation-watchdog.service
      propagation-kiosk.service

    install.sh
    update.sh
    backup.sh

  docs/
    api.md
    hardware.md
    safety.md
    plugin-development.md
    deployment.md

  tests/
    integration/
    hardware/
    simulation/
```

---

# 6. Device Model

All physical hardware should be represented as registered devices.

## 6.1 Device Types

```text
relay_controller
relay_channel
modbus_bus
modbus_sensor
pump
valve
light
reservoir_sensor
soil_sensor
```

## 6.2 Device States

Each device should report:

```text
unknown
online
offline
error
disabled
simulated
maintenance
```

## 6.3 Device Health Fields

Each device should track:

* Last seen timestamp
* Last successful command
* Last error
* Error count
* Online/offline state
* Firmware or identification data, where available
* Config revision

---

# 7. Relay Architecture

The Numato USB relay controller should be abstracted behind a relay driver interface.

## 7.1 Relay Assignments

| Relay | Device                |
| ----: | --------------------- |
|     1 | Barina grow lights    |
|     2 | SEAFLO diaphragm pump |
|     3 | Solenoid Valve 1      |
|     4 | Solenoid Valve 2      |
|     5 | Solenoid Valve 3      |
|     6 | Solenoid Valve 4      |
|     7 | Spare                 |
|     8 | Spare                 |

## 7.2 Relay Driver Interface

```go
type RelayController interface {
    Open(ctx context.Context) error
    Close() error
    SetChannel(ctx context.Context, channel int, state RelayState) error
    GetChannel(ctx context.Context, channel int) (RelayState, error)
    GetAll(ctx context.Context) (map[int]RelayState, error)
    Health(ctx context.Context) DeviceHealth
}
```

## 7.3 Relay State

```text
on
off
unknown
error
```

## 7.4 Relay Action Logging

Every relay action should create a durable event:

```text
timestamp
relay_channel
device_name
previous_state
requested_state
actual_state
actor
reason
success
error_message
```

Actors:

```text
system
schedule
manual_web
manual_touch
safety_supervisor
watchdog
startup
api
```

---

# 8. Modbus Sensor Architecture

All RS485 sensors should use a shared Modbus RTU bus.

## 8.1 Sensor Configuration

Each sensor should have configurable:

* Name
* Modbus address
* Baud rate
* Parity
* Stop bits
* Polling interval
* Register map
* Data type
* Scale factor
* Offset
* Engineering unit
* Calibration values
* Offline timeout
* Enabled/disabled state

## 8.2 Sensor Driver Interface

```go
type SensorDriver interface {
    Read(ctx context.Context) ([]SensorReading, error)
    Health(ctx context.Context) DeviceHealth
    Metadata() SensorMetadata
}
```

## 8.3 Generic Modbus Sensor

A generic Modbus sensor driver should support future sensors without code changes.

Example configurable fields:

```json
{
  "name": "Reservoir Level",
  "type": "reservoir_level",
  "modbus_address": 3,
  "registers": [
    {
      "name": "level",
      "register": 0,
      "function_code": 3,
      "data_type": "uint16",
      "scale": 0.1,
      "offset": 0,
      "unit": "%"
    }
  ],
  "poll_interval_seconds": 10
}
```

## 8.4 Current Sensors

Initial supported sensors:

* Soil moisture
* Soil temperature
* Soil EC (substrate conductivity, µS/cm — ZTS-3000 register 0x0002)
* Reservoir level

Future sensor types (would need added or different hardware — the current ZTS-3000 probe has no pH channel):

* Water temperature
* Air temperature
* Air humidity
* pH
* CO₂
* PAR

---

# 9. Irrigation Architecture

## 9.1 Irrigation Zones

The system supports four independent irrigation zones.

Each zone should have:

* Zone ID
* Name
* Assigned valve relay
* Optional soil sensor association
* Enabled/disabled state
* Schedule rules
* Maximum flood duration
* Drain/rest duration
* Minimum reservoir level
* Lockout rules
* Manual override permissions

## 9.2 Irrigation Cycle State Machine

```text
idle
precheck
starting_pump
opening_valve
flooding
holding
closing_valve
stopping_pump
draining
complete
aborted
failed
locked_out
```

## 9.3 Irrigation Prechecks

Before starting a cycle:

* Reservoir level must be above configured minimum
* Pump must not already be locked out
* Zone must be enabled
* Valve relay must be available
* Pump relay must be available
* No emergency stop active
* No conflicting irrigation cycle active unless multi-zone watering is explicitly allowed

## 9.4 Irrigation Safety Limits

Required limits:

* Maximum flood duration
* Maximum pump runtime
* Minimum reservoir level
* Maximum cycles per hour
* Sensor offline lockout, optional per zone
* Manual run timeout
* Automatic stop on controller shutdown
* Safe relay state after reboot

## 9.5 Irrigation Cycle Logging

Each cycle should log:

```text
cycle_id
zone_id
started_at
ended_at
requested_duration
actual_duration
trigger_type
trigger_source
reservoir_level_start
reservoir_level_end
soil_moisture_start
soil_moisture_end
result
abort_reason
error_message
```

Trigger types:

```text
schedule
manual_web
manual_touch
api
test
automation_rule
```

---

# 10. Lighting Architecture

## 10.1 Current Lighting Mode

Version 1.0 supports:

* Lights on
* Lights off
* Fixed daily schedule

## 10.2 Future Lighting Modes

The lighting module should be designed to support:

* Sunrise ramp
* Sunset ramp
* Seasonal photoperiod
* Manual maintenance override
* Temporary pause
* Holiday/vacation mode

## 10.3 Lighting State Machine

```text
off
scheduled_on
manual_on
manual_off
maintenance
fault
```

## 10.4 Lighting Schedule

Each schedule should support:

* Enabled/disabled
* On time
* Off time
* Days of week
* Timezone
* Manual override expiration

---

# 11. Scheduler Architecture

The scheduler should be internal to the backend service.

It should support:

* Sensor polling jobs
* Irrigation schedules
* Lighting schedules
* Data aggregation jobs
* Data retention cleanup
* Device health checks
* Watchdog heartbeat updates

## 11.1 Scheduler Requirements

* Persistent schedule definitions
* Runtime schedule reload after UI changes
* No manual service restart required
* Job-level logging
* Missed-job handling after power loss
* Safe startup behavior

## 11.2 Missed Schedule Behavior

Each schedule should define what happens if the controller was offline:

```text
skip
run_immediately_if_within_grace_period
run_next_occurrence_only
manual_review_required
```

For irrigation, default should be:

```text
skip
```

For lighting, default should be:

```text
restore_current_expected_state
```

---

# 12. Safety Supervisor

The safety supervisor should run independently inside the backend process and optionally be monitored by a separate watchdog process.

## 12.1 Safety Rules

Required rules:

1. Pump cannot run longer than configured maximum runtime.
2. Any irrigation cycle exceeding maximum flood duration is aborted.
3. Pump cannot run if reservoir level is below minimum.
4. If reservoir sensor is offline, irrigation is locked out unless override is explicitly enabled.
5. If relay controller is offline, all automation is suspended.
6. If Modbus bus is offline, sensor-dependent automation is suspended.
7. Manual pump operation requires timeout.
8. Startup state is configurable but defaults to all relays off.
9. Emergency stop forces all relays off.
10. Watchdog restart should force safe relay state.

## 12.2 Safe Startup State

Default startup state:

```text
lights: restore scheduled state
pump: off
valves: off
spares: off
automation: enabled after health checks
```

Startup sequence:

1. Open database
2. Start logger
3. Load configuration
4. Initialize relay controller
5. Force pump and valves off
6. Initialize Modbus bus
7. Poll reservoir sensor
8. Start safety supervisor
9. Start scheduler
10. Start API server
11. Start touchscreen UI

## 12.3 Emergency Stop

Emergency stop should be available from:

* Touchscreen
* Web UI
* REST API
* Watchdog process

Emergency stop action:

* Pump off
* All valves off
* Optional lights off depending on configuration
* Automation paused
* Event logged
* Manual reset required

---

# 13. Database Schema

## 13.1 Core Tables

```sql
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
```

```sql
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
```

```sql
CREATE INDEX idx_sensor_readings_sensor_time
ON sensor_readings(sensor_key, read_at);
```

```sql
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
```

```sql
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
```

```sql
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
```

```sql
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
```

```sql
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
```

```sql
CREATE TABLE system_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    details_json TEXT,
    actor TEXT,
    occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

```sql
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

```sql
CREATE TABLE sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id)
);
```

---

# 14. Data Retention

## 14.1 Raw Sensor Data

Raw sensor data retention:

```text
30 days
```

## 14.2 Hourly Aggregates

Hourly aggregates retention:

```text
forever
```

## 14.3 Retention Job

A scheduled retention job should:

1. Aggregate raw readings into hourly summaries.
2. Verify aggregate completeness.
3. Delete raw readings older than 30 days.
4. Log the retention operation.

---

# 15. REST API Specification

Base path:

```text
/api/v1
```

## 15.1 Health

```http
GET /api/v1/health
```

Response:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime_seconds": 12345,
  "database": "ok",
  "relay_controller": "online",
  "modbus_bus": "online",
  "scheduler": "running",
  "safety_supervisor": "running"
}
```

---

## 15.2 Devices

```http
GET /api/v1/devices
```

```http
GET /api/v1/devices/{device_key}
```

```http
PATCH /api/v1/devices/{device_key}
```

```http
POST /api/v1/devices/{device_key}/test
```

---

## 15.3 Sensors

```http
GET /api/v1/sensors
```

```http
GET /api/v1/sensors/{sensor_key}/latest
```

```http
GET /api/v1/sensors/{sensor_key}/readings?from=&to=&interval=
```

```http
POST /api/v1/sensors/{sensor_key}/poll
```

```http
PATCH /api/v1/sensors/{sensor_key}/config
```

Example latest response:

```json
{
  "sensor_key": "soil_moisture_zone_1",
  "name": "Soil Moisture Zone 1",
  "value": 31.4,
  "unit": "%",
  "quality": "good",
  "read_at": "2026-06-29T20:15:00Z",
  "status": "online"
}
```

---

## 15.4 Relays

```http
GET /api/v1/relays
```

```http
GET /api/v1/relays/{channel}
```

```http
POST /api/v1/relays/{channel}/set
```

Request:

```json
{
  "state": "on",
  "reason": "manual test",
  "timeout_seconds": 60
}
```

```http
POST /api/v1/relays/all-off
```

---

## 15.5 Lighting

```http
GET /api/v1/lighting/status
```

```http
POST /api/v1/lighting/on
```

```http
POST /api/v1/lighting/off
```

```http
POST /api/v1/lighting/override
```

```http
GET /api/v1/lighting/schedule
```

```http
PUT /api/v1/lighting/schedule
```

---

## 15.6 Irrigation

```http
GET /api/v1/irrigation/zones
```

```http
GET /api/v1/irrigation/zones/{zone_key}
```

```http
PATCH /api/v1/irrigation/zones/{zone_key}
```

```http
POST /api/v1/irrigation/zones/{zone_key}/run
```

Request:

```json
{
  "duration_seconds": 180,
  "reason": "manual watering"
}
```

```http
POST /api/v1/irrigation/zones/{zone_key}/stop
```

```http
GET /api/v1/irrigation/cycles
```

```http
GET /api/v1/irrigation/cycles/{cycle_id}
```

---

## 15.7 Schedules

```http
GET /api/v1/schedules
```

```http
GET /api/v1/schedules/{schedule_key}
```

```http
POST /api/v1/schedules
```

```http
PUT /api/v1/schedules/{schedule_key}
```

```http
DELETE /api/v1/schedules/{schedule_key}
```

---

## 15.8 Events and Logs

```http
GET /api/v1/events?level=&type=&from=&to=
```

```http
GET /api/v1/relay-actions?from=&to=
```

```http
GET /api/v1/irrigation-cycles?from=&to=
```

---

## 15.9 Settings

```http
GET /api/v1/settings
```

```http
PUT /api/v1/settings
```

---

## 15.10 Safety

```http
GET /api/v1/safety/status
```

```http
POST /api/v1/safety/emergency-stop
```

```http
POST /api/v1/safety/reset
```

```http
GET /api/v1/safety/lockouts
```

```http
POST /api/v1/safety/lockouts/{lockout_key}/clear
```

---

## 15.11 Diagnostics

```http
GET /api/v1/diagnostics/system
```

```http
GET /api/v1/diagnostics/modbus
```

```http
POST /api/v1/diagnostics/modbus/scan
```

```http
GET /api/v1/diagnostics/relay-controller
```

```http
POST /api/v1/diagnostics/relay-controller/test
```

---

# 16. Event Streaming

The UI should receive live updates through either WebSocket or Server-Sent Events.

Recommended for Version 1.0:

```text
Server-Sent Events
```

Endpoint:

```http
GET /api/v1/events/stream
```

Event types:

```text
sensor_reading
relay_action
irrigation_cycle_update
device_status_change
safety_lockout
system_event
```

---

# 17. Web UI Specification

The web UI should feel more like:

```text
Grafana + Ubiquiti
```

and less like:

```text
Home Assistant
```

Design goals:

* Dark mode first
* Minimal
* Fast
* Professional
* Desktop first
* Clear system status
* No childish dashboards
* No excessive cards
* No skeuomorphic controls

## 17.1 Web Navigation

Primary sections:

```text
Dashboard
Irrigation
Lighting
Sensors
Schedules
Events
Diagnostics
Settings
Maintenance
```

---

## 17.2 Web Dashboard Wireframe

```text
┌──────────────────────────────────────────────────────────────┐
│ Propagation Unit 01                         Online  8:42 PM  │
├──────────────────────────────────────────────────────────────┤
│ System Status                                                │
│ ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ │
│ │ Reservoir  │ │ Moisture   │ │ Temp       │ │ Automation │ │
│ │ 82%        │ │ 31.4% avg  │ │ 73.2°F     │ │ Enabled    │ │
│ └────────────┘ └────────────┘ └────────────┘ └────────────┘ │
│                                                              │
│ Active State                                                 │
│ Lights: ON        Pump: OFF        Active Zone: None         │
│                                                              │
│ Irrigation Zones                                             │
│ ┌──────┬──────────────┬──────────┬────────────┬───────────┐ │
│ │ Zone │ Name         │ Moisture │ Last Run   │ Status    │ │
│ ├──────┼──────────────┼──────────┼────────────┼───────────┤ │
│ │ 1    │ Tray 1       │ 32%      │ 2h ago     │ Ready     │ │
│ │ 2    │ Tray 2       │ 29%      │ 4h ago     │ Ready     │ │
│ │ 3    │ Tray 3       │ --       │ Never      │ Disabled  │ │
│ │ 4    │ Tray 4       │ --       │ Never      │ Disabled  │ │
│ └──────┴──────────────┴──────────┴────────────┴───────────┘ │
│                                                              │
│ Recent Events                                                │
│ 20:41  Sensor poll complete                                  │
│ 20:30  Lights turned on by schedule                          │
│ 19:10  Zone 1 irrigation complete                            │
└──────────────────────────────────────────────────────────────┘
```

---

## 17.3 Irrigation Screen

```text
┌──────────────────────────────────────────────────────────────┐
│ Irrigation                                                   │
├──────────────────────────────────────────────────────────────┤
│ Global Status: Ready                                         │
│ Reservoir: 82%                                               │
│ Pump: Off                                                    │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Zone 1 - Tray 1                                          │ │
│ │ Moisture: 32%      Last Run: 2h ago       Status: Ready  │ │
│ │ [Run 1 min] [Run 3 min] [Custom Run] [Stop]              │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Zone 2 - Tray 2                                          │ │
│ │ Moisture: 29%      Last Run: 4h ago       Status: Ready  │ │
│ │ [Run 1 min] [Run 3 min] [Custom Run] [Stop]              │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ Schedule Table                                               │
│ Zone | Days | Start | Duration | Enabled                     │
└──────────────────────────────────────────────────────────────┘
```

---

## 17.4 Diagnostics Screen

```text
┌──────────────────────────────────────────────────────────────┐
│ Diagnostics                                                  │
├──────────────────────────────────────────────────────────────┤
│ Relay Controller                                             │
│ Status: Online                                               │
│ Port: /dev/serial/by-id/usb-Numato...                        │
│ [Test Relay 1] [Test Relay 2] ...                            │
│                                                              │
│ Modbus RTU                                                   │
│ Status: Online                                               │
│ Adapter: /dev/serial/by-id/usb-FTDI...                       │
│ Baud: 4800                                                   │
│ [Scan Bus] [Poll All Sensors]                                │
│                                                              │
│ Sensors                                                      │
│ Sensor              Address  Last Seen   Status              │
│ Soil Moisture        1       5s ago      Online              │
│ Soil Temp            2       5s ago      Online              │
│ Reservoir Level      3       10s ago     Online              │
└──────────────────────────────────────────────────────────────┘
```

---

# 18. Touchscreen UI Specification

The touchscreen UI is not a copy of the web UI.

It should be optimized for:

* Standing in front of the rack
* Fast glances
* Big buttons
* Minimal typing
* Manual control
* Diagnostics
* Maintenance

## 18.1 Touch Navigation

Primary touchscreen screens:

```text
Home
Manual
Schedules
Sensors
Graphs
Settings
Maintenance
Emergency Stop
```

---

## 18.2 Touch Home Wireframe

```text
┌──────────────────────────────┐
│ Propagation Unit 01   Online │
├──────────────────────────────┤
│ Reservoir                    │
│ 82%                          │
│                              │
│ Moisture Avg                 │
│ 31.4%                        │
│                              │
│ Temp                         │
│ 73.2°F                       │
│                              │
│ Lights: ON                   │
│ Pump: OFF                    │
│ Active Zone: None            │
│                              │
│ [Manual] [Schedules]         │
│ [Sensors] [Graphs]           │
│                              │
│ [EMERGENCY STOP]             │
└──────────────────────────────┘
```

---

## 18.3 Touch Manual Controls

```text
┌──────────────────────────────┐
│ Manual Controls              │
├──────────────────────────────┤
│ Lights                       │
│ [ ON ] [ OFF ]               │
│                              │
│ Pump                         │
│ [ Run 10s ] [ Run 30s ]      │
│                              │
│ Zones                        │
│ [ Zone 1: 1 min ]            │
│ [ Zone 2: 1 min ]            │
│ [ Zone 3: 1 min ]            │
│ [ Zone 4: 1 min ]            │
│                              │
│ [Stop All]                   │
└──────────────────────────────┘
```

Manual controls should always use bounded timeouts.

No touchscreen button should allow indefinite pump operation.

---

# 19. Configuration Model

Everything should be configurable through the UI.

No manual configuration file editing should be required.

## 19.1 Configurable Items

* Device names
* Relay assignments
* Sensor Modbus addresses
* Sensor polling intervals
* Sensor calibration
* Engineering units
* Irrigation zone names
* Irrigation schedules
* Max flood duration
* Pump timeout
* Reservoir lockout level
* Lighting schedule
* Startup behavior
* Timezone
* Data retention
* Network display settings
* Touchscreen preferences

## 19.2 Configuration Storage

Configuration should be stored in the database as structured JSON where flexibility is needed, but important operational entities should have their own relational tables.

Examples:

* Devices: table + JSON config
* Schedules: table + JSON rule
* Settings: key-value JSON
* Sensor readings: relational time-series rows

## 19.3 Config Revisioning

Each major config update should log:

```text
timestamp
actor
config_area
previous_value
new_value
validation_result
```

---

# 20. Authentication and Access Control

Version 1.0 should support local authentication.

## 20.1 User Roles

```text
admin
operator
viewer
```

## 20.2 Permissions

Viewer:

* View dashboard
* View readings
* View logs

Operator:

* Manual controls
* Run irrigation
* Toggle lights
* View diagnostics

Admin:

* Change settings
* Edit schedules
* Configure devices
* Manage users
* Reset safety lockouts

## 20.3 Initial Setup

On first boot:

1. Generate local setup token.
2. Display setup URL on touchscreen.
3. Require admin account creation.
4. Store password hash securely.

---

# 21. Deployment Architecture

## 21.1 Installation

Install location:

```text
/opt/propagation-controller
```

Data location:

```text
/var/lib/propagation-controller
```

Logs:

```text
/var/log/propagation-controller
```

Config/environment:

```text
/etc/propagation-controller
```

## 21.2 Systemd Service

```ini
[Unit]
Description=Propagation Controller
After=network.target

[Service]
ExecStart=/opt/propagation-controller/propagationd
Restart=always
RestartSec=3
User=propagation
Group=propagation
WorkingDirectory=/var/lib/propagation-controller

[Install]
WantedBy=multi-user.target
```

## 21.3 Startup Behavior After Power Loss

After power loss:

1. System boots.
2. systemd starts backend.
3. Backend initializes hardware.
4. Pump and valves forced off.
5. Current schedule state restored.
6. Missed irrigation runs skipped by default.
7. Event logged: `power_recovery`.
8. UI returns to dashboard.

---

# 22. Driver Architecture

## 22.1 Driver Registry

Each driver should register with a central registry.

```go
type DriverFactory interface {
    Type() string
    Create(config DeviceConfig) (DeviceDriver, error)
}
```

## 22.2 Device Driver Interface

```go
type DeviceDriver interface {
    Open(ctx context.Context) error
    Close() error
    Health(ctx context.Context) DeviceHealth
}
```

## 22.3 Plugin-Ready Drivers

Initial drivers should be compiled into the binary.

Future plugin support should allow:

* New Modbus sensors
* New USB relay controllers
* New power monitors
* New environmental sensors
* New automation modules
* New UI panels

---

# 23. Future Plugin Architecture

Version 1.0 should not require external dynamic plugins, but the architecture should be plugin-ready.

## 23.1 Plugin Types

```text
hardware_driver
sensor_decoder
automation_module
schedule_rule
ui_panel
exporter
alert_sink
```

## 23.2 Plugin Manifest

Future plugins should define:

```json
{
  "id": "sensor.par.modbus.v1",
  "name": "Modbus PAR Sensor",
  "version": "1.0.0",
  "type": "sensor_driver",
  "capabilities": ["modbus_rtu", "sensor_reading"],
  "config_schema": {},
  "api_routes": [],
  "ui_panels": []
}
```

## 23.3 Recommended Plugin Strategy

Phase 1:

* Internal Go interfaces only

Phase 2:

* JSON-configurable generic Modbus devices

Phase 3:

* External process plugins over local RPC

Phase 4:

* Signed plugin bundles

Avoid Go dynamic plugins for this product because they complicate deployment and compatibility.

---

# 24. Testing Strategy

## 24.1 Unit Tests

Required unit test coverage:

* Relay driver command formatting
* Modbus decoding
* Sensor calibration
* Irrigation state machine
* Safety lockouts
* Schedule evaluation
* API validation
* Database repositories

## 24.2 Integration Tests

Integration tests should cover:

* SQLite migrations
* API endpoints
* Scheduler execution
* Sensor polling loop
* Relay action logging
* Irrigation cycle logging
* Emergency stop behavior

## 24.3 Hardware-in-the-Loop Tests

Hardware tests should include:

* Numato relay detection
* Relay on/off command
* FTDI RS485 adapter detection
* Modbus sensor polling
* Sensor offline simulation
* Relay controller disconnect
* Power loss/reboot recovery
* Pump timeout behavior
* Reservoir lockout behavior

## 24.4 Simulation Mode

The system should support simulation mode.

Simulation mode allows development without hardware.

Simulated devices:

* Relay controller
* Soil moisture sensor
* Soil temperature sensor
* Reservoir level sensor
* Pump
* Valves
* Lights

Simulation mode should be selectable at startup and visible in the UI.

## 24.5 Safety Tests

Required safety scenarios:

1. Pump relay stuck on command failure.
2. Modbus bus offline.
3. Reservoir sensor offline.
4. Reservoir level below minimum.
5. Irrigation cycle exceeds max flood duration.
6. Manual pump command exceeds timeout.
7. Controller reboots during active irrigation.
8. Relay controller unplugged.
9. Database unavailable.
10. Emergency stop triggered.

---

# 25. Development Roadmap

## Phase 0 — Product Foundation

Goals:

* Repository setup
* Go backend skeleton
* SQLite migrations
* Basic REST API
* Systemd deployment
* Logging framework
* Configuration foundation

Deliverables:

* Running backend service
* Health endpoint
* Database initialized
* Basic web shell
* Touchscreen shell

---

## Phase 1 — Hardware Control

Goals:

* Numato relay driver
* Relay API
* Relay logging
* FTDI serial detection
* Modbus RTU bus abstraction
* Basic sensor polling

Deliverables:

* Relays controllable through API
* Relays controllable through web UI
* Modbus sensors visible in diagnostics
* Sensor readings stored in SQLite

---

## Phase 2 — Safety Foundation

Goals:

* Safe startup state
* Pump timeout
* Max flood duration
* Reservoir lockout
* Sensor offline detection
* Emergency stop

Deliverables:

* Safety supervisor
* Safety status API
* Touchscreen emergency stop
* Web emergency stop
* Safety event logging

---

## Phase 3 — Irrigation Automation

Goals:

* Four-zone irrigation model
* Manual zone runs
* Scheduled irrigation
* Irrigation cycle state machine
* Irrigation cycle history

Deliverables:

* Independent zone configuration
* Manual bounded runs
* Schedule-based watering
* Cycle logs
* Abort/failure handling

---

## Phase 4 — Lighting Automation

Goals:

* Light on/off control
* Daily schedule
* Manual override
* Restore expected state after reboot

Deliverables:

* Lighting dashboard
* Lighting schedule editor
* Lighting event history

---

## Phase 5 — Web UI

Goals:

* Desktop dashboard
* Irrigation page
* Lighting page
* Sensor page
* Graphs
* Events
* Diagnostics
* Settings

Deliverables:

* Professional local web UI
* Dark mode
* Live updates
* Historical charts

---

## Phase 6 — Touchscreen UI

Goals:

* Kiosk boot
* Touch home screen
* Manual controls
* Schedules
* Sensor diagnostics
* Maintenance screen

Deliverables:

* Local touchscreen UI
* Large-button workflow
* No keyboard dependency for normal use

---

## Phase 7 — Data Retention and Aggregation

Goals:

* Raw readings retained for 30 days
* Hourly averages retained forever
* Aggregation job
* Retention cleanup

Deliverables:

* Hourly sensor history
* Long-term graph support
* Storage usage control

---

## Phase 8 — Production Hardening

Goals:

* Watchdog process
* Installer
* Backup/restore
* Update script
* User authentication
* Role permissions
* Hardware-in-loop test suite

Deliverables:

* Appliance-style install
* Recoverable reboot behavior
* Admin setup flow
* Production documentation

---

# 26. Version 1.0 Minimum Viable Product

The first complete version should include:

* Go backend
* SQLite database
* REST API
* Numato relay control
* FTDI Modbus RTU support
* Soil moisture sensor
* Soil temperature sensor
* Reservoir level sensor
* Four irrigation zones
* Manual irrigation
* Scheduled irrigation
* Lighting schedule
* Web dashboard
* Touchscreen dashboard
* Safety supervisor
* Emergency stop
* Event logs
* Raw data retention
* Hourly aggregates
* systemd deployment
* Simulation mode

---

# 27. Non-Goals for Version 1.0

Do not include in Version 1.0:

* Cloud sync
* Home Assistant integration
* MQTT dependency
* Arduino firmware
* GPIO assumptions
* AI plant diagnosis
* Camera analysis
* Multi-unit fleet management
* PostgreSQL requirement
* Mobile app
* Dynamic external plugins
* Complex rule builder

These can be added later after the core control appliance is stable.

---

# 28. Engineering Principles

The system should follow these principles:

1. **Local-first**

   * The unit must work without Internet.

2. **Hardware abstraction**

   * Automation logic should not know USB command details.

3. **Safety before convenience**

   * Pump and valves should fail safe.

4. **Everything observable**

   * Relay actions, sensor polls, and errors should be logged.

5. **Everything configurable**

   * UI configuration should replace manual file editing.

6. **Touchscreen is operational**

   * Web UI is administrative.
   * Touch UI is for standing at the rack.

7. **Simulation is required**

   * Development should not require live hardware.

8. **Modular expansion**

   * Future units should reuse the core system.

9. **No hidden automation**

   * Every automatic action should have a visible cause.

10. **Commercial appliance mindset**

* Boot behavior, failure recovery, logs, updates, and safety matter.

---

# 29. Recommended Next Implementation Step

The first engineering milestone should be:

```text
Bring up propagationd with SQLite, health API, relay driver, relay API, event logging, and a minimal web dashboard.
```

This proves:

* Linux service deployment
* Database persistence
* USB relay communication
* API structure
* Logging model
* UI-to-backend control loop

Only after that should Modbus sensors and irrigation automation be layered in.

---

# 30. Summary

Propagation Unit 01 should be implemented as a local-first embedded Linux control appliance with a Go backend, SQLite storage, USB relay control, Modbus RTU sensor polling, REST APIs, a professional web UI, and a purpose-built touchscreen interface.

The core system should be modular enough to support additional sensors, automation modules, and future hardware products without rewriting the foundation.

Version 1.0 should prioritize reliability, safety, observability, and maintainability over unnecessary features.

## Sensor-Triggered Irrigation

Each irrigation zone should support automatic irrigation based on soil moisture readings.

Every zone may be assigned one or more soil moisture sensors. The system should allow the user to configure the target moisture behavior through the UI.

### Configurable Per-Zone Settings

Each irrigation zone should support:

* Soil moisture sensor assignment
* Low moisture trigger threshold
* Target moisture level, optional
* Minimum time between irrigation cycles
* Maximum irrigation cycles per day
* Irrigation duration per trigger
* Maximum flood duration
* Reservoir minimum level lockout
* Sensor offline behavior
* Manual enable/disable for sensor-triggered watering

### Example

Zone 1 may be configured as:

```text
Zone: Tray 1
Assigned sensor: Soil Moisture Sensor 1
Trigger when moisture falls below: 28%
Run irrigation for: 90 seconds
Minimum time between runs: 4 hours
Maximum runs per day: 3
Reservoir lockout below: 20%
If sensor offline: do not irrigate
```

### Sensor-Triggered Irrigation Logic

The automation engine should evaluate soil moisture readings on every sensor poll.

If a zone is enabled and the latest valid soil moisture value is below the configured trigger threshold, the system should request an irrigation cycle.

Before running the cycle, the safety supervisor must verify:

1. Reservoir level is above the configured minimum.
2. Pump is available.
3. Valve relay is available.
4. The zone is not locked out.
5. The minimum time between cycles has passed.
6. The zone has not exceeded its maximum daily cycle count.
7. The assigned sensor reading is recent and valid.
8. Emergency stop is not active.

If all checks pass, the irrigation cycle runs for the configured duration.

### Hysteresis / Anti-Chatter

The system should avoid repeatedly cycling the pump when moisture is near the threshold.

Each zone should support optional hysteresis:

```text
Trigger below: 28%
Consider recovered above: 34%
```

This means the zone becomes eligible for watering when moisture falls below 28%, but it will not be considered fully recovered until moisture rises above 34%.

### Manual Threshold Control

The low-moisture trigger value must be editable through both the web UI and, at minimum, viewable from the touchscreen UI.

The web UI should allow full configuration.

The touchscreen UI should allow quick adjustment of common values if enabled by the admin.

### Logging

Every sensor-triggered irrigation event should log:

* Zone
* Sensor that triggered the cycle
* Moisture value at trigger time
* Configured threshold
* Irrigation duration
* Reservoir level at start
* Reservoir level at end
* Result
* Safety checks passed or failed
* Lockout reason, if blocked

### API Additions

```http
GET /api/v1/irrigation/zones/{zone_key}/moisture-control
```

```http
PUT /api/v1/irrigation/zones/{zone_key}/moisture-control
```

Example request:

```json
{
  "enabled": true,
  "sensor_key": "soil_moisture_zone_1",
  "trigger_below": 28.0,
  "recovered_above": 34.0,
  "run_duration_seconds": 90,
  "minimum_interval_minutes": 240,
  "max_cycles_per_day": 3,
  "sensor_offline_behavior": "lockout"
}
```

### Database Addition

```sql
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
```

### Updated Irrigation Trigger Types

The irrigation cycle trigger types should include:

```text
schedule
manual_web
manual_touch
api
test
moisture_threshold
automation_rule
```
