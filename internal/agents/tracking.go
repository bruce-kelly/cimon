package agents

import (
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// FindPRForTask scans recent PRs to find one created by a completed agent task.
// Matches by: same repo, PR created after task started, PR is agent-created.
func FindPRForTask(task DispatchedAgent, pulls []models.PullRequest) *models.PullRequest {
	if task.Status != "completed" && task.Status != "failed" {
		return nil
	}

	for i := range pulls {
		pr := &pulls[i]
		// Must be same repo
		if pr.Repo != task.Repo {
			continue
		}
		// Must be agent-created
		if !pr.IsAgent {
			continue
		}
		// PR must have been created after the task started
		if pr.CreatedAt.Before(task.StartedAt) {
			continue
		}
		// PR must have been created within a reasonable window (2 hours)
		if pr.CreatedAt.After(task.StartedAt.Add(2 * time.Hour)) {
			continue
		}
		return pr
	}
	return nil
}
