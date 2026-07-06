// Package devices owns hardware drivers and is the only layer allowed to
// touch them (spec §3.1). All relay changes flow through Manager.SetRelay so
// every action is logged and timeouts are enforced in one place.
package devices

import (
	"context"
	"fmt"
	"sync"
	"time"

	"propagation-controller/internal/bus"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

// Relay assignments (spec §7.1, updated 2026-07-05: fans on 7).
// All circulation fans are ganged on one relay — they're single-speed 12V
// units and Austin wants a simple on/off toggle.
var DefaultAssignments = map[int]string{
	1: "Barina grow lights",
	2: "SEAFLO diaphragm pump",
	3: "Solenoid Valve 1 (Shelf 1)",
	4: "Solenoid Valve 2 (Shelf 2)",
	5: "Solenoid Valve 3 (Shelf 3)",
	6: "Solenoid Valve 4 (Shelf 4, future)",
	7: "Circulation fans",
	8: "Spare",
}

const (
	LightsChannel = 1
	PumpChannel   = 2
	FansChannel   = 7
)

// safeOffChannels are forced off at startup and shutdown (spec §12.2): pump
// and valves. Lights are left to the (future) lighting module.
var safeOffChannels = []int{2, 3, 4, 5, 6}

type RelayInfo struct {
	Channel int         `json:"channel"`
	Name    string      `json:"name"`
	State   relay.State `json:"state"`
}

type Manager struct {
	ctrl       relay.Controller
	driverName string
	actions    *storage.RelayActionRepo
	deviceRepo *storage.DeviceRepo
	events     *logging.EventLogger
	simulation bool

	hub *bus.Hub

	mu     sync.Mutex
	timers map[int]*time.Timer
}

// AttachHub enables live relay_action events over SSE.
func (m *Manager) AttachHub(h *bus.Hub) { m.hub = h }

func NewManager(ctrl relay.Controller, driverName string, actions *storage.RelayActionRepo,
	deviceRepo *storage.DeviceRepo, events *logging.EventLogger, simulation bool) *Manager {
	return &Manager{
		ctrl:       ctrl,
		driverName: driverName,
		actions:    actions,
		deviceRepo: deviceRepo,
		events:     events,
		simulation: simulation,
		timers:     map[int]*time.Timer{},
	}
}

// Start opens the relay controller, registers devices, and applies the safe
// startup state: pump and valves forced off (spec §12.2).
func (m *Manager) Start(ctx context.Context) error {
	if err := m.ctrl.Open(ctx); err != nil {
		m.registerDevices("offline")
		m.events.Error("relay_controller_offline",
			"relay controller failed to open; automation must remain suspended", "startup",
			map[string]any{"error": err.Error()})
		return err
	}
	status := "online"
	if m.simulation {
		status = "simulated"
	}
	m.registerDevices(status)

	for _, ch := range safeOffChannels {
		if _, err := m.SetRelay(ctx, ch, relay.Off, "startup", "safe startup state", 0); err != nil {
			m.events.Error("startup_safe_state_failed",
				fmt.Sprintf("failed to force relay %d off at startup", ch), "startup",
				map[string]any{"channel": ch, "error": err.Error()})
		}
	}
	m.events.Info("startup_safe_state", "pump and valves forced off at startup", "startup", nil)
	return nil
}

// Shutdown forces pump and valves off, cancels pending timeouts, and closes
// the controller (spec §9.4: automatic stop on controller shutdown).
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	for ch, t := range m.timers {
		t.Stop()
		delete(m.timers, ch)
	}
	m.mu.Unlock()

	for _, ch := range safeOffChannels {
		if _, err := m.SetRelay(ctx, ch, relay.Off, "system", "controller shutdown", 0); err != nil {
			m.events.Error("shutdown_safe_state_failed",
				fmt.Sprintf("failed to force relay %d off at shutdown", ch), "system",
				map[string]any{"channel": ch, "error": err.Error()})
		}
	}
	m.ctrl.Close()
}

// SetRelay performs a relay change, durably logs the action, and (for "on"
// with timeoutSeconds > 0) schedules an automatic off. A new command on the
// same channel cancels any pending timeout.
func (m *Manager) SetRelay(ctx context.Context, channel int, state relay.State,
	actor, reason string, timeoutSeconds int) (previous relay.State, err error) {

	previous, prevErr := m.ctrl.GetChannel(ctx, channel)
	if prevErr != nil {
		previous = relay.Unknown
	}

	err = m.ctrl.SetChannel(ctx, channel, state)

	action := storage.RelayAction{
		RelayChannel:   channel,
		RequestedState: string(state),
		Actor:          actor,
		Success:        err == nil,
	}
	deviceKey := fmt.Sprintf("relay_%d", channel)
	action.DeviceKey = &deviceKey
	if previous != relay.Unknown {
		p := string(previous)
		action.PreviousState = &p
	}
	if reason != "" {
		action.Reason = &reason
	}
	if err == nil {
		a := string(state)
		action.ActualState = &a
	} else {
		msg := err.Error()
		action.ErrorMessage = &msg
		m.events.Error("relay_command_failed",
			fmt.Sprintf("relay %d -> %s failed", channel, state), actor,
			map[string]any{"channel": channel, "error": msg})
	}
	if logErr := m.actions.Insert(action); logErr != nil {
		m.events.Error("relay_action_log_failed", "failed to persist relay action", "system",
			map[string]any{"channel": channel, "error": logErr.Error()})
	}
	m.hub.Publish("relay_action", map[string]any{
		"channel": channel, "name": DefaultAssignments[channel],
		"state": string(state), "actor": actor, "reason": reason, "success": err == nil,
	})
	if err != nil {
		return previous, err
	}

	m.mu.Lock()
	if t, ok := m.timers[channel]; ok {
		t.Stop()
		delete(m.timers, channel)
	}
	if state == relay.On && timeoutSeconds > 0 {
		m.timers[channel] = time.AfterFunc(time.Duration(timeoutSeconds)*time.Second, func() {
			m.expireTimeout(channel, timeoutSeconds)
		})
	}
	m.mu.Unlock()

	return previous, nil
}

func (m *Manager) expireTimeout(channel int, timeoutSeconds int) {
	m.mu.Lock()
	delete(m.timers, channel)
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reason := fmt.Sprintf("timeout expired after %ds", timeoutSeconds)
	if _, err := m.SetRelay(ctx, channel, relay.Off, "system", reason, 0); err != nil {
		m.events.Error("relay_timeout_off_failed",
			fmt.Sprintf("relay %d failed to turn off after timeout — check hardware", channel),
			"system", map[string]any{"channel": channel, "error": err.Error()})
	}
}

// AllOff turns every channel off. Used by the all-off endpoint and as the
// Phase 0 stand-in for emergency stop until the safety supervisor lands.
func (m *Manager) AllOff(ctx context.Context, actor, reason string) error {
	var firstErr error
	for ch := 1; ch <= relay.NumChannels; ch++ {
		if _, err := m.SetRelay(ctx, ch, relay.Off, actor, reason, 0); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Snapshot returns current channel states with their assigned names.
func (m *Manager) Snapshot(ctx context.Context) ([]RelayInfo, error) {
	states, err := m.ctrl.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	infos := make([]RelayInfo, 0, relay.NumChannels)
	for ch := 1; ch <= relay.NumChannels; ch++ {
		state, ok := states[ch]
		if !ok {
			state = relay.Unknown
		}
		infos = append(infos, RelayInfo{Channel: ch, Name: DefaultAssignments[ch], State: state})
	}
	return infos, nil
}

func (m *Manager) GetRelay(ctx context.Context, channel int) (RelayInfo, error) {
	state, err := m.ctrl.GetChannel(ctx, channel)
	if err != nil {
		return RelayInfo{}, err
	}
	return RelayInfo{Channel: channel, Name: DefaultAssignments[channel], State: state}, nil
}

func (m *Manager) ControllerHealth(ctx context.Context) relay.Health {
	return m.ctrl.Health(ctx)
}

func (m *Manager) registerDevices(status string) {
	if err := m.deviceRepo.Upsert("relay_controller", "Numato 8-channel USB relay",
		"relay_controller", m.driverName, "{}", status); err != nil {
		m.events.Error("device_registry_failed", "failed to register relay controller", "startup",
			map[string]any{"error": err.Error()})
	}
	for ch := 1; ch <= relay.NumChannels; ch++ {
		key := fmt.Sprintf("relay_%d", ch)
		cfg := fmt.Sprintf(`{"channel":%d}`, ch)
		if err := m.deviceRepo.Upsert(key, DefaultAssignments[ch], "relay_channel", m.driverName, cfg, status); err != nil {
			m.events.Error("device_registry_failed", "failed to register relay channel", "startup",
				map[string]any{"channel": ch, "error": err.Error()})
		}
	}
}
