package relay

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// fakePort scripts the Numato conversation: each Write queues the next canned
// reply (echo + payload + prompt, exactly as the hardware sends it).
type fakePort struct {
	writes  []string
	replies []string
	cur     *strings.Reader
	// writeErr makes every Write fail — a yanked USB cable.
	writeErr error
	closed   bool
}

func (f *fakePort) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.writes = append(f.writes, string(p))
	if len(f.replies) > 0 {
		f.cur = strings.NewReader(f.replies[0])
		f.replies = f.replies[1:]
	}
	return len(p), nil
}

func (f *fakePort) Read(p []byte) (int, error) {
	if f.cur == nil {
		return 0, nil // behaves like a serial read timeout tick
	}
	n, err := f.cur.Read(p)
	if err == io.EOF {
		return n, nil
	}
	return n, err
}

func (f *fakePort) Close() error { f.closed = true; return nil }

func newTestDriver(replies ...string) (*NumatoUSB, *fakePort) {
	port := &fakePort{replies: replies}
	n := NewNumatoUSB("/dev/fake")
	n.port = port
	n.wantOpen = true
	n.health.Status = "online"
	return n, port
}

// TestIOErrorDropsPortAndReopensAfterBackoff: a write failure closes the
// port, the next command inside the backoff fails fast without touching the
// device, and once the backoff passes the driver reopens and the command
// goes through (audit 2026-11-10, 2.2b).
func TestIOErrorDropsPortAndReopensAfterBackoff(t *testing.T) {
	n, broken := newTestDriver()
	broken.writeErr = errors.New("input/output error")
	fresh := &fakePort{replies: []string{"relay off 6\n\r>"}}
	opens := 0
	n.openFn = func(string) (io.ReadWriteCloser, error) { opens++; return fresh, nil }

	ctx := context.Background()
	if err := n.SetChannel(ctx, 7, Off); err == nil {
		t.Fatal("expected the broken port to fail")
	}
	if !broken.closed || n.port != nil {
		t.Fatal("port not dropped after I/O error")
	}
	if h := n.Health(ctx); h.Status != "error" {
		t.Errorf("health after I/O error = %q, want error", h.Status)
	}
	// Inside the backoff: fail fast, no reopen attempt.
	if err := n.SetChannel(ctx, 7, Off); err == nil || opens != 0 {
		t.Fatalf("expected fast failure inside backoff (err=%v opens=%d)", err, opens)
	}
	// Backoff elapsed: reopen and succeed.
	n.mu.Lock()
	n.nextReopen = time.Time{}
	n.mu.Unlock()
	if err := n.SetChannel(ctx, 7, Off); err != nil {
		t.Fatalf("SetChannel after reopen: %v", err)
	}
	if opens != 1 || fresh.writes[0] != "relay off 6\r" {
		t.Errorf("reopen did not happen as expected (opens=%d writes=%v)", opens, fresh.writes)
	}
	if h := n.Health(ctx); h.Status != "online" || h.Info["reopened_at"] == "" {
		t.Errorf("health after reopen = %+v", h)
	}
	// Close must stop the reopen loop.
	n.Close()
	if err := n.SetChannel(ctx, 7, Off); err == nil || opens != 1 {
		t.Errorf("command after Close should fail without reopening (err=%v opens=%d)", err, opens)
	}
}

func TestSetChannelMapsToZeroIndexedWire(t *testing.T) {
	n, port := newTestDriver("relay on 0\n\r>")
	if err := n.SetChannel(context.Background(), 1, On); err != nil {
		t.Fatalf("SetChannel: %v", err)
	}
	if got, want := port.writes[0], "relay on 0\r"; got != want {
		t.Errorf("wire command = %q, want %q", got, want)
	}
}

func TestSetChannelHighChannel(t *testing.T) {
	n, port := newTestDriver("relay off 7\n\r>")
	if err := n.SetChannel(context.Background(), 8, Off); err != nil {
		t.Fatalf("SetChannel: %v", err)
	}
	if got, want := port.writes[0], "relay off 7\r"; got != want {
		t.Errorf("wire command = %q, want %q", got, want)
	}
}

func TestSetChannelRejectsOutOfRange(t *testing.T) {
	n, _ := newTestDriver()
	for _, ch := range []int{0, 9, -1} {
		if err := n.SetChannel(context.Background(), ch, On); err == nil {
			t.Errorf("channel %d: expected error", ch)
		}
	}
}

func TestGetChannelParsesEchoedResponse(t *testing.T) {
	n, port := newTestDriver("relay read 3\n\ron\n\r>")
	state, err := n.GetChannel(context.Background(), 4)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if state != On {
		t.Errorf("state = %q, want on", state)
	}
	if got, want := port.writes[0], "relay read 3\r"; got != want {
		t.Errorf("wire command = %q, want %q", got, want)
	}
}

func TestGetAllParsesHexBitmask(t *testing.T) {
	n, _ := newTestDriver("relay readall\n\r05\n\r>")
	states, err := n.GetAll(context.Background())
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	// 0x05 = bits 0 and 2 set = spec channels 1 and 3 on.
	want := map[int]State{1: On, 2: Off, 3: On, 4: Off, 5: Off, 6: Off, 7: Off, 8: Off}
	for ch, w := range want {
		if states[ch] != w {
			t.Errorf("channel %d = %q, want %q", ch, states[ch], w)
		}
	}
}

func TestParseNumatoResponse(t *testing.T) {
	cases := []struct{ raw, cmd, want string }{
		{"relay read 0\n\ron\n\r>", "relay read 0", "on"},
		{"relay on 0\n\r>", "relay on 0", ""},
		{"relay readall\n\rff\n\r>", "relay readall", "ff"},
		{"ver\n\r00000008\n\r>", "ver", "00000008"},
	}
	for _, c := range cases {
		if got := parseNumatoResponse(c.raw, c.cmd); got != c.want {
			t.Errorf("parseNumatoResponse(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}
