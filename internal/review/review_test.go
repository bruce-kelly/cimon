package review

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
)

func freshPR() models.PullRequest {
	return models.PullRequest{
		Number:    1,
		Title:     "Test PR",
		Author:    "user",
		Repo:      "owner/repo",
		State:     "open",
		CreatedAt: time.Now(),
	}
}

func TestScorePR_BaseScore(t *testing.T) {
	pr := freshPR()
	score := ScorePR(pr)
	assert.Equal(t, 100, score)
}

func TestScorePR_CIFailure(t *testing.T) {
	pr := freshPR()
	pr.CIStatus = "failure"
	score := ScorePR(pr)
	assert.Equal(t, 120, score)
}

func TestScorePR_Agent(t *testing.T) {
	pr := freshPR()
	pr.IsAgent = true
	score := ScorePR(pr)
	assert.Equal(t, 115, score)
}

func TestScorePR_Approved(t *testing.T) {
	pr := freshPR()
	pr.ReviewState = "approved"
	score := ScorePR(pr)
	assert.Equal(t, 70, score)
}

func TestScorePR_Draft(t *testing.T) {
	pr := freshPR()
	pr.Draft = true
	score := ScorePR(pr)
	assert.Equal(t, 80, score)
}

func TestScorePR_Combined(t *testing.T) {
	// agent + CI failure + approved = 100 + 15 + 20 - 30 = 105
	pr := freshPR()
	pr.IsAgent = true
	pr.CIStatus = "failure"
	pr.ReviewState = "approved"
	score := ScorePR(pr)
	assert.Equal(t, 105, score)
}

func TestScorePR_Age(t *testing.T) {
	pr := freshPR()
	pr.CreatedAt = time.Now().Add(-24 * time.Hour)
	score := ScorePR(pr)
	// 100 base + 24*2 = 148
	assert.Equal(t, 148, score)
}

func TestEscalation_Fresh(t *testing.T) {
	pr := freshPR()
	level := Escalation(pr, 24, 48)
	assert.Equal(t, EscalationNormal, level)
}

func TestEscalation_Amber(t *testing.T) {
	pr := freshPR()
	pr.CreatedAt = time.Now().Add(-25 * time.Hour)
	level := Escalation(pr, 24, 48)
	assert.Equal(t, EscalationAmber, level)
}

func TestEscalation_Red(t *testing.T) {
	pr := freshPR()
	pr.CreatedAt = time.Now().Add(-49 * time.Hour)
	level := Escalation(pr, 24, 48)
	assert.Equal(t, EscalationRed, level)
}

func TestEscalationLevel_String(t *testing.T) {
	assert.Equal(t, "normal", EscalationNormal.String())
	assert.Equal(t, "amber", EscalationAmber.String())
	assert.Equal(t, "red", EscalationRed.String())
}

func TestReviewItemsFromPulls_FiltersDismissed(t *testing.T) {
	pr := freshPR()
	pr.Number = 42
	pr.Repo = "owner/repo"
	dismissed := map[string]bool{"owner/repo:42": true}

	items := ReviewItemsFromPulls([]models.PullRequest{pr}, 24, 48, dismissed)
	assert.Empty(t, items)
}

func TestReviewItemsFromPulls_FiltersNonOpen(t *testing.T) {
	pr := freshPR()
	pr.State = "closed"

	items := ReviewItemsFromPulls([]models.PullRequest{pr}, 24, 48, nil)
	assert.Empty(t, items)
}

func TestReviewItemsFromPulls_SortsByScoreDescending(t *testing.T) {
	low := freshPR()
	low.Number = 1
	low.Draft = true // score 80

	high := freshPR()
	high.Number = 2
	high.CIStatus = "failure"
	high.IsAgent = true // score 135

	mid := freshPR()
	mid.Number = 3 // score 100

	items := ReviewItemsFromPulls(
		[]models.PullRequest{low, high, mid},
		24, 48, nil,
	)

	assert.Len(t, items, 3)
	assert.Equal(t, 2, items[0].PR.Number) // highest score first
	assert.Equal(t, 3, items[1].PR.Number)
	assert.Equal(t, 1, items[2].PR.Number)
}

func TestReviewItemsFromPulls_Empty(t *testing.T) {
	items := ReviewItemsFromPulls(nil, 24, 48, nil)
	assert.Empty(t, items)
}

func TestQueue_UpdateAndItems(t *testing.T) {
	q := NewQueue()
	assert.Empty(t, q.Items())

	input := []ReviewItem{
		{PR: freshPR(), Score: 100},
		{PR: freshPR(), Score: 200},
	}
	q.Update(input)

	got := q.Items()
	assert.Len(t, got, 2)
	assert.Equal(t, 100, got[0].Score)
	assert.Equal(t, 200, got[1].Score)
}

func TestQueue_Len(t *testing.T) {
	q := NewQueue()
	assert.Equal(t, 0, q.Len())

	q.Update([]ReviewItem{{Score: 1}, {Score: 2}, {Score: 3}})
	assert.Equal(t, 3, q.Len())
}
