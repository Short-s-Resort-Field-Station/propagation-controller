package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

// NumatoUSB drives the Numato Lab 8-channel USB relay module.
//
// Protocol notes (see HARDWARE.md):
//   - USB CDC ACM serial; baud rate is ignored by the device.
//   - ASCII commands terminated with \r; the device echoes the command and
//     ends every response with a '>' prompt.
//   - Channels are 0-indexed on the wire; this driver takes 1-based channels
//     and maps them down.
type NumatoUSB struct {
	portName string

	mu     sync.Mutex
	port   io.ReadWriteCloser
	openFn func(name string) (io.ReadWriteCloser, error)
	health Health
}

const numatoCommandTimeout = 2 * time.Second

func NewNumatoUSB(portName string) *NumatoUSB {
	return &NumatoUSB{
		portName: portName,
		openFn:   openSerialPort,
		health:   Health{Status: "offline", Info: map[string]string{"port": portName}},
	}
}

func openSerialPort(name string) (io.ReadWriteCloser, error) {
	port, err := serial.Open(name, &serial.Mode{BaudRate: 19200})
	if err != nil {
		return nil, err
	}
	// Short read timeout lets the response loop poll until the '>' prompt
	// arrives without blocking forever on a wedged device.
	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		port.Close()
		return nil, err
	}
	return port, nil
}

// DetectNumatoPort finds the relay module's serial device. Prefers the stable
// /dev/serial/by-id path on Linux; falls back to ACM/usbmodem globs.
func DetectNumatoPort() (string, error) {
	patterns := []string{
		"/dev/serial/by-id/*Numato*",
		"/dev/serial/by-id/*numato*",
		"/dev/ttyACM*",
		"/dev/tty.usbmodem*", // macOS
	}
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", errors.New("no Numato relay module found (tried /dev/serial/by-id, /dev/ttyACM*, /dev/tty.usbmodem*)")
}

func (n *NumatoUSB) Open(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.port != nil {
		return nil
	}
	port, err := n.openFn(n.portName)
	if err != nil {
		n.recordError(err)
		return fmt.Errorf("open relay serial port %s: %w", n.portName, err)
	}
	n.port = port
	n.health.Status = "online"
	n.health.LastSeen = time.Now()

	// Identification is best-effort; a module that answers commands but not
	// "ver" should not be treated as broken.
	if ver, err := n.execLocked(ctx, "ver"); err == nil && ver != "" {
		n.health.Info["firmware_version"] = ver
	}
	if id, err := n.execLocked(ctx, "id get"); err == nil && id != "" {
		n.health.Info["module_id"] = id
	}
	return nil
}

func (n *NumatoUSB) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.port == nil {
		return nil
	}
	err := n.port.Close()
	n.port = nil
	n.health.Status = "offline"
	return err
}

func (n *NumatoUSB) SetChannel(ctx context.Context, channel int, state State) error {
	if !validChannel(channel) {
		return fmt.Errorf("relay channel %d out of range 1..%d", channel, NumChannels)
	}
	var verb string
	switch state {
	case On:
		verb = "on"
	case Off:
		verb = "off"
	default:
		return fmt.Errorf("cannot set relay to state %q", state)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	_, err := n.execLocked(ctx, fmt.Sprintf("relay %s %d", verb, channel-1))
	return err
}

func (n *NumatoUSB) GetChannel(ctx context.Context, channel int) (State, error) {
	if !validChannel(channel) {
		return Unknown, fmt.Errorf("relay channel %d out of range 1..%d", channel, NumChannels)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	resp, err := n.execLocked(ctx, fmt.Sprintf("relay read %d", channel-1))
	if err != nil {
		return Unknown, err
	}
	switch strings.ToLower(resp) {
	case "on":
		return On, nil
	case "off":
		return Off, nil
	default:
		return Unknown, fmt.Errorf("unexpected relay read response %q", resp)
	}
}

func (n *NumatoUSB) GetAll(ctx context.Context) (map[int]State, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	resp, err := n.execLocked(ctx, "relay readall")
	if err != nil {
		return nil, err
	}
	bits, err := strconv.ParseUint(strings.TrimSpace(resp), 16, 16)
	if err != nil {
		return nil, fmt.Errorf("unexpected relay readall response %q", resp)
	}
	states := make(map[int]State, NumChannels)
	for ch := 1; ch <= NumChannels; ch++ {
		if bits&(1<<(ch-1)) != 0 {
			states[ch] = On
		} else {
			states[ch] = Off
		}
	}
	return states, nil
}

func (n *NumatoUSB) Health(ctx context.Context) Health {
	n.mu.Lock()
	defer n.mu.Unlock()
	h := n.health
	h.Info = make(map[string]string, len(n.health.Info))
	for k, v := range n.health.Info {
		h.Info[k] = v
	}
	return h
}

// execLocked sends one command and returns the response with the echo and
// prompt stripped. Caller must hold n.mu.
func (n *NumatoUSB) execLocked(ctx context.Context, cmd string) (string, error) {
	if n.port == nil {
		return "", errors.New("relay serial port not open")
	}
	if _, err := n.port.Write([]byte(cmd + "\r")); err != nil {
		n.recordError(err)
		return "", fmt.Errorf("write %q: %w", cmd, err)
	}

	deadline := time.Now().Add(numatoCommandTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	var buf bytes.Buffer
	chunk := make([]byte, 256)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		nRead, err := n.port.Read(chunk)
		if err != nil && !errors.Is(err, io.EOF) {
			n.recordError(err)
			return "", fmt.Errorf("read response to %q: %w", cmd, err)
		}
		if nRead > 0 {
			buf.Write(chunk[:nRead])
			if bytes.Contains(buf.Bytes(), []byte(">")) {
				break
			}
		}
		if time.Now().After(deadline) {
			err := fmt.Errorf("timeout waiting for response to %q", cmd)
			n.recordError(err)
			return "", err
		}
	}

	n.health.Status = "online"
	n.health.LastSeen = time.Now()
	return parseNumatoResponse(buf.String(), cmd), nil
}

// parseNumatoResponse strips the command echo, blank lines, and the '>'
// prompt from a raw response, returning whatever payload remains.
func parseNumatoResponse(raw, cmd string) string {
	raw = strings.ReplaceAll(raw, "\r", "\n")
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ">"))
		if line == "" || line == cmd {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, " ")
}

func (n *NumatoUSB) recordError(err error) {
	n.health.Status = "error"
	n.health.LastError = err.Error()
	n.health.ErrorCount++
}
