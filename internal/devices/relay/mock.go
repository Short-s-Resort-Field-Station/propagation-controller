package relay

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Mock is the simulation-mode relay controller (spec §24.4). It behaves like
// the real module: all channels power up off, and state is remembered until
// changed.
type Mock struct {
	mu     sync.Mutex
	opened bool
	states map[int]State
}

func NewMock() *Mock {
	states := make(map[int]State, NumChannels)
	for ch := 1; ch <= NumChannels; ch++ {
		states[ch] = Off
	}
	return &Mock{states: states}
}

func (m *Mock) Open(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = true
	return nil
}

func (m *Mock) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = false
	return nil
}

func (m *Mock) SetChannel(ctx context.Context, channel int, state State) error {
	if !validChannel(channel) {
		return fmt.Errorf("relay channel %d out of range 1..%d", channel, NumChannels)
	}
	if state != On && state != Off {
		return fmt.Errorf("cannot set relay to state %q", state)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.opened {
		return fmt.Errorf("mock relay controller not open")
	}
	m.states[channel] = state
	return nil
}

func (m *Mock) GetChannel(ctx context.Context, channel int) (State, error) {
	if !validChannel(channel) {
		return Unknown, fmt.Errorf("relay channel %d out of range 1..%d", channel, NumChannels)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[channel], nil
}

func (m *Mock) GetAll(ctx context.Context) (map[int]State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int]State, NumChannels)
	for ch, s := range m.states {
		out[ch] = s
	}
	return out, nil
}

func (m *Mock) Health(ctx context.Context) Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := "simulated"
	if !m.opened {
		status = "offline"
	}
	return Health{Status: status, LastSeen: time.Now(), Info: map[string]string{"driver": "mock"}}
}
