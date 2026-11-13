package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
)

type capture struct {
	mu   sync.Mutex
	reqs []*http.Request
	body []string
}

func (c *capture) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		c.mu.Lock()
		c.reqs = append(c.reqs, r)
		c.body = append(c.body, string(buf[:n]))
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (c *capture) wait(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := len(c.reqs)
		c.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d requests", n)
}

func lockoutEvent(kind, lockout, message string) map[string]any {
	return map[string]any{
		"level": "warning", "event_type": kind, "message": message,
		"actor": "safety_supervisor", "details": map[string]any{"lockout": lockout},
	}
}

func TestReservoirLockoutPushesToNtfy(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(c.handler(t))
	defer srv.Close()

	hub := bus.New()
	svc := New(Config{UnitName: "Unit 01", NtfyServer: srv.URL, NtfyTopic: "pu01-test"}, hub, zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	hub.Publish("system_event", lockoutEvent("safety_lockout", "reservoir_low", "reservoir level 18.0 below minimum 20.0"))
	c.wait(t, 1)
	if c.reqs[0].URL.Path != "/pu01-test" {
		t.Errorf("path = %q", c.reqs[0].URL.Path)
	}
	if got := c.reqs[0].Header.Get("Title"); got != "Unit 01: Reservoir empty" {
		t.Errorf("title = %q", got)
	}
	if got := c.reqs[0].Header.Get("Priority"); got != "urgent" {
		t.Errorf("priority = %q", got)
	}
	if c.body[0] == "" || c.body[0][:16] != "Pump locked out:" {
		t.Errorf("body = %q", c.body[0])
	}

	// The clear is a quiet follow-up.
	hub.Publish("system_event", lockoutEvent("safety_lockout_cleared", "reservoir_low", "lockout reservoir_low cleared"))
	c.wait(t, 2)
	if got := c.reqs[1].Header.Get("Title"); got != "Unit 01: Reservoir refilled" {
		t.Errorf("clear title = %q", got)
	}
	if got := c.reqs[1].Header.Get("Priority"); got != "default" {
		t.Errorf("clear priority = %q", got)
	}
}

func TestOnlyReservoirEventsArePushed(t *testing.T) {
	for _, tc := range []struct {
		kind, lockout string
		want          bool
	}{
		{"safety_lockout", "reservoir_low", true},
		{"safety_lockout", "reservoir_sensor_offline", true},
		{"safety_lockout_cleared", "reservoir_low", true},
		{"safety_lockout", "pump_runtime", false},
		{"safety_lockout", "emergency_stop", false},
		{"moisture_trigger_blocked", "", false},
		{"user_signed_in", "", false},
	} {
		_, _, ok := Translate(lockoutEvent(tc.kind, tc.lockout, "x"))
		if ok != tc.want {
			t.Errorf("%s/%s pushed = %v, want %v", tc.kind, tc.lockout, ok, tc.want)
		}
	}
	if _, _, ok := Translate(nil); ok {
		t.Error("nil payload must not push")
	}
}

func TestFlappingLockoutIsDeduped(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(c.handler(t))
	defer srv.Close()
	hub := bus.New()
	svc := New(Config{NtfyServer: srv.URL, NtfyTopic: "t"}, hub, zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	for i := 0; i < 5; i++ {
		hub.Publish("system_event", lockoutEvent("safety_lockout", "reservoir_low", "low"))
	}
	c.wait(t, 1)
	time.Sleep(100 * time.Millisecond)
	c.mu.Lock()
	n := len(c.reqs)
	c.mu.Unlock()
	if n != 1 {
		t.Errorf("sent %d pushes for one flapping condition, want 1", n)
	}
}

func TestTwilioPostsFormWithBasicAuth(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(c.handler(t))
	defer srv.Close()
	old := twilioBase
	twilioBase = srv.URL
	defer func() { twilioBase = old }()

	hub := bus.New()
	svc := New(Config{UnitName: "Unit 01", TwilioAccountSID: "ACtest", TwilioAuthToken: "tok",
		TwilioFrom: "+15550001111", TwilioTo: "+15550002222"}, hub, zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	hub.Publish("system_event", lockoutEvent("safety_lockout", "reservoir_low", "reservoir level 18.0 below minimum 20.0"))
	c.wait(t, 1)
	r := c.reqs[0]
	if r.URL.Path != "/2010-04-01/Accounts/ACtest/Messages.json" {
		t.Errorf("path = %q", r.URL.Path)
	}
	if u, p, ok := r.BasicAuth(); !ok || u != "ACtest" || p != "tok" {
		t.Errorf("basic auth = %q/%q/%v", u, p, ok)
	}
	if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
	}
	for _, want := range []string{"To=%2B15550002222", "From=%2B15550001111", "Reservoir+empty"} {
		if !contains(c.body[0], want) {
			t.Errorf("form body %q lacks %q", c.body[0], want)
		}
	}
}

func TestDisabledServiceIsInert(t *testing.T) {
	hub := bus.New()
	svc := New(Config{}, hub, zerolog.Nop())
	svc.Start(context.Background())
	hub.Publish("system_event", lockoutEvent("safety_lockout", "reservoir_low", "low"))
	if svc.cfg.Enabled() || len(svc.cfg.Transports()) != 0 {
		t.Error("empty config must be disabled")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
