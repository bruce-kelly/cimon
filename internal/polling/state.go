package polling

import "time"

// Cadence represents the adaptive polling speed tier.
type Cadence int

const (
	CadenceIdle     Cadence = iota
	CadenceActive
	CadenceCooldown
)

func (c Cadence) String() string {
	switch c {
	case CadenceActive:
		return "active"
	case CadenceCooldown:
		return "cooldown"
	default:
		return "idle"
	}
}

// PollState tracks the adaptive cadence state machine that controls how
// frequently the poller hits the GitHub API. Three tiers: idle (nothing
// running), active (in-progress runs detected), cooldown (brief fast-poll
// window after runs finish, to catch final status updates).
type PollState struct {
	cadence          Cadence
	idleInterval     time.Duration
	activeInterval   time.Duration
	cooldownInterval time.Duration
	cooldownUntil    time.Time
}

// NewPollState creates a PollState from interval values in seconds.
func NewPollState(idleSec, activeSec, cooldownSec int) *PollState {
	return &PollState{
		cadence:          CadenceIdle,
		idleInterval:     time.Duration(idleSec) * time.Second,
		activeInterval:   time.Duration(activeSec) * time.Second,
		cooldownInterval: time.Duration(cooldownSec) * time.Second,
	}
}

// Cadence returns the current cadence tier.
func (s *PollState) Cadence() Cadence { return s.cadence }

// Interval returns the poll interval for the current cadence. If cooldown
// has expired, transitions back to idle automatically.
func (s *PollState) Interval() time.Duration {
	switch s.cadence {
	case CadenceActive:
		return s.activeInterval
	case CadenceCooldown:
		if time.Now().After(s.cooldownUntil) {
			s.cadence = CadenceIdle
			return s.idleInterval
		}
		return s.cooldownInterval
	default:
		return s.idleInterval
	}
}

// Update transitions the cadence based on whether any runs are currently active.
func (s *PollState) Update(hasActive bool) {
	if hasActive {
		s.cadence = CadenceActive
	} else if s.cadence == CadenceActive {
		s.cadence = CadenceCooldown
		s.cooldownUntil = time.Now().Add(30 * time.Second)
	}
}
