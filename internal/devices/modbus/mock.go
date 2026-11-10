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

// MockBus is the simulation-mode Modbus bus (spec §24.4). Slaves 1-3 mimic the
// shelf soil probes (11-13), slave 1 the reservoir level transmitter, with raw values that
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
			// Three ZTS-3000 soil probes, one per shelf (moisture/temp raw x10,
			// EC raw µS/cm), at addresses 11-13 @ 9600 — as commissioned 2026-11-10.
			11: soilBank(312, 225, 1150),
			12: soilBank(288, 221, 980),
			13: soilBank(340, 228, 1320),
			// Hydrostatic reservoir level transmitter at factory address 1: register 4
			// is level x10 in cm (full scale 100.0 cm). ~24.6 cm of a 30 cm
			// placeholder full depth ≈ 82 %.
			1: {
				0: {value: 1, min: 1, max: 1, step: 0},
				1: {value: 3, min: 3, max: 3, step: 0},
				2: {value: 17, min: 17, max: 17, step: 0},
				3: {value: 1, min: 1, max: 1, step: 0},
				4: {value: 246, min: 195, max: 270, step: 2},
				5: {value: 0, min: 0, max: 0, step: 0},
				6: {value: 1000, min: 1000, max: 1000, step: 0},
			},
		},
	}
}

func soilBank(moist, temp, ec int) map[uint16]*driftReg {
	return map[uint16]*driftReg{
		0: {value: moist, min: 250, max: 420, step: 3},
		1: {value: temp, min: 200, max: 260, step: 2},
		2: {value: ec, min: 900, max: 1450, step: 8},
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
