package agents

import (
	"os"
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ClassifyOutcome tests ---

func TestClassifyOutcome_SuccessWithArtifacts(t *testing.T) {
	assert.Equal(t, BucketActed, ClassifyOutcome("success", 2))
}

func TestClassifyOutcome_SuccessWithoutArtifacts(t *testing.T) {
	assert.Equal(t, BucketSilent, ClassifyOutcome("success", 0))
}

func TestClassifyOutcome_Failure(t *testing.T) {
	assert.Equal(t, BucketAlert, ClassifyOutcome("failure", 0))
}

func TestClassifyOutcome_TimedOut(t *testing.T) {
	assert.Equal(t, BucketAlert, ClassifyOutcome("timed_out", 0))
}

func TestClassifyOutcome_Cancelled(t *testing.T) {
	assert.Equal(t, BucketSilent, ClassifyOutcome("cancelled", 0))
}

func TestClassifyOutcome_Unknown(t *testing.T) {
	assert.Equal(t, BucketSilent, ClassifyOutcome("neutral", 0))
}

// --- ClassifyTrigger tests ---

func TestClassifyTrigger_Schedule(t *testing.T) {
	assert.Equal(t, TriggerCron, ClassifyTrigger("schedule"))
}

func TestClassifyTrigger_WorkflowDispatch(t *testing.T) {
	assert.Equal(t, TriggerManual, ClassifyTrigger("workflow_dispatch"))
}

func TestClassifyTrigger_Push(t *testing.T) {
	assert.Equal(t, TriggerEvent, ClassifyTrigger("push"))
}

func TestClassifyTrigger_PullRequest(t *testing.T) {
	assert.Equal(t, TriggerEvent, ClassifyTrigger("pull_request"))
}

// --- String() tests ---

func TestOutcomeBucketString(t *testing.T) {
	assert.Equal(t, "silent", BucketSilent.String())
	assert.Equal(t, "acted", BucketActed.String())
	assert.Equal(t, "alert", BucketAlert.String())
}

func TestTriggerTypeString(t *testing.T) {
	assert.Equal(t, "cron", TriggerCron.String())
	assert.Equal(t, "event", TriggerEvent.String())
	assert.Equal(t, "manual", TriggerManual.String())
}

// --- BuildAgentProfiles tests ---

func TestBuildAgentProfiles_GroupsByWorkflow(t *testing.T) {
	now := time.Now()
	runs := []models.WorkflowRun{
		{
			WorkflowFile: "claude.yml",
			Repo:         "owner/repo",
			Conclusion:   "success",
			Event:        "schedule",
			UpdatedAt:    now,
			ArtifactsProduced: []models.ArtifactRef{
				{Type: "pr", Number: 1},
			},
		},
		{
			WorkflowFile: "claude.yml",
			Repo:         "owner/repo",
			Conclusion:   "success",
			Event:        "schedule",
			UpdatedAt:    now.Add(-time.Hour),
		},
		{
			WorkflowFile: "claude.yml",
			Repo:         "owner/repo",
			Conclusion:   "failure",
			Event:        "schedule",
			UpdatedAt:    now.Add(-2 * time.Hour),
		},
	}
	agentWorkflows := map[string]bool{"claude.yml": true}

	profiles := BuildAgentProfiles(runs, agentWorkflows)
	require.Len(t, profiles, 1)

	p := profiles[0]
	assert.Equal(t, "claude.yml", p.WorkflowFile)
	assert.Equal(t, "owner/repo", p.Repo)
	assert.Equal(t, 3, p.TotalRuns)
	assert.Equal(t, TriggerCron, p.Trigger)
	assert.Equal(t, BucketActed, p.LastOutcome)

	// Success rate: 2 out of 3 succeeded
	assert.InDelta(t, 2.0/3.0, p.SuccessRate, 0.01)

	// Sparkline: acted(1.0), silent(0.5), alert(0.0)
	require.Len(t, p.History, 3)
	assert.Equal(t, 1.0, p.History[0])
	assert.Equal(t, 0.5, p.History[1])
	assert.Equal(t, 0.0, p.History[2])
}

func TestBuildAgentProfiles_SkipsNonAgentWorkflows(t *testing.T) {
	runs := []models.WorkflowRun{
		{
			WorkflowFile: "ci.yml",
			Repo:         "owner/repo",
			Conclusion:   "success",
			UpdatedAt:    time.Now(),
		},
		{
			WorkflowFile: "claude.yml",
			Repo:         "owner/repo",
			Conclusion:   "success",
			UpdatedAt:    time.Now(),
		},
	}
	agentWorkflows := map[string]bool{"claude.yml": true}

	profiles := BuildAgentProfiles(runs, agentWorkflows)
	require.Len(t, profiles, 1)
	assert.Equal(t, "claude.yml", profiles[0].WorkflowFile)
}

func TestBuildAgentProfiles_EmptyInput(t *testing.T) {
	profiles := BuildAgentProfiles(nil, map[string]bool{"claude.yml": true})
	assert.Nil(t, profiles)
}

func TestBuildAgentProfiles_EmptyAgentWorkflows(t *testing.T) {
	runs := []models.WorkflowRun{
		{
			WorkflowFile: "ci.yml",
			Conclusion:   "success",
			UpdatedAt:    time.Now(),
		},
	}
	profiles := BuildAgentProfiles(runs, map[string]bool{})
	assert.Nil(t, profiles)
}

// --- Dispatcher tests (struct-level, no real processes) ---

func TestDispatcher_ConcurrencyLimit(t *testing.T) {
	d := &Dispatcher{
		agents:  make(map[string]*DispatchedAgent),
		maxConc: 1,
		maxLife: 30 * time.Minute,
	}

	// Inject a running agent directly into the map
	d.agents["agent-1"] = &DispatchedAgent{
		ID:     "agent-1",
		Repo:   "owner/repo",
		Task:   "fix tests",
		Status: "running",
	}

	// Dispatch should fail because we're at the limit
	_, err := d.Dispatch("owner/repo", "another task")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent agents")
}

func TestDispatcher_GetAgent(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}

	agent := &DispatchedAgent{
		ID:     "agent-test",
		Repo:   "owner/repo",
		Task:   "do stuff",
		Status: "completed",
	}
	d.agents["agent-test"] = agent

	got := d.GetAgent("agent-test")
	assert.Equal(t, agent, got)

	assert.Nil(t, d.GetAgent("nonexistent"))
}

func TestDispatcher_RunningAgents(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}

	d.agents["a1"] = &DispatchedAgent{ID: "a1", Status: "running"}
	d.agents["a2"] = &DispatchedAgent{ID: "a2", Status: "completed"}
	d.agents["a3"] = &DispatchedAgent{ID: "a3", Status: "running"}

	running := d.RunningAgents()
	assert.Len(t, running, 2)
}

func TestDispatcher_AllAgents(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}

	d.agents["a1"] = &DispatchedAgent{ID: "a1", Status: "running"}
	d.agents["a2"] = &DispatchedAgent{ID: "a2", Status: "completed"}

	all := d.AllAgents()
	assert.Len(t, all, 2)
}

func TestDispatcher_Shutdown(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}

	// Use a PID that won't exist; kill will fail with ESRCH but status still updates
	d.agents["a1"] = &DispatchedAgent{ID: "a1", Status: "running", PID: 999999999}
	d.agents["a2"] = &DispatchedAgent{ID: "a2", Status: "completed", PID: 999999999}

	d.Shutdown()

	assert.Equal(t, "killed", d.agents["a1"].Status)
	// Completed agents should not be affected
	assert.Equal(t, "completed", d.agents["a2"].Status)
}

func TestDispatcher_GetOutput_NotFound(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}

	_, err := d.GetOutput("nonexistent", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")
}

func TestDispatcher_GetOutput_NoOutputPath(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}
	d.agents["a1"] = &DispatchedAgent{ID: "a1", OutputPath: ""}

	out, err := d.GetOutput("a1", 10)
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

func TestDispatcher_GetOutput_WithFile(t *testing.T) {
	d := &Dispatcher{
		agents: make(map[string]*DispatchedAgent),
	}

	// Write a temp output file
	tmpFile := t.TempDir() + "/test-output.log"
	require.NoError(t, writeTestFile(tmpFile, "line1\nline2\nline3\nline4\nline5"))

	d.agents["a1"] = &DispatchedAgent{ID: "a1", OutputPath: tmpFile}

	// Tail last 3 lines
	out, err := d.GetOutput("a1", 3)
	require.NoError(t, err)
	assert.Equal(t, "line3\nline4\nline5", out)
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// --- Scheduler tests ---

func TestScheduler_DueTasks_EveryMinute(t *testing.T) {
	configs := []config.ScheduledAgentConfig{
		{Name: "every-minute", Cron: "* * * * *", Prompt: "test"},
	}
	s := NewScheduler(configs)
	require.Len(t, s.tasks, 1)

	// Task should be due immediately (LastFired is zero)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	due := s.DueTasks(now)
	assert.Len(t, due, 1)
	assert.Equal(t, "every-minute", due[0].Config.Name)
}

func TestScheduler_DoubleFire_Prevention(t *testing.T) {
	configs := []config.ScheduledAgentConfig{
		{Name: "every-minute", Cron: "* * * * *", Prompt: "test"},
	}
	s := NewScheduler(configs)

	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	// First call should fire
	due := s.DueTasks(now)
	assert.Len(t, due, 1)

	// Second call in the same minute should NOT fire
	due2 := s.DueTasks(now.Add(30 * time.Second))
	assert.Len(t, due2, 0)

	// Next minute should fire again
	due3 := s.DueTasks(now.Add(time.Minute))
	assert.Len(t, due3, 1)
}

func TestScheduler_NextFireTime(t *testing.T) {
	configs := []config.ScheduledAgentConfig{
		{Name: "hourly", Cron: "0 * * * *", Prompt: "test"},
	}
	s := NewScheduler(configs)

	nft := s.NextFireTime("hourly")
	require.NotNil(t, nft)
	// Next fire time should be in the future
	assert.True(t, nft.After(time.Now()))
}

func TestScheduler_NextFireTime_NotFound(t *testing.T) {
	s := NewScheduler(nil)
	assert.Nil(t, s.NextFireTime("nonexistent"))
}

func TestScheduler_InvalidCron(t *testing.T) {
	configs := []config.ScheduledAgentConfig{
		{Name: "bad-cron", Cron: "not-a-cron", Prompt: "test"},
		{Name: "good-cron", Cron: "* * * * *", Prompt: "test"},
	}
	s := NewScheduler(configs)
	// Invalid cron should be skipped; only the good one remains
	assert.Len(t, s.tasks, 1)
	assert.Equal(t, "good-cron", s.tasks[0].Config.Name)
}
