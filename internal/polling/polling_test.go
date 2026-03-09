package polling

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
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

func TestPoller_PollsAndSendsResults(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/actions/workflows/ci.yml/runs":
			json.NewEncoder(w).Encode(map[string]any{
				"workflow_runs": []map[string]any{
					{
						"id": 1, "name": "CI", "path": ".github/workflows/ci.yml",
						"status": "completed", "conclusion": "success",
						"head_branch": "main", "head_sha": "abc123",
						"event": "push", "actor": map[string]string{"login": "alice"},
						"created_at": now, "updated_at": now,
						"html_url": "https://example.com/runs/1",
					},
				},
			})
		case r.URL.Path == "/repos/owner/repo/pulls":
			json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{
				Repo:   "owner/repo",
				Branch: "main",
				Groups: map[string]config.GroupConfig{
					"ci": {Workflows: []string{"ci.yml"}},
				},
			},
		},
		Polling: config.PollingConfig{Idle: 1, Active: 1, Cooldown: 1},
	}

	resultCh := make(chan models.PollResult, 1)
	client := github.NewTestClient("test-token", srv.URL)
	p := New(client, cfg, resultCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p.Start(ctx)
	defer p.Stop()

	select {
	case result := <-resultCh:
		assert.Equal(t, "owner/repo", result.Repo)
		require.Len(t, result.Runs, 1)
		assert.Equal(t, "CI", result.Runs[0].Name)
		assert.Equal(t, int64(1), result.Runs[0].ID)
	case <-ctx.Done():
		t.Fatal("timed out waiting for poll result")
	}
}
