// Package bus is the in-process event hub feeding the SSE stream (spec §16).
// Publishers never block: a slow or stalled subscriber drops events rather
// than stalling relay control.
package bus

import "sync"

type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func New() *Hub {
	return &Hub{subs: map[chan Event]struct{}{}}
}

// Subscribe returns a buffered event channel and a cancel func.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, cancel
}

// Publish fans the event out. Nil-safe so components can be wired without a
// hub (tests, the watchdog binary).
func (h *Hub) Publish(eventType string, payload any) {
	if h == nil {
		return
	}
	ev := Event{Type: eventType, Payload: payload}
	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // subscriber is full — drop rather than block control paths
		}
	}
	h.mu.Unlock()
}
