package components

import (
	"testing"
	"time"

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

// --- FilterBar ---

func TestFilterBar_MatchesSingleTerm(t *testing.T) {
	f := &FilterBar{Query: "hello"}
	assert.True(t, f.Matches("hello world"))
	assert.False(t, f.Matches("goodbye world"))
}

func TestFilterBar_MatchesMultipleTerms(t *testing.T) {
	f := &FilterBar{Query: "hello world"}
	assert.True(t, f.Matches("hello beautiful world"))
	assert.False(t, f.Matches("hello beautiful place"))
}

func TestFilterBar_EmptyQueryMatchesAll(t *testing.T) {
	f := &FilterBar{Query: ""}
	assert.True(t, f.Matches("anything"))
	assert.True(t, f.Matches(""))
}

func TestFilterBar_CaseInsensitive(t *testing.T) {
	f := &FilterBar{Query: "Hello"}
	assert.True(t, f.Matches("HELLO WORLD"))
	assert.True(t, f.Matches("hello world"))
	assert.True(t, f.Matches("HeLLo WoRLd"))
}

func TestFilterBar_HandleKeyBackspace(t *testing.T) {
	f := &FilterBar{Query: "abc", Active: true}
	consumed := f.HandleKey("backspace")
	assert.True(t, consumed)
	assert.Equal(t, "ab", f.Query)
}

func TestFilterBar_HandleKeyBackspaceEmpty(t *testing.T) {
	f := &FilterBar{Query: "", Active: true}
	consumed := f.HandleKey("backspace")
	assert.True(t, consumed)
	assert.Equal(t, "", f.Query)
}

func TestFilterBar_HandleKeyEsc(t *testing.T) {
	f := &FilterBar{Query: "test", Active: true}
	consumed := f.HandleKey("esc")
	assert.True(t, consumed)
	assert.Equal(t, "", f.Query)
	assert.False(t, f.Active)
}

func TestFilterBar_HandleKeyAppend(t *testing.T) {
	f := &FilterBar{Query: "ab", Active: true}
	consumed := f.HandleKey("c")
	assert.True(t, consumed)
	assert.Equal(t, "abc", f.Query)
}

func TestFilterBar_HandleKeyEnter(t *testing.T) {
	f := &FilterBar{Query: "test", Active: true}
	consumed := f.HandleKey("enter")
	assert.True(t, consumed)
	assert.Equal(t, "test", f.Query) // query preserved
	assert.False(t, f.Active)
}

func TestFilterBar_InactiveIgnoresKeys(t *testing.T) {
	f := &FilterBar{Query: "", Active: false}
	consumed := f.HandleKey("a")
	assert.False(t, consumed)
	assert.Equal(t, "", f.Query)
}

func TestFilterBar_Clear(t *testing.T) {
	f := &FilterBar{Query: "test", Active: true}
	f.Clear()
	assert.Equal(t, "", f.Query)
	assert.False(t, f.Active)
}

// --- Sparkline ---

func TestSparkline_Basic(t *testing.T) {
	result := RenderSparkline([]float64{0, 0.5, 1.0})
	assert.Equal(t, 3, len([]rune(result)))
	runes := []rune(result)
	assert.Equal(t, sparkChars[0], runes[0])
	assert.Equal(t, sparkChars[len(sparkChars)-1], runes[2])
}

func TestSparkline_AllZeros(t *testing.T) {
	result := RenderSparkline([]float64{0, 0, 0})
	expected := "▁▁▁"
	assert.Equal(t, expected, result)
}

func TestSparkline_SingleValue(t *testing.T) {
	result := RenderSparkline([]float64{5.0})
	runes := []rune(result)
	assert.Equal(t, 1, len(runes))
	assert.Equal(t, sparkChars[len(sparkChars)-1], runes[0])
}

func TestSparkline_Empty(t *testing.T) {
	result := RenderSparkline([]float64{})
	assert.Equal(t, "", result)
}

func TestSparkline_EqualValues(t *testing.T) {
	result := RenderSparkline([]float64{3, 3, 3})
	runes := []rune(result)
	// All equal non-zero values should all map to max
	for _, r := range runes {
		assert.Equal(t, sparkChars[len(sparkChars)-1], r)
	}
}

// --- ConfirmBar ---

func TestConfirmBar_ShowAndConfirm(t *testing.T) {
	c := &ConfirmBar{}
	confirmed := false
	c.Show("Do the thing?", func() { confirmed = true }, nil)
	assert.True(t, c.Active)
	assert.Equal(t, "Do the thing?", c.Message)

	consumed := c.HandleKey("y")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, confirmed)
}

func TestConfirmBar_ShowAndCancel(t *testing.T) {
	c := &ConfirmBar{}
	cancelled := false
	c.Show("Do the thing?", nil, func() { cancelled = true })

	consumed := c.HandleKey("n")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cancelled)
}

func TestConfirmBar_EscCancels(t *testing.T) {
	c := &ConfirmBar{}
	cancelled := false
	c.Show("Do the thing?", nil, func() { cancelled = true })

	consumed := c.HandleKey("esc")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cancelled)
}

func TestConfirmBar_ConsumesOtherKeys(t *testing.T) {
	c := &ConfirmBar{}
	c.Show("Do the thing?", nil, nil)

	consumed := c.HandleKey("x")
	assert.True(t, consumed)     // consumed
	assert.True(t, c.Active)     // still active
}

func TestConfirmBar_InactiveIgnoresKeys(t *testing.T) {
	c := &ConfirmBar{}
	consumed := c.HandleKey("y")
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

// --- ActionMenu ---

func TestActionMenu_ShowAndHide(t *testing.T) {
	m := &ActionMenu{}
	items := []ActionMenuItem{
		{Label: "Approve", Key: "a"},
		{Label: "Merge", Key: "m"},
	}
	m.Show(items)
	assert.True(t, m.Active)
	assert.Equal(t, 2, len(m.Items))
	assert.Equal(t, 0, m.Selector.Index())

	m.Hide()
	assert.False(t, m.Active)
	assert.Nil(t, m.Items)
}

func TestActionMenu_Navigation(t *testing.T) {
	m := &ActionMenu{}
	items := []ActionMenuItem{
		{Label: "A", Key: "a"},
		{Label: "B", Key: "b"},
		{Label: "C", Key: "c"},
	}
	m.Show(items)

	m.HandleKey("j")
	assert.Equal(t, 1, m.Selector.Index())
	m.HandleKey("j")
	assert.Equal(t, 2, m.Selector.Index())
	m.HandleKey("k")
	assert.Equal(t, 1, m.Selector.Index())
}

func TestActionMenu_EnterExecutes(t *testing.T) {
	m := &ActionMenu{}
	executed := ""
	items := []ActionMenuItem{
		{Label: "First", Key: "1", Action: func() { executed = "first" }},
		{Label: "Second", Key: "2", Action: func() { executed = "second" }},
	}
	m.Show(items)
	m.HandleKey("j") // move to second
	m.HandleKey("enter")
	assert.Equal(t, "second", executed)
	assert.False(t, m.Active) // hidden after execute
}

func TestActionMenu_EscHides(t *testing.T) {
	m := &ActionMenu{}
	m.Show([]ActionMenuItem{{Label: "A", Key: "a"}})
	m.HandleKey("esc")
	assert.False(t, m.Active)
}

func TestActionMenu_ConsumesAllKeys(t *testing.T) {
	m := &ActionMenu{}
	m.Show([]ActionMenuItem{{Label: "A", Key: "a"}})
	consumed := m.HandleKey("z")
	assert.True(t, consumed)
}

func TestActionMenu_InactiveIgnoresKeys(t *testing.T) {
	m := &ActionMenu{}
	consumed := m.HandleKey("j")
	assert.False(t, consumed)
}

func TestActionMenu_Render(t *testing.T) {
	m := &ActionMenu{}
	items := []ActionMenuItem{
		{Label: "Approve", Key: "a"},
		{Label: "Merge", Key: "m"},
	}
	m.Show(items)
	rendered := m.Render()
	assert.Contains(t, rendered, "> a  Approve")
	assert.Contains(t, rendered, "  m  Merge")
}

func TestActionMenu_RenderInactive(t *testing.T) {
	m := &ActionMenu{}
	assert.Equal(t, "", m.Render())
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
