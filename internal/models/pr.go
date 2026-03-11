package models

import "time"

type PullRequest struct {
	Number      int
	Title       string
	Body        string
	Author      string
	Repo        string
	HeadSHA     string
	HTMLURL     string
	State       string
	Draft       bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CIStatus    string // success, failure, pending, unknown
	ReviewState string // approved, changes_requested, pending, none
	IsAgent     bool
	AgentSource string
	Additions   int
	Deletions   int
}

func (p PullRequest) Size() int {
	return p.Additions + p.Deletions
}
