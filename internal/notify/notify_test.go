package notify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
	"propagation-controller/internal/storage"
)

func openRepo(t *testing.T) *storage.PushRepo {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return storage.NewPushRepo(db)
}

type fakeSender struct {
	mu    sync.Mutex
	sent  []Message
	to    []string
	codes map[string]int
}

func (f *fakeSender) fn(_ context.Context, sub storage.PushSubscription, payload []byte, _ bool) (int, error) {
	var m Message
	_ = json.Unmarshal(payload, &m)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	f.to = append(f.to, sub.Endpoint)
	if c, ok := f.codes[sub.Endpoint]; ok {
		return c, nil
	}
	return 201, nil
}

func (f *fakeSender) wait(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := len(f.sent)
		f.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d pushes", n)
}

func newSvc(t *testing.T, repo *storage.PushRepo, fs *fakeSender) (*Service, *bus.Hub) {
	hub := bus.New()
	s := New(Keys{PublicKey: "pub", PrivateKey: "priv"}, repo, hub, nil, nil, nil, zerolog.Nop())
	s.sendFn = fs.fn
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.Start(ctx)
	return s, hub
}

func ev(kind, msg string, details map[string]any) map[string]any {
	return map[string]any{"event_type": kind, "message": msg, "details": details, "actor": "system"}
}

func TestKeysPersistAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreateKeys(dir)
	if err != nil || a.PublicKey == "" || a.PrivateKey == "" {
		t.Fatalf("keys: %+v %v", a, err)
	}
	b, _ := LoadOrCreateKeys(dir)
	if a != b {
		t.Error("second load generated new keys")
	}
}

func TestTranslateCategories(t *testing.T) {
	s := New(Keys{}, nil, nil, nil, nil, nil, zerolog.Nop())
	s.names["zone_2"] = zoneInfo{N: "2", Species: "Buttonbush · Inland Sea Oats"}
	cases := []struct {
		kind    string
		details map[string]any
		cat     string
		title   string
		ok      bool
	}{
		{"irrigation_cycle_started", map[string]any{"zone": "zone_2"}, CatWatering, "Shelf 2 watering", true},
		{"irrigation_cycle_complete", map[string]any{"zone": "zone_2"}, CatWatering, "Shelf 2 watered", true},
		{"irrigation_cycle_aborted", map[string]any{"zone": "zone_2"}, CatWatering, "Shelf 2 watering aborted", true},
		{"irrigation_cycle_failed", map[string]any{"zone": "zone_2"}, CatWatering, "Shelf 2 watering failed", true},
		{"moisture_trigger", map[string]any{"zone": "zone_2"}, CatWatering, "Shelf 2 below band · watering", true},
		{"moisture_trigger_blocked", map[string]any{"zone": "zone_2"}, CatDry, "Shelf 2 below band · not watering", true},
		{"safety_lockout", map[string]any{"lockout": "reservoir_low"}, CatReservoir, "Reservoir empty", true},
		{"safety_lockout_cleared", map[string]any{"lockout": "reservoir_low"}, CatReservoir, "Reservoir refilled", true},
		{"safety_lockout", map[string]any{"lockout": "emergency_stop"}, "", "", false},
		{"device_status_change", map[string]any{"device_key": "soil_zone_2", "status": "offline"}, CatFaults, "Sensor offline", true},
		{"user_signed_in", nil, "", "", false},
		{"lighting_override", nil, "", "", false},
	}
	for _, c := range cases {
		m, _, ok := s.Translate(ev(c.kind, "x", c.details))
		if ok != c.ok {
			t.Errorf("%s: ok=%v want %v", c.kind, ok, c.ok)
			continue
		}
		if ok && (m.Category != c.cat || m.Title != c.title) {
			t.Errorf("%s: got %q/%q want %q/%q", c.kind, m.Category, m.Title, c.cat, c.title)
		}
	}
	m, _, _ := s.Translate(ev("irrigation_cycle_started", "x", map[string]any{"zone": "zone_2"}))
	if m.Body != "Buttonbush · Inland Sea Oats · soak started" {
		t.Errorf("body = %q", m.Body)
	}
}

func TestPrefsGateDelivery(t *testing.T) {
	repo := openRepo(t)
	fs := &fakeSender{}
	_, hub := newSvc(t, repo, fs)
	_ = repo.Upsert(storage.PushSubscription{Username: "a", Endpoint: "https://p/1", P256dh: "k", Auth: "a", PrefsJSON: `{"watering":true,"dry":false}`})
	_ = repo.Upsert(storage.PushSubscription{Username: "b", Endpoint: "https://p/2", P256dh: "k", Auth: "a", PrefsJSON: `{"watering":false,"dry":true}`})
	_ = repo.Upsert(storage.PushSubscription{Username: "c", Endpoint: "https://p/3", P256dh: "k", Auth: "a", PrefsJSON: `{}`}) // defaults

	hub.Publish("system_event", ev("irrigation_cycle_started", "x", map[string]any{"zone": "zone_1"}))
	fs.wait(t, 2)
	time.Sleep(50 * time.Millisecond)
	fs.mu.Lock()
	got := append([]string{}, fs.to...)
	fs.mu.Unlock()
	if len(got) != 2 || got[0] != "https://p/1" || got[1] != "https://p/3" {
		t.Errorf("watering went to %v, want devices 1 and 3", got)
	}
	// faults are opt-in: the default device must not get them
	hub.Publish("system_event", ev("device_status_change", "x", map[string]any{"device_key": "d", "status": "offline"}))
	time.Sleep(100 * time.Millisecond)
	fs.mu.Lock()
	n := len(fs.sent)
	fs.mu.Unlock()
	if n != 2 {
		t.Errorf("faults reached %d devices with defaults, want none", n-2)
	}
}

func TestGoneSubscriptionIsRemoved(t *testing.T) {
	repo := openRepo(t)
	fs := &fakeSender{codes: map[string]int{"https://p/gone": 410}}
	_, hub := newSvc(t, repo, fs)
	_ = repo.Upsert(storage.PushSubscription{Username: "a", Endpoint: "https://p/gone", P256dh: "k", Auth: "a", PrefsJSON: `{}`})
	_ = repo.Upsert(storage.PushSubscription{Username: "a", Endpoint: "https://p/ok", P256dh: "k", Auth: "a", PrefsJSON: `{}`})
	hub.Publish("system_event", ev("safety_lockout", "low", map[string]any{"lockout": "reservoir_low"}))
	fs.wait(t, 2)
	time.Sleep(50 * time.Millisecond)
	subs, _ := repo.List()
	if len(subs) != 1 || subs[0].Endpoint != "https://p/ok" {
		t.Errorf("after a 410, subscriptions = %+v", subs)
	}
	if subs[0].LastOKAt == nil {
		t.Error("successful push did not mark last_ok_at")
	}
}

func TestFlappingIsDeduped(t *testing.T) {
	repo := openRepo(t)
	fs := &fakeSender{}
	_, hub := newSvc(t, repo, fs)
	_ = repo.Upsert(storage.PushSubscription{Username: "a", Endpoint: "https://p/1", P256dh: "k", Auth: "a", PrefsJSON: `{}`})
	for i := 0; i < 4; i++ {
		hub.Publish("system_event", ev("safety_lockout", "low", map[string]any{"lockout": "reservoir_low"}))
	}
	fs.wait(t, 1)
	time.Sleep(100 * time.Millisecond)
	fs.mu.Lock()
	n := len(fs.sent)
	fs.mu.Unlock()
	if n != 1 {
		t.Errorf("sent %d for one flapping condition, want 1", n)
	}
}

func TestDryTransitions(t *testing.T) {
	ra := 30.0
	rules := []storage.MoistureRule{{ZoneKey: "zone_1", SensorKey: "s1", TriggerBelow: 25, RecoveredAbove: &ra}}
	now := time.Now()
	readings := map[string]float64{"s1": 28}
	latest := func(k string) *storage.StoredReading {
		return &storage.StoredReading{SensorKey: k, Value: readings[k], ReadAt: now}
	}
	flagged := map[string]bool{}
	if got := DryTransitions(flagged, rules, latest, now); len(got) != 0 {
		t.Fatalf("in band: crossed %v", got)
	}
	readings["s1"] = 24
	if got := DryTransitions(flagged, rules, latest, now); len(got) != 1 {
		t.Fatalf("fell below: crossed %v", got)
	}
	readings["s1"] = 23
	if got := DryTransitions(flagged, rules, latest, now); len(got) != 0 {
		t.Fatalf("still below: should not re-push, got %v", got)
	}
	readings["s1"] = 27 // above trigger but below recovered_above: still dry
	if got := DryTransitions(flagged, rules, latest, now); len(got) != 0 || !flagged["zone_1"] {
		t.Fatalf("hysteresis: %v %v", got, flagged)
	}
	readings["s1"] = 31
	DryTransitions(flagged, rules, latest, now)
	if flagged["zone_1"] {
		t.Fatal("recovered above 30 should clear the flag")
	}
	readings["s1"] = 10
	stale := func(k string) *storage.StoredReading {
		return &storage.StoredReading{SensorKey: k, Value: 10, ReadAt: now.Add(-10 * time.Minute)}
	}
	if got := DryTransitions(flagged, rules, stale, now); len(got) != 0 {
		t.Fatal("a stale reading must not raise dry")
	}
}

func TestPrefsFromJSON(t *testing.T) {
	p := PrefsFromJSON(`{"faults":true,"bogus":true,"dry":false}`)
	if !p[CatFaults] || p[CatDry] || !p[CatWatering] || !p[CatReservoir] {
		t.Errorf("prefs = %v", p)
	}
	if _, ok := p["bogus"]; ok {
		t.Error("unknown category kept")
	}
}

type fakeRules struct{ rules []storage.MoistureRule }

func (f fakeRules) ListEnabled() ([]storage.MoistureRule, error) { return f.rules, nil }

type fakeReadings struct {
	mu sync.Mutex
	v  map[string]float64
}

func (f *fakeReadings) Latest(k string) (*storage.StoredReading, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &storage.StoredReading{SensorKey: k, Value: f.v[k], ReadAt: time.Now()}, nil
}
func (f *fakeReadings) set(k string, v float64) { f.mu.Lock(); f.v[k] = v; f.mu.Unlock() }

func TestDryWatcherQuietInMaintenance(t *testing.T) {
	repo := openRepo(t)
	_ = repo.Upsert(storage.PushSubscription{Username: "a", Endpoint: "https://p/1", P256dh: "k", Auth: "a", PrefsJSON: `{}`})
	fs := &fakeSender{}
	rd := &fakeReadings{v: map[string]float64{"s1": 30}}
	hub := bus.New()
	s := New(Keys{}, repo, hub, nil, fakeRules{[]storage.MoistureRule{{ZoneKey: "zone_1", SensorKey: "s1", TriggerBelow: 25}}}, rd, zerolog.Nop())
	s.sendFn = fs.fn
	s.tick = 20 * time.Millisecond
	maint := true
	s.Maintenance = func() bool { return maint }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	time.Sleep(50 * time.Millisecond) // first tick swallowed, in band anyway
	rd.set("s1", 10)                  // probe pulled into a dry test pot
	time.Sleep(100 * time.Millisecond)
	fs.mu.Lock()
	n := len(fs.sent)
	fs.mu.Unlock()
	if n != 0 {
		t.Fatalf("pushed %d dry alerts during maintenance, want 0", n)
	}
	// Back in its pot and maintenance off: a real drop must still page.
	rd.set("s1", 30)
	time.Sleep(60 * time.Millisecond)
	maint = false
	rd.set("s1", 10)
	fs.wait(t, 1)
	if fs.sent[0].Category != CatDry || fs.sent[0].Title != "Shelf 1 dry" {
		t.Errorf("got %+v", fs.sent[0])
	}
}
