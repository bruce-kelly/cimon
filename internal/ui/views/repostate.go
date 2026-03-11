package views

import (
	"sort"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
)

// RepoStatus represents the worst CI state for a repo.
type RepoStatus int

const (
	StatusPassing RepoStatus = iota
	StatusPending
	StatusActive
	StatusFailed
)

// ActiveRunInfo summarizes a running workflow for inline display.
type ActiveRunInfo struct {
	Name          string
	CompletedJobs int
	TotalJobs     int
	Elapsed       time.Duration
	IsRelease     bool
}

// InlineStatus is the computed CI state for inline expansion in compact view.
type InlineStatus struct {
	Worst          RepoStatus
	FailedWorkflow string
	FailedJobs     []string
	FailedAt       time.Time
	ActiveRuns     []ActiveRunInfo
	Releasing      bool
}

// PRSummary is the computed PR state for the repo summary line.
type PRSummary struct {
	Total     int
	Ready     int // approved + CI passing + not draft
	CIPending bool
}

// RepoState holds all display state for one repo in the compact view.
type RepoState struct {
	RepoName    string // short name (repo portion only, no owner/)
	FullName    string // owner/repo
	Runs        []models.WorkflowRun
	PRs         []models.PullRequest
	ReviewItems []review.ReviewItem
	Inline      InlineStatus
	PRSummary   PRSummary

	// NEW flag state
	NewFlag           bool
	LastNotableChange time.Time
	UserAcknowledged  bool
}

// ComputeInlineStatus derives the inline display state from a repo's runs.
func ComputeInlineStatus(runs []models.WorkflowRun) InlineStatus {
	var status InlineStatus
	status.Worst = StatusPassing

	for i := range runs {
		r := &runs[i]
		if r.IsActive() {
			if status.Worst < StatusActive {
				status.Worst = StatusActive
			}
			completed := 0
			total := len(r.Jobs)
			for _, j := range r.Jobs {
				if j.Conclusion != "" {
					completed++
				}
			}
			status.ActiveRuns = append(status.ActiveRuns, ActiveRunInfo{
				Name:          r.Name,
				CompletedJobs: completed,
				TotalJobs:     total,
				Elapsed:       r.Elapsed(),
			})
		} else if r.Conclusion == "failure" {
			status.Worst = StatusFailed
			if status.FailedWorkflow == "" {
				status.FailedWorkflow = r.Name
				status.FailedAt = r.UpdatedAt
				for _, j := range r.Jobs {
					if j.Conclusion == "failure" {
						status.FailedJobs = append(status.FailedJobs, j.Name)
					}
				}
			}
		}
	}

	for i := range status.ActiveRuns {
		if status.ActiveRuns[i].IsRelease {
			status.Releasing = true
			break
		}
	}

	return status
}

// ComputePRSummary derives the PR summary from a repo's pull requests.
func ComputePRSummary(prs []models.PullRequest) PRSummary {
	var s PRSummary
	s.Total = len(prs)
	for _, pr := range prs {
		if pr.CIStatus == "pending" {
			s.CIPending = true
		}
		if pr.CIStatus == "success" && pr.ReviewState == "approved" && !pr.Draft {
			s.Ready++
		}
	}
	return s
}

// SortByAttention sorts repo states by attention priority:
// 1. Failures (newest first)
// 2. Active runs
// 3. Repos with ready-to-merge PRs
// 4. All green
func SortByAttention(states []RepoState) {
	sort.SliceStable(states, func(i, j int) bool {
		si, sj := states[i], states[j]
		if (si.Inline.Worst == StatusFailed) != (sj.Inline.Worst == StatusFailed) {
			return si.Inline.Worst == StatusFailed
		}
		if (si.Inline.Worst == StatusActive) != (sj.Inline.Worst == StatusActive) {
			return si.Inline.Worst == StatusActive
		}
		if (si.PRSummary.Ready > 0) != (sj.PRSummary.Ready > 0) {
			return si.PRSummary.Ready > 0
		}
		return false
	})
}

// DetectNewFlag returns true if the repo state changed in a notable way.
func DetectNewFlag(prev, curr RepoState) bool {
	if prev.Inline.Worst != StatusFailed && curr.Inline.Worst == StatusFailed {
		return true
	}
	if curr.PRSummary.Ready > prev.PRSummary.Ready {
		return true
	}
	if !prev.Inline.Releasing && curr.Inline.Releasing {
		return true
	}
	return false
}
