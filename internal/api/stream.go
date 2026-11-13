package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleEventStream implements GET /api/v1/events/stream (spec §16, SSE).
// Public read, like every other GET: followers see the unit breathe live.
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	ch, cancel := s.bus.Subscribe()
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": connected %s\n\n", time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	anonymous := currentUser(r) == nil

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			body := ev.Payload
			if anonymous && ev.Type == "system_event" {
				body = redactSystemEvent(body)
			}
			payload, err := json.Marshal(body)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// redactSystemEvent is the stream's twin of redactAccountEvents: the public
// feed never names an operator.
func redactSystemEvent(payload any) any {
	m, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	typ, _ := m["event_type"].(string)
	msg, ok := publicAccountEvents[typ]
	if !ok {
		return payload
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out["message"] = msg
	out["details"] = nil
	if a, _ := m["actor"].(string); a != "" {
		out["actor"] = "operator"
	}
	return out
}
