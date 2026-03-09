package confidence

import (
	"fmt"

	"github.com/bruce-kelly/cimon/internal/models"
)

type Level string

const (
	LevelLow    Level = "LOW"
	LevelMedium Level = "MEDIUM"
	LevelHigh   Level = "HIGH"
)

type Signal struct {
	Name   string
	Points int
	Max    int
	Detail string
}

type ConfidenceResult struct {
	Score   int
	Level   Level
	Signals []Signal
}

// ComputeConfidence scores release readiness from 0-100 using 5 signals.
func ComputeConfidence(
	runs []models.WorkflowRun,
	pulls []models.PullRequest,
	reviewQueueDepth int,
	newFailureCount int,
) ConfidenceResult {
	var signals []Signal
	totalScore := 0

	// Signal 1: CI passing rate (0-40 pts)
	ciSignal := scoreCIRate(runs)
	signals = append(signals, ciSignal)
	totalScore += ciSignal.Points

	// Signal 2: No new failures (0-20 pts)
	failSignal := scoreNewFailures(newFailureCount)
	signals = append(signals, failSignal)
	totalScore += failSignal.Points

	// Signal 3: No failing agent PRs (0-15 pts)
	agentFailSignal := scoreAgentPRFailures(pulls)
	signals = append(signals, agentFailSignal)
	totalScore += agentFailSignal.Points

	// Signal 4: All agent PRs resolved (0-15 pts)
	agentResolvedSignal := scoreAgentPRsResolved(pulls)
	signals = append(signals, agentResolvedSignal)
	totalScore += agentResolvedSignal.Points

	// Signal 5: Review queue clear (0-10 pts)
	reviewSignal := scoreReviewQueue(reviewQueueDepth)
	signals = append(signals, reviewSignal)
	totalScore += reviewSignal.Points

	level := LevelLow
	if totalScore >= 80 {
		level = LevelHigh
	} else if totalScore >= 50 {
		level = LevelMedium
	}

	return ConfidenceResult{
		Score:   totalScore,
		Level:   level,
		Signals: signals,
	}
}

func scoreCIRate(runs []models.WorkflowRun) Signal {
	if len(runs) == 0 {
		return Signal{Name: "CI pass rate", Points: 40, Max: 40, Detail: "no runs"}
	}
	passing := 0
	completed := 0
	for _, r := range runs {
		if r.Status != "completed" {
			continue
		}
		completed++
		if r.Conclusion == "success" {
			passing++
		}
	}
	if completed == 0 {
		return Signal{Name: "CI pass rate", Points: 40, Max: 40, Detail: "no completed runs"}
	}
	rate := float64(passing) / float64(completed)
	points := int(rate * 40)
	return Signal{
		Name:   "CI pass rate",
		Points: points,
		Max:    40,
		Detail: fmt.Sprintf("%d/%d passing (%.0f%%)", passing, completed, rate*100),
	}
}

func scoreNewFailures(count int) Signal {
	points := 20
	if count > 0 {
		points = max(0, 20-count*5)
	}
	return Signal{
		Name:   "New failures",
		Points: points,
		Max:    20,
		Detail: fmt.Sprintf("%d new failures", count),
	}
}

func scoreAgentPRFailures(pulls []models.PullRequest) Signal {
	failing := 0
	total := 0
	for _, pr := range pulls {
		if !pr.IsAgent || pr.State != "open" {
			continue
		}
		total++
		if pr.CIStatus == "failure" {
			failing++
		}
	}
	points := 15
	if failing > 0 {
		points = max(0, 15-failing*5)
	}
	return Signal{
		Name:   "Agent PR CI",
		Points: points,
		Max:    15,
		Detail: fmt.Sprintf("%d/%d agent PRs failing", failing, total),
	}
}

func scoreAgentPRsResolved(pulls []models.PullRequest) Signal {
	unresolved := 0
	for _, pr := range pulls {
		if !pr.IsAgent || pr.State != "open" {
			continue
		}
		unresolved++
	}
	points := 15
	if unresolved > 0 {
		points = max(0, 15-unresolved*3)
	}
	return Signal{
		Name:   "Agent PRs resolved",
		Points: points,
		Max:    15,
		Detail: fmt.Sprintf("%d unresolved agent PRs", unresolved),
	}
}

func scoreReviewQueue(depth int) Signal {
	points := 10
	if depth > 0 {
		points = max(0, 10-depth*2)
	}
	return Signal{
		Name:   "Review queue",
		Points: points,
		Max:    10,
		Detail: fmt.Sprintf("%d items in queue", depth),
	}
}
