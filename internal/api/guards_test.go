package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/irrigation"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
)

// newGuardServer builds just enough of a Server (mock relays, supervisor,
// engine) to exercise the handler guard rails without the full router.
func newGuardServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	deviceRepo := storage.NewDeviceRepo(db)
	readings := storage.NewReadingRepo(db)
	settings := storage.NewSettingsRepo(db)
	eventLog := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))

	mgr := devices.NewManager(relay.NewMock(), "mock", storage.NewRelayActionRepo(db), deviceRepo, eventLog, true)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	deviceRepo.Upsert("reservoir_level", "Reservoir", "reservoir_sensor", "generic_modbus", "{}", "simulated")
	dev, _ := deviceRepo.GetByKey("reservoir_level")
	cfg := safety.DefaultConfig()
	cfg.SensorStaleAfterSeconds = 3600
	body, _ := json.Marshal(cfg)
	settings.Set("safety", string(body))
	readings.InsertBatch(dev.ID, []storage.ReadingInsert{
		{SensorKey: "reservoir_level_level", Value: 80, Unit: "%", Quality: "good", ReadAt: time.Now()},
	})

	sup := safety.NewSupervisor(mgr, readings, settings, eventLog, zerolog.Nop(), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("supervisor start: %v", err)
	}
	zones := storage.NewZoneRepo(db)
	engine := irrigation.NewEngine(mgr, sup, zones, storage.NewCycleRepo(db), readings, eventLog, zerolog.Nop(), "reservoir_level_level")
	if err := engine.SeedDefaults(); err != nil {
		t.Fatalf("seed zones: %v", err)
	}
	engine.Start(ctx)
	t.Cleanup(func() { engine.StopAll("test") })

	s := &Server{mgr: mgr, safety: sup, irrigation: engine, eventLog: eventLog, log: zerolog.Nop()}
	r := chi.NewRouter()
	r.Post("/relays/{channel}/set", s.handleSetRelay)
	r.Post("/diagnostics/modbus/scan", s.handleModbusScan)
	r.Post("/system/shutdown", s.handleSystemShutdown)
	r.Post("/irrigation/zones/{zone_key}/drain", s.handleDrainZone)
	r.Post("/irrigation/zones/{zone_key}/stop", s.handleStopZone)
	s.zones = zones
	return s, r
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func waitFlooding(t *testing.T, engine *irrigation.Engine) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := engine.Status(); st.Active != nil && st.Active.State == irrigation.StateFlooding {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cycle never reached flooding")
}

func waitIdle(t *testing.T, engine *irrigation.Engine) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := engine.Status(); st.Active == nil && len(st.Queue) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("engine never went idle")
}

// TestPumpOnRequiresBoundedTimeout (audit 2.4): a raw pump-on without a
// 1..60 s timeout is refused.
func TestPumpOnRequiresBoundedTimeout(t *testing.T) {
	_, h := newGuardServer(t)
	for _, body := range []string{`{"state":"on"}`, `{"state":"on","timeout_seconds":0}`, `{"state":"on","timeout_seconds":61}`} {
		if w := post(h, "/relays/7/set", body); w.Code != http.StatusBadRequest {
			t.Errorf("pump-on %s: status %d, want 400", body, w.Code)
		}
	}
	if w := post(h, "/relays/7/set", `{"state":"on","timeout_seconds":5}`); w.Code != http.StatusOK {
		t.Errorf("bounded pump-on: status %d body %s", w.Code, w.Body.String())
	}
	if w := post(h, "/relays/7/set", `{"state":"off"}`); w.Code != http.StatusOK {
		t.Errorf("pump-off: status %d", w.Code)
	}
}

// TestWateringChannelsRefusedDuringCycle (audit 2.4, 2.8): valves and pump
// cannot be commanded on, and the bus cannot be scanned, while a cycle runs;
// lights/fans and any "off" still work.
func TestWateringChannelsRefusedDuringCycle(t *testing.T) {
	s, h := newGuardServer(t)
	if _, err := s.irrigation.Enqueue(irrigation.Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitFlooding(t, s.irrigation)

	if w := post(h, "/relays/2/set", `{"state":"on","timeout_seconds":5}`); w.Code != http.StatusConflict {
		t.Errorf("valve-on during cycle: status %d, want 409", w.Code)
	}
	if w := post(h, "/relays/7/set", `{"state":"on","timeout_seconds":5}`); w.Code != http.StatusConflict {
		t.Errorf("pump-on during cycle: status %d, want 409", w.Code)
	}
	if w := post(h, "/diagnostics/modbus/scan", ``); w.Code != http.StatusConflict {
		t.Errorf("modbus scan during cycle: status %d, want 409", w.Code)
	}
	if w := post(h, "/relays/4/set", `{"state":"on"}`); w.Code != http.StatusOK {
		t.Errorf("fans-on during cycle: status %d body %s", w.Code, w.Body.String())
	}
	if w := post(h, "/relays/8/set", `{"state":"on"}`); w.Code != http.StatusOK {
		t.Errorf("lights-on during cycle: status %d", w.Code)
	}
	// Off is always allowed — even on the pump mid-cycle.
	if w := post(h, "/relays/7/set", `{"state":"off"}`); w.Code != http.StatusOK {
		t.Errorf("pump-off during cycle: status %d", w.Code)
	}
}

// TestSetupOnlyFromLoopback (audit 1.2): the LAN is an IoT subnet; only the
// unit itself may claim first-boot setup.
func TestSetupOnlyFromLoopback(t *testing.T) {
	cases := []struct {
		remote string
		xff    string
		want   bool
	}{
		{"127.0.0.1:5000", "", true},
		{"[::1]:5000", "", true},
		{"192.168.5.20:5000", "", false},
		{"10.0.0.2:5000", "", false},
		{"127.0.0.1:5000", "203.0.113.9", false}, // via a proxy on the box
		{"203.0.113.9:5000", "", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", nil)
		req.RemoteAddr = c.remote
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := setupAllowedFrom(req); got != c.want {
			t.Errorf("setupAllowedFrom(%s xff=%q) = %v, want %v", c.remote, c.xff, got, c.want)
		}
	}
}

// TestSystemShutdownMakesSafeThenPowersOff: the hidden power-down stops the
// engine, forces relays off, answers 202, then calls the power-off hook.
func TestSystemShutdownMakesSafeThenPowersOff(t *testing.T) {
	s, r := newGuardServer(t)
	called := make(chan struct{}, 1)
	s.powerOff = func() error { called <- struct{}{}; return nil }
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/system/shutdown", strings.NewReader(`{"actor":"manual_touch"}`))
	req.RemoteAddr = "127.0.0.1:1"
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case <-called:
	case <-time.After(4 * time.Second):
		t.Fatal("power-off hook not called")
	}
}

// TestHandPurgeUsesZonePurgeSeconds (Austin, 2026-11-13): purge:true on the
// drain endpoint is a drain-only cycle for the zone's purge_seconds, tagged
// "purge"; a zone with purge off refuses it.
func TestHandPurgeUsesZonePurgeSeconds(t *testing.T) {
	s, h := newGuardServer(t)
	zone, err := s.zones.GetByKey("zone_1")
	if err != nil || zone == nil {
		t.Fatalf("zone_1: %v", err)
	}
	var zcfg irrigation.ZoneConfig
	json.Unmarshal([]byte(zone.ConfigJSON), &zcfg)
	zcfg.PurgeSeconds = 45
	cfg, _ := json.Marshal(zcfg)
	zone.ConfigJSON = string(cfg)
	if err := s.zones.Update(*zone); err != nil {
		t.Fatalf("update zone: %v", err)
	}

	w := post(h, "/irrigation/zones/zone_1/drain", `{"actor":"manual_touch","purge":true}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("purge: status %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["duration_seconds"] != float64(45) || resp["purge"] != true {
		t.Errorf("purge response %v, want duration 45 and purge true", resp)
	}
	// A second request for the same shelf lands in conflict (already running or queued).
	if w := post(h, "/irrigation/zones/zone_1/drain", `{"actor":"manual_touch","purge":true}`); w.Code != http.StatusConflict {
		t.Errorf("duplicate purge: status %d, want 409", w.Code)
	}
	s.irrigation.StopAll("test")
	waitIdle(t, s.irrigation)

	// Purge off for the zone → refused, nothing queued.
	zcfg.PurgeSeconds = 0
	cfg, _ = json.Marshal(zcfg)
	zone.ConfigJSON = string(cfg)
	if err := s.zones.Update(*zone); err != nil {
		t.Fatalf("update zone: %v", err)
	}
	if w := post(h, "/irrigation/zones/zone_1/drain", `{"actor":"manual_touch","purge":true}`); w.Code != http.StatusConflict {
		t.Errorf("purge with purge off: status %d body %s, want 409", w.Code, w.Body.String())
	}
	if st := s.irrigation.Status(); st.Active != nil || len(st.Queue) != 0 {
		t.Errorf("refused purge still reached the engine: %+v", st)
	}
}

// TestStopDequeuesWaitingShelf: a shelf queued behind another's cycle can be
// taken back out with the stop endpoint (touch-screen Cancel, 2026-11-13).
func TestStopDequeuesWaitingShelf(t *testing.T) {
	s, h := newGuardServer(t)
	if _, err := s.irrigation.Enqueue(irrigation.Request{ZoneKey: "zone_1", DurationSeconds: 60, TriggerType: "manual_web"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitFlooding(t, s.irrigation)
	if w := post(h, "/irrigation/zones/zone_2/drain", `{"actor":"manual_touch"}`); w.Code != http.StatusAccepted {
		t.Fatalf("queue drain: status %d body %s", w.Code, w.Body.String())
	}
	if st := s.irrigation.Status(); len(st.Queue) != 1 || st.Queue[0].ZoneKey != "zone_2" {
		t.Fatalf("queue = %+v, want zone_2 waiting", st.Queue)
	}
	if w := post(h, "/irrigation/zones/zone_2/stop", `{"actor":"manual_touch"}`); w.Code != http.StatusOK {
		t.Fatalf("cancel queued: status %d body %s", w.Code, w.Body.String())
	}
	st := s.irrigation.Status()
	if len(st.Queue) != 0 {
		t.Errorf("queue still holds %+v after cancel", st.Queue)
	}
	if st.Active == nil || st.Active.ZoneKey != "zone_1" {
		t.Errorf("cancelling the queued shelf disturbed the running one: %+v", st.Active)
	}
}
