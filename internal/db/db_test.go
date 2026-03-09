package db

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustOpenMemory(t *testing.T) *Database {
	t.Helper()
	db, err := OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenClose(t *testing.T) {
	db := mustOpenMemory(t)
	assert.NotNil(t, db)
	assert.Equal(t, ":memory:", db.path)
}

// --- Workflow runs ---

func TestUpsertRunAndQuery(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	run := models.WorkflowRun{
		ID:           100,
		Repo:         "owner/repo",
		Name:         "CI",
		WorkflowFile: "ci.yml",
		HeadBranch:   "main",
		HeadSHA:      "abc123",
		Status:       "completed",
		Conclusion:   "success",
		Event:        "push",
		Actor:        "alice",
		CreatedAt:    now.Add(-5 * time.Minute),
		UpdatedAt:    now,
		HTMLURL:      "https://github.com/owner/repo/actions/runs/100",
	}

	require.NoError(t, db.UpsertRun(run))

	runs, err := db.QueryRuns("owner/repo", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	got := runs[0]
	assert.Equal(t, int64(100), got.ID)
	assert.Equal(t, "owner/repo", got.Repo)
	assert.Equal(t, "CI", got.Name)
	assert.Equal(t, "ci.yml", got.WorkflowFile)
	assert.Equal(t, "main", got.HeadBranch)
	assert.Equal(t, "abc123", got.HeadSHA)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, "success", got.Conclusion)
	assert.Equal(t, "push", got.Event)
	assert.Equal(t, "alice", got.Actor)
	assert.Equal(t, run.HTMLURL, got.HTMLURL)
}

func TestUpsertRunIdempotency(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	run := models.WorkflowRun{
		ID:           200,
		Repo:         "owner/repo",
		Name:         "CI",
		WorkflowFile: "ci.yml",
		Status:       "in_progress",
		Conclusion:   "",
		CreatedAt:    now.Add(-5 * time.Minute),
		UpdatedAt:    now.Add(-2 * time.Minute),
	}
	require.NoError(t, db.UpsertRun(run))

	// Update same run with new status
	run.Status = "completed"
	run.Conclusion = "failure"
	run.UpdatedAt = now
	require.NoError(t, db.UpsertRun(run))

	runs, err := db.QueryRuns("owner/repo", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "completed", runs[0].Status)
	assert.Equal(t, "failure", runs[0].Conclusion)
}

func TestUpsertJobsAndIsKnownFailure(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Create a run on main branch
	for i := range 3 {
		run := models.WorkflowRun{
			ID:           int64(300 + i),
			Repo:         "owner/repo",
			Name:         "CI",
			WorkflowFile: "ci.yml",
			HeadBranch:   "main",
			Status:       "completed",
			Conclusion:   "failure",
			CreatedAt:    now.Add(-time.Duration(i) * time.Hour),
			UpdatedAt:    now.Add(-time.Duration(i) * time.Hour),
		}
		require.NoError(t, db.UpsertRun(run))

		completed := now.Add(-time.Duration(i) * time.Hour)
		jobs := []models.Job{{
			ID:          int64(1000 + i),
			Name:        "test",
			Conclusion:  "failure",
			CompletedAt: &completed,
		}}
		require.NoError(t, db.UpsertJobs(run.ID, "owner/repo", jobs))
	}

	known, err := db.IsKnownFailure("owner/repo", "test", 24)
	require.NoError(t, err)
	assert.True(t, known, "3 failures on main should be known")
}

func TestIsKnownFailureBelowThreshold(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Only 2 failures — below the threshold of 3
	for i := range 2 {
		run := models.WorkflowRun{
			ID:           int64(400 + i),
			Repo:         "owner/repo",
			Name:         "CI",
			WorkflowFile: "ci.yml",
			HeadBranch:   "main",
			Status:       "completed",
			Conclusion:   "failure",
			CreatedAt:    now.Add(-time.Duration(i) * time.Hour),
			UpdatedAt:    now.Add(-time.Duration(i) * time.Hour),
		}
		require.NoError(t, db.UpsertRun(run))

		completed := now.Add(-time.Duration(i) * time.Hour)
		jobs := []models.Job{{
			ID:          int64(2000 + i),
			Name:        "test",
			Conclusion:  "failure",
			CompletedAt: &completed,
		}}
		require.NoError(t, db.UpsertJobs(run.ID, "owner/repo", jobs))
	}

	known, err := db.IsKnownFailure("owner/repo", "test", 24)
	require.NoError(t, err)
	assert.False(t, known, "2 failures should not be known")
}

// --- Pull requests ---

func TestUpsertPullAndQuery(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	pr := models.PullRequest{
		Repo:        "owner/repo",
		Number:      42,
		Title:       "Add feature",
		Author:      "bob",
		State:       "open",
		Draft:       true,
		IsAgent:     true,
		CIStatus:    "success",
		ReviewState: "approved",
		CreatedAt:   now.Add(-1 * time.Hour),
		UpdatedAt:   now,
		HTMLURL:     "https://github.com/owner/repo/pull/42",
	}
	require.NoError(t, db.UpsertPull(pr))

	pulls, err := db.QueryPulls("owner/repo")
	require.NoError(t, err)
	require.Len(t, pulls, 1)

	got := pulls[0]
	assert.Equal(t, 42, got.Number)
	assert.Equal(t, "Add feature", got.Title)
	assert.Equal(t, "bob", got.Author)
	assert.True(t, got.Draft)
	assert.True(t, got.IsAgent)
	assert.Equal(t, "success", got.CIStatus)
	assert.Equal(t, "approved", got.ReviewState)
}

func TestQueryPullsExcludesClosed(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	open := models.PullRequest{
		Repo: "owner/repo", Number: 1, State: "open",
		CreatedAt: now, UpdatedAt: now,
	}
	closed := models.PullRequest{
		Repo: "owner/repo", Number: 2, State: "closed",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.UpsertPull(open))
	require.NoError(t, db.UpsertPull(closed))

	pulls, err := db.QueryPulls("owner/repo")
	require.NoError(t, err)
	require.Len(t, pulls, 1)
	assert.Equal(t, 1, pulls[0].Number)
}

// --- Prune ---

func TestPruneRuns(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)
	old := now.AddDate(0, 0, -100)

	oldRun := models.WorkflowRun{
		ID: 500, Repo: "owner/repo", Name: "CI", WorkflowFile: "ci.yml",
		CreatedAt: old, UpdatedAt: old,
	}
	newRun := models.WorkflowRun{
		ID: 501, Repo: "owner/repo", Name: "CI", WorkflowFile: "ci.yml",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.UpsertRun(oldRun))
	require.NoError(t, db.UpsertRun(newRun))

	// Also add jobs to the old run to verify cascade delete
	completed := old
	require.NoError(t, db.UpsertJobs(500, "owner/repo", []models.Job{
		{ID: 5000, Name: "test", CompletedAt: &completed},
	}))

	pruned, err := db.PruneRuns(90)
	require.NoError(t, err)
	assert.Equal(t, int64(1), pruned)

	runs, err := db.QueryRuns("owner/repo", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, int64(501), runs[0].ID)
}

// --- Agent tasks ---

func TestInsertTaskAndQuery(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("task-1", "owner/repo", "Fix tests", now))

	tasks, err := db.QueryTasks("owner/repo", "running")
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	got := tasks[0]
	assert.Equal(t, "task-1", got.ID)
	assert.Equal(t, "owner/repo", got.Repo)
	assert.Equal(t, "Fix tests", got.Task)
	assert.Equal(t, "running", got.Status)
	assert.Nil(t, got.ExitCode)
	assert.Nil(t, got.CompletedAt)
	assert.Nil(t, got.PRNumber)
}

func TestUpdateTaskStatus(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("task-2", "owner/repo", "Deploy", now))

	exitCode := 0
	require.NoError(t, db.UpdateTaskStatus("task-2", "completed", &exitCode))

	tasks, err := db.QueryTasks("owner/repo", "completed")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "completed", tasks[0].Status)
	require.NotNil(t, tasks[0].ExitCode)
	assert.Equal(t, 0, *tasks[0].ExitCode)
	assert.NotNil(t, tasks[0].CompletedAt)
}

func TestLinkTaskToPR(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("task-3", "owner/repo", "Create PR", now))
	require.NoError(t, db.LinkTaskToPR("task-3", 99))

	tasks, err := db.QueryTasks("owner/repo", "")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].PRNumber)
	assert.Equal(t, 99, *tasks[0].PRNumber)
}

func TestMarkOrphanedTasks(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("active-1", "owner/repo", "Running", now))
	require.NoError(t, db.InsertTask("orphan-1", "owner/repo", "Abandoned", now))
	require.NoError(t, db.InsertTask("orphan-2", "owner/repo", "Also abandoned", now))

	// Only active-1 is still alive
	active := map[string]bool{"active-1": true}
	count, err := db.MarkOrphanedTasks(active)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify orphans are failed
	tasks, err := db.QueryTasks("owner/repo", "failed")
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	// Active one should still be running
	tasks, err = db.QueryTasks("owner/repo", "running")
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "active-1", tasks[0].ID)
}

// --- Stats ---

func TestRunStats(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	runs := []models.WorkflowRun{
		{ID: 600, Repo: "r", Name: "CI", WorkflowFile: "ci.yml",
			Conclusion: "success", CreatedAt: now, UpdatedAt: now},
		{ID: 601, Repo: "r", Name: "CI", WorkflowFile: "ci.yml",
			Conclusion: "success", CreatedAt: now, UpdatedAt: now},
		{ID: 602, Repo: "r", Name: "CI", WorkflowFile: "ci.yml",
			Conclusion: "failure", CreatedAt: now, UpdatedAt: now},
	}
	for _, r := range runs {
		require.NoError(t, db.UpsertRun(r))
	}

	stats, err := db.RunStats(7)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 2, stats.Success)
	assert.Equal(t, 1, stats.Failure)
}

func TestTaskStats(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("ts-1", "r", "task1", now))
	require.NoError(t, db.InsertTask("ts-2", "r", "task2", now))
	require.NoError(t, db.InsertTask("ts-3", "r", "task3", now))

	ec := 0
	require.NoError(t, db.UpdateTaskStatus("ts-1", "completed", &ec))
	ec = 1
	require.NoError(t, db.UpdateTaskStatus("ts-2", "failed", &ec))

	stats, err := db.TaskStats(7)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 1, stats.Completed)
	assert.Equal(t, 1, stats.Failed)
}

func TestAgentEffectivenessStats(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("eff-1", "r", "t1", now))
	require.NoError(t, db.InsertTask("eff-2", "r", "t2", now))
	require.NoError(t, db.InsertTask("eff-3", "r", "t3", now))

	ec := 0
	require.NoError(t, db.UpdateTaskStatus("eff-1", "completed", &ec))
	require.NoError(t, db.LinkTaskToPR("eff-1", 10))

	ec = 1
	require.NoError(t, db.UpdateTaskStatus("eff-2", "failed", &ec))

	stats, err := db.AgentEffectivenessStats(30)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Dispatched)
	assert.Equal(t, 1, stats.CreatedPR)
	assert.InDelta(t, 1.0/3.0, stats.FailureRate, 0.001)
}

// --- Dismissed ---

func TestAddAndIsDismissed(t *testing.T) {
	db := mustOpenMemory(t)

	dismissed, err := db.IsDismissed("owner/repo", 10)
	require.NoError(t, err)
	assert.False(t, dismissed)

	require.NoError(t, db.AddDismissed("owner/repo", 10))

	dismissed, err = db.IsDismissed("owner/repo", 10)
	require.NoError(t, err)
	assert.True(t, dismissed)
}

func TestRemoveDismissed(t *testing.T) {
	db := mustOpenMemory(t)

	require.NoError(t, db.AddDismissed("owner/repo", 20))
	require.NoError(t, db.RemoveDismissed("owner/repo", 20))

	dismissed, err := db.IsDismissed("owner/repo", 20)
	require.NoError(t, err)
	assert.False(t, dismissed)
}

func TestLoadDismissed(t *testing.T) {
	db := mustOpenMemory(t)

	require.NoError(t, db.AddDismissed("owner/repo-a", 1))
	require.NoError(t, db.AddDismissed("owner/repo-a", 2))
	require.NoError(t, db.AddDismissed("owner/repo-b", 5))

	m, err := db.LoadDismissed()
	require.NoError(t, err)
	assert.Len(t, m, 3)
	assert.True(t, m["owner/repo-a:1"])
	assert.True(t, m["owner/repo-a:2"])
	assert.True(t, m["owner/repo-b:5"])
	assert.False(t, m["owner/repo-b:99"])
}

// --- Since queries ---

func TestRunsSince(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	// One recent, one old
	require.NoError(t, db.UpsertRun(models.WorkflowRun{
		ID: 700, Repo: "r", Name: "CI", WorkflowFile: "ci.yml",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, db.UpsertRun(models.WorkflowRun{
		ID: 701, Repo: "r", Name: "CI", WorkflowFile: "ci.yml",
		CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-48 * time.Hour),
	}))

	count, err := db.RunsSince("r", now.Add(-1*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	count, err = db.RunsSince("r", now.Add(-72*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestTasksSince(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.InsertTask("since-1", "r", "t1", now))
	require.NoError(t, db.InsertTask("since-2", "r", "t2", now.Add(-48*time.Hour)))

	count, err := db.TasksSince("r", now.Add(-1*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestPullsChangedSince(t *testing.T) {
	db := mustOpenMemory(t)
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.UpsertPull(models.PullRequest{
		Repo: "r", Number: 1, State: "open",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, db.UpsertPull(models.PullRequest{
		Repo: "r", Number: 2, State: "open",
		CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-48 * time.Hour),
	}))

	count, err := db.PullsChangedSince("r", now.Add(-1*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
