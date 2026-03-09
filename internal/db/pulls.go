package db

import (
	"fmt"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// UpsertPull inserts or replaces a pull request record.
func (d *Database) UpsertPull(pr models.PullRequest) error {
	draft := 0
	if pr.Draft {
		draft = 1
	}
	isAgent := 0
	if pr.IsAgent {
		isAgent = 1
	}

	_, err := d.writer.Exec(`
		INSERT OR REPLACE INTO pull_requests
			(repo, number, title, author, state, draft,
			 created_at, updated_at, ci_status, review_state, is_agent, html_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pr.Repo, pr.Number, pr.Title, pr.Author, pr.State, draft,
		pr.CreatedAt.UTC().Format(time.RFC3339),
		pr.UpdatedAt.UTC().Format(time.RFC3339),
		pr.CIStatus, pr.ReviewState, isAgent, pr.HTMLURL,
	)
	if err != nil {
		return fmt.Errorf("upserting pull %s#%d: %w", pr.Repo, pr.Number, err)
	}
	return nil
}

// QueryPulls returns open pull requests for a repo.
func (d *Database) QueryPulls(repo string) ([]models.PullRequest, error) {
	rows, err := d.reader.Query(`
		SELECT repo, number, title, author, state, draft,
		       created_at, updated_at, ci_status, review_state, is_agent, html_url
		FROM pull_requests
		WHERE repo = ? AND state = 'open'
		ORDER BY updated_at DESC`, repo)
	if err != nil {
		return nil, fmt.Errorf("querying pulls: %w", err)
	}
	defer rows.Close()

	var pulls []models.PullRequest
	for rows.Next() {
		var pr models.PullRequest
		var draft, isAgent int
		var createdStr, updatedStr string
		var title, author, state, ciStatus, reviewState, htmlURL *string

		if err := rows.Scan(
			&pr.Repo, &pr.Number, &title, &author, &state, &draft,
			&createdStr, &updatedStr, &ciStatus, &reviewState, &isAgent, &htmlURL,
		); err != nil {
			return nil, fmt.Errorf("scanning pull: %w", err)
		}

		pr.Draft = draft != 0
		pr.IsAgent = isAgent != 0
		if title != nil {
			pr.Title = *title
		}
		if author != nil {
			pr.Author = *author
		}
		if state != nil {
			pr.State = *state
		}
		if ciStatus != nil {
			pr.CIStatus = *ciStatus
		}
		if reviewState != nil {
			pr.ReviewState = *reviewState
		}
		if htmlURL != nil {
			pr.HTMLURL = *htmlURL
		}

		pr.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		pr.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		pulls = append(pulls, pr)
	}
	return pulls, rows.Err()
}

// PullsChangedSince returns the count of PRs updated since the given time.
func (d *Database) PullsChangedSince(repo string, since time.Time) (int, error) {
	var count int
	err := d.reader.QueryRow(`
		SELECT COUNT(*) FROM pull_requests
		WHERE repo = ? AND updated_at >= ?`,
		repo, since.UTC().Format(time.RFC3339),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting pulls since: %w", err)
	}
	return count, nil
}
