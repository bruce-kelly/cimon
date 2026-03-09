package models

import "time"

type Step struct {
	Name       string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, skipped, cancelled
	Number     int
}

type Job struct {
	ID          int64
	Name        string
	Status      string
	Conclusion  string
	Steps       []Step
	StartedAt   *time.Time
	CompletedAt *time.Time
	RunnerName  string
}

type WorkflowRun struct {
	ID                int64
	Name              string
	WorkflowFile      string
	HeadBranch        string
	HeadSHA           string
	Status            string
	Conclusion        string
	Event             string
	Actor             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	HTMLURL           string
	Repo              string
	Jobs              []Job
	ArtifactsProduced []ArtifactRef
}

func (r WorkflowRun) IsActive() bool {
	return r.Status == "queued" || r.Status == "in_progress"
}

func (r WorkflowRun) Elapsed() time.Duration {
	if r.IsActive() {
		return time.Since(r.CreatedAt)
	}
	return r.UpdatedAt.Sub(r.CreatedAt)
}

type ArtifactRef struct {
	Type   string // pr, issue, comment
	Repo   string
	Number int
	URL    string
}

type SecretInfo struct {
	Name   string
	Exists bool
}
