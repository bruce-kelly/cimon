package components

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Selector ---

func TestSelector_NextWraps(t *testing.T) {
	var s Selector
	s.SetCount(3)
	assert.Equal(t, 0, s.Index())
	s.Next()
	assert.Equal(t, 1, s.Index())
	s.Next()
	assert.Equal(t, 2, s.Index())
	s.Next()
	assert.Equal(t, 0, s.Index()) // wraps
}

func TestSelector_PrevWraps(t *testing.T) {
	var s Selector
	s.SetCount(3)
	assert.Equal(t, 0, s.Index())
	s.Prev()
	assert.Equal(t, 2, s.Index()) // wraps to end
	s.Prev()
	assert.Equal(t, 1, s.Index())
}

func TestSelector_SetCountClampsIndex(t *testing.T) {
	var s Selector
	s.SetCount(5)
	s.Next()
	s.Next()
	s.Next()
	s.Next() // index = 4
	assert.Equal(t, 4, s.Index())
	s.SetCount(2) // clamp from 4 to 1
	assert.Equal(t, 1, s.Index())
}

func TestSelector_ZeroCount(t *testing.T) {
	var s Selector
	s.SetCount(0)
	s.Next() // should be no-op
	assert.Equal(t, 0, s.Index())
	s.Prev() // should be no-op
	assert.Equal(t, 0, s.Index())
	assert.Equal(t, 0, s.Count())
}

func TestSelector_SetCountZeroResetsIndex(t *testing.T) {
	var s Selector
	s.SetCount(5)
	s.Next()
	s.Next()
	assert.Equal(t, 2, s.Index())
	s.SetCount(0)
	assert.Equal(t, 0, s.Index())
}

// --- ConfirmBar ---

func TestConfirmBar_ShowAndConfirm(t *testing.T) {
	c := &ConfirmBar{}
	confirmed := false
	c.Show("Do the thing?", func() tea.Cmd { confirmed = true; return nil }, nil)
	assert.True(t, c.Active)
	assert.Equal(t, "Do the thing?", c.Message)

	consumed, _ := c.HandleKey("y")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, confirmed)
}

func TestConfirmBar_ShowAndCancel(t *testing.T) {
	c := &ConfirmBar{}
	cancelled := false
	c.Show("Do the thing?", nil, func() tea.Cmd { cancelled = true; return nil })

	consumed, _ := c.HandleKey("n")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cancelled)
}

func TestConfirmBar_EscCancels(t *testing.T) {
	c := &ConfirmBar{}
	cancelled := false
	c.Show("Do the thing?", nil, func() tea.Cmd { cancelled = true; return nil })

	consumed, _ := c.HandleKey("esc")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cancelled)
}

func TestConfirmBar_ConsumesOtherKeys(t *testing.T) {
	c := &ConfirmBar{}
	c.Show("Do the thing?", nil, nil)

	consumed, _ := c.HandleKey("x")
	assert.True(t, consumed)     // consumed
	assert.True(t, c.Active)     // still active
}

func TestConfirmBar_InactiveIgnoresKeys(t *testing.T) {
	c := &ConfirmBar{}
	consumed, _ := c.HandleKey("y")
	assert.False(t, consumed)
}

func TestConfirmBar_RenderInactive(t *testing.T) {
	c := &ConfirmBar{}
	assert.Equal(t, "", c.Render(80))
}

func TestConfirmBar_RenderActive(t *testing.T) {
	c := &ConfirmBar{}
	c.Show("Merge PR?", nil, nil)
	rendered := c.Render(80)
	assert.Contains(t, rendered, "Merge PR?")
	assert.Contains(t, rendered, "[y/n/Esc]")
}

func TestConfirmBar_ShowAndConfirmReturnsCmd(t *testing.T) {
	c := &ConfirmBar{}
	cmdCalled := false
	c.Show("Do it?",
		func() tea.Cmd { cmdCalled = true; return nil },
		func() tea.Cmd { return nil },
	)
	assert.True(t, c.Active)
	consumed, cmd := c.HandleKey("y")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cmdCalled)
	assert.Nil(t, cmd)
}

func TestConfirmBar_CancelReturnsCmd(t *testing.T) {
	c := &ConfirmBar{}
	cancelCalled := false
	c.Show("Do it?",
		func() tea.Cmd { return nil },
		func() tea.Cmd { cancelCalled = true; return nil },
	)
	consumed, cmd := c.HandleKey("n")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cancelCalled)
	assert.Nil(t, cmd)
}

func TestConfirmBar_HandleKeyReturnsNilCmdForOtherKeys(t *testing.T) {
	c := &ConfirmBar{}
	c.Show("Do it?",
		func() tea.Cmd { return nil },
		func() tea.Cmd { return nil },
	)
	consumed, cmd := c.HandleKey("x")
	assert.True(t, consumed)
	assert.True(t, c.Active)
	assert.Nil(t, cmd)
}

func TestConfirmBar_InactiveReturnsNilCmd(t *testing.T) {
	c := &ConfirmBar{}
	consumed, cmd := c.HandleKey("y")
	assert.False(t, consumed)
	assert.Nil(t, cmd)
}

// --- Flash ---

func TestFlash_ShowAndVisible(t *testing.T) {
	f := &Flash{}
	f.Show("Success!", false)
	assert.True(t, f.Visible())
	assert.Equal(t, "Success!", f.Message)
	assert.False(t, f.IsError)
}

func TestFlash_VisibleExpires(t *testing.T) {
	f := &Flash{}
	f.Show("Success!", false)
	// Set ShowAt to 4 seconds ago so it should be expired
	f.ShowAt = time.Now().Add(-4 * time.Second)
	assert.False(t, f.Visible())
}

func TestFlash_EmptyNotVisible(t *testing.T) {
	f := &Flash{}
	assert.False(t, f.Visible())
}

func TestFlash_Clear(t *testing.T) {
	f := &Flash{}
	f.Show("test", false)
	f.Clear()
	assert.False(t, f.Visible())
	assert.Equal(t, "", f.Message)
}

func TestFlash_Error(t *testing.T) {
	f := &Flash{}
	f.Show("Failed!", true)
	assert.True(t, f.IsError)
	assert.True(t, f.Visible())
}

// --- PipelineView ---

func TestPipelineView_FilteredRuns(t *testing.T) {
	p := NewPipelineView()
	p.Runs = []models.WorkflowRun{
		{Name: "CI", HeadBranch: "main", Actor: "alice", Conclusion: "success"},
		{Name: "Release", HeadBranch: "release/v1", Actor: "bob", Conclusion: "failure"},
		{Name: "CI", HeadBranch: "feature", Actor: "alice", Conclusion: "failure"},
	}
	p.Filter.Query = "alice"
	filtered := p.FilteredRuns()
	require.Equal(t, 2, len(filtered))
	assert.Equal(t, "CI", filtered[0].Name)
	assert.Equal(t, "CI", filtered[1].Name)
}

func TestPipelineView_FilteredRunsNoFilter(t *testing.T) {
	p := NewPipelineView()
	p.Runs = []models.WorkflowRun{
		{Name: "CI"},
		{Name: "Release"},
	}
	filtered := p.FilteredRuns()
	assert.Equal(t, 2, len(filtered))
}

func TestPipelineView_FilteredRunsMultiTerm(t *testing.T) {
	p := NewPipelineView()
	p.Runs = []models.WorkflowRun{
		{Name: "CI", HeadBranch: "main", Actor: "alice", Conclusion: "success"},
		{Name: "Release", HeadBranch: "main", Actor: "bob", Conclusion: "failure"},
	}
	p.Filter.Query = "main failure"
	filtered := p.FilteredRuns()
	require.Equal(t, 1, len(filtered))
	assert.Equal(t, "Release", filtered[0].Name)
}

func TestPipelineView_SelectedRunNil(t *testing.T) {
	p := NewPipelineView()
	assert.Nil(t, p.SelectedRun())
}

func TestPipelineView_SetRunsUpdatesSelector(t *testing.T) {
	p := NewPipelineView()
	runs := []models.WorkflowRun{{Name: "A"}, {Name: "B"}, {Name: "C"}}
	p.SetRuns(runs)
	assert.Equal(t, 3, p.Selector.Count())
}

func TestPipelineView_SelectedRun(t *testing.T) {
	p := NewPipelineView()
	p.SetRuns([]models.WorkflowRun{
		{Name: "A"},
		{Name: "B"},
	})
	p.Selector.Next()
	sel := p.SelectedRun()
	require.NotNil(t, sel)
	assert.Equal(t, "B", sel.Name)
}

// --- FormatDuration ---

func TestFormatDuration_Seconds(t *testing.T) {
	assert.Equal(t, "30s", FormatDuration(30*time.Second))
}

func TestFormatDuration_Minutes(t *testing.T) {
	assert.Equal(t, "2m30s", FormatDuration(2*time.Minute+30*time.Second))
}

func TestFormatDuration_Hours(t *testing.T) {
	assert.Equal(t, "1h30m", FormatDuration(1*time.Hour+30*time.Minute))
}

func TestFormatDuration_Zero(t *testing.T) {
	assert.Equal(t, "0s", FormatDuration(0))
}

// --- FormatTimeAgo ---

func TestFormatTimeAgo_Now(t *testing.T) {
	assert.Equal(t, "now", FormatTimeAgo(time.Now()))
}

func TestFormatTimeAgo_Seconds(t *testing.T) {
	assert.Equal(t, "now", FormatTimeAgo(time.Now().Add(-30*time.Second)))
}

func TestFormatTimeAgo_Minutes(t *testing.T) {
	assert.Equal(t, "3m ago", FormatTimeAgo(time.Now().Add(-3*time.Minute)))
}

func TestFormatTimeAgo_OneMinute(t *testing.T) {
	assert.Equal(t, "1m ago", FormatTimeAgo(time.Now().Add(-1*time.Minute)))
}

func TestFormatTimeAgo_Hours(t *testing.T) {
	assert.Equal(t, "2h ago", FormatTimeAgo(time.Now().Add(-2*time.Hour)))
}

func TestFormatTimeAgo_OneHour(t *testing.T) {
	assert.Equal(t, "1h ago", FormatTimeAgo(time.Now().Add(-1*time.Hour)))
}

func TestFormatTimeAgo_Days(t *testing.T) {
	assert.Equal(t, "3d ago", FormatTimeAgo(time.Now().Add(-3*24*time.Hour)))
}

func TestFormatTimeAgo_Weeks(t *testing.T) {
	assert.Equal(t, "2w ago", FormatTimeAgo(time.Now().Add(-14*24*time.Hour)))
}

func TestFormatTimeAgo_Months(t *testing.T) {
	assert.Equal(t, "2mo ago", FormatTimeAgo(time.Now().Add(-60*24*time.Hour)))
}

func TestFormatTimeAgo_ZeroTime(t *testing.T) {
	assert.Equal(t, "", FormatTimeAgo(time.Time{}))
}

// --- FormatTimeAbsolute ---

func TestFormatTimeAbsolute_Today(t *testing.T) {
	now := time.Now()
	ts := time.Date(now.Year(), now.Month(), now.Day(), 14, 23, 0, 0, now.Location())
	assert.Equal(t, "14:23", FormatTimeAbsolute(ts))
}

func TestFormatTimeAbsolute_ThisYear(t *testing.T) {
	now := time.Now()
	// Pick a date that's definitely not today but in the same year
	ts := time.Date(now.Year(), 1, 1, 9, 5, 0, 0, now.Location())
	if now.Month() == 1 && now.Day() == 1 {
		ts = time.Date(now.Year(), 2, 1, 9, 5, 0, 0, now.Location())
	}
	result := FormatTimeAbsolute(ts)
	assert.Contains(t, result, "09:05")
	// Check for month abbreviation
	if now.Month() == 1 && now.Day() == 1 {
		assert.Contains(t, result, "Feb")
	} else {
		assert.Contains(t, result, "Jan")
	}
}

func TestFormatTimeAbsolute_OlderYear(t *testing.T) {
	ts := time.Date(2024, 6, 15, 8, 30, 0, 0, time.Local)
	assert.Equal(t, "2024-06-15 08:30", FormatTimeAbsolute(ts))
}

func TestFormatTimeAbsolute_ZeroTime(t *testing.T) {
	assert.Equal(t, "", FormatTimeAbsolute(time.Time{}))
}

// --- Progress Bar ---

func TestRenderJobProgress_AllComplete(t *testing.T) {
	jobs := []models.Job{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
	}
	result := RenderJobProgress(jobs)
	assert.Contains(t, result, "[3/3]")
}

func TestRenderJobProgress_Mixed(t *testing.T) {
	jobs := []models.Job{
		{Status: "completed", Conclusion: "success"},
		{Status: "in_progress"},
		{Status: "queued"},
	}
	result := RenderJobProgress(jobs)
	assert.Contains(t, result, "[1/3]")
}

func TestRenderJobProgress_Empty(t *testing.T) {
	result := RenderJobProgress(nil)
	assert.Equal(t, "", result)
}

func TestRenderJobProgress_WithFailure(t *testing.T) {
	jobs := []models.Job{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "failure"},
		{Status: "completed", Conclusion: "success"},
	}
	result := RenderJobProgress(jobs)
	assert.Contains(t, result, "[3/3]")
}

func TestJobProgressColor_Success(t *testing.T) {
	jobs := []models.Job{{Status: "completed", Conclusion: "success"}}
	color := JobProgressColor(jobs)
	assert.Equal(t, "green", color)
}

func TestJobProgressColor_InProgress(t *testing.T) {
	jobs := []models.Job{{Status: "in_progress"}}
	color := JobProgressColor(jobs)
	assert.Equal(t, "amber", color)
}

func TestJobProgressColor_Failure(t *testing.T) {
	jobs := []models.Job{{Status: "completed", Conclusion: "failure"}}
	color := JobProgressColor(jobs)
	assert.Equal(t, "red", color)
}

func TestRenderMiniBar_Full(t *testing.T) {
	assert.Equal(t, "█████", RenderMiniBar(5, 5, 5))
}

func TestRenderMiniBar_Half(t *testing.T) {
	result := RenderMiniBar(3, 6, 6)
	assert.Equal(t, "███░░░", result)
}

func TestRenderMiniBar_Zero(t *testing.T) {
	assert.Equal(t, "░░░░░", RenderMiniBar(0, 5, 5))
}
