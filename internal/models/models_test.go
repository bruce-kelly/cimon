package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowRun_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"queued is active", "queued", true},
		{"in_progress is active", "in_progress", true},
		{"completed is not active", "completed", false},
		{"empty status is not active", "", false},
		{"failure is not active", "failure", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := WorkflowRun{Status: tt.status}
			assert.Equal(t, tt.want, r.IsActive())
		})
	}
}

func TestWorkflowRun_Elapsed_Completed(t *testing.T) {
	created := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 3, 9, 10, 5, 0, 0, time.UTC)

	r := WorkflowRun{
		Status:    "completed",
		CreatedAt: created,
		UpdatedAt: updated,
	}

	assert.Equal(t, 5*time.Minute, r.Elapsed())
}

func TestWorkflowRun_Elapsed_Active(t *testing.T) {
	// Created 2 seconds ago — Elapsed() should return ~2s via time.Since
	created := time.Now().Add(-2 * time.Second)

	r := WorkflowRun{
		Status:    "in_progress",
		CreatedAt: created,
	}

	elapsed := r.Elapsed()
	assert.InDelta(t, 2*time.Second, elapsed, float64(500*time.Millisecond),
		"elapsed should be approximately 2 seconds")
}

func TestPullRequest_Size(t *testing.T) {
	pr := PullRequest{Additions: 42, Deletions: 18}
	assert.Equal(t, 60, pr.Size())
}

func TestPullRequest_Size_OnlyAdditions(t *testing.T) {
	pr := PullRequest{Additions: 100}
	assert.Equal(t, 100, pr.Size())
}

func TestPullRequest_Size_OnlyDeletions(t *testing.T) {
	pr := PullRequest{Deletions: 50}
	assert.Equal(t, 50, pr.Size())
}

func TestZeroValueWorkflowRun(t *testing.T) {
	var r WorkflowRun

	assert.False(t, r.IsActive(), "zero-value run should not be active")
	assert.Equal(t, time.Duration(0), r.Elapsed(), "zero-value run elapsed should be 0")
}

func TestZeroValuePullRequest(t *testing.T) {
	var pr PullRequest

	assert.Equal(t, 0, pr.Size(), "zero-value PR size should be 0")
}

func TestPollResult_NilSlices(t *testing.T) {
	// Verify that accessing fields on a PollResult with nil slices doesn't panic
	var result PollResult

	assert.Nil(t, result.Runs)
	assert.Nil(t, result.PullRequests)
	assert.Nil(t, result.Secrets)
	assert.Equal(t, 0, len(result.Runs))
	assert.Equal(t, 0, len(result.PullRequests))
	assert.Equal(t, 0, len(result.Secrets))
	assert.Empty(t, result.Repo)
	assert.Nil(t, result.Error)
	assert.Equal(t, 0, result.RateLimitRemaining)

	// Range over nil slices — should not panic
	for range result.Runs {
		t.Fatal("should not iterate over nil Runs")
	}
	for range result.PullRequests {
		t.Fatal("should not iterate over nil PullRequests")
	}
	for range result.Secrets {
		t.Fatal("should not iterate over nil Secrets")
	}
}
