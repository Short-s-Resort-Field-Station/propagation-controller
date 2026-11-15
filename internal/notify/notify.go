// Package notify pushes rack conditions to operators' phones over Web Push.
//
// Austin, 2026-11-15: "build this and scrap the ntfy idea. I want to be able
// to control notifications for watering, shelves being below their watering
// band." The dashboard is an installed PWA; its service worker shows what the
// daemon sends here. No third-party app, no account: a VAPID key pair lives
// in the data dir, each browser that opted in is a row in push_subscriptions
// with its own category switches, and the daemon encrypts and posts straight
// to the browser's push service (outbound HTTPS only).
//
// Categories: watering (cycles start / finish / fail), dry (a shelf below its
// band, whether or not automation is allowed to act), reservoir (the pump
// lockouts), faults (probes and devices going offline; off by default).
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
	"propagation-controller/internal/storage"
)

// Keys is the unit's VAPID identity. Generated once, kept at
// <data-dir>/vapid.json (0600), never served except the public half.
type Keys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	Subject    string `json:"subject"`
}

const DefaultSubject = "https://shortsfieldstation.org/propagate/"

func LoadOrCreateKeys(dataDir string) (Keys, error) {
	path := filepath.Join(dataDir, "vapid.json")
	var k Keys
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &k); err == nil && k.PublicKey != "" && k.PrivateKey != "" {
			if k.Subject == "" {
				k.Subject = DefaultSubject
			}
			return k, nil
		}
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return Keys{}, fmt.Errorf("generate vapid keys: %w", err)
	}
	k = Keys{PublicKey: pub, PrivateKey: priv, Subject: DefaultSubject}
	b, _ := json.MarshalIndent(k, "", "  ")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return Keys{}, fmt.Errorf("write %s: %w", path, err)
	}
	return k, nil
}

// Categories, in the order the dashboard lists them.
const (
	CatWatering  = "watering"
	CatDry       = "dry"
	CatReservoir = "reservoir"
	CatFaults    = "faults"
)

var Categories = []string{CatWatering, CatDry, CatReservoir, CatFaults}

// DefaultPrefs: the two Austin asked for plus the reservoir; faults opt-in.
func DefaultPrefs() map[string]bool {
	return map[string]bool{CatWatering: true, CatDry: true, CatReservoir: true, CatFaults: false}
}

func PrefsFromJSON(raw string) map[string]bool {
	p := DefaultPrefs()
	var in map[string]bool
	if json.Unmarshal([]byte(raw), &in) == nil {
		for k, v := range in {
			if _, known := p[k]; known {
				p[k] = v
			}
		}
	}
	return p
}

// Message is what the service worker receives.
type Message struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Tag      string `json:"tag,omitempty"`
	URL      string `json:"url,omitempty"`
	Category string `json:"category"`
	Urgent   bool   `json:"-"`
}

type zonesLister interface {
	List() ([]storage.Zone, error)
}
type rulesLister interface {
	ListEnabled() ([]storage.MoistureRule, error)
}
type readingsLatest interface {
	Latest(sensorKey string) (*storage.StoredReading, error)
}

type Service struct {
	keys     Keys
	subs     *storage.PushRepo
	hub      *bus.Hub
	zones    zonesLister
	rules    rulesLister
	readings readingsLatest
	log      zerolog.Logger

	mu       sync.Mutex
	lastSent map[string]time.Time
	window   time.Duration
	dryFlag  map[string]bool // zone_key → currently below band
	dryInit  bool
	names    map[string]zoneInfo

	// sendFn is swapped by the tests; production posts through webpush-go.
	sendFn func(ctx context.Context, sub storage.PushSubscription, payload []byte, urgent bool) (int, error)
	tick   time.Duration
	// Maintenance reports the rack's maintenance switch. While it is on the
	// dry watcher stays quiet: pulling probes for calibration (Austin,
	// 2026-11-15) must not page anyone, same as the dashboard alarms.
	Maintenance func() bool
}

type zoneInfo struct {
	N       string
	Species string
}

func New(keys Keys, subs *storage.PushRepo, hub *bus.Hub, zones zonesLister, rules rulesLister, readings readingsLatest, log zerolog.Logger) *Service {
	s := &Service{
		keys: keys, subs: subs, hub: hub, zones: zones, rules: rules, readings: readings,
		log:      log.With().Str("component", "notify").Logger(),
		lastSent: map[string]time.Time{},
		window:   10 * time.Minute,
		dryFlag:  map[string]bool{},
		names:    map[string]zoneInfo{},
		tick:     60 * time.Second,
	}
	s.sendFn = s.sendWebPush
	return s
}

func (s *Service) PublicKey() string { return s.keys.PublicKey }

// Start subscribes to the hub and runs the dry watcher. Returns at once.
func (s *Service) Start(ctx context.Context) {
	if s.hub == nil {
		return
	}
	s.refreshNames()
	ch, cancel := s.hub.Subscribe()
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, open := <-ch:
				if !open {
					return
				}
				if ev.Type != "system_event" {
					continue
				}
				payload, _ := ev.Payload.(map[string]any)
				if msg, key, ok := s.Translate(payload); ok {
					s.Deliver(ctx, key, msg)
				}
			}
		}
	}()
	if s.rules != nil && s.readings != nil {
		go func() {
			t := time.NewTicker(s.tick)
			defer t.Stop()
			s.dryTick(ctx)
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					s.dryTick(ctx)
				}
			}
		}()
	}
}

func (s *Service) refreshNames() {
	if s.zones == nil {
		return
	}
	zs, err := s.zones.List()
	if err != nil {
		return
	}
	names := map[string]zoneInfo{}
	for _, z := range zs {
		var cfg struct {
			Species  string `json:"species"`
			Species2 string `json:"species2"`
		}
		_ = json.Unmarshal([]byte(z.ConfigJSON), &cfg)
		sp := cfg.Species
		if cfg.Species2 != "" {
			sp = sp + " · " + cfg.Species2
		}
		if sp == "" {
			sp = z.Name
		}
		names[z.ZoneKey] = zoneInfo{N: shelfNumber(z.ZoneKey), Species: sp}
	}
	s.mu.Lock()
	s.names = names
	s.mu.Unlock()
}

func shelfNumber(zoneKey string) string {
	if i := strings.LastIndex(zoneKey, "_"); i >= 0 && i+1 < len(zoneKey) {
		return zoneKey[i+1:]
	}
	return zoneKey
}

func (s *Service) shelf(zoneKey string) (string, string) {
	s.mu.Lock()
	zi, ok := s.names[zoneKey]
	s.mu.Unlock()
	if !ok {
		return shelfNumber(zoneKey), ""
	}
	return zi.N, zi.Species
}

// Translate maps a system_event payload to a push, or reports it is not one
// we send. The dedupe key keeps a flapping condition to one push per window.
func (s *Service) Translate(p map[string]any) (Message, string, bool) {
	if p == nil {
		return Message{}, "", false
	}
	eventType, _ := p["event_type"].(string)
	details, _ := p["details"].(map[string]any)
	text, _ := p["message"].(string)
	zoneKey, _ := details["zone"].(string)
	if zoneKey == "" {
		zoneKey, _ = details["zone_key"].(string)
	}
	n, species := s.shelf(zoneKey)
	with := func(body string) string {
		if species != "" {
			return species + " · " + body
		}
		return body
	}

	switch {
	case eventType == "irrigation_cycle_started":
		return Message{Category: CatWatering, Title: "Shelf " + n + " watering", Body: with("soak started"), Tag: "water-" + zoneKey}, "start-" + zoneKey + "-" + text, true
	case eventType == "irrigation_cycle_complete":
		return Message{Category: CatWatering, Title: "Shelf " + n + " watered", Body: with("cycle complete, tray draining back"), Tag: "water-" + zoneKey}, "done-" + zoneKey + "-" + text, true
	case eventType == "irrigation_cycle_skipped":
		return Message{Category: CatWatering, Title: "Shelf " + n + " watering skipped", Body: text, Tag: "water-" + zoneKey}, "skip-" + zoneKey, true
	case eventType == "irrigation_cycle_failed":
		return Message{Category: CatWatering, Title: "Shelf " + n + " watering failed", Body: text, Tag: "water-" + zoneKey, Urgent: true}, "fail-" + zoneKey, true
	case strings.HasPrefix(eventType, "irrigation_cycle_"):
		result := strings.TrimPrefix(eventType, "irrigation_cycle_")
		return Message{Category: CatWatering, Title: "Shelf " + n + " watering " + result, Body: text, Tag: "water-" + zoneKey}, result + "-" + zoneKey, true
	case eventType == "moisture_trigger":
		return Message{Category: CatWatering, Title: "Shelf " + n + " below band · watering", Body: with(text), Tag: "dry-" + zoneKey}, "trigger-" + zoneKey, true
	case eventType == "moisture_trigger_blocked":
		return Message{Category: CatDry, Title: "Shelf " + n + " below band · not watering", Body: with(text), Tag: "dry-" + zoneKey}, "blocked-" + zoneKey, true
	case eventType == "safety_lockout" || eventType == "safety_lockout_cleared":
		lockout, _ := details["lockout"].(string)
		cleared := eventType == "safety_lockout_cleared"
		switch lockout {
		case "reservoir_low":
			if cleared {
				return Message{Category: CatReservoir, Title: "Reservoir refilled", Body: "Level is back above the cutoff; watering can resume.", Tag: "reservoir"}, "reservoir_low_cleared", true
			}
			return Message{Category: CatReservoir, Title: "Reservoir empty", Body: "Pump locked out: " + text + ". Refill the reservoir.", Tag: "reservoir", Urgent: true}, "reservoir_low", true
		case "reservoir_sensor_offline":
			if cleared {
				return Message{Category: CatReservoir, Title: "Reservoir sensor back", Body: "The level sensor is reporting again; watering can resume.", Tag: "reservoir"}, "reservoir_offline_cleared", true
			}
			return Message{Category: CatReservoir, Title: "Reservoir sensor offline", Body: "Pump locked out: " + text + ". Check the level sensor.", Tag: "reservoir", Urgent: true}, "reservoir_offline", true
		}
	case eventType == "device_status_change":
		dev, _ := details["device_key"].(string)
		status, _ := details["status"].(string)
		if status == "offline" {
			return Message{Category: CatFaults, Title: "Sensor offline", Body: text, Tag: "dev-" + dev, Urgent: true}, "dev-off-" + dev, true
		}
		return Message{Category: CatFaults, Title: "Sensor back online", Body: text, Tag: "dev-" + dev}, "dev-on-" + dev, true
	}
	return Message{}, "", false
}

// DryTransitions is the pure core of the dry watcher: given which shelves
// were below band last tick, say which just crossed down (push) and update
// the set. A reading older than five minutes is ignored, like the dashboard.
func DryTransitions(flagged map[string]bool, rules []storage.MoistureRule, latest func(string) *storage.StoredReading, now time.Time) []storage.MoistureRule {
	var crossed []storage.MoistureRule
	for _, r := range rules {
		rd := latest(r.SensorKey)
		if rd == nil || now.Sub(rd.ReadAt) > 5*time.Minute || rd.Quality == storage.QualityOutOfMedia {
			continue
		}
		recoverAt := r.TriggerBelow
		if r.RecoveredAbove != nil {
			recoverAt = *r.RecoveredAbove
		}
		switch {
		case rd.Value < r.TriggerBelow && !flagged[r.ZoneKey]:
			flagged[r.ZoneKey] = true
			crossed = append(crossed, r)
		case rd.Value >= recoverAt && flagged[r.ZoneKey]:
			flagged[r.ZoneKey] = false
		}
	}
	return crossed
}

func (s *Service) dryTick(ctx context.Context) {
	s.refreshNames()
	rules, err := s.rules.ListEnabled()
	if err != nil {
		return
	}
	latest := func(key string) *storage.StoredReading {
		rd, err := s.readings.Latest(key)
		if err != nil {
			return nil
		}
		return rd
	}
	s.mu.Lock()
	first := !s.dryInit
	s.dryInit = true
	crossed := DryTransitions(s.dryFlag, rules, latest, time.Now())
	s.mu.Unlock()
	if first || (s.Maintenance != nil && s.Maintenance()) {
		return // startup state is not news; maintenance means probes may be out
	}
	for _, r := range crossed {
		n, species := s.shelf(r.ZoneKey)
		rd := latest(r.SensorKey)
		body := fmt.Sprintf("%.0f %%VWC, band starts at %.0f %%", rd.Value, r.TriggerBelow)
		if species != "" {
			body = species + " · " + body
		}
		s.Deliver(ctx, "dry-"+r.ZoneKey, Message{Category: CatDry, Title: "Shelf " + n + " dry", Body: body, Tag: "dry-" + r.ZoneKey})
	}
}

// Deliver fans one message out to every subscription whose switches allow
// it. Dedupes on key within the window; sends run off the hub goroutine.
func (s *Service) Deliver(ctx context.Context, key string, msg Message) {
	s.mu.Lock()
	if t, ok := s.lastSent[key]; ok && time.Since(t) < s.window {
		s.mu.Unlock()
		return
	}
	s.lastSent[key] = time.Now()
	s.mu.Unlock()
	if s.subs == nil {
		return
	}
	subs, err := s.subs.List()
	if err != nil {
		s.log.Warn().Err(err).Msg("list push subscriptions")
		return
	}
	if msg.URL == "" {
		msg.URL = "/propagate/"
	}
	payload, _ := json.Marshal(msg)
	go func() {
		sent := 0
		for _, sub := range subs {
			if !PrefsFromJSON(sub.PrefsJSON)[msg.Category] {
				continue
			}
			if s.send(ctx, sub, payload, msg.Urgent) {
				sent++
			}
		}
		s.log.Info().Str("title", msg.Title).Str("category", msg.Category).Int("devices", sent).Msg("push sent")
	}()
}

// SendTo pushes one message to one subscription, ignoring its switches —
// the dashboard's "Send test" button.
func (s *Service) SendTo(ctx context.Context, sub storage.PushSubscription, msg Message) error {
	if msg.URL == "" {
		msg.URL = "/propagate/"
	}
	payload, _ := json.Marshal(msg)
	if !s.send(ctx, sub, payload, msg.Urgent) {
		return errors.New("push rejected by the browser's push service")
	}
	return nil
}

func (s *Service) send(ctx context.Context, sub storage.PushSubscription, payload []byte, urgent bool) bool {
	code, err := s.sendFn(ctx, sub, payload, urgent)
	switch {
	case err == nil && code >= 200 && code < 300:
		_ = s.subs.MarkOK(sub.Endpoint)
		return true
	case code == 404 || code == 410:
		// The browser unsubscribed (or the user revoked permission): forget it.
		s.log.Info().Str("user", sub.Username).Msg("push subscription gone, removing")
		_ = s.subs.Delete(sub.Endpoint)
		return false
	default:
		s.log.Warn().Err(err).Int("status", code).Str("user", sub.Username).Msg("push failed")
		_ = s.subs.MarkFailure(sub.Endpoint)
		return false
	}
}

func (s *Service) sendWebPush(ctx context.Context, sub storage.PushSubscription, payload []byte, urgent bool) (int, error) {
	urg := webpush.UrgencyNormal
	if urgent {
		urg = webpush.UrgencyHigh
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      s.keys.Subject,
		VAPIDPublicKey:  s.keys.PublicKey,
		VAPIDPrivateKey: s.keys.PrivateKey,
		TTL:             3600,
		Urgency:         urg,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return resp.StatusCode, fmt.Errorf("push service: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp.StatusCode, nil
}
