package bus

import "testing"

func TestPubSub(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe()
	defer cancel()

	h.Publish("sensor_reading", map[string]int{"v": 1})
	ev := <-ch
	if ev.Type != "sensor_reading" {
		t.Errorf("type = %q", ev.Type)
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	h := New()
	_, cancel := h.Subscribe() // never drained
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ { // far beyond the 64 buffer
			h.Publish("x", i)
		}
		close(done)
	}()
	<-done // must complete without blocking
}

func TestNilHubIsSafe(t *testing.T) {
	var h *Hub
	h.Publish("x", nil) // must not panic
}

func TestCancelStopsDelivery(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe()
	cancel()
	cancel() // double-cancel must be safe
	h.Publish("x", nil)
	if _, open := <-ch; open {
		t.Error("channel should be closed after cancel")
	}
}
