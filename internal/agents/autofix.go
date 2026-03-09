package agents

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// AutoFixDecision describes whether an auto-fix should be dispatched.
type AutoFixDecision struct {
	ShouldDispatch bool
	Repo           string
	WorkflowFile   string
	FailingJobs    []string
	Prompt         string
	Reason         string // why skipped, if not dispatching
}

// AutoFixTracker evaluates new failures and decides whether to dispatch fix agents.
type AutoFixTracker struct {
	cooldowns     map[string]time.Time // "repo:workflow" → last dispatch time
	cooldownSec   map[string]int       // "repo:workflow" → cooldown seconds
	maxConcurrent int
	mu            sync.Mutex
}

func NewAutoFixTracker(maxConcurrent int) *AutoFixTracker {
	return &AutoFixTracker{
		cooldowns:     make(map[string]time.Time),
		cooldownSec:   make(map[string]int),
		maxConcurrent: maxConcurrent,
	}
}

// SetCooldown configures cooldown for a repo:workflow pair.
func (t *AutoFixTracker) SetCooldown(repo, workflow string, seconds int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := repo + ":" + workflow
	t.cooldownSec[key] = seconds
}

// Evaluate checks if a fix agent should be dispatched for a set of failing runs.
// isKnownFailure is a callback that checks if a job is a known recurring failure.
func (t *AutoFixTracker) Evaluate(
	repo, workflow string,
	failedJobs []models.Job,
	runningAgentCount int,
	isKnownFailure func(repo, jobName string) bool,
) AutoFixDecision {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := repo + ":" + workflow

	// Filter out known failures
	var newFailures []models.Job
	for _, job := range failedJobs {
		if isKnownFailure != nil && isKnownFailure(repo, job.Name) {
			continue
		}
		if job.Conclusion == "failure" {
			newFailures = append(newFailures, job)
		}
	}

	if len(newFailures) == 0 {
		return AutoFixDecision{
			Repo:         repo,
			WorkflowFile: workflow,
			Reason:       "no new failures (all known)",
		}
	}

	// Check cooldown
	if lastDispatch, ok := t.cooldowns[key]; ok {
		cooldown := 300 // default
		if c, ok := t.cooldownSec[key]; ok {
			cooldown = c
		}
		if time.Since(lastDispatch) < time.Duration(cooldown)*time.Second {
			return AutoFixDecision{
				Repo:         repo,
				WorkflowFile: workflow,
				Reason:       "cooldown active",
			}
		}
	}

	// Check concurrency
	if runningAgentCount >= t.maxConcurrent {
		return AutoFixDecision{
			Repo:         repo,
			WorkflowFile: workflow,
			Reason:       fmt.Sprintf("max concurrent agents (%d) reached", t.maxConcurrent),
		}
	}

	// Build dispatch decision
	var jobNames []string
	for _, j := range newFailures {
		jobNames = append(jobNames, j.Name)
	}

	t.cooldowns[key] = time.Now()

	return AutoFixDecision{
		ShouldDispatch: true,
		Repo:           repo,
		WorkflowFile:   workflow,
		FailingJobs:    jobNames,
		Prompt:         BuildFixPrompt(repo, workflow, jobNames),
	}
}

// BuildFixPrompt creates a prompt for the fix agent.
func BuildFixPrompt(repo, workflow string, failingJobs []string) string {
	return fmt.Sprintf(
		"Fix failing CI jobs in %s (workflow: %s). Failing jobs: %s. "+
			"Analyze the failures, identify root causes, and create a PR with fixes.",
		repo, workflow, strings.Join(failingJobs, ", "),
	)
}
