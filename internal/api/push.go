package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"propagation-controller/internal/notify"
	"propagation-controller/internal/storage"
)

// Web Push (2026-11-15). Every route needs a real signed-in operator (the
// rack console's implicit identity cannot subscribe: the kiosk has no phone),
// and a browser may only read or change its own subscription.

func (s *Server) pushUser(w http.ResponseWriter, r *http.Request) *requestUser {
	u := currentUser(r)
	if u == nil || u.Implicit {
		writeError(w, http.StatusUnauthorized, "sign in to manage notifications")
		return nil
	}
	return u
}

func (s *Server) handlePushVAPID(w http.ResponseWriter, r *http.Request) {
	if s.push == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"public_key": s.push.PublicKey(), "categories": notify.Categories})
}

type pushBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Prefs map[string]bool `json:"prefs"`
	Label string          `json:"label"`
}

func readPushBody(w http.ResponseWriter, r *http.Request) (*pushBody, bool) {
	var b pushBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&b); err != nil || strings.TrimSpace(b.Endpoint) == "" {
		writeError(w, http.StatusBadRequest, "endpoint required")
		return nil, false
	}
	if !strings.HasPrefix(b.Endpoint, "https://") {
		writeError(w, http.StatusBadRequest, "endpoint must be an https push service URL")
		return nil, false
	}
	return &b, true
}

func pushView(sub *storage.PushSubscription) map[string]any {
	return map[string]any{
		"endpoint": sub.Endpoint, "prefs": notify.PrefsFromJSON(sub.PrefsJSON), "label": sub.Label,
		"created_at": sub.CreatedAt, "last_ok_at": sub.LastOKAt, "failures": sub.Failures,
	}
}

// POST /push/subscriptions/lookup {endpoint} → this browser's switches.
func (s *Server) handlePushLookup(w http.ResponseWriter, r *http.Request) {
	u := s.pushUser(w, r)
	if u == nil {
		return
	}
	b, ok := readPushBody(w, r)
	if !ok {
		return
	}
	sub, err := s.pushRepo.GetByEndpoint(b.Endpoint)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sub == nil || sub.Username != u.Username {
		writeJSON(w, http.StatusOK, map[string]any{"subscription": nil, "defaults": notify.DefaultPrefs()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": pushView(sub)})
}

// PUT /push/subscriptions {endpoint, keys, prefs, label} → subscribe / update.
func (s *Server) handlePushUpsert(w http.ResponseWriter, r *http.Request) {
	u := s.pushUser(w, r)
	if u == nil {
		return
	}
	b, ok := readPushBody(w, r)
	if !ok {
		return
	}
	existing, _ := s.pushRepo.GetByEndpoint(b.Endpoint)
	if existing != nil && existing.Username != u.Username {
		writeError(w, http.StatusForbidden, "this browser is subscribed under another operator")
		return
	}
	p256, auth := b.Keys.P256dh, b.Keys.Auth
	if (p256 == "" || auth == "") && existing != nil {
		p256, auth = existing.P256dh, existing.Auth // prefs-only update
	}
	if p256 == "" || auth == "" {
		writeError(w, http.StatusBadRequest, "subscription keys required")
		return
	}
	prefs := notify.DefaultPrefs()
	if existing != nil {
		prefs = notify.PrefsFromJSON(existing.PrefsJSON)
	}
	for k, v := range b.Prefs {
		if _, known := prefs[k]; known {
			prefs[k] = v
		}
	}
	pj, _ := json.Marshal(prefs)
	label := strings.TrimSpace(b.Label)
	if label == "" && existing != nil {
		label = existing.Label // prefs-only update keeps the device name
	}
	sub := storage.PushSubscription{Username: u.Username, Endpoint: b.Endpoint, P256dh: p256, Auth: auth,
		PrefsJSON: string(pj), Label: label}
	if len(sub.Label) > 80 {
		sub.Label = sub.Label[:80]
	}
	if err := s.pushRepo.Upsert(sub); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		s.eventLog.Info("push_subscribed", "an operator enabled phone notifications", u.Username,
			map[string]any{"label": sub.Label})
	}
	saved, _ := s.pushRepo.GetByEndpoint(b.Endpoint)
	writeJSON(w, http.StatusOK, map[string]any{"subscription": pushView(saved)})
}

// DELETE /push/subscriptions {endpoint}
func (s *Server) handlePushDelete(w http.ResponseWriter, r *http.Request) {
	u := s.pushUser(w, r)
	if u == nil {
		return
	}
	b, ok := readPushBody(w, r)
	if !ok {
		return
	}
	existing, _ := s.pushRepo.GetByEndpoint(b.Endpoint)
	if existing != nil && existing.Username != u.Username {
		writeError(w, http.StatusForbidden, "this browser is subscribed under another operator")
		return
	}
	if err := s.pushRepo.Delete(b.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /push/subscriptions/test {endpoint} → one push to that browser.
func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	u := s.pushUser(w, r)
	if u == nil {
		return
	}
	if s.push == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	b, ok := readPushBody(w, r)
	if !ok {
		return
	}
	sub, _ := s.pushRepo.GetByEndpoint(b.Endpoint)
	if sub == nil || sub.Username != u.Username {
		writeError(w, http.StatusNotFound, "this browser is not subscribed")
		return
	}
	err := s.push.SendTo(r.Context(), *sub, notify.Message{Category: "test", Title: "Propagation Unit 01",
		Body: "Notifications are working on this device.", Tag: "test"})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
