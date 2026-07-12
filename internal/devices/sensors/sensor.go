// Package sensors implements the generic, JSON-configured Modbus sensor
// driver (spec §8.3) and the polling service. New sensors are added with
// configuration, not code.
package sensors

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"propagation-controller/internal/devices/modbus"
)

// RegisterSpec describes one measurement within a sensor (spec §8.3 example).
type RegisterSpec struct {
	Name         string  `json:"name"`
	Register     uint16  `json:"register"`
	FunctionCode uint8   `json:"function_code,omitempty"` // default 3
	DataType     string  `json:"data_type"`               // uint16|int16|uint32|int32|float32
	Scale        float64 `json:"scale,omitempty"`         // default 1
	Offset       float64 `json:"offset,omitempty"`
	Unit         string  `json:"unit"`
	SensorKey    string  `json:"sensor_key,omitempty"` // default <device_key>_<name>
	// ClipAt marks readings at/above this scaled value as quality "clipped" —
	// the sensor is pinned at its range ceiling and the true value is higher
	// (e.g. the ZTS-3000 EC channel tops out at exactly 20000 µS/cm).
	ClipAt *float64 `json:"clip_at,omitempty"`
}

// Config is the device config_json payload for a generic_modbus sensor.
type Config struct {
	ModbusAddress        uint8          `json:"modbus_address"`
	PollIntervalSeconds  int            `json:"poll_interval_seconds"`
	OfflineAfterFailures int            `json:"offline_after_failures,omitempty"` // default 3
	Registers            []RegisterSpec `json:"registers"`
}

func (c Config) Validate() error {
	if c.ModbusAddress < 1 || c.ModbusAddress > 247 {
		return fmt.Errorf("modbus_address must be 1..247")
	}
	if c.PollIntervalSeconds < 1 {
		return fmt.Errorf("poll_interval_seconds must be >= 1")
	}
	if len(c.Registers) == 0 {
		return fmt.Errorf("at least one register is required")
	}
	for _, r := range c.Registers {
		if r.Name == "" {
			return fmt.Errorf("register name is required")
		}
		if _, err := registerQuantity(r.DataType); err != nil {
			return err
		}
		if r.FunctionCode != 0 && r.FunctionCode != 3 && r.FunctionCode != 4 {
			return fmt.Errorf("register %s: function_code must be 3 or 4", r.Name)
		}
	}
	return nil
}

type Reading struct {
	SensorKey string    `json:"sensor_key"`
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit"`
	Quality   string    `json:"quality"`
	ReadAt    time.Time `json:"read_at"`
}

// ReadAll polls every configured register of one sensor over the bus.
func ReadAll(ctx context.Context, bus modbus.Bus, deviceKey string, cfg Config) ([]Reading, error) {
	readings := make([]Reading, 0, len(cfg.Registers))
	for _, reg := range cfg.Registers {
		qty, err := registerQuantity(reg.DataType)
		if err != nil {
			return nil, err
		}
		data, err := bus.ReadRegisters(ctx, cfg.ModbusAddress, reg.FunctionCode, reg.Register, qty)
		if err != nil {
			return nil, fmt.Errorf("read %s register %d: %w", reg.Name, reg.Register, err)
		}
		raw, err := decodeValue(data, reg.DataType)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", reg.Name, err)
		}
		scale := reg.Scale
		if scale == 0 {
			scale = 1
		}
		key := reg.SensorKey
		if key == "" {
			key = deviceKey + "_" + reg.Name
		}
		value := raw*scale + reg.Offset
		quality := "good"
		if reg.ClipAt != nil && value >= *reg.ClipAt {
			quality = "clipped"
		}
		readings = append(readings, Reading{
			SensorKey: key,
			Name:      reg.Name,
			Value:     value,
			Unit:      reg.Unit,
			Quality:   quality,
			ReadAt:    time.Now().UTC(),
		})
	}
	return readings, nil
}

func registerQuantity(dataType string) (uint16, error) {
	switch dataType {
	case "uint16", "int16":
		return 1, nil
	case "uint32", "int32", "float32":
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported data_type %q (use uint16, int16, uint32, int32, or float32)", dataType)
	}
}

func decodeValue(data []byte, dataType string) (float64, error) {
	switch dataType {
	case "uint16":
		if len(data) < 2 {
			return 0, fmt.Errorf("short response: %d bytes", len(data))
		}
		return float64(binary.BigEndian.Uint16(data)), nil
	case "int16":
		if len(data) < 2 {
			return 0, fmt.Errorf("short response: %d bytes", len(data))
		}
		return float64(int16(binary.BigEndian.Uint16(data))), nil
	case "uint32":
		if len(data) < 4 {
			return 0, fmt.Errorf("short response: %d bytes", len(data))
		}
		return float64(binary.BigEndian.Uint32(data)), nil
	case "int32":
		if len(data) < 4 {
			return 0, fmt.Errorf("short response: %d bytes", len(data))
		}
		return float64(int32(binary.BigEndian.Uint32(data))), nil
	case "float32":
		if len(data) < 4 {
			return 0, fmt.Errorf("short response: %d bytes", len(data))
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(data))), nil
	default:
		return 0, fmt.Errorf("unsupported data_type %q", dataType)
	}
}
