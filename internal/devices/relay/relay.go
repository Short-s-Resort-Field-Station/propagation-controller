// Package relay defines the relay controller abstraction (spec §7) and its
// two v1.0 implementations: the Numato USB driver and the simulation mock.
//
// Channels are 1-based (1..8) everywhere above the driver, matching the spec's
// relay assignment table. Drivers translate to whatever the wire wants.
package relay

import (
	"context"
	"time"
)

type State string

const (
	On      State = "on"
	Off     State = "off"
	Unknown State = "unknown"
	Errored State = "error"
)

const NumChannels = 8

type Health struct {
	Status     string            `json:"status"` // online | offline | error | simulated
	LastSeen   time.Time         `json:"last_seen,omitzero"`
	LastError  string            `json:"last_error,omitempty"`
	ErrorCount int               `json:"error_count"`
	Info       map[string]string `json:"info,omitempty"`
}

type Controller interface {
	Open(ctx context.Context) error
	Close() error
	SetChannel(ctx context.Context, channel int, state State) error
	GetChannel(ctx context.Context, channel int) (State, error)
	GetAll(ctx context.Context) (map[int]State, error)
	Health(ctx context.Context) Health
}

func validChannel(channel int) bool {
	return channel >= 1 && channel <= NumChannels
}
