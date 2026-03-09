package review

import (
	"fmt"
	"sort"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

type EscalationLevel int

const (
	EscalationNormal EscalationLevel = iota
	EscalationAmber
	EscalationRed
)

func (e EscalationLevel) String() string {
	switch e {
	case EscalationAmber:
		return "amber"
	case EscalationRed:
		return "red"
	default:
		return "normal"
	}
}

type ReviewItem struct {
	PR         models.PullRequest
	Score      int
	Escalation EscalationLevel
	Age        time.Duration
}

// ScorePR computes priority score for a PR.
// Higher score = higher priority = needs attention sooner.
func ScorePR(pr models.PullRequest) int {
	score := 100

	// +2 per hour of age
	age := time.Since(pr.CreatedAt)
	score += int(age.Hours()) * 2

	// +20 if CI failing
	if pr.CIStatus == "failure" {
		score += 20
	}

	// +15 if agent PR
	if pr.IsAgent {
		score += 15
	}

	// -30 if approved
	if pr.ReviewState == "approved" {
		score -= 30
	}

	// -20 if draft
	if pr.Draft {
		score -= 20
	}

	return score
}

// Escalation determines escalation level based on PR age and thresholds.
func Escalation(pr models.PullRequest, amberHours, redHours int) EscalationLevel {
	age := time.Since(pr.CreatedAt)
	hours := age.Hours()
	if hours >= float64(redHours) {
		return EscalationRed
	}
	if hours >= float64(amberHours) {
		return EscalationAmber
	}
	return EscalationNormal
}

// ReviewItemsFromPulls converts PRs into prioritized ReviewItems.
func ReviewItemsFromPulls(pulls []models.PullRequest, amberHours, redHours int, dismissed map[string]bool) []ReviewItem {
	var items []ReviewItem
	for _, pr := range pulls {
		// Skip dismissed
		key := pr.Repo + ":" + fmt.Sprint(pr.Number)
		if dismissed[key] {
			continue
		}
		// Skip closed/merged
		if pr.State != "open" {
			continue
		}

		item := ReviewItem{
			PR:         pr,
			Score:      ScorePR(pr),
			Escalation: Escalation(pr, amberHours, redHours),
			Age:        time.Since(pr.CreatedAt),
		}
		items = append(items, item)
	}

	// Sort by score descending (highest priority first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	return items
}
