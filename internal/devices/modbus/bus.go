// Package modbus provides the shared RS485 Modbus RTU bus (spec §8). All
// sensors hang off one bus; access is serialized so requests never interleave
// on the wire.
package modbus

import (
	"context"
	"time"
)

type Health struct {
	Status     string            `json:"status"` // online | offline | error | simulated
	LastSeen   time.Time         `json:"last_seen,omitzero"`
	LastError  string            `json:"last_error,omitempty"`
	ErrorCount int               `json:"error_count"`
	Info       map[string]string `json:"info,omitempty"`
}

type Bus interface {
	Open(ctx context.Context) error
	Close() error
	// ReadRegisters reads quantity registers starting at address from the
	// given slave. functionCode 3 = holding registers, 4 = input registers.
	// Returns raw big-endian register bytes (2 per register).
	ReadRegisters(ctx context.Context, slaveID uint8, functionCode uint8, address, quantity uint16) ([]byte, error)
	// Scan probes each address in [start, end] and returns those that
	// answered anything at all (including Modbus exceptions — an exception
	// still proves a device is present).
	Scan(ctx context.Context, start, end uint8) ([]uint8, error)
	Health(ctx context.Context) Health
}
