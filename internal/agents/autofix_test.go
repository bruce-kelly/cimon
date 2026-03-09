package agents

import (
	"strings"
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- AutoFixTracker tests ---

func TestEvaluate_NewFailures_NoBlock(t *testing.T) {
	tracker := NewAutoFixTracker(2)
	jobs := []models.Job{
		{Name: "test", Conclusion: "failure"},
		{Name: "lint", Conclusion: "failure"},
	}

	neverKnown := func(repo, jobName string) bool { return false }
	decision := tracker.Evaluate("owner/repo", "ci.yml", jobs, 0, neverKnown)

	assert.True(t, decision.ShouldDispatch)
	assert.Equal(t, "owner/repo", decision.Repo)
	assert.Equal(t, "ci.yml", decision.WorkflowFile)
	assert.ElementsMatch(t, []string{"test", "lint"}, decision.FailingJobs)
	assert.NotEmpty(t, decision.Prompt)
	assert.Empty(t, decision.Reason)
}

func TestEvaluate_AllKnownFailures(t *testing.T) {
	tracker := NewAutoFixTracker(2)
	jobs := []models.Job{
		{Name: "test", Conclusion: "failure"},
		{Name: "lint", Conclusion: "failure"},
	}

	allKnown := func(repo, jobName string) bool { return true }
	decision := tracker.Evaluate("owner/repo", "ci.yml", jobs, 0, allKnown)

	assert.False(t, decision.ShouldDispatch)
	assert.Contains(t, decision.Reason, "all known")
}

func TestEvaluate_CooldownActive(t *testing.T) {
	tracker := NewAutoFixTracker(2)
	tracker.SetCooldown("owner/repo", "ci.yml", 600)
	jobs := []models.Job{{Name: "test", Conclusion: "failure"}}
	neverKnown := func(repo, jobName string) bool { return false }

	// First call succeeds and sets cooldown
	d1 := tracker.Evaluate("owner/repo", "ci.yml", jobs, 0, neverKnown)
	assert.True(t, d1.ShouldDispatch)

	// Second call blocked by cooldown
	d2 := tracker.Evaluate("owner/repo", "ci.yml", jobs, 0, neverKnown)
	assert.False(t, d2.ShouldDispatch)
	assert.Contains(t, d2.Reason, "cooldown")
}

func TestEvaluate_MaxConcurrentReached(t *testing.T) {
	tracker := NewAutoFixTracker(1)
	jobs := []models.Job{{Name: "test", Conclusion: "failure"}}
	neverKnown := func(repo, jobName string) bool { return false }

	decision := tracker.Evaluate("owner/repo", "ci.yml", jobs, 1, neverKnown)
	assert.False(t, decision.ShouldDispatch)
	assert.Contains(t, decision.Reason, "max concurrent")
}

func TestBuildFixPrompt(t *testing.T) {
	prompt := BuildFixPrompt("owner/repo", "ci.yml", []string{"test", "lint"})
	assert.Contains(t, prompt, "owner/repo")
	assert.Contains(t, prompt, "ci.yml")
	assert.Contains(t, prompt, "test")
	assert.Contains(t, prompt, "lint")
}

// --- FindPRForTask tests ---

func TestFindPRForTask_MatchesSameRepo(t *testing.T) {
	now := time.Now()
	task := DispatchedAgent{
		Repo:      "owner/repo",
		Status:    "completed",
		StartedAt: now.Add(-30 * time.Minute),
	}
	pulls := []models.PullRequest{
		{Repo: "owner/repo", IsAgent: true, CreatedAt: now.Add(-10 * time.Minute)},
	}

	pr := FindPRForTask(task, pulls)
	require.NotNil(t, pr)
	assert.Equal(t, "owner/repo", pr.Repo)
}

func TestFindPRForTask_SkipsDifferentRepo(t *testing.T) {
	now := time.Now()
	task := DispatchedAgent{
		Repo:      "owner/repo-a",
		Status:    "completed",
		StartedAt: now.Add(-30 * time.Minute),
	}
	pulls := []models.PullRequest{
		{Repo: "owner/repo-b", IsAgent: true, CreatedAt: now.Add(-10 * time.Minute)},
	}

	pr := FindPRForTask(task, pulls)
	assert.Nil(t, pr)
}

func TestFindPRForTask_SkipsNonAgentPR(t *testing.T) {
	now := time.Now()
	task := DispatchedAgent{
		Repo:      "owner/repo",
		Status:    "completed",
		StartedAt: now.Add(-30 * time.Minute),
	}
	pulls := []models.PullRequest{
		{Repo: "owner/repo", IsAgent: false, CreatedAt: now.Add(-10 * time.Minute)},
	}

	pr := FindPRForTask(task, pulls)
	assert.Nil(t, pr)
}

func TestFindPRForTask_SkipsPRBeforeTask(t *testing.T) {
	now := time.Now()
	task := DispatchedAgent{
		Repo:      "owner/repo",
		Status:    "completed",
		StartedAt: now,
	}
	pulls := []models.PullRequest{
		{Repo: "owner/repo", IsAgent: true, CreatedAt: now.Add(-1 * time.Hour)},
	}

	pr := FindPRForTask(task, pulls)
	assert.Nil(t, pr)
}

func TestFindPRForTask_ReturnsNilForRunningTask(t *testing.T) {
	now := time.Now()
	task := DispatchedAgent{
		Repo:      "owner/repo",
		Status:    "running",
		StartedAt: now.Add(-30 * time.Minute),
	}
	pulls := []models.PullRequest{
		{Repo: "owner/repo", IsAgent: true, CreatedAt: now.Add(-10 * time.Minute)},
	}

	pr := FindPRForTask(task, pulls)
	assert.Nil(t, pr)
}

// Verify BuildFixPrompt joins job names correctly
func TestBuildFixPrompt_JoinsJobNames(t *testing.T) {
	prompt := BuildFixPrompt("o/r", "build.yml", []string{"a", "b", "c"})
	assert.True(t, strings.Contains(prompt, "a, b, c"))
}
