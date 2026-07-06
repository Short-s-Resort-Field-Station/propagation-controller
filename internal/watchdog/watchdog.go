// Package watchdog holds the decision core of propagation-watchdog (spec §4,
// §12 rule 10). Kept free of I/O so the escalation ladder is unit-testable:
//
//	healthy                    → reset counters
//	N consecutive failures     → restart the service
//	still failing after M restarts → force the safe relay state directly
//	                                 (the service is stopped first; the
//	                                 watchdog owns the port only then)
package watchdog

type Action int

const (
	ActionNone Action = iota
	ActionRestart
	ActionForceSafe
)

func (a Action) String() string {
	switch a {
	case ActionRestart:
		return "restart"
	case ActionForceSafe:
		return "force_safe"
	default:
		return "none"
	}
}

type Decider struct {
	FailThreshold int // consecutive failed checks before a restart
	RestartLimit  int // restarts without recovery before forcing safe state

	failures      int
	restarts      int
	healthyChecks int
	forcedSafe    bool
}

func NewDecider(failThreshold, restartLimit int) *Decider {
	if failThreshold < 1 {
		failThreshold = 3
	}
	if restartLimit < 1 {
		restartLimit = 2
	}
	return &Decider{FailThreshold: failThreshold, RestartLimit: restartLimit}
}

// Step consumes one health-check result and returns what to do.
func (d *Decider) Step(healthy bool) Action {
	if healthy {
		d.healthyChecks++
		d.failures = 0
		// Sustained recovery clears the escalation ladder.
		if d.healthyChecks >= 3 {
			d.restarts = 0
			d.forcedSafe = false
		}
		return ActionNone
	}

	d.healthyChecks = 0
	d.failures++
	if d.failures < d.FailThreshold {
		return ActionNone
	}
	d.failures = 0

	if d.restarts >= d.RestartLimit {
		if d.forcedSafe {
			// Safe state already forced; keep attempting restarts at the
			// normal cadence rather than repeatedly seizing the port.
			return ActionRestart
		}
		d.forcedSafe = true
		return ActionForceSafe
	}
	d.restarts++
	return ActionRestart
}
