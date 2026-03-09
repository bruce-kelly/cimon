package screens

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDashboard_HasPipelineView(t *testing.T) {
	d := NewDashboard()
	require.NotNil(t, d.Pipeline)
	assert.Equal(t, FocusPipeline, d.Focus)
}

func TestDashboardModel_CycleFocus(t *testing.T) {
	t.Run("with roster", func(t *testing.T) {
		d := NewDashboard()
		d.ShowRoster = true
		assert.Equal(t, FocusPipeline, d.Focus)
		d.CycleFocus()
		assert.Equal(t, FocusReview, d.Focus)
		d.CycleFocus()
		assert.Equal(t, FocusRoster, d.Focus)
		d.CycleFocus()
		assert.Equal(t, FocusPipeline, d.Focus) // wraps
	})

	t.Run("without roster", func(t *testing.T) {
		d := NewDashboard()
		assert.Equal(t, FocusPipeline, d.Focus)
		d.CycleFocus()
		assert.Equal(t, FocusReview, d.Focus)
		d.CycleFocus()
		assert.Equal(t, FocusPipeline, d.Focus) // skips roster, wraps
	})
}

func TestDashboardModel_SetSize(t *testing.T) {
	d := NewDashboard()
	d.SetSize(120, 40)
	assert.Equal(t, 120, d.Width)
	assert.Equal(t, 40, d.Height)
}

func TestDashboardModel_RenderZeroWidth(t *testing.T) {
	d := NewDashboard()
	assert.Equal(t, "", d.Render())
}

func TestDashboardModel_RenderEmptyState(t *testing.T) {
	d := NewDashboard()
	d.SetSize(120, 40)
	rendered := d.Render()
	assert.Contains(t, rendered, "Pipeline")
	assert.Contains(t, rendered, "Review Queue")
	assert.Contains(t, rendered, "Agent Roster")
	assert.Contains(t, rendered, "No items for review")
	assert.Contains(t, rendered, "No agent activity")
}

func TestTimelineModel_SetRunsSortsByUpdatedAt(t *testing.T) {
	tl := NewTimeline()
	now := time.Now()

	runs := []models.WorkflowRun{
		{Name: "oldest", Repo: "org/a", UpdatedAt: now.Add(-2 * time.Hour)},
		{Name: "newest", Repo: "org/b", UpdatedAt: now},
		{Name: "middle", Repo: "org/a", UpdatedAt: now.Add(-1 * time.Hour)},
	}
	tl.SetRuns(runs)

	assert.Equal(t, "newest", tl.Runs[0].Name)
	assert.Equal(t, "middle", tl.Runs[1].Name)
	assert.Equal(t, "oldest", tl.Runs[2].Name)
}

func TestTimelineModel_RepoColorAssignment(t *testing.T) {
	tl := NewTimeline()
	now := time.Now()

	runs := []models.WorkflowRun{
		{Name: "a", Repo: "org/first", UpdatedAt: now},
		{Name: "b", Repo: "org/second", UpdatedAt: now.Add(-1 * time.Hour)},
		{Name: "c", Repo: "org/first", UpdatedAt: now.Add(-2 * time.Hour)},
	}
	tl.SetRuns(runs)

	// "org/first" should get index 0, "org/second" should get index 1
	assert.Equal(t, 0, tl.repoColorIdx["org/first"])
	assert.Equal(t, 1, tl.repoColorIdx["org/second"])
}

func TestTimelineModel_FilteredRuns(t *testing.T) {
	tl := NewTimeline()
	now := time.Now()
	tl.SetRuns([]models.WorkflowRun{
		{Name: "CI", Repo: "org/app", Actor: "alice", UpdatedAt: now},
		{Name: "Release", Repo: "org/lib", Actor: "bob", UpdatedAt: now},
	})
	tl.Filter.Query = "alice"
	filtered := tl.filteredRuns()
	require.Equal(t, 1, len(filtered))
	assert.Equal(t, "CI", filtered[0].Name)
}

func TestTimelineModel_RenderEmpty(t *testing.T) {
	tl := NewTimeline()
	rendered := tl.Render()
	assert.Contains(t, rendered, "No timeline events")
}

func TestReleaseModel_NextRepoWraps(t *testing.T) {
	r := NewRelease()
	r.Repos = []string{"org/a", "org/b", "org/c"}
	assert.Equal(t, 0, r.CurrentRepo)
	r.NextRepo()
	assert.Equal(t, 1, r.CurrentRepo)
	r.NextRepo()
	assert.Equal(t, 2, r.CurrentRepo)
	r.NextRepo()
	assert.Equal(t, 0, r.CurrentRepo) // wraps
}

func TestReleaseModel_PrevRepoWraps(t *testing.T) {
	r := NewRelease()
	r.Repos = []string{"org/a", "org/b", "org/c"}
	assert.Equal(t, 0, r.CurrentRepo)
	r.PrevRepo()
	assert.Equal(t, 2, r.CurrentRepo) // wraps to end
	r.PrevRepo()
	assert.Equal(t, 1, r.CurrentRepo)
}

func TestReleaseModel_NextPrevNoRepos(t *testing.T) {
	r := NewRelease()
	r.NextRepo() // should not panic
	r.PrevRepo() // should not panic
	assert.Equal(t, 0, r.CurrentRepo)
}

func TestReleaseModel_RenderNoRepos(t *testing.T) {
	r := NewRelease()
	rendered := r.Render()
	assert.Contains(t, rendered, "No release workflows configured")
}

func TestReleaseModel_RenderWithRepo(t *testing.T) {
	r := NewRelease()
	r.Repos = []string{"org/app"}
	r.Width = 80
	rendered := r.Render()
	assert.Contains(t, rendered, "org/app")
	assert.Contains(t, rendered, "1/1")
	assert.Contains(t, rendered, "No release runs")
}

func TestNewMetrics_NilStats(t *testing.T) {
	m := NewMetrics()
	assert.Nil(t, m.RunStats)
	assert.Nil(t, m.TaskStats)
	assert.Nil(t, m.Effectiveness)
}

func TestMetricsModel_RenderNoData(t *testing.T) {
	m := NewMetrics()
	rendered := m.Render()
	assert.Contains(t, rendered, "CI Health")
	assert.Contains(t, rendered, "Agent Tasks")
	assert.Contains(t, rendered, "Agent Effectiveness")
	assert.Contains(t, rendered, "No data")
}

func TestRenderBar(t *testing.T) {
	// Full bar
	bar := renderBar(10, 10)
	assert.Contains(t, bar, "██████████")

	// Empty bar
	bar = renderBar(0, 10)
	assert.Contains(t, bar, "░░░░░░░░░░")

	// Half bar
	bar = renderBar(5, 10)
	assert.Contains(t, bar, "█████")
	assert.Contains(t, bar, "░░░░░")
}

func TestRenderBar_ZeroMax(t *testing.T) {
	bar := renderBar(0, 0)
	assert.Contains(t, bar, "░░░░░░░░░░")
}
