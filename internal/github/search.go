package github

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// SearchPulls searches for PRs using GitHub search API.
func (c *Client) SearchPulls(ctx context.Context, query string) ([]models.PullRequest, error) {
	path := fmt.Sprintf("/search/issues?q=%s&per_page=100", url.QueryEscape(query))
	var resp struct {
		Items []ghSearchItem `json:"items"`
	}
	if _, err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	pulls := make([]models.PullRequest, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.PullRequest == nil {
			continue // skip non-PR issues
		}
		pulls = append(pulls, item.toModel())
	}
	return pulls, nil
}

// DiscoverWorkflows fetches active workflow filenames for a repo.
func (c *Client) DiscoverWorkflows(ctx context.Context, repo string) ([]string, error) {
	path := fmt.Sprintf("/repos/%s/actions/workflows?per_page=100", repo)
	var resp struct {
		Workflows []struct {
			Path  string `json:"path"`
			State string `json:"state"`
		} `json:"workflows"`
	}
	if _, err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	var files []string
	for _, wf := range resp.Workflows {
		if wf.State != "active" {
			continue
		}
		// Extract filename from path like ".github/workflows/ci.yml"
		name := wf.Path
		const prefix = ".github/workflows/"
		if len(wf.Path) > len(prefix) && wf.Path[:len(prefix)] == prefix {
			name = wf.Path[len(prefix):]
		}
		files = append(files, name)
	}
	return files, nil
}

type ghSearchItem struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	User          ghActor   `json:"user"`
	HTMLURL       string    `json:"html_url"`
	State         string    `json:"state"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
	Body          string    `json:"body"`
	PullRequest   *struct{} `json:"pull_request"` // non-nil means it's a PR
	RepositoryURL string    `json:"repository_url"`
}

func (item ghSearchItem) toModel() models.PullRequest {
	created, _ := time.Parse(time.RFC3339, item.CreatedAt)
	updated, _ := time.Parse(time.RFC3339, item.UpdatedAt)
	// Extract repo from repository_url: https://api.github.com/repos/owner/repo
	repo := ""
	const prefix = "https://api.github.com/repos/"
	if len(item.RepositoryURL) > len(prefix) {
		repo = item.RepositoryURL[len(prefix):]
	}
	return models.PullRequest{
		Number:    item.Number,
		Title:     item.Title,
		Author:    item.User.Login,
		Repo:      repo,
		HTMLURL:   item.HTMLURL,
		State:     item.State,
		CreatedAt: created,
		UpdatedAt: updated,
		CIStatus:  "unknown",
	}
}
