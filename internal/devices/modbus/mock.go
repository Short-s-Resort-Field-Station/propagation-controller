package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// driftReg is a simulated register whose value takes a small random walk
// between min and max, so simulation-mode graphs look alive.
type driftReg struct {
	value, min, max, step int
}

// MockBus is the simulation-mode Modbus bus (spec §24.4). Slave 1 mimics the
// soil sensor, slave 2 the reservoir level transmitter, with raw values that
// match the seeded default register maps.
type MockBus struct {
	mu      sync.Mutex
	opened  bool
	failing bool
	banks   map[uint8]map[uint16]*driftReg
	rng     *rand.Rand
}

func NewMockBus() *MockBus {
	return &MockBus{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		banks: map[uint8]map[uint16]*driftReg{
			1: { // soil sensor (values are raw x10)
				0: {value: 312, min: 250, max: 420, step: 3}, // moisture %
				1: {value: 225, min: 200, max: 260, step: 2}, // temperature °C
				2: {value: 550, min: 450, max: 700, step: 4}, // humidity
				3: {value: 65, min: 58, max: 72, step: 1},    // pH
			},
			2: { // reservoir level transmitter
				0: {value: 820, min: 650, max: 900, step: 2}, // level %
			},
		},
	}
}

// SetFailing makes every read fail, for exercising offline detection.
func (m *MockBus) SetFailing(failing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failing = failing
}

func (m *MockBus) Open(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = true
	return nil
}

func (m *MockBus) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = false
	return nil
}

func (m *MockBus) ReadRegisters(ctx context.Context, slaveID uint8, functionCode uint8, address, quantity uint16) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.opened {
		return nil, fmt.Errorf("mock modbus bus not open")
	}
	if m.failing {
		return nil, fmt.Errorf("simulated bus failure")
	}
	bank, ok := m.banks[slaveID]
	if !ok {
		return nil, fmt.Errorf("simulated timeout: no device at address %d", slaveID)
	}
	out := make([]byte, 2*quantity)
	for i := uint16(0); i < quantity; i++ {
		reg, ok := bank[address+i]
		if !ok {
			return nil, fmt.Errorf("simulated illegal data address %d on slave %d", address+i, slaveID)
		}
		reg.value += m.rng.Intn(2*reg.step+1) - reg.step
		if reg.value < reg.min {
			reg.value = reg.min
		}
		if reg.value > reg.max {
			reg.value = reg.max
		}
		binary.BigEndian.PutUint16(out[2*i:], uint16(reg.value))
	}
	return out, nil
}

func (m *MockBus) Scan(ctx context.Context, start, end uint8) ([]uint8, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found []uint8
	for addr := start; addr <= end && addr != 0; addr++ {
		if _, ok := m.banks[addr]; ok && !m.failing {
			found = append(found, addr)
		}
	}
	return found, nil
}

func (m *MockBus) Health(ctx context.Context) Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := "simulated"
	if !m.opened || m.failing {
		status = "offline"
	}
	return Health{Status: status, LastSeen: time.Now(), Info: map[string]string{"driver": "mock"}}
}
