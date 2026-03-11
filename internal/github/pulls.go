package github

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/models"
)

// ListPulls fetches open PRs for a repo.
func (c *Client) ListPulls(ctx context.Context, repo string) ([]models.PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/pulls?state=open&per_page=100", repo)
	var ghPRs []ghPull
	if _, err := c.getJSON(ctx, path, &ghPRs); err != nil {
		return nil, err
	}
	pulls := make([]models.PullRequest, 0, len(ghPRs))
	for _, p := range ghPRs {
		pulls = append(pulls, p.toModel(repo))
	}
	return pulls, nil
}

// GetPullDiff fetches the raw diff for a PR.
func (c *Client) GetPullDiff(ctx context.Context, repo string, number int) (string, error) {
	url := c.baseURL + fmt.Sprintf("/repos/%s/pulls/%d", repo, number)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.diff")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	c.trackRateLimit(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GitHub API %d fetching diff for %s#%d", resp.StatusCode, repo, number)
	}
	return string(body), nil
}

// GetCombinedStatus fetches CI status for a commit SHA.
func (c *Client) GetCombinedStatus(ctx context.Context, repo, sha string) (string, error) {
	path := fmt.Sprintf("/repos/%s/commits/%s/status", repo, sha)
	var resp struct {
		State string `json:"state"`
	}
	if _, err := c.getJSON(ctx, path, &resp); err != nil {
		return "unknown", nil // non-fatal
	}
	return resp.State, nil
}

// GetReviewState fetches reviews for a PR and reduces them to a single
// current state for display and merge gating.
func (c *Client) GetReviewState(ctx context.Context, repo string, number int) (string, error) {
	path := fmt.Sprintf("/repos/%s/pulls/%d/reviews?per_page=100", repo, number)
	var reviews []struct {
		State       string `json:"state"`
		SubmittedAt string `json:"submitted_at"`
	}
	if _, err := c.getJSON(ctx, path, &reviews); err != nil {
		return "none", nil // non-fatal
	}

	type decision struct {
		state string
		at    time.Time
	}
	var decisions []decision
	for _, review := range reviews {
		var state string
		switch strings.ToUpper(review.State) {
		case "APPROVED":
			state = "approved"
		case "CHANGES_REQUESTED":
			state = "changes_requested"
		default:
			continue
		}

		submittedAt, err := time.Parse(time.RFC3339, review.SubmittedAt)
		if err != nil {
			submittedAt = time.Time{}
		}
		decisions = append(decisions, decision{state: state, at: submittedAt})
	}

	if len(decisions) == 0 {
		return "none", nil
	}

	sort.SliceStable(decisions, func(i, j int) bool {
		return decisions[i].at.Before(decisions[j].at)
	})
	return decisions[len(decisions)-1].state, nil
}

// FetchPRStatuses concurrently fetches CI status and review state for PRs.
func (c *Client) FetchPRStatuses(ctx context.Context, pulls []models.PullRequest, repo string) []models.PullRequest {
	var wg sync.WaitGroup
	var mu sync.Mutex
	result := make([]models.PullRequest, len(pulls))
	copy(result, pulls)

	for i := range result {
		needsStatus := result[i].HeadSHA != "" && (result[i].CIStatus == "" || result[i].CIStatus == "unknown")
		needsReview := result[i].ReviewState == "" || result[i].ReviewState == "pending" || result[i].ReviewState == "none"
		if !needsStatus && !needsReview {
			continue
		}

		if needsStatus {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				status, err := c.GetCombinedStatus(ctx, repo, result[idx].HeadSHA)
				if err != nil {
					return
				}
				mu.Lock()
				result[idx].CIStatus = status
				mu.Unlock()
			}(i)
		}

		if needsReview {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				reviewState, err := c.GetReviewState(ctx, repo, result[idx].Number)
				if err != nil {
					return
				}
				mu.Lock()
				result[idx].ReviewState = reviewState
				mu.Unlock()
			}(i)
		}
	}
	wg.Wait()
	return result
}

// EnrichPulls applies agent detection, CI status, and review state to PRs.
func (c *Client) EnrichPulls(ctx context.Context, repo string, pulls []models.PullRequest, patterns config.AgentPatternsConfig) []models.PullRequest {
	for i := range pulls {
		DetectAgent(&pulls[i], patterns, pulls[i].Body)
	}
	return c.FetchPRStatuses(ctx, pulls, repo)
}

// DetectAgent checks if a PR was created by an AI agent based on configurable patterns.
func DetectAgent(pr *models.PullRequest, patterns config.AgentPatternsConfig, body string) {
	if body == "" {
		body = pr.Body
	}
	// Check bot authors
	for _, bot := range patterns.BotAuthors {
		if strings.EqualFold(pr.Author, bot) {
			pr.IsAgent = true
			pr.AgentSource = "bot:" + bot
			return
		}
	}
	// Check PR body text
	if patterns.PRBody != "" && strings.Contains(body, patterns.PRBody) {
		pr.IsAgent = true
		pr.AgentSource = "body"
		return
	}
	// Check commit trailer — uses body content as proxy
	if patterns.CommitTrailer != "" && strings.Contains(body, patterns.CommitTrailer) {
		pr.IsAgent = true
		pr.AgentSource = "trailer"
		return
	}
}

// ghPull is the GitHub API representation of a pull request.
type ghPull struct {
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	User      ghActor `json:"user"`
	HTMLURL   string  `json:"html_url"`
	State     string  `json:"state"`
	Draft     bool    `json:"draft"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Body      string  `json:"body"`
	Head      struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

func (p ghPull) toModel(repo string) models.PullRequest {
	created, err := time.Parse(time.RFC3339, p.CreatedAt)
	if err != nil {
		slog.Warn("failed to parse PR created_at", "value", p.CreatedAt, "err", err)
		created = time.Now()
	}
	updated, err := time.Parse(time.RFC3339, p.UpdatedAt)
	if err != nil {
		slog.Warn("failed to parse PR updated_at", "value", p.UpdatedAt, "err", err)
		updated = time.Now()
	}
	return models.PullRequest{
		Number:      p.Number,
		Title:       p.Title,
		Body:        p.Body,
		Author:      p.User.Login,
		Repo:        repo,
		HeadSHA:     p.Head.SHA,
		HTMLURL:     p.HTMLURL,
		State:       p.State,
		Draft:       p.Draft,
		CreatedAt:   created,
		UpdatedAt:   updated,
		Additions:   p.Additions,
		Deletions:   p.Deletions,
		CIStatus:    "unknown",
		ReviewState: "none",
	}
}
