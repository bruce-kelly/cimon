package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRun() models.WorkflowRun {
	now := time.Now()
	return models.WorkflowRun{
		ID:         123,
		Name:       "CI",
		HeadBranch: "main",
		HeadSHA:    "abc123def456",
		Status:     "completed",
		Conclusion: "failure",
		Event:      "push",
		Actor:      "bfk",
		CreatedAt:  now.Add(-5 * time.Minute),
		UpdatedAt:  now,
		HTMLURL:    "https://github.com/owner/repo/actions/runs/123",
		Repo:       "owner/repo",
		Jobs: []models.Job{
			{ID: 1, Name: "build", Status: "completed", Conclusion: "success",
				StartedAt: timePtr(now.Add(-4 * time.Minute)), CompletedAt: timePtr(now.Add(-3 * time.Minute)),
				RunnerName: "ubuntu-latest"},
			{ID: 2, Name: "test", Status: "completed", Conclusion: "failure",
				StartedAt: timePtr(now.Add(-3 * time.Minute)), CompletedAt: timePtr(now),
				RunnerName: "ubuntu-latest",
				Steps: []models.Step{
					{Name: "Setup", Status: "completed", Conclusion: "success", Number: 1},
					{Name: "Run tests", Status: "completed", Conclusion: "failure", Number: 2},
					{Name: "Cleanup", Status: "completed", Conclusion: "success", Number: 3},
				}},
			{ID: 3, Name: "lint", Status: "completed", Conclusion: "success",
				StartedAt: timePtr(now.Add(-2 * time.Minute)), CompletedAt: timePtr(now.Add(-1 * time.Minute)),
				RunnerName: "ubuntu-latest"},
		},
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestRunDetailView_Render(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 30)

	assert.Contains(t, out, "main")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "push")
	assert.Contains(t, out, "bfk")
	assert.Contains(t, out, "Jobs")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "test")
	assert.Contains(t, out, "lint")
}

func TestRunDetailView_StatsLine(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 30)

	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "2/3")
}

func TestRunDetailView_CursorNavigatesJobs(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")

	assert.Equal(t, 0, rv.Cursor.Index())
	assert.Equal(t, 3, rv.Cursor.Count())

	rv.Cursor.Next()
	assert.Equal(t, 1, rv.Cursor.Index())

	job := rv.SelectedJob()
	require.NotNil(t, job)
	assert.Equal(t, "test", job.Name)
}

func TestRunDetailView_ExpandCollapseJob(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")

	// Job 0 (build, success) is not expanded
	assert.False(t, rv.IsExpanded(0))

	// Toggle expand job 0
	rv.ToggleExpand() // cursor at 0
	assert.True(t, rv.IsExpanded(0))

	// Toggle collapse job 0
	rv.ToggleExpand()
	assert.False(t, rv.IsExpanded(0))
}

func TestRunDetailView_FailedJobsAutoExpand(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")

	assert.True(t, rv.IsExpanded(1), "failed jobs should auto-expand")
	assert.False(t, rv.IsExpanded(0), "passing jobs should not auto-expand")
}

func TestRunDetailView_ExpandedRender(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 30)

	// "test" job (index 1) is auto-expanded because it failed, so steps are visible
	assert.Contains(t, out, "Setup")
	assert.Contains(t, out, "Run tests")
	assert.Contains(t, out, "Cleanup")
}

func TestRunDetailView_EmptyJobs(t *testing.T) {
	run := models.WorkflowRun{
		ID: 1, Name: "CI", HeadBranch: "main", HeadSHA: "abc123",
		Status: "completed", Conclusion: "success",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 20)
	assert.Contains(t, out, "Loading jobs")
	assert.Equal(t, 0, rv.Cursor.Count())
}

func TestRunDetailView_SetJobs(t *testing.T) {
	run := models.WorkflowRun{
		ID: 1, Name: "CI", HeadBranch: "main", HeadSHA: "abc123",
		Status: "completed", Conclusion: "success",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	rv := NewRunDetailView(run, "owner/repo")
	assert.Equal(t, 0, rv.Cursor.Count())

	jobs := []models.Job{
		{Name: "build", Conclusion: "success"},
		{Name: "test", Conclusion: "failure"},
	}
	rv.SetJobs(jobs)
	assert.Equal(t, 2, rv.Cursor.Count())
	assert.True(t, rv.IsExpanded(1))
}

func TestRunDetailView_ActiveRun(t *testing.T) {
	run := models.WorkflowRun{
		ID: 1, Name: "CI", HeadBranch: "main", HeadSHA: "abc123",
		Status: "in_progress",
		CreatedAt: time.Now().Add(-2 * time.Minute), UpdatedAt: time.Now(),
	}
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 20)
	assert.Contains(t, out, "in_progress")
}
