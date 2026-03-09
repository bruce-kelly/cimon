package agents

import (
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// AgentProfile summarizes an agent workflow's history.
type AgentProfile struct {
	WorkflowFile string
	Repo         string
	Trigger      TriggerType
	History      []float64 // sparkline data: 1.0=success, 0.5=silent, 0.0=failure
	SuccessRate  float64
	LastOutcome  OutcomeBucket
	LastRunAt    time.Time
	TotalRuns    int
}

// BuildAgentProfiles groups workflow runs by file and computes profiles.
func BuildAgentProfiles(runs []models.WorkflowRun, agentWorkflows map[string]bool) []AgentProfile {
	// Group runs by workflow file
	byWorkflow := make(map[string][]models.WorkflowRun)
	for _, run := range runs {
		if !agentWorkflows[run.WorkflowFile] {
			continue
		}
		byWorkflow[run.WorkflowFile] = append(byWorkflow[run.WorkflowFile], run)
	}

	var profiles []AgentProfile
	for wf, wfRuns := range byWorkflow {
		if len(wfRuns) == 0 {
			continue
		}

		profile := AgentProfile{
			WorkflowFile: wf,
			Repo:         wfRuns[0].Repo,
			TotalRuns:    len(wfRuns),
		}

		// Most recent run
		latest := wfRuns[0]
		for _, r := range wfRuns {
			if r.UpdatedAt.After(latest.UpdatedAt) {
				latest = r
			}
		}
		profile.LastRunAt = latest.UpdatedAt
		profile.Trigger = ClassifyTrigger(latest.Event)
		profile.LastOutcome = ClassifyOutcome(latest.Conclusion, len(latest.ArtifactsProduced))

		// Compute sparkline data from last N runs
		maxHistory := 20
		if len(wfRuns) < maxHistory {
			maxHistory = len(wfRuns)
		}
		for i := 0; i < maxHistory; i++ {
			r := wfRuns[i]
			outcome := ClassifyOutcome(r.Conclusion, len(r.ArtifactsProduced))
			switch outcome {
			case BucketActed:
				profile.History = append(profile.History, 1.0)
			case BucketSilent:
				profile.History = append(profile.History, 0.5)
			case BucketAlert:
				profile.History = append(profile.History, 0.0)
			}
		}

		// Compute success rate across all runs
		if profile.TotalRuns > 0 {
			totalSuccess := 0
			for _, r := range wfRuns {
				if r.Conclusion == "success" {
					totalSuccess++
				}
			}
			profile.SuccessRate = float64(totalSuccess) / float64(profile.TotalRuns)
		}

		profiles = append(profiles, profile)
	}

	return profiles
}
