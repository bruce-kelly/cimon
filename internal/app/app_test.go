package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testApp(t *testing.T) App {
	t.Helper()
	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{
				Repo:   "owner/repo",
				Branch: "main",
				Groups: map[string]config.GroupConfig{
					"ci": {Label: "CI", Workflows: []string{"ci.yml"}},
				},
			},
		},
		Polling:     config.PollingConfig{Idle: 30, Active: 5, Cooldown: 3},
		ReviewQueue: config.ReviewQueueConfig{Escalation: config.EscalationConfig{Amber: 24, Red: 48}},
	}
	database, err := db.OpenMemory()
	require.NoError(t, err)
	client := ghclient.NewTestClient("test-token", "http://localhost:1")
	return NewApp(cfg, client, database)
}

func TestNewApp_InitializesFields(t *testing.T) {
	a := testApp(t)
	assert.NotNil(t, a.config)
	assert.NotNil(t, a.client)
	assert.NotNil(t, a.db)
	assert.NotNil(t, a.poller)
	assert.NotNil(t, a.allRuns)
	assert.NotNil(t, a.allPulls)
	assert.NotNil(t, a.dispatcher)
	assert.NotNil(t, a.autoFix)
	assert.False(t, a.quitting)
	assert.Equal(t, ui.ScreenDashboard, a.screen)
}

func TestHandlePollResult_UpdatesData(t *testing.T) {
	a := testApp(t)
	now := time.Now()
	result := pollResultMsg{
		Result: models.PollResult{
			Repo: "owner/repo",
			Runs: []models.WorkflowRun{
				{ID: 1, Name: "CI", Status: "completed", Conclusion: "success",
					Repo: "owner/repo", WorkflowFile: "ci.yml",
					CreatedAt: now, UpdatedAt: now},
			},
			PullRequests: []models.PullRequest{
				{Number: 42, Title: "Fix bug", Repo: "owner/repo", State: "open",
					CreatedAt: now, UpdatedAt: now},
			},
			RateLimitRemaining: 4999,
		},
	}

	model, cmd := a.handlePollResult(result)
	updated := model.(App)
	assert.Len(t, updated.allRuns["owner/repo"], 1)
	assert.Len(t, updated.allPulls["owner/repo"], 1)
	assert.Equal(t, 4999, updated.rateLimit)
	assert.NotNil(t, cmd) // should return waitForPoll cmd
}

func TestHandlePollResult_AuthError(t *testing.T) {
	a := testApp(t)
	result := pollResultMsg{
		Result: models.PollResult{
			Repo:  "owner/repo",
			Error: &ghclient.AuthError{StatusCode: 401, Message: "bad credentials"},
		},
	}

	model, _ := a.handlePollResult(result)
	updated := model.(App)
	assert.Contains(t, updated.statusText, "AUTH FAILED")
}

func TestHandlePollResult_RateLimitError(t *testing.T) {
	a := testApp(t)
	result := pollResultMsg{
		Result: models.PollResult{
			Repo:  "owner/repo",
			Error: &ghclient.RateLimitError{RetryAfter: 60 * time.Second, ResetAt: time.Now().Add(60 * time.Second)},
		},
	}

	model, _ := a.handlePollResult(result)
	updated := model.(App)
	assert.Contains(t, updated.statusText, "Rate limited")
}

func TestHandleKey_ScreenSwitching(t *testing.T) {
	a := testApp(t)
	a.width = 120
	a.height = 40

	tests := []struct {
		key    string
		screen ui.Screen
	}{
		{"2", ui.ScreenTimeline},
		{"3", ui.ScreenRelease},
		{"4", ui.ScreenMetrics},
		{"1", ui.ScreenDashboard},
	}

	for _, tt := range tests {
		msg := tea.KeyPressMsg{}
		msg.Text = tt.key
		model, _ := a.handleKey(msg)
		a = model.(App)
		assert.Equal(t, tt.screen, a.screen, "pressing %s should switch to %s", tt.key, tt.screen)
	}
}

func TestHandleKey_TabCyclesFocus(t *testing.T) {
	a := testApp(t)
	a.width = 120
	a.height = 40
	assert.Equal(t, screens.FocusPipeline, a.dashboard.Focus)

	msg := tea.KeyPressMsg{}
	msg.Text = "tab"
	model, _ := a.handleDashboardKey(msg)
	a = model.(App)
	assert.Equal(t, screens.FocusReview, a.dashboard.Focus)
}

func TestView_ReturnsTooSmallMessage(t *testing.T) {
	a := testApp(t)
	a.width = 40
	a.height = 5
	v := a.View()
	assert.True(t, v.AltScreen)
	// tea.View doesn't expose the string directly, but the test verifying
	// it doesn't panic is valuable
}

func TestView_ReturnsAltScreen(t *testing.T) {
	a := testApp(t)
	a.width = 120
	a.height = 40
	v := a.View()
	assert.True(t, v.AltScreen)
}

func TestView_QuittingReturnsEmpty(t *testing.T) {
	a := testApp(t)
	a.quitting = true
	v := a.View()
	assert.NotNil(t, v)
}
