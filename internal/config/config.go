package config

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds process-level settings. Everything operational (schedules,
// zones, calibration, ...) lives in the database per the spec; this is only
// what the daemon needs before the database is open.
type Config struct {
	ListenAddr string
	DataDir    string
	Simulation bool
	// PowerOffCmd is what the touch-screen shutdown runs once the unit is safe.
	PowerOffCmd string
	RelayPort   string
	ModbusPort  string
	ModbusBaud  int
	LogLevel    string
	LogPretty   bool
}

func Load() Config {
	var c Config
	flag.StringVar(&c.ListenAddr, "listen", envOr("PROPAGATION_LISTEN", ":8080"), "HTTP listen address")
	flag.StringVar(&c.DataDir, "data-dir", envOr("PROPAGATION_DATA_DIR", "./data"), "directory for database and runtime data")
	flag.BoolVar(&c.Simulation, "simulation", envBool("PROPAGATION_SIMULATION"), "run with simulated hardware drivers")
	flag.StringVar(&c.RelayPort, "relay-port", envOr("PROPAGATION_RELAY_PORT", ""), "serial port of the Numato relay controller (autodetected if empty)")
	flag.StringVar(&c.ModbusPort, "modbus-port", envOr("PROPAGATION_MODBUS_PORT", ""), "serial port of the RS485 adapter (autodetected if empty)")
	// The ZTS-3000 soil probes ship at 4800 baud (verified against the real
	// sensor 2026-07-10) — a 9600 default would show them as silently offline.
	flag.IntVar(&c.ModbusBaud, "modbus-baud", envIntOr("PROPAGATION_MODBUS_BAUD", 9600), "Modbus RTU baud rate (bus runs at the level transmitter's factory 9600; soil probes set to match)")
	flag.StringVar(&c.PowerOffCmd, "poweroff-cmd", envOr("PROPAGATION_POWEROFF_CMD", "sudo -n /usr/bin/systemctl poweroff"), "command run by POST /system/shutdown after relays are off (needs a sudoers rule; see deploy/sudoers)")
	flag.StringVar(&c.LogLevel, "log-level", envOr("PROPAGATION_LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	flag.BoolVar(&c.LogPretty, "log-pretty", envBool("PROPAGATION_LOG_PRETTY"), "human-readable console logs instead of JSON")
	flag.Parse()
	return c
}

func (c Config) DatabasePath() string {
	return filepath.Join(c.DataDir, "propagation.db")
}

func (c Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0o755)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && v
}

func envIntOr(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return fallback
}
