package views

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompactView_NoRepos(t *testing.T) {
	cv := NewCompactView(nil)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "No repos configured")
	assert.Contains(t, out, "cimon init")
}

func TestCompactView_AllGreen(t *testing.T) {
	repos := []RepoState{
		{RepoName: "repo-a", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 0}},
		{RepoName: "repo-b", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 0}},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "repo-a")
	assert.Contains(t, out, "repo-b")
	assert.Contains(t, out, "all passing")
}

func TestCompactView_FailureInlineExpands(t *testing.T) {
	repos := []RepoState{
		{
			RepoName: "repo-c",
			Inline: InlineStatus{
				Worst:          StatusFailed,
				FailedWorkflow: "ci",
				FailedJobs:     []string{"build", "test"},
				FailedAt:       time.Now().Add(-4 * time.Minute),
			},
			PRSummary: PRSummary{Total: 3, Ready: 1},
		},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "repo-c")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "test")
}

func TestCompactView_ActiveRunShowsProgress(t *testing.T) {
	repos := []RepoState{
		{
			RepoName: "repo-d",
			Inline: InlineStatus{
				Worst: StatusActive,
				ActiveRuns: []ActiveRunInfo{
					{Name: "deploy", CompletedJobs: 7, TotalJobs: 10, Elapsed: 82 * time.Second},
				},
				Releasing: true,
			},
		},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "repo-d")
	assert.Contains(t, out, "deploy")
	assert.Contains(t, out, "7/10")
}

func TestCompactView_CursorNavigation(t *testing.T) {
	repos := []RepoState{
		{RepoName: "repo-a", Inline: InlineStatus{Worst: StatusPassing}},
		{RepoName: "repo-b", Inline: InlineStatus{Worst: StatusPassing}},
		{RepoName: "repo-c", Inline: InlineStatus{Worst: StatusPassing}},
	}
	cv := NewCompactView(repos)
	assert.Equal(t, 0, cv.Cursor.Index())

	cv.Cursor.Next()
	assert.Equal(t, 1, cv.Cursor.Index())

	cv.Cursor.Next()
	assert.Equal(t, 2, cv.Cursor.Index())

	cv.Cursor.Next() // wraps
	assert.Equal(t, 0, cv.Cursor.Index())
}

func TestCompactView_NewFlagShows(t *testing.T) {
	repos := []RepoState{
		{
			RepoName: "repo-a",
			Inline:   InlineStatus{Worst: StatusFailed},
			NewFlag:  true,
		},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "NEW")
}

func TestCompactView_PRSummaryRendering(t *testing.T) {
	repos := []RepoState{
		{RepoName: "repo-a", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 2, Ready: 2}},
		{RepoName: "repo-b", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 1, CIPending: true}},
	}
	cv := NewCompactView(repos)
	out := cv.Render(50, 20)
	assert.Contains(t, out, "2 PRs")
	assert.Contains(t, out, "2 ready")
	assert.Contains(t, out, "1 PR")
}
