package polling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPollState(t *testing.T) {
	s := NewPollState(30, 5, 3)
	assert.Equal(t, CadenceIdle, s.Cadence())
	assert.Equal(t, 30*time.Second, s.idleInterval)
	assert.Equal(t, 5*time.Second, s.activeInterval)
	assert.Equal(t, 3*time.Second, s.cooldownInterval)
}

func TestPollStateStartsIdle(t *testing.T) {
	s := NewPollState(30, 5, 3)
	assert.Equal(t, CadenceIdle, s.Cadence())
}

func TestPollStateIntervalIdle(t *testing.T) {
	s := NewPollState(30, 5, 3)
	assert.Equal(t, 30*time.Second, s.Interval())
}

func TestPollStateIntervalActive(t *testing.T) {
	s := NewPollState(30, 5, 3)
	s.Update(true)
	assert.Equal(t, 5*time.Second, s.Interval())
}

func TestPollStateIntervalCooldown(t *testing.T) {
	s := NewPollState(30, 5, 3)
	// Move to active, then cooldown
	s.Update(true)
	s.Update(false)
	require.Equal(t, CadenceCooldown, s.Cadence())
	assert.Equal(t, 3*time.Second, s.Interval())
}

func TestPollStateTransitionIdleToActive(t *testing.T) {
	s := NewPollState(30, 5, 3)
	s.Update(true)
	assert.Equal(t, CadenceActive, s.Cadence())
}

func TestPollStateTransitionActiveToCooldown(t *testing.T) {
	s := NewPollState(30, 5, 3)
	s.Update(true)
	assert.Equal(t, CadenceActive, s.Cadence())
	s.Update(false)
	assert.Equal(t, CadenceCooldown, s.Cadence())
}

func TestPollStateCooldownExpiresBackToIdle(t *testing.T) {
	s := NewPollState(30, 5, 3)
	s.Update(true)
	s.Update(false)
	require.Equal(t, CadenceCooldown, s.Cadence())

	// Force cooldown to have already expired
	s.cooldownUntil = time.Now().Add(-1 * time.Second)

	// Interval() should detect expiration and switch to idle
	interval := s.Interval()
	assert.Equal(t, 30*time.Second, interval)
	assert.Equal(t, CadenceIdle, s.Cadence())
}

func TestPollStateIdleStaysIdleOnNoActive(t *testing.T) {
	s := NewPollState(30, 5, 3)
	s.Update(false)
	assert.Equal(t, CadenceIdle, s.Cadence(), "idle + no active should stay idle")
}

func TestPollStateCooldownStaysOnActive(t *testing.T) {
	s := NewPollState(30, 5, 3)
	s.Update(true)
	s.Update(false)
	require.Equal(t, CadenceCooldown, s.Cadence())

	// New active runs should jump straight to active
	s.Update(true)
	assert.Equal(t, CadenceActive, s.Cadence())
}

func TestCadenceString(t *testing.T) {
	tests := []struct {
		cadence Cadence
		want    string
	}{
		{CadenceIdle, "idle"},
		{CadenceActive, "active"},
		{CadenceCooldown, "cooldown"},
		{Cadence(99), "idle"}, // unknown defaults to idle
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cadence.String())
		})
	}
}
