package models

type PollResult struct {
	Runs               []WorkflowRun
	PullRequests       []PullRequest
	Secrets            []SecretInfo
	Repo               string
	Error              error
	RateLimitRemaining int
	ETag               string
}
