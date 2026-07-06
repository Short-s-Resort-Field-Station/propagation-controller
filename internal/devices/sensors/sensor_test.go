package sensors

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"propagation-controller/internal/devices/modbus"
)

func TestDecodeValue(t *testing.T) {
	u16 := func(v uint16) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }

	cases := []struct {
		name     string
		data     []byte
		dataType string
		want     float64
	}{
		{"uint16", u16(312), "uint16", 312},
		{"int16 negative", u16(0xFFF6), "int16", -10}, // -1.0°C at scale 0.1
		{"uint32", u32(70000), "uint32", 70000},
		{"int32 negative", u32(0xFFFFFFFE), "int32", -2},
		{"float32", u32(math.Float32bits(3.14)), "float32", float64(float32(3.14))},
	}
	for _, c := range cases {
		got, err := decodeValue(c.data, c.dataType)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	if _, err := decodeValue([]byte{0x01}, "uint16"); err == nil {
		t.Error("short data: expected error")
	}
	if _, err := decodeValue(u16(1), "uint64"); err == nil {
		t.Error("unknown type: expected error")
	}
}

func TestReadAllAppliesScaleOffsetAndKeys(t *testing.T) {
	bus := modbus.NewMockBus()
	bus.Open(context.Background())

	cfg := Config{
		ModbusAddress:       1,
		PollIntervalSeconds: 10,
		Registers: []RegisterSpec{
			{Name: "moisture", Register: 0, DataType: "uint16", Scale: 0.1, Unit: "%"},
			{Name: "temperature", Register: 1, DataType: "int16", Scale: 0.1, Unit: "°C"},
			{Name: "custom", Register: 3, DataType: "uint16", Scale: 0.1, Unit: "pH", SensorKey: "my_explicit_key"},
		},
	}
	readings, err := ReadAll(context.Background(), bus, "soil_zone_1", cfg)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(readings) != 3 {
		t.Fatalf("got %d readings, want 3", len(readings))
	}
	if readings[0].SensorKey != "soil_zone_1_moisture" {
		t.Errorf("derived key = %q", readings[0].SensorKey)
	}
	if readings[2].SensorKey != "my_explicit_key" {
		t.Errorf("explicit key = %q", readings[2].SensorKey)
	}
	// Mock moisture raw is clamped to 250..420, so scaled must be 25..42.
	if v := readings[0].Value; v < 25 || v > 42 {
		t.Errorf("moisture %v outside simulated range", v)
	}
	if readings[0].Unit != "%" || readings[0].Quality != "good" {
		t.Errorf("unexpected unit/quality: %+v", readings[0])
	}
}

func TestReadAllUnknownSlaveFails(t *testing.T) {
	bus := modbus.NewMockBus()
	bus.Open(context.Background())
	cfg := Config{
		ModbusAddress:       99,
		PollIntervalSeconds: 10,
		Registers:           []RegisterSpec{{Name: "x", Register: 0, DataType: "uint16", Unit: "u"}},
	}
	if _, err := ReadAll(context.Background(), bus, "ghost", cfg); err == nil {
		t.Error("expected error for unknown slave address")
	}
}

func TestConfigValidate(t *testing.T) {
	good := Config{
		ModbusAddress:       1,
		PollIntervalSeconds: 10,
		Registers:           []RegisterSpec{{Name: "m", Register: 0, DataType: "uint16", Unit: "%"}},
	}
	if err := good.Validate(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
	bad := []Config{
		{ModbusAddress: 0, PollIntervalSeconds: 10, Registers: good.Registers},
		{ModbusAddress: 1, PollIntervalSeconds: 0, Registers: good.Registers},
		{ModbusAddress: 1, PollIntervalSeconds: 10},
		{ModbusAddress: 1, PollIntervalSeconds: 10,
			Registers: []RegisterSpec{{Name: "m", DataType: "uint64", Unit: "%"}}},
		{ModbusAddress: 1, PollIntervalSeconds: 10,
			Registers: []RegisterSpec{{Name: "m", DataType: "uint16", FunctionCode: 6, Unit: "%"}}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("bad config %d accepted", i)
		}
	}
}
