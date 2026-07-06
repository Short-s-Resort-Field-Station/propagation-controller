package api

import (
	"net/http"
	"strconv"
	"time"

	"propagation-controller/internal/storage"
)

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := storage.EventFilter{
		Level:     q.Get("level"),
		EventType: q.Get("type"),
		Limit:     parseIntOr(q.Get("limit"), 100),
	}
	var ok bool
	if filter.From, ok = parseTimeParam(w, q.Get("from"), "from"); !ok {
		return
	}
	if filter.To, ok = parseTimeParam(w, q.Get("to"), "to"); !ok {
		return
	}
	events, err := s.events.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []storage.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleListRelayActions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, ok := parseTimeParam(w, q.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := parseTimeParam(w, q.Get("to"), "to")
	if !ok {
		return
	}
	actions, err := s.actions.List(from, to, parseIntOr(q.Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if actions == nil {
		actions = []storage.RelayAction{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"relay_actions": actions})
}

func parseTimeParam(w http.ResponseWriter, value, name string) (*time.Time, bool) {
	if value == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		writeError(w, http.StatusBadRequest, name+" must be RFC3339 (e.g. 2026-07-05T12:00:00Z)")
		return nil, false
	}
	return &t, true
}

func parseIntOr(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
