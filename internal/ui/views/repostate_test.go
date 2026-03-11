package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestComputeInlineStatus_AllPassing(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
	}
	status := ComputeInlineStatus(runs, nil)
	assert.Equal(t, StatusPassing, status.Worst)
	assert.Empty(t, status.FailedJobs)
	assert.Empty(t, status.ActiveRuns)
}

func TestComputeInlineStatus_HasFailure(t *testing.T) {
	now := time.Now()
	runs := []models.WorkflowRun{
		{
			Status:       "completed",
			Conclusion:   "failure",
			Name:         "ci",
			WorkflowFile: "ci.yml",
			UpdatedAt:    now.Add(-4 * time.Minute),
			Jobs: []models.Job{
				{Name: "build", Conclusion: "failure"},
				{Name: "test", Conclusion: "failure"},
				{Name: "lint", Conclusion: "success"},
			},
		},
	}
	status := ComputeInlineStatus(runs, nil)
	assert.Equal(t, StatusFailed, status.Worst)
	assert.Equal(t, []string{"build", "test"}, status.FailedJobs)
	assert.Equal(t, "ci", status.FailedWorkflow)
}

func TestComputeInlineStatus_HasActive(t *testing.T) {
	runs := []models.WorkflowRun{
		{
			Status: "in_progress",
			Name:   "deploy",
			Jobs: []models.Job{
				{Name: "build", Conclusion: "success"},
				{Name: "test", Conclusion: ""},
				{Name: "deploy", Conclusion: ""},
			},
		},
	}
	status := ComputeInlineStatus(runs, nil)
	assert.Equal(t, StatusActive, status.Worst)
	assert.Len(t, status.ActiveRuns, 1)
	assert.Equal(t, "deploy", status.ActiveRuns[0].Name)
	assert.Equal(t, 1, status.ActiveRuns[0].CompletedJobs)
	assert.Equal(t, 3, status.ActiveRuns[0].TotalJobs)
}

func TestComputeInlineStatus_FailureTrumpsActive(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "failure", Name: "ci", WorkflowFile: "ci.yml"},
		{Status: "in_progress", Name: "deploy", WorkflowFile: "deploy.yml"},
	}
	status := ComputeInlineStatus(runs, nil)
	assert.Equal(t, StatusFailed, status.Worst)
}

func TestComputeInlineStatus_OldFailureSupersededByNewSuccess(t *testing.T) {
	now := time.Now()
	// Runs arrive newest-first. Latest CI run passed, old one failed.
	runs := []models.WorkflowRun{
		{
			Status:       "completed",
			Conclusion:   "success",
			Name:         "CI",
			WorkflowFile: "ci.yml",
			UpdatedAt:    now.Add(-5 * time.Minute),
		},
		{
			Status:       "completed",
			Conclusion:   "failure",
			Name:         "CI",
			WorkflowFile: "ci.yml",
			UpdatedAt:    now.Add(-15 * time.Hour),
		},
	}
	status := ComputeInlineStatus(runs, nil)
	assert.Equal(t, StatusPassing, status.Worst, "old failure should not count when latest run passed")
	assert.Empty(t, status.FailedWorkflow)
}

func TestComputeInlineStatus_LatestFailedOtherPassing(t *testing.T) {
	now := time.Now()
	// CI latest is failed, release latest is passing
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "failure", Name: "CI", WorkflowFile: "ci.yml", UpdatedAt: now},
		{Status: "completed", Conclusion: "success", Name: "CI", WorkflowFile: "ci.yml", UpdatedAt: now.Add(-1 * time.Hour)},
		{Status: "completed", Conclusion: "success", Name: "Release", WorkflowFile: "release.yml", UpdatedAt: now},
	}
	status := ComputeInlineStatus(runs, nil)
	assert.Equal(t, StatusFailed, status.Worst, "latest CI run failed so repo should be failed")
	assert.Equal(t, "CI", status.FailedWorkflow)
}

func TestComputeInlineStatus_AgentFailureIsAmber(t *testing.T) {
	now := time.Now()
	critical := map[string]bool{"ci.yml": true}
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "success", Name: "CI", WorkflowFile: "ci.yml", UpdatedAt: now},
		{Status: "completed", Conclusion: "failure", Name: "claude-test-gaps", WorkflowFile: "claude-test-gaps.yml", UpdatedAt: now},
		{Status: "completed", Conclusion: "failure", Name: "claude-dep-audit", WorkflowFile: "claude-dep-audit.yml", UpdatedAt: now},
	}
	status := ComputeInlineStatus(runs, critical)
	assert.Equal(t, StatusAgentFailed, status.Worst, "agent-only failures should be amber, not red")
	assert.Equal(t, 2, status.AgentFailCount)
	assert.Empty(t, status.FailedWorkflow, "no critical workflow failed")
}

func TestComputeInlineStatus_CriticalFailureTrumpsAgent(t *testing.T) {
	now := time.Now()
	critical := map[string]bool{"ci.yml": true}
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "failure", Name: "CI", WorkflowFile: "ci.yml", UpdatedAt: now},
		{Status: "completed", Conclusion: "failure", Name: "claude-test-gaps", WorkflowFile: "claude-test-gaps.yml", UpdatedAt: now},
	}
	status := ComputeInlineStatus(runs, critical)
	assert.Equal(t, StatusFailed, status.Worst, "critical failure should still be red")
	assert.Equal(t, 1, status.AgentFailCount)
	assert.Equal(t, "CI", status.FailedWorkflow)
}

func TestComputePRSummary_Empty(t *testing.T) {
	summary := ComputePRSummary(nil)
	assert.Equal(t, 0, summary.Total)
	assert.Equal(t, 0, summary.Ready)
	assert.False(t, summary.CIPending)
}

func TestComputePRSummary_MixedStates(t *testing.T) {
	prs := []models.PullRequest{
		{CIStatus: "success", ReviewState: "approved", Draft: false},
		{CIStatus: "success", ReviewState: "approved", Draft: false},
		{CIStatus: "pending", ReviewState: "", Draft: false},
		{CIStatus: "success", ReviewState: "", Draft: true},
	}
	summary := ComputePRSummary(prs)
	assert.Equal(t, 4, summary.Total)
	assert.Equal(t, 2, summary.Ready)
	assert.True(t, summary.CIPending)
}

func TestComputePRSummary_AgentPRs(t *testing.T) {
	prs := []models.PullRequest{
		{CIStatus: "success", ReviewState: "approved", IsAgent: true},
		{CIStatus: "failure", IsAgent: true},
	}
	summary := ComputePRSummary(prs)
	assert.Equal(t, 1, summary.Ready)
}

func TestSortByAttention_FailuresFirst(t *testing.T) {
	states := []RepoState{
		{RepoName: "green", Inline: InlineStatus{Worst: StatusPassing}},
		{RepoName: "failed", Inline: InlineStatus{Worst: StatusFailed}},
		{RepoName: "active", Inline: InlineStatus{Worst: StatusActive}},
	}
	SortByAttention(states)
	assert.Equal(t, "failed", states[0].RepoName)
	assert.Equal(t, "active", states[1].RepoName)
	assert.Equal(t, "green", states[2].RepoName)
}

func TestSortByAttention_ReadyPRsBeforeQuiet(t *testing.T) {
	states := []RepoState{
		{RepoName: "quiet", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 0}},
		{RepoName: "has-ready", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 2, Ready: 1}},
	}
	SortByAttention(states)
	assert.Equal(t, "has-ready", states[0].RepoName)
	assert.Equal(t, "quiet", states[1].RepoName)
}

func TestSortByAttention_AgentFailuresBeforeActive(t *testing.T) {
	states := []RepoState{
		{RepoName: "active", Inline: InlineStatus{Worst: StatusActive}},
		{RepoName: "agent", Inline: InlineStatus{Worst: StatusAgentFailed}},
		{RepoName: "quiet", Inline: InlineStatus{Worst: StatusPassing}},
	}
	SortByAttention(states)
	assert.Equal(t, "agent", states[0].RepoName)
	assert.Equal(t, "active", states[1].RepoName)
	assert.Equal(t, "quiet", states[2].RepoName)
}

func TestDetectNewFlag_NewFailure(t *testing.T) {
	prev := RepoState{Inline: InlineStatus{Worst: StatusPassing}}
	curr := RepoState{Inline: InlineStatus{Worst: StatusFailed}}
	assert.True(t, DetectNewFlag(prev, curr))
}

func TestDetectNewFlag_NoChange(t *testing.T) {
	prev := RepoState{Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Ready: 1}}
	curr := RepoState{Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Ready: 1}}
	assert.False(t, DetectNewFlag(prev, curr))
}

func TestDetectNewFlag_NewReadyPR(t *testing.T) {
	prev := RepoState{PRSummary: PRSummary{Ready: 0}}
	curr := RepoState{PRSummary: PRSummary{Ready: 1}}
	assert.True(t, DetectNewFlag(prev, curr))
}

func TestDetectNewFlag_ReleaseStarted(t *testing.T) {
	prev := RepoState{Inline: InlineStatus{Releasing: false}}
	curr := RepoState{Inline: InlineStatus{Releasing: true}}
	assert.True(t, DetectNewFlag(prev, curr))
}
