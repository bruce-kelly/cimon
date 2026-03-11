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
	"github.com/bruce-kelly/cimon/internal/ui/views"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testApp(t *testing.T) App {
	t.Helper()
	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{Repo: "owner/repo-a", Branch: "main"},
			{Repo: "owner/repo-b", Branch: "main"},
		},
		Polling:     config.PollingConfig{Idle: 30, Active: 5, Cooldown: 3},
		ReviewQueue: config.ReviewQueueConfig{Escalation: config.EscalationConfig{Amber: 24, Red: 48}},
	}
	database, err := db.OpenMemory()
	require.NoError(t, err)
	client := ghclient.NewTestClient("test-token", "http://localhost:1")
	return NewApp(cfg, client, database)
}

func TestNewApp_InitializesCompactMode(t *testing.T) {
	a := testApp(t)
	assert.Equal(t, ui.ViewCompact, a.mode)
	assert.NotNil(t, a.compactView)
	assert.Nil(t, a.detailView)
	assert.Len(t, a.repos, 2)
	assert.False(t, a.quitting)
}

func TestHandlePollResult_UpdatesRepoStates(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	result := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{
			{ID: 1, Name: "ci", Status: "completed", Conclusion: "success",
				Repo: "owner/repo-a", WorkflowFile: "ci.yml",
				CreatedAt: now, UpdatedAt: now},
		},
		PullRequests:       []models.PullRequest{{Number: 42, Title: "Test PR", Repo: "owner/repo-a", State: "open", CreatedAt: now, UpdatedAt: now}},
		RateLimitRemaining: 4500,
	}

	msg := pollResultMsg{Result: result}
	m, _ := a.handlePollResult(msg)
	a = m.(App)

	assert.Equal(t, 4500, a.rateLimit)
	assert.Len(t, a.allRuns["owner/repo-a"], 1)
	assert.Len(t, a.allPulls["owner/repo-a"], 1)
}

func TestHandlePollResult_AuthError(t *testing.T) {
	a := testApp(t)
	result := models.PollResult{
		Repo:  "owner/repo-a",
		Error: &ghclient.AuthError{StatusCode: 401, Message: "bad credentials"},
	}
	msg := pollResultMsg{Result: result}
	m, _ := a.handlePollResult(msg)
	a = m.(App)
	assert.Contains(t, a.statusText, "AUTH FAILED")
}

func TestHandlePollResult_RateLimitError(t *testing.T) {
	a := testApp(t)
	result := models.PollResult{
		Repo:  "owner/repo-a",
		Error: &ghclient.RateLimitError{RetryAfter: 60 * time.Second, ResetAt: time.Now().Add(60 * time.Second)},
	}
	msg := pollResultMsg{Result: result}
	m, _ := a.handlePollResult(msg)
	a = m.(App)
	assert.Contains(t, a.statusText, "Rate limited")
}

func TestHandleKey_CompactNavigation(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	downMsg := tea.KeyPressMsg{}
	downMsg.Text = "s"
	m, _ := a.handleKey(downMsg)
	a = m.(App)
	assert.Equal(t, 1, a.compactView.Cursor.Index())
}

func TestHandleKey_EnterDrillsIn(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	enterMsg := tea.KeyPressMsg{}
	enterMsg.Text = "enter"
	m, _ := a.handleKey(enterMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.NotNil(t, a.detailView)
}

func TestHandleKey_EscReturnsToCompact(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(views.RepoState{RepoName: "test", FullName: "owner/test"})

	escMsg := tea.KeyPressMsg{}
	escMsg.Text = "esc"
	m, _ := a.handleKey(escMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewCompact, a.mode)
	assert.Nil(t, a.detailView)
}

func TestHandlePollResult_DetectsNewFlag(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	// First poll: all passing
	result1 := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{{ID: 1, Status: "completed", Conclusion: "success",
			Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now}},
	}
	m, _ := a.handlePollResult(pollResultMsg{Result: result1})
	a = m.(App)

	// Second poll: failure
	result2 := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{{ID: 2, Status: "completed", Conclusion: "failure",
			Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now}},
	}
	m, _ = a.handlePollResult(pollResultMsg{Result: result2})
	a = m.(App)

	found := false
	for _, r := range a.repos {
		if r.FullName == "owner/repo-a" {
			assert.True(t, r.NewFlag, "repo-a should have NEW flag after failure")
			found = true
		}
	}
	assert.True(t, found)
}

func TestView_ReturnsAltScreen(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	v := a.View()
	assert.True(t, v.AltScreen)
}

func TestView_TooSmall(t *testing.T) {
	a := testApp(t)
	a.width = 30
	a.height = 5
	v := a.View()
	assert.True(t, v.AltScreen)
}

func TestView_Quitting(t *testing.T) {
	a := testApp(t)
	a.quitting = true
	v := a.View()
	assert.NotNil(t, v)
}
