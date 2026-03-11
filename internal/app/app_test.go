package app

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
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

func TestUpdate_ActionResultMsg_ShowsFlash(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	msg := actionResultMsg{Message: "Rerun triggered", IsError: false}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "Rerun triggered", a.flash.Message)
	assert.False(t, a.flash.IsError)
}

func TestUpdate_ActionResultMsg_Error(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	msg := actionResultMsg{Message: "Rerun failed: 404", IsError: true}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "Rerun failed: 404", a.flash.Message)
	assert.True(t, a.flash.IsError)
}

func TestUpdate_DiffResultMsg_PopulatesLogPane(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewDetail
	msg := diffResultMsg{Title: "PR #42 diff", Content: "+added line\n-removed line"}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "PR #42 diff", a.logPane.Title)
	assert.Contains(t, a.logPane.Content, "+added line")
}

func TestUpdate_DiffResultMsg_Error(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewDetail
	msg := diffResultMsg{Err: fmt.Errorf("404 not found")}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Contains(t, a.flash.Message, "404 not found")
	assert.True(t, a.flash.IsError)
}

func TestUpdate_LogsResultMsg_IgnoredWhenNotInRunDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewCompact
	msg := logsResultMsg{Title: "logs", Content: "some logs"}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "", a.logPane.Title)
}

func TestHandleKey_DDrillsIn(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	dMsg := tea.KeyPressMsg{}
	dMsg.Text = "d"
	m, _ := a.handleKey(dMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.NotNil(t, a.detailView)
}

func TestHandleKey_AReturnsToCompact(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(views.RepoState{RepoName: "test", FullName: "owner/test"})

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewCompact, a.mode)
	assert.Nil(t, a.detailView)
}

func TestHandleKey_AAtCompactIsNoop(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	assert.Equal(t, ui.ViewCompact, a.mode)

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewCompact, a.mode)
}

func TestHandleKey_DrillIntoRunDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	repo := views.RepoState{
		RepoName: "repo-a",
		FullName: "owner/repo-a",
		Runs: []models.WorkflowRun{
			{ID: 1, Name: "ci", HeadBranch: "main", HeadSHA: "abc123",
				Status: "completed", Conclusion: "failure",
				CreatedAt: now, UpdatedAt: now},
		},
	}
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(repo)

	dMsg := tea.KeyPressMsg{}
	dMsg.Text = "d"
	m, _ := a.handleKey(dMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewRunDetail, a.mode)
	assert.NotNil(t, a.runDetailView)
}

func TestHandleKey_DrillIntoPRDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	repo := views.RepoState{
		RepoName: "repo-a",
		FullName: "owner/repo-a",
		ReviewItems: []review.ReviewItem{
			{PR: models.PullRequest{Number: 42, Title: "Test PR", Repo: "owner/repo-a",
				CreatedAt: now, UpdatedAt: now}, Age: time.Hour},
		},
	}
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(repo)

	dMsg := tea.KeyPressMsg{}
	dMsg.Text = "d"
	m, _ := a.handleKey(dMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewPRDetail, a.mode)
	assert.NotNil(t, a.prDetailView)
}

func TestHandleKey_BackFromRunDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewRunDetail
	run := models.WorkflowRun{ID: 1, CreatedAt: now, UpdatedAt: now}
	a.runDetailView = views.NewRunDetailView(run, "owner/repo-a")
	a.detailView = views.NewDetailView(views.RepoState{FullName: "owner/repo-a"})

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.Nil(t, a.runDetailView)
}

func TestHandleKey_BackFromPRDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewPRDetail
	pr := models.PullRequest{Number: 42, CreatedAt: now, UpdatedAt: now}
	a.prDetailView = views.NewPRDetailView(pr, "owner/repo-a")
	a.detailView = views.NewDetailView(views.RepoState{FullName: "owner/repo-a"})

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.Nil(t, a.prDetailView)
}

func TestView_RunDetailMode(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewRunDetail
	run := models.WorkflowRun{
		ID: 1, Name: "ci", HeadBranch: "main", HeadSHA: "abc123",
		Status: "completed", Conclusion: "failure",
		CreatedAt: now, UpdatedAt: now,
	}
	a.runDetailView = views.NewRunDetailView(run, "owner/repo-a")
	v := a.View()
	assert.True(t, v.AltScreen)
}

func TestView_PRDetailMode(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewPRDetail
	pr := models.PullRequest{Number: 42, Title: "Test PR", CreatedAt: now, UpdatedAt: now}
	a.prDetailView = views.NewPRDetailView(pr, "owner/repo-a")
	v := a.View()
	assert.True(t, v.AltScreen)
}

func TestUpdate_DiffResultMsg_ParsesFilesForPRDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewPRDetail
	pr := models.PullRequest{Number: 42, Title: "Test PR", CreatedAt: now, UpdatedAt: now}
	a.prDetailView = views.NewPRDetailView(pr, "owner/repo-a")

	diff := "diff --git a/foo.go b/foo.go\n+added\n-removed\ndiff --git a/bar.go b/bar.go\n+new line"
	msg := diffResultMsg{Title: "PR #42 diff", Content: diff}
	m, _ := a.Update(msg)
	a = m.(App)

	assert.Len(t, a.prDetailView.Files, 2)
	assert.Equal(t, "foo.go", a.prDetailView.Files[0].Path)
	assert.Equal(t, "bar.go", a.prDetailView.Files[1].Path)
	assert.Equal(t, diff, a.prDetailView.RawDiff)
}

func TestHandlePollResult_PreservesRunDetailView(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	// Set up a run detail view
	run := models.WorkflowRun{ID: 1, Name: "ci", Status: "in_progress",
		Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now}
	a.mode = ui.ViewRunDetail
	a.runDetailView = views.NewRunDetailView(run, "owner/repo-a")

	// Poll with updated run
	result := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{
			{ID: 1, Name: "ci", Status: "completed", Conclusion: "success",
				Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now},
		},
		RateLimitRemaining: 4500,
	}
	m, _ := a.handlePollResult(pollResultMsg{Result: result})
	a = m.(App)

	assert.Equal(t, ui.ViewRunDetail, a.mode)
	assert.NotNil(t, a.runDetailView)
	assert.Equal(t, "success", a.runDetailView.Run.Conclusion)
}

func TestHandlePollResult_RunDetailViewDisappearedRun(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	run := models.WorkflowRun{ID: 99, Name: "ci", Status: "in_progress",
		Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now}
	a.mode = ui.ViewRunDetail
	a.runDetailView = views.NewRunDetailView(run, "owner/repo-a")
	a.detailView = views.NewDetailView(views.RepoState{FullName: "owner/repo-a"})

	// Poll without the run
	result := models.PollResult{
		Repo:               "owner/repo-a",
		Runs:               []models.WorkflowRun{},
		RateLimitRemaining: 4500,
	}
	m, _ := a.handlePollResult(pollResultMsg{Result: result})
	a = m.(App)

	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.Nil(t, a.runDetailView)
	assert.Contains(t, a.flash.Message, "Run no longer available")
}

func TestHandlePollResult_PreservesPRDetailView(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	pr := models.PullRequest{Number: 42, Title: "Test PR", Repo: "owner/repo-a",
		CreatedAt: now, UpdatedAt: now}
	a.mode = ui.ViewPRDetail
	a.prDetailView = views.NewPRDetailView(pr, "owner/repo-a")

	// Poll with updated PR
	result := models.PollResult{
		Repo: "owner/repo-a",
		PullRequests: []models.PullRequest{
			{Number: 42, Title: "Test PR updated", Repo: "owner/repo-a",
				CIStatus: "success", CreatedAt: now, UpdatedAt: now},
		},
		RateLimitRemaining: 4500,
	}
	m, _ := a.handlePollResult(pollResultMsg{Result: result})
	a = m.(App)

	assert.Equal(t, ui.ViewPRDetail, a.mode)
	assert.NotNil(t, a.prDetailView)
	assert.Equal(t, "Test PR updated", a.prDetailView.PR.Title)
}

func TestHandlePollResult_PRDetailViewClosedPR(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	pr := models.PullRequest{Number: 42, Title: "Test PR", Repo: "owner/repo-a",
		CreatedAt: now, UpdatedAt: now}
	a.mode = ui.ViewPRDetail
	a.prDetailView = views.NewPRDetailView(pr, "owner/repo-a")
	a.detailView = views.NewDetailView(views.RepoState{FullName: "owner/repo-a"})

	// Poll without the PR
	result := models.PollResult{
		Repo:               "owner/repo-a",
		PullRequests:       []models.PullRequest{},
		RateLimitRemaining: 4500,
	}
	m, _ := a.handlePollResult(pollResultMsg{Result: result})
	a = m.(App)

	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.Nil(t, a.prDetailView)
	assert.Contains(t, a.flash.Message, "PR #42 was closed")
}
