package modbus

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	gm "github.com/goburrow/modbus"
)

// RTUBus drives the physical RS485 bus through the FTDI (FT232RQ + SP485EN)
// USB adapter. The SP485EN handles TX direction automatically via the FTDI
// TXDEN line, so no RTS toggling is needed.
type RTUBus struct {
	mu      sync.Mutex
	handler *gm.RTUClientHandler
	client  gm.Client
	opened  bool
	health  Health
}

const (
	defaultTimeout = 500 * time.Millisecond
	scanTimeout    = 200 * time.Millisecond
)

func NewRTUBus(port string, baud int) *RTUBus {
	h := gm.NewRTUClientHandler(port)
	h.BaudRate = baud
	h.DataBits = 8
	h.Parity = "N"
	h.StopBits = 1
	h.Timeout = defaultTimeout
	return &RTUBus{
		handler: h,
		client:  gm.NewClient(h),
		health:  Health{Status: "offline", Info: map[string]string{"port": port, "baud": fmt.Sprint(baud)}},
	}
}

// DetectRS485Port finds the FTDI adapter. Prefers stable by-id paths.
func DetectRS485Port() (string, error) {
	patterns := []string{
		"/dev/serial/by-id/*FTDI*",
		"/dev/serial/by-id/*ftdi*",
		"/dev/ttyUSB*",
		"/dev/tty.usbserial*", // macOS
	}
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", errors.New("no RS485 adapter found (tried /dev/serial/by-id/*FTDI*, /dev/ttyUSB*)")
}

func (b *RTUBus) Open(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.opened {
		return nil
	}
	if err := b.handler.Connect(); err != nil {
		b.recordError(err)
		return fmt.Errorf("open RS485 port: %w", err)
	}
	b.opened = true
	b.health.Status = "online"
	b.health.LastSeen = time.Now()
	return nil
}

func (b *RTUBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.opened {
		return nil
	}
	b.opened = false
	b.health.Status = "offline"
	return b.handler.Close()
}

func (b *RTUBus) ReadRegisters(ctx context.Context, slaveID uint8, functionCode uint8, address, quantity uint16) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.opened {
		return nil, errors.New("modbus bus not open")
	}
	data, err := b.readLocked(slaveID, functionCode, address, quantity)
	if err != nil {
		b.recordError(err)
		return nil, err
	}
	b.health.Status = "online"
	b.health.LastSeen = time.Now()
	return data, nil
}

func (b *RTUBus) readLocked(slaveID uint8, functionCode uint8, address, quantity uint16) ([]byte, error) {
	b.handler.SlaveId = slaveID
	switch functionCode {
	case 3, 0: // holding registers; 0 = unspecified, default to 3
		return b.client.ReadHoldingRegisters(address, quantity)
	case 4:
		return b.client.ReadInputRegisters(address, quantity)
	default:
		return nil, fmt.Errorf("unsupported modbus function code %d", functionCode)
	}
}

func (b *RTUBus) Scan(ctx context.Context, start, end uint8) ([]uint8, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.opened {
		return nil, errors.New("modbus bus not open")
	}
	saved := b.handler.Timeout
	b.handler.Timeout = scanTimeout
	defer func() { b.handler.Timeout = saved }()

	var found []uint8
	for addr := start; addr <= end && addr != 0; addr++ {
		if ctx.Err() != nil {
			return found, ctx.Err()
		}
		_, err := b.readLocked(addr, 3, 0, 1)
		var mbErr *gm.ModbusError
		if err == nil || errors.As(err, &mbErr) {
			// Any well-formed reply — data or exception — means a device
			// is listening at this address.
			found = append(found, addr)
		}
	}
	return found, nil
}

func (b *RTUBus) Health(ctx context.Context) Health {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.health
	h.Info = map[string]string{}
	for k, v := range b.health.Info {
		h.Info[k] = v
	}
	return h
}

func (b *RTUBus) recordError(err error) {
	b.health.Status = "error"
	b.health.LastError = err.Error()
	b.health.ErrorCount++
}
