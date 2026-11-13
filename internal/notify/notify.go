// Package notify pushes a handful of rack conditions to the steward's phone.
//
// Austin, 2026-11-13: "I would love a text when the reservoir is empty."
// The unit has no outbound channel of its own, so this subscribes to the
// event hub (the same feed the SSE stream reads) and forwards the reservoir
// lockout transitions to whichever transports are configured:
//
//   - ntfy (https://ntfy.sh or a self-hosted server): a push notification to
//     the ntfy app, no account needed, works the moment a topic is set.
//   - Twilio SMS: a real text, needs Austin's own Twilio account.
//
// Configuration is environment-only (deploy/notify.env.example, loaded by
// systemd from /etc/propagation-controller/notify.env) so no credential ever
// lands in the database or the public settings API.
package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
)

// Config is read from the environment by LoadFromEnv.
type Config struct {
	UnitName string

	NtfyServer string
	NtfyTopic  string

	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFrom       string
	TwilioTo         string
}

func LoadFromEnv() Config {
	c := Config{
		UnitName:         envOr("PROPAGATION_UNIT_NAME", "Propagation Unit 01"),
		NtfyServer:       strings.TrimRight(envOr("PROPAGATION_NTFY_SERVER", "https://ntfy.sh"), "/"),
		NtfyTopic:        os.Getenv("PROPAGATION_NTFY_TOPIC"),
		TwilioAccountSID: os.Getenv("PROPAGATION_TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:  os.Getenv("PROPAGATION_TWILIO_AUTH_TOKEN"),
		TwilioFrom:       os.Getenv("PROPAGATION_TWILIO_FROM"),
		TwilioTo:         os.Getenv("PROPAGATION_TWILIO_TO"),
	}
	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (c Config) NtfyEnabled() bool { return c.NtfyTopic != "" }
func (c Config) TwilioEnabled() bool {
	return c.TwilioAccountSID != "" && c.TwilioAuthToken != "" && c.TwilioFrom != "" && c.TwilioTo != ""
}
func (c Config) Enabled() bool { return c.NtfyEnabled() || c.TwilioEnabled() }

// Transports returns the human names of the configured transports, for the
// startup log (never the credentials).
func (c Config) Transports() []string {
	var out []string
	if c.NtfyEnabled() {
		out = append(out, "ntfy")
	}
	if c.TwilioEnabled() {
		out = append(out, "twilio_sms")
	}
	return out
}

// Message is one notification. Urgent messages get the phone's attention
// (ntfy priority "urgent"); the rest are quiet follow-ups ("refilled").
type Message struct {
	Title  string
	Body   string
	Urgent bool
}

// Service forwards matching hub events to the transports.
type Service struct {
	cfg    Config
	hub    *bus.Hub
	log    zerolog.Logger
	client *http.Client

	mu       sync.Mutex
	lastSent map[string]time.Time // dedupe key → when
	// Dedupe window: the supervisor only logs a lockout once per transition,
	// but a flapping float switch could clear and re-lock every poll.
	window time.Duration
}

func New(cfg Config, hub *bus.Hub, log zerolog.Logger) *Service {
	return &Service{
		cfg: cfg, hub: hub, log: log.With().Str("component", "notify").Logger(),
		client:   &http.Client{Timeout: 15 * time.Second},
		lastSent: map[string]time.Time{},
		window:   10 * time.Minute,
	}
}

// Start subscribes to the hub and returns immediately. With nothing
// configured it is a no-op, so the daemon runs exactly as before.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Enabled() || s.hub == nil {
		return
	}
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
				if msg, key, ok := Translate(payload); ok {
					s.deliver(ctx, key, msg)
				}
			}
		}
	}()
}

// Translate turns a system_event payload into a notification, or reports
// that the event is not one we push. Exported for the tests.
//
// Only the reservoir conditions are pushed (Austin, 2026-11-13: "I'm good
// with those two gaps" — no probe-fault or generic alert traffic).
func Translate(p map[string]any) (Message, string, bool) {
	if p == nil {
		return Message{}, "", false
	}
	eventType, _ := p["event_type"].(string)
	details, _ := p["details"].(map[string]any)
	lockout, _ := details["lockout"].(string)
	reason, _ := p["message"].(string)

	switch eventType {
	case "safety_lockout":
		switch lockout {
		case "reservoir_low":
			return Message{Title: "Reservoir empty", Body: "Pump locked out: " + reason + ". Refill the reservoir.", Urgent: true}, "reservoir_low", true
		case "reservoir_sensor_offline":
			return Message{Title: "Reservoir sensor offline", Body: "Pump locked out: " + reason + ". Check the level sensor.", Urgent: true}, "reservoir_sensor_offline", true
		}
	case "safety_lockout_cleared":
		switch lockout {
		case "reservoir_low":
			return Message{Title: "Reservoir refilled", Body: "Level is back above the cutoff; watering can resume."}, "reservoir_low_cleared", true
		case "reservoir_sensor_offline":
			return Message{Title: "Reservoir sensor back", Body: "The level sensor is reporting again; watering can resume."}, "reservoir_sensor_offline_cleared", true
		}
	}
	return Message{}, "", false
}

func (s *Service) deliver(ctx context.Context, key string, msg Message) {
	s.mu.Lock()
	if t, ok := s.lastSent[key]; ok && time.Since(t) < s.window {
		s.mu.Unlock()
		return
	}
	s.lastSent[key] = time.Now()
	s.mu.Unlock()

	// Each transport is independent: ntfy failing must not stop the text.
	if s.cfg.NtfyEnabled() {
		if err := s.sendNtfy(ctx, msg); err != nil {
			s.log.Warn().Err(err).Str("title", msg.Title).Msg("ntfy push failed")
		} else {
			s.log.Info().Str("title", msg.Title).Msg("ntfy push sent")
		}
	}
	if s.cfg.TwilioEnabled() {
		if err := s.sendTwilio(ctx, msg); err != nil {
			s.log.Warn().Err(err).Str("title", msg.Title).Msg("twilio sms failed")
		} else {
			s.log.Info().Str("title", msg.Title).Msg("twilio sms sent")
		}
	}
}

// sendNtfy publishes to {server}/{topic}; see https://docs.ntfy.sh/publish/.
func (s *Service) sendNtfy(ctx context.Context, msg Message) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.NtfyServer+"/"+url.PathEscape(s.cfg.NtfyTopic), strings.NewReader(msg.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", s.cfg.UnitName+": "+msg.Title)
	req.Header.Set("Tags", "droplet")
	if msg.Urgent {
		req.Header.Set("Priority", "urgent")
	} else {
		req.Header.Set("Priority", "default")
	}
	return s.do(req)
}

// sendTwilio posts to the Messages resource of Austin's account.
func (s *Service) sendTwilio(ctx context.Context, msg Message) error {
	form := url.Values{}
	form.Set("To", s.cfg.TwilioTo)
	form.Set("From", s.cfg.TwilioFrom)
	form.Set("Body", s.cfg.UnitName+" — "+msg.Title+". "+msg.Body)
	endpoint := twilioBase + "/2010-04-01/Accounts/" + url.PathEscape(s.cfg.TwilioAccountSID) + "/Messages.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.TwilioAccountSID, s.cfg.TwilioAuthToken)
	return s.do(req)
}

// twilioBase is a variable so the tests can point it at a local server.
var twilioBase = "https://api.twilio.com"

func (s *Service) do(req *http.Request) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: HTTP %d: %s", req.URL.Host, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
