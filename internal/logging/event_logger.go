package logging

import (
	"encoding/json"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/bus"
	"propagation-controller/internal/storage"
)

// EventLogger writes durable system_events rows and mirrors them to the
// process log, so every operational event is visible in both places. With a
// hub attached, events also stream live over SSE.
type EventLogger struct {
	zl   zerolog.Logger
	repo *storage.EventRepo
	hub  *bus.Hub
}

func NewEventLogger(zl zerolog.Logger, repo *storage.EventRepo) *EventLogger {
	return &EventLogger{zl: zl, repo: repo}
}

func (l *EventLogger) AttachHub(h *bus.Hub) { l.hub = h }

func (l *EventLogger) Info(eventType, message, actor string, details map[string]any) {
	l.log(zerolog.InfoLevel, "info", eventType, message, actor, details)
}

func (l *EventLogger) Warn(eventType, message, actor string, details map[string]any) {
	l.log(zerolog.WarnLevel, "warning", eventType, message, actor, details)
}

func (l *EventLogger) Error(eventType, message, actor string, details map[string]any) {
	l.log(zerolog.ErrorLevel, "error", eventType, message, actor, details)
}

func (l *EventLogger) log(zlvl zerolog.Level, level, eventType, message, actor string, details map[string]any) {
	var detailsJSON *string
	if len(details) > 0 {
		if b, err := json.Marshal(details); err == nil {
			s := string(b)
			detailsJSON = &s
		}
	}
	var actorPtr *string
	if actor != "" {
		actorPtr = &actor
	}
	if err := l.repo.Insert(level, eventType, message, detailsJSON, actorPtr); err != nil {
		l.zl.Error().Err(err).Str("event_type", eventType).Msg("failed to persist system event")
	}
	l.hub.Publish("system_event", map[string]any{
		"level": level, "event_type": eventType, "message": message,
		"actor": actor, "details": details, "occurred_at": time.Now().UTC(),
	})
	ev := l.zl.WithLevel(zlvl).Str("event_type", eventType)
	if actor != "" {
		ev = ev.Str("actor", actor)
	}
	if detailsJSON != nil {
		ev = ev.RawJSON("details", []byte(*detailsJSON))
	}
	ev.Msg(message)
}
