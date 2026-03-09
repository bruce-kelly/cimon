package github

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// ListRuns fetches workflow runs for a repo+workflow, scoped to branch.
// Returns up to 20 most recent runs.
func (c *Client) ListRuns(ctx context.Context, repo, workflowFile, branch string) ([]models.WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/runs?branch=%s&per_page=20", repo, workflowFile, branch)
	var resp struct {
		WorkflowRuns []ghRun `json:"workflow_runs"`
	}
	if _, err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	runs := make([]models.WorkflowRun, 0, len(resp.WorkflowRuns))
	for _, r := range resp.WorkflowRuns {
		runs = append(runs, r.toModel(repo))
	}
	return runs, nil
}

// ListRunsUnscoped fetches runs without branch filter (for workflows not tied to a branch).
func (c *Client) ListRunsUnscoped(ctx context.Context, repo, workflowFile string) ([]models.WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/runs?per_page=20", repo, workflowFile)
	var resp struct {
		WorkflowRuns []ghRun `json:"workflow_runs"`
	}
	if _, err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	runs := make([]models.WorkflowRun, 0, len(resp.WorkflowRuns))
	for _, r := range resp.WorkflowRuns {
		runs = append(runs, r.toModel(repo))
	}
	return runs, nil
}

// GetJobs fetches jobs for a workflow run.
func (c *Client) GetJobs(ctx context.Context, repo string, runID int64) ([]models.Job, error) {
	path := fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?per_page=100", repo, runID)
	var resp struct {
		Jobs []ghJob `json:"jobs"`
	}
	if _, err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	jobs := make([]models.Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobs = append(jobs, j.toModel())
	}
	return jobs, nil
}

// GetFailedLogs fetches the log output for a job.
func (c *Client) GetFailedLogs(ctx context.Context, repo string, jobID int64) (string, error) {
	path := fmt.Sprintf("/repos/%s/actions/jobs/%d/logs", repo, jobID)
	body, _, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ghRun is the GitHub API representation of a workflow run.
type ghRun struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Path       string  `json:"path"` // .github/workflows/ci.yml
	HeadBranch string  `json:"head_branch"`
	HeadSHA    string  `json:"head_sha"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
	Event      string  `json:"event"`
	Actor      ghActor `json:"actor"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	HTMLURL    string  `json:"html_url"`
}

type ghActor struct {
	Login string `json:"login"`
}

func (r ghRun) toModel(repo string) models.WorkflowRun {
	created, err := time.Parse(time.RFC3339, r.CreatedAt)
	if err != nil {
		slog.Warn("failed to parse run created_at", "value", r.CreatedAt, "err", err)
		created = time.Now()
	}
	updated, err := time.Parse(time.RFC3339, r.UpdatedAt)
	if err != nil {
		slog.Warn("failed to parse run updated_at", "value", r.UpdatedAt, "err", err)
		updated = time.Now()
	}
	conclusion := ""
	if r.Conclusion != nil {
		conclusion = *r.Conclusion
	}
	// Extract workflow filename from path
	wfFile := r.Path
	const prefix = ".github/workflows/"
	if len(r.Path) > len(prefix) && r.Path[:len(prefix)] == prefix {
		wfFile = r.Path[len(prefix):]
	}
	return models.WorkflowRun{
		ID:           r.ID,
		Name:         r.Name,
		WorkflowFile: wfFile,
		HeadBranch:   r.HeadBranch,
		HeadSHA:      r.HeadSHA,
		Status:       r.Status,
		Conclusion:   conclusion,
		Event:        r.Event,
		Actor:        r.Actor.Login,
		CreatedAt:    created,
		UpdatedAt:    updated,
		HTMLURL:      r.HTMLURL,
		Repo:         repo,
	}
}

// ghJob is the GitHub API representation of a job.
type ghJob struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Conclusion  *string  `json:"conclusion"`
	StartedAt   *string  `json:"started_at"`
	CompletedAt *string  `json:"completed_at"`
	RunnerName  string   `json:"runner_name"`
	Steps       []ghStep `json:"steps"`
}

type ghStep struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
	Number     int     `json:"number"`
}

func (j ghJob) toModel() models.Job {
	conclusion := ""
	if j.Conclusion != nil {
		conclusion = *j.Conclusion
	}
	job := models.Job{
		ID:         j.ID,
		Name:       j.Name,
		Status:     j.Status,
		Conclusion: conclusion,
		RunnerName: j.RunnerName,
	}
	if j.StartedAt != nil {
		if t, err := time.Parse(time.RFC3339, *j.StartedAt); err == nil {
			job.StartedAt = &t
		}
	}
	if j.CompletedAt != nil {
		if t, err := time.Parse(time.RFC3339, *j.CompletedAt); err == nil {
			job.CompletedAt = &t
		}
	}
	for _, s := range j.Steps {
		stepConclusion := ""
		if s.Conclusion != nil {
			stepConclusion = *s.Conclusion
		}
		job.Steps = append(job.Steps, models.Step{
			Name:       s.Name,
			Status:     s.Status,
			Conclusion: stepConclusion,
			Number:     s.Number,
		})
	}
	return job
}
