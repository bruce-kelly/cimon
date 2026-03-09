package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

// FetchPRStatuses concurrently fetches CI status for all PRs that lack one.
func (c *Client) FetchPRStatuses(ctx context.Context, pulls []models.PullRequest, repo string) []models.PullRequest {
	var wg sync.WaitGroup
	var mu sync.Mutex
	result := make([]models.PullRequest, len(pulls))
	copy(result, pulls)

	for i := range result {
		if result[i].HeadSHA == "" {
			continue
		}
		if result[i].CIStatus != "" && result[i].CIStatus != "unknown" {
			continue
		}
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
	wg.Wait()
	return result
}

// DetectAgent checks if a PR was created by an AI agent based on configurable patterns.
func DetectAgent(pr *models.PullRequest, patterns config.AgentPatternsConfig, body string) {
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
	created, _ := time.Parse(time.RFC3339, p.CreatedAt)
	updated, _ := time.Parse(time.RFC3339, p.UpdatedAt)
	return models.PullRequest{
		Number:    p.Number,
		Title:     p.Title,
		Author:    p.User.Login,
		Repo:      repo,
		HeadSHA:   p.Head.SHA,
		HTMLURL:   p.HTMLURL,
		State:     p.State,
		Draft:     p.Draft,
		CreatedAt: created,
		UpdatedAt: updated,
		Additions: p.Additions,
		Deletions: p.Deletions,
		CIStatus:  "unknown",
	}
}
