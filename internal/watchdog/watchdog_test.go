package watchdog

import "testing"

func steps(d *Decider, healthy bool, n int) []Action {
	out := make([]Action, n)
	for i := range out {
		out[i] = d.Step(healthy)
	}
	return out
}

func TestEscalationLadder(t *testing.T) {
	d := NewDecider(3, 2)

	// Two failures: below threshold, no action.
	if got := steps(d, false, 2); got[0] != ActionNone || got[1] != ActionNone {
		t.Fatalf("early failures acted: %v", got)
	}
	// Third consecutive failure → first restart.
	if got := d.Step(false); got != ActionRestart {
		t.Fatalf("want restart, got %v", got)
	}
	// Still failing → second restart after threshold again.
	if got := steps(d, false, 3); got[2] != ActionRestart {
		t.Fatalf("want second restart, got %v", got)
	}
	// Restart limit exhausted → force safe state once.
	if got := steps(d, false, 3); got[2] != ActionForceSafe {
		t.Fatalf("want force_safe, got %v", got)
	}
	// Continued failure → back to periodic restarts, never re-forcing.
	if got := steps(d, false, 3); got[2] != ActionRestart {
		t.Fatalf("after force_safe want restart, got %v", got)
	}
}

func TestRecoveryResetsLadder(t *testing.T) {
	d := NewDecider(3, 2)
	steps(d, false, 3) // restart #1
	steps(d, true, 3)  // sustained recovery

	// Ladder reset: the next incident starts from restart #1 again, and it
	// takes a full threshold of failures to trigger.
	if got := steps(d, false, 2); got[1] != ActionNone {
		t.Fatalf("failures should not carry across recovery: %v", got)
	}
	if got := d.Step(false); got != ActionRestart {
		t.Fatalf("want restart after fresh threshold, got %v", got)
	}
}

func TestBriefBlipDoesNothing(t *testing.T) {
	d := NewDecider(3, 2)
	for i := 0; i < 10; i++ {
		if got := d.Step(i%2 == 0); got != ActionNone {
			t.Fatalf("alternating health must never act, got %v at %d", got, i)
		}
	}
}

func TestRelayMonitorTripsAfterThreshold(t *testing.T) {
	m := NewRelayMonitor(3)
	for i, st := range []string{"online", "error", "error"} {
		if !m.Step(st) {
			t.Fatalf("step %d (%q) should still be healthy", i, st)
		}
	}
	if m.Step("offline") {
		t.Fatal("third consecutive bad status must be unhealthy")
	}
	if m.Step("error") {
		t.Fatal("stays unhealthy while the status stays bad")
	}
	// A good reading resets the count; it takes a full threshold again.
	if !m.Step("simulated") || !m.Step("error") || !m.Step("error") {
		t.Fatal("recovery did not reset the counter")
	}
	if m.Step("") {
		t.Fatal("empty status counts as bad and trips at threshold")
	}
}
