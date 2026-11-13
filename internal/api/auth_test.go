package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"propagation-controller/internal/storage"
)

// The public page is read-only for anyone who is not signed in — and the
// operator list is not part of "read-only" (Austin, 2026-11-13).
func TestListUsersAnonymousForbidden(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	s.handleListUsers(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("anonymous GET /users: want 403, got %d (%s)", w.Code, w.Body.String())
	}
}

// Loopback without forwarding headers is the rack console; the same loopback
// socket behind a proxy (cloudflared adds X-Forwarded-For / Cf-Connecting-Ip)
// is an anonymous visitor.
func TestResolveUserLoopbackOnlyWithoutProxyHeaders(t *testing.T) {
	s := &Server{}
	mk := func(remote string, hdr map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		r.RemoteAddr = remote
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	if u := s.resolveUser(mk("127.0.0.1:51000", nil)); u == nil || !u.Implicit || u.Role != "admin" {
		t.Fatalf("bare loopback should be the implicit rack console, got %+v", u)
	}
	if u := s.resolveUser(mk("127.0.0.1:51000", map[string]string{"X-Forwarded-For": "203.0.113.9"})); u != nil {
		t.Fatalf("forwarded loopback must be anonymous, got %+v", u)
	}
	if u := s.resolveUser(mk("127.0.0.1:51000", map[string]string{"Cf-Connecting-Ip": "203.0.113.9"})); u != nil {
		t.Fatalf("cloudflare loopback must be anonymous, got %+v", u)
	}
	if u := s.resolveUser(mk("192.168.5.20:51000", nil)); u != nil {
		t.Fatalf("LAN caller must be anonymous, got %+v", u)
	}
}

// The public event feed never carries an operator's name.
func TestEventsAnonymisedForAnonymousCallers(t *testing.T) {
	steward := "austin-test"
	evs := []storage.Event{
		{EventType: "user_signed_in", Message: steward + " signed in", Actor: &steward},
		{EventType: "user_created", Message: "operator account created: " + steward + " (admin)", Actor: strPtr("system")},
		{EventType: "startup_complete", Message: "propagationd started", Actor: strPtr("system")},
	}
	redactAccountEvents(evs)
	for _, e := range evs[:2] {
		if strings.Contains(e.Message, steward) || (e.Actor != nil && *e.Actor == steward) {
			t.Fatalf("operator name leaked into public event: %+v", e)
		}
	}
	if evs[0].Message != "an operator signed in" || *evs[0].Actor != "operator" {
		t.Fatalf("sign-in not anonymised: %+v", evs[0])
	}
	if evs[2].Message != "propagationd started" || *evs[2].Actor != "system" {
		t.Fatalf("non-account event must pass through untouched: %+v", evs[2])
	}
}

func strPtr(s string) *string { return &s }

func TestStreamPayloadAnonymised(t *testing.T) {
	in := map[string]any{"event_type": "user_signed_in", "message": "austin-test signed in", "actor": "austin-test", "level": "info"}
	out := redactSystemEvent(in).(map[string]any)
	if out["message"] != "an operator signed in" || out["actor"] != "operator" || out["level"] != "info" {
		t.Fatalf("stream payload not anonymised: %+v", out)
	}
	if in["actor"] != "austin-test" {
		t.Fatal("redaction must not mutate the shared payload")
	}
	plain := map[string]any{"event_type": "startup_complete", "message": "propagationd started", "actor": "system"}
	if got := redactSystemEvent(plain).(map[string]any); got["message"] != "propagationd started" {
		t.Fatalf("non-account event altered: %+v", got)
	}
}
