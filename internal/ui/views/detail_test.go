package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/stretchr/testify/assert"
)

func TestDetailView_RenderCIPipeline(t *testing.T) {
	repo := RepoState{
		RepoName: "repo-c",
		FullName: "owner/repo-c",
		Runs: []models.WorkflowRun{
			{
				Name:       "ci",
				HeadBranch: "main",
				HeadSHA:    "a1b2c3d4",
				Conclusion: "failure",
				Status:     "completed",
				UpdatedAt:  time.Now().Add(-4 * time.Minute),
				Jobs: []models.Job{
					{Name: "build", Conclusion: "failure"},
					{Name: "test", Conclusion: "failure"},
					{Name: "lint", Conclusion: "success"},
				},
			},
			{
				Name:       "ci",
				HeadBranch: "main",
				HeadSHA:    "f4e5d6a7",
				Conclusion: "success",
				Status:     "completed",
				UpdatedAt:  time.Now().Add(-2 * time.Hour),
			},
		},
	}
	dv := NewDetailView(repo)
	out := dv.Render(50, 30)
	assert.Contains(t, out, "Workflows")
	assert.Contains(t, out, "main")
	assert.Contains(t, out, "a1b2c3")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "Pull Requests")
}

func TestDetailView_RenderPullRequests(t *testing.T) {
	repo := RepoState{
		RepoName: "repo-c",
		FullName: "owner/repo-c",
		ReviewItems: []review.ReviewItem{
			{
				PR:  models.PullRequest{Number: 201, Title: "Fix auth middleware", CIStatus: "failure", IsAgent: true},
				Age: 3 * time.Hour,
			},
			{
				PR:  models.PullRequest{Number: 198, Title: "Add cache layer", CIStatus: "success", ReviewState: "approved"},
				Age: 24 * time.Hour,
			},
		},
	}
	dv := NewDetailView(repo)
	out := dv.Render(50, 30)
	assert.Contains(t, out, "#201")
	assert.Contains(t, out, "Fix auth")
	assert.Contains(t, out, "agent")
	assert.Contains(t, out, "#198")
}

func TestDetailView_LinearCursor(t *testing.T) {
	repo := RepoState{
		RepoName: "repo-c",
		FullName: "owner/repo-c",
		Runs: []models.WorkflowRun{
			{Name: "ci", HeadBranch: "main", Status: "completed", WorkflowFile: "ci.yml"},
			{Name: "release", HeadBranch: "main", Status: "completed", WorkflowFile: "release.yml"},
		},
		ReviewItems: []review.ReviewItem{
			{PR: models.PullRequest{Number: 201}},
			{PR: models.PullRequest{Number: 198}},
		},
	}
	dv := NewDetailView(repo)

	assert.Equal(t, 0, dv.Cursor.Index())
	assert.True(t, dv.IsRunSelected(), "cursor 0 should be a run")

	dv.Cursor.Next()
	assert.Equal(t, 1, dv.Cursor.Index())
	assert.True(t, dv.IsRunSelected(), "cursor 1 should be a run")

	dv.Cursor.Next()
	assert.Equal(t, 2, dv.Cursor.Index())
	assert.False(t, dv.IsRunSelected(), "cursor 2 should be a PR")

	dv.Cursor.Next()
	assert.Equal(t, 3, dv.Cursor.Index())
	assert.False(t, dv.IsRunSelected(), "cursor 3 should be a PR")

	dv.Cursor.Next() // wraps
	assert.Equal(t, 0, dv.Cursor.Index())
}

func TestDetailView_SelectedRun(t *testing.T) {
	repo := RepoState{
		Runs:        []models.WorkflowRun{{ID: 100}},
		ReviewItems: []review.ReviewItem{{PR: models.PullRequest{Number: 42}}},
	}
	dv := NewDetailView(repo)
	assert.NotNil(t, dv.SelectedRun())
	assert.Nil(t, dv.SelectedReviewItem())

	dv.Cursor.Next()
	assert.Nil(t, dv.SelectedRun())
	assert.NotNil(t, dv.SelectedReviewItem())
}

func TestDetailView_EmptyRepo(t *testing.T) {
	dv := NewDetailView(RepoState{RepoName: "empty", FullName: "owner/empty"})
	out := dv.Render(50, 20)
	assert.Contains(t, out, "empty")
	assert.Nil(t, dv.SelectedRun())
	assert.Nil(t, dv.SelectedReviewItem())
}
