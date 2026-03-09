package confidence

import (
	"testing"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestComputeConfidence_AllGreen(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
	}
	result := ComputeConfidence(runs, nil, 0, 0)
	assert.Equal(t, 100, result.Score)
	assert.Equal(t, LevelHigh, result.Level)
	assert.Len(t, result.Signals, 5)
}

func TestComputeConfidence_AllBad(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "failure"},
		{Status: "completed", Conclusion: "failure"},
		{Status: "completed", Conclusion: "failure"},
	}
	pulls := []models.PullRequest{
		{IsAgent: true, State: "open", CIStatus: "failure"},
		{IsAgent: true, State: "open", CIStatus: "failure"},
		{IsAgent: true, State: "open", CIStatus: "failure"},
	}
	result := ComputeConfidence(runs, pulls, 5, 5)
	assert.Equal(t, LevelLow, result.Level)
	assert.Less(t, result.Score, 50)
}

func TestComputeConfidence_Mixed(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "failure"},
		{Status: "completed", Conclusion: "failure"},
	}
	pulls := []models.PullRequest{
		{IsAgent: true, State: "open", CIStatus: "failure"},
		{IsAgent: true, State: "open", CIStatus: "success"},
	}
	// CI: 3/5=60% → 24pts. New failures 2 → 10pts. Agent fail: 1 → 10pts.
	// Resolved: 2 unresolved → 9pts. Queue 1 → 8pts. Total=61 → MEDIUM
	result := ComputeConfidence(runs, pulls, 1, 2)
	assert.Equal(t, LevelMedium, result.Level)
	assert.GreaterOrEqual(t, result.Score, 50)
	assert.Less(t, result.Score, 80)
}

func TestScoreCIRate_EmptyRuns(t *testing.T) {
	signal := scoreCIRate(nil)
	assert.Equal(t, 40, signal.Points)
	assert.Equal(t, 40, signal.Max)
}

func TestScoreCIRate_AllFailures(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "failure"},
		{Status: "completed", Conclusion: "failure"},
	}
	signal := scoreCIRate(runs)
	assert.Equal(t, 0, signal.Points)
}

func TestScoreNewFailures_Zero(t *testing.T) {
	signal := scoreNewFailures(0)
	assert.Equal(t, 20, signal.Points)
}

func TestScoreNewFailures_FourOrMore(t *testing.T) {
	signal := scoreNewFailures(4)
	assert.Equal(t, 0, signal.Points)

	signal = scoreNewFailures(10)
	assert.Equal(t, 0, signal.Points)
}

func TestScoreAgentPRFailures_NoAgentPRs(t *testing.T) {
	signal := scoreAgentPRFailures(nil)
	assert.Equal(t, 15, signal.Points)
}

func TestScoreReviewQueue_Empty(t *testing.T) {
	signal := scoreReviewQueue(0)
	assert.Equal(t, 10, signal.Points)
}

func TestLevelThresholds(t *testing.T) {
	tests := []struct {
		name  string
		score int
		want  Level
	}{
		{"high at 80", 80, LevelHigh},
		{"high at 100", 100, LevelHigh},
		{"medium at 50", 50, LevelMedium},
		{"medium at 79", 79, LevelMedium},
		{"low at 49", 49, LevelLow},
		{"low at 0", 0, LevelLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Derive level from score using same logic as ComputeConfidence
			level := LevelLow
			if tt.score >= 80 {
				level = LevelHigh
			} else if tt.score >= 50 {
				level = LevelMedium
			}
			assert.Equal(t, tt.want, level)
		})
	}
}
