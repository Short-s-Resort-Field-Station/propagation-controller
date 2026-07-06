package sensors

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
	"propagation-controller/internal/devices/modbus"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

// Service owns all Modbus sensors: it seeds default configs, runs one poller
// per enabled sensor, stores readings, and tracks online/offline state.
type Service struct {
	bus        modbus.Bus
	readings   *storage.ReadingRepo
	devices    *storage.DeviceRepo
	events     *logging.EventLogger
	log        zerolog.Logger
	simulation bool

	hub *bus.Hub

	mu      sync.Mutex
	rootCtx context.Context
	pollers map[string]*poller // by device_key
}

// AttachHub enables live sensor_reading events over SSE.
func (s *Service) AttachHub(h *bus.Hub) { s.hub = h }

type poller struct {
	cancel   context.CancelFunc
	deviceID int64
	cfg      Config

	mu       sync.Mutex
	failures int
	offline  bool
}

func NewService(bus modbus.Bus, readings *storage.ReadingRepo, devices *storage.DeviceRepo,
	events *logging.EventLogger, log zerolog.Logger, simulation bool) *Service {
	return &Service{
		bus:        bus,
		readings:   readings,
		devices:    devices,
		events:     events,
		log:        log,
		simulation: simulation,
		pollers:    map[string]*poller{},
	}
}

// EnsureDefaults seeds sensor devices on first boot. Register maps are
// placeholders for the common generic-RS485 layouts and MUST be verified
// against the vendor datasheets at hardware bring-up — they are editable at
// runtime via PATCH /api/v1/sensors/{key}/config, no code changes needed.
func (s *Service) EnsureDefaults() error {
	existing, err := s.devices.ListByType("soil_sensor")
	if err != nil {
		return err
	}
	reservoirs, err := s.devices.ListByType("reservoir_sensor")
	if err != nil {
		return err
	}
	if len(existing) > 0 || len(reservoirs) > 0 {
		return nil
	}

	soil := Config{
		ModbusAddress:       1,
		PollIntervalSeconds: 10,
		Registers: []RegisterSpec{
			{Name: "moisture", Register: 0, FunctionCode: 3, DataType: "uint16", Scale: 0.1, Unit: "%"},
			{Name: "temperature", Register: 1, FunctionCode: 3, DataType: "int16", Scale: 0.1, Unit: "°C"},
			{Name: "ph", Register: 3, FunctionCode: 3, DataType: "uint16", Scale: 0.1, Unit: "pH"},
		},
	}
	reservoir := Config{
		ModbusAddress:       2,
		PollIntervalSeconds: 10,
		Registers: []RegisterSpec{
			{Name: "level", Register: 0, FunctionCode: 3, DataType: "uint16", Scale: 0.1, Unit: "%"},
		},
	}
	for _, seed := range []struct {
		key, name, typ string
		cfg            Config
	}{
		{"soil_zone_1", "Soil Sensor Zone 1", "soil_sensor", soil},
		{"reservoir_level", "Reservoir Level", "reservoir_sensor", reservoir},
	} {
		body, err := json.Marshal(seed.cfg)
		if err != nil {
			return err
		}
		if err := s.devices.Upsert(seed.key, seed.name, seed.typ, "generic_modbus", string(body), "unknown"); err != nil {
			return err
		}
	}
	s.events.Info("sensors_seeded", "default sensor configurations created (verify register maps against datasheets)", "startup", nil)
	return nil
}

// Start launches a poller per enabled sensor. Also registers the bus itself
// as a device.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	s.rootCtx = ctx
	s.mu.Unlock()

	busStatus := "online"
	if s.simulation {
		busStatus = "simulated"
	}
	if h := s.bus.Health(ctx); h.Status == "offline" || h.Status == "error" {
		busStatus = "offline"
	}
	if err := s.devices.Upsert("modbus_bus", "RS485 Modbus RTU bus", "modbus_bus", "rtu", "{}", busStatus); err != nil {
		return err
	}

	sensors, err := s.sensorDevices()
	if err != nil {
		return err
	}
	for _, dev := range sensors {
		if !dev.Enabled {
			continue
		}
		if err := s.startPoller(dev); err != nil {
			s.events.Error("sensor_start_failed", fmt.Sprintf("sensor %s failed to start", dev.DeviceKey),
				"startup", map[string]any{"device_key": dev.DeviceKey, "error": err.Error()})
		}
	}
	return nil
}

func (s *Service) sensorDevices() ([]storage.Device, error) {
	all, err := s.devices.List()
	if err != nil {
		return nil, err
	}
	var out []storage.Device
	for _, d := range all {
		if d.Driver == "generic_modbus" {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *Service) startPoller(dev storage.Device) error {
	var cfg Config
	if err := json.Unmarshal([]byte(dev.ConfigJSON), &cfg); err != nil {
		return fmt.Errorf("bad config for %s: %w", dev.DeviceKey, err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config for %s: %w", dev.DeviceKey, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.pollers[dev.DeviceKey]; ok {
		old.cancel()
		delete(s.pollers, dev.DeviceKey)
	}
	ctx, cancel := context.WithCancel(s.rootCtx)
	p := &poller{cancel: cancel, deviceID: dev.ID, cfg: cfg}
	s.pollers[dev.DeviceKey] = p
	go s.run(ctx, dev.DeviceKey, p)
	return nil
}

func (s *Service) run(ctx context.Context, deviceKey string, p *poller) {
	interval := time.Duration(p.cfg.PollIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.pollOnce(ctx, deviceKey, p) // first reading immediately, not after one interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollOnce(ctx, deviceKey, p)
		}
	}
}

func (s *Service) pollOnce(ctx context.Context, deviceKey string, p *poller) {
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	readings, err := ReadAll(readCtx, s.bus, deviceKey, p.cfg)

	threshold := p.cfg.OfflineAfterFailures
	if threshold <= 0 {
		threshold = 3
	}

	p.mu.Lock()
	if err != nil {
		p.failures++
		wentOffline := !p.offline && p.failures >= threshold
		if wentOffline {
			p.offline = true
		}
		p.mu.Unlock()

		s.log.Warn().Str("sensor", deviceKey).Err(err).Msg("sensor poll failed")
		if wentOffline {
			errMsg := err.Error()
			s.devices.SetStatus(deviceKey, "offline", &errMsg)
			s.events.Warn("device_status_change",
				fmt.Sprintf("sensor %s is offline after %d consecutive failures", deviceKey, threshold),
				"system", map[string]any{"device_key": deviceKey, "status": "offline", "error": errMsg})
		}
		return
	}
	cameBack := p.offline
	p.failures = 0
	p.offline = false
	p.mu.Unlock()

	inserts := make([]storage.ReadingInsert, len(readings))
	for i, rd := range readings {
		inserts[i] = storage.ReadingInsert{
			SensorKey: rd.SensorKey, Value: rd.Value, Unit: rd.Unit, Quality: rd.Quality, ReadAt: rd.ReadAt,
		}
	}
	if err := s.readings.InsertBatch(p.deviceID, inserts); err != nil {
		s.log.Error().Str("sensor", deviceKey).Err(err).Msg("failed to store readings")
		return
	}

	status := "online"
	if s.simulation {
		status = "simulated"
	}
	s.devices.SetStatus(deviceKey, status, nil)
	s.hub.Publish("sensor_reading", map[string]any{"device_key": deviceKey, "readings": readings})
	if cameBack {
		s.events.Info("device_status_change", fmt.Sprintf("sensor %s is back online", deviceKey),
			"system", map[string]any{"device_key": deviceKey, "status": status})
	}
}

// PollNow forces an immediate synchronous poll (POST /sensors/{key}/poll).
func (s *Service) PollNow(ctx context.Context, deviceKey string) ([]Reading, error) {
	s.mu.Lock()
	p, ok := s.pollers[deviceKey]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no running poller for sensor %q", deviceKey)
	}
	readings, err := ReadAll(ctx, s.bus, deviceKey, p.cfg)
	if err != nil {
		return nil, err
	}
	inserts := make([]storage.ReadingInsert, len(readings))
	for i, rd := range readings {
		inserts[i] = storage.ReadingInsert{
			SensorKey: rd.SensorKey, Value: rd.Value, Unit: rd.Unit, Quality: rd.Quality, ReadAt: rd.ReadAt,
		}
	}
	if err := s.readings.InsertBatch(p.deviceID, inserts); err != nil {
		return nil, err
	}
	return readings, nil
}

// UpdateConfig validates and persists a new sensor config, then restarts the
// poller so changes apply without a service restart (spec §11.1).
func (s *Service) UpdateConfig(deviceKey string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	dev, err := s.devices.GetByKey(deviceKey)
	if err != nil {
		return err
	}
	if dev == nil {
		return fmt.Errorf("unknown sensor %q", deviceKey)
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.devices.UpdateConfig(deviceKey, string(body)); err != nil {
		return err
	}
	s.events.Info("sensor_config_updated", fmt.Sprintf("sensor %s configuration updated", deviceKey),
		"api", map[string]any{"device_key": deviceKey})
	dev.ConfigJSON = string(body)
	return s.startPoller(*dev)
}

// SensorStatus is the API view of one sensor device with latest values.
type SensorStatus struct {
	DeviceKey           string        `json:"device_key"`
	Name                string        `json:"name"`
	Type                string        `json:"type"`
	Status              string        `json:"status"`
	ModbusAddress       uint8         `json:"modbus_address"`
	PollIntervalSeconds int           `json:"poll_interval_seconds"`
	Enabled             bool          `json:"enabled"`
	Measurements        []Measurement `json:"measurements"`
}

type Measurement struct {
	SensorKey string     `json:"sensor_key"`
	Name      string     `json:"name"`
	Value     *float64   `json:"value"`
	Unit      string     `json:"unit"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

func (s *Service) ListSensors(ctx context.Context) ([]SensorStatus, error) {
	devices, err := s.sensorDevices()
	if err != nil {
		return nil, err
	}
	out := make([]SensorStatus, 0, len(devices))
	for _, dev := range devices {
		st, err := s.statusFor(dev)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Service) GetSensor(ctx context.Context, deviceKey string) (*SensorStatus, error) {
	dev, err := s.devices.GetByKey(deviceKey)
	if err != nil || dev == nil {
		return nil, err
	}
	st, err := s.statusFor(*dev)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Service) statusFor(dev storage.Device) (SensorStatus, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(dev.ConfigJSON), &cfg); err != nil {
		return SensorStatus{}, fmt.Errorf("bad config for %s: %w", dev.DeviceKey, err)
	}
	st := SensorStatus{
		DeviceKey:           dev.DeviceKey,
		Name:                dev.Name,
		Type:                dev.Type,
		Status:              dev.Status,
		ModbusAddress:       cfg.ModbusAddress,
		PollIntervalSeconds: cfg.PollIntervalSeconds,
		Enabled:             dev.Enabled,
	}
	for _, reg := range cfg.Registers {
		key := reg.SensorKey
		if key == "" {
			key = dev.DeviceKey + "_" + reg.Name
		}
		m := Measurement{SensorKey: key, Name: reg.Name, Unit: reg.Unit}
		if latest, err := s.readings.Latest(key); err == nil && latest != nil {
			m.Value = &latest.Value
			m.ReadAt = &latest.ReadAt
		}
		st.Measurements = append(st.Measurements, m)
	}
	return st, nil
}

func (s *Service) BusHealth(ctx context.Context) modbus.Health {
	return s.bus.Health(ctx)
}

func (s *Service) ScanBus(ctx context.Context, start, end uint8) ([]uint8, error) {
	return s.bus.Scan(ctx, start, end)
}
