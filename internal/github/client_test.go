package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- ETag caching ----------

func TestETagCaching(t *testing.T) {
	callCount := 0
	body := `{"message":"hello"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == `"etag-123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-123"`)
		w.Header().Set("X-RateLimit-Remaining", "4999")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	// First request — fresh data
	data, fromCache, err := client.get(ctx, "/test")
	require.NoError(t, err)
	assert.False(t, fromCache)
	assert.Equal(t, body, string(data))
	assert.Equal(t, 1, callCount)

	// Second request — should get 304 and return cached data
	data, fromCache, err = client.get(ctx, "/test")
	require.NoError(t, err)
	assert.True(t, fromCache)
	assert.Equal(t, body, string(data))
	assert.Equal(t, 2, callCount) // request was made, but got 304
}

// ---------- Rate limit tracking ----------

func TestRateLimitTracking(t *testing.T) {
	resetTime := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "4500")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	_, _, err := client.get(ctx, "/rate-test")
	require.NoError(t, err)

	rl := client.GetRateLimit()
	assert.Equal(t, 4500, rl.Remaining)
	assert.Equal(t, 5000, rl.Limit)
	assert.Equal(t, resetTime.Unix(), rl.ResetAt.Unix())
}

// ---------- Error handling ----------

func TestErrorHandling404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	_, _, err := client.get(ctx, "/repos/owner/missing")
	require.Error(t, err)
	var nfErr *NotFoundError
	assert.True(t, errors.As(err, &nfErr))
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "/repos/owner/missing")
}

func TestErrorHandling403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"rate limit exceeded"}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	_, _, err := client.get(ctx, "/repos/owner/repo")
	require.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr))
	assert.Equal(t, 403, authErr.StatusCode)
}

func TestClient_Get401ReturnsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	}))
	defer srv.Close()

	c := NewTestClient("bad-token", srv.URL)
	_, _, err := c.get(context.Background(), "/repos/owner/repo")
	assert.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr))
	assert.Equal(t, 401, authErr.StatusCode)
}

func TestClient_Get429ReturnsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"message":"rate limit exceeded"}`)
	}))
	defer srv.Close()

	c := NewTestClient("token", srv.URL)
	_, _, err := c.get(context.Background(), "/repos/owner/repo")
	assert.Error(t, err)

	var rlErr *RateLimitError
	assert.True(t, errors.As(err, &rlErr))
	assert.Equal(t, 120*time.Second, rlErr.RetryAfter)
}

func TestClient_Get429DefaultsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer srv.Close()

	c := NewTestClient("token", srv.URL)
	_, _, err := c.get(context.Background(), "/repos/owner/repo")

	var rlErr *RateLimitError
	assert.True(t, errors.As(err, &rlErr))
	assert.Equal(t, 60*time.Second, rlErr.RetryAfter)
}

// ---------- ListRuns parsing ----------

func TestListRuns(t *testing.T) {
	conclusion := "success"
	runsJSON := map[string]any{
		"workflow_runs": []map[string]any{
			{
				"id":          12345,
				"name":        "CI",
				"path":        ".github/workflows/ci.yml",
				"head_branch": "main",
				"head_sha":    "abc123",
				"status":      "completed",
				"conclusion":  conclusion,
				"event":       "push",
				"actor":       map[string]any{"login": "octocat"},
				"created_at":  "2026-03-09T10:00:00Z",
				"updated_at":  "2026-03-09T10:05:00Z",
				"html_url":    "https://github.com/owner/repo/actions/runs/12345",
			},
			{
				"id":          12346,
				"name":        "CI",
				"path":        ".github/workflows/ci.yml",
				"head_branch": "main",
				"head_sha":    "def456",
				"status":      "in_progress",
				"conclusion":  nil,
				"event":       "pull_request",
				"actor":       map[string]any{"login": "dev"},
				"created_at":  "2026-03-09T11:00:00Z",
				"updated_at":  "2026-03-09T11:01:00Z",
				"html_url":    "https://github.com/owner/repo/actions/runs/12346",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repos/owner/repo/actions/workflows/ci.yml/runs")
		assert.Equal(t, "main", r.URL.Query().Get("branch"))
		json.NewEncoder(w).Encode(runsJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	runs, err := client.ListRuns(ctx, "owner/repo", "ci.yml", "main")
	require.NoError(t, err)
	require.Len(t, runs, 2)

	// First run — completed
	assert.Equal(t, int64(12345), runs[0].ID)
	assert.Equal(t, "CI", runs[0].Name)
	assert.Equal(t, "ci.yml", runs[0].WorkflowFile)
	assert.Equal(t, "main", runs[0].HeadBranch)
	assert.Equal(t, "abc123", runs[0].HeadSHA)
	assert.Equal(t, "completed", runs[0].Status)
	assert.Equal(t, "success", runs[0].Conclusion)
	assert.Equal(t, "push", runs[0].Event)
	assert.Equal(t, "octocat", runs[0].Actor)
	assert.Equal(t, "owner/repo", runs[0].Repo)
	assert.Equal(t, 2026, runs[0].CreatedAt.Year())

	// Second run — in_progress, nil conclusion
	assert.Equal(t, int64(12346), runs[1].ID)
	assert.Equal(t, "in_progress", runs[1].Status)
	assert.Equal(t, "", runs[1].Conclusion)
	assert.Equal(t, "pull_request", runs[1].Event)
}

func TestListRunsUnscoped(t *testing.T) {
	runsJSON := map[string]any{
		"workflow_runs": []map[string]any{
			{
				"id":          99,
				"name":        "Deploy",
				"path":        ".github/workflows/deploy.yml",
				"head_branch": "main",
				"head_sha":    "aaa",
				"status":      "completed",
				"conclusion":  "success",
				"event":       "workflow_dispatch",
				"actor":       map[string]any{"login": "admin"},
				"created_at":  "2026-03-09T08:00:00Z",
				"updated_at":  "2026-03-09T08:10:00Z",
				"html_url":    "https://github.com/owner/repo/actions/runs/99",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should NOT have a branch parameter
		assert.Empty(t, r.URL.Query().Get("branch"))
		json.NewEncoder(w).Encode(runsJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	runs, err := client.ListRunsUnscoped(ctx, "owner/repo", "deploy.yml")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "deploy.yml", runs[0].WorkflowFile)
}

// ---------- GetJobs parsing ----------

func TestGetJobs(t *testing.T) {
	jobsJSON := map[string]any{
		"jobs": []map[string]any{
			{
				"id":           5001,
				"name":         "test",
				"status":       "completed",
				"conclusion":   "failure",
				"started_at":   "2026-03-09T10:00:00Z",
				"completed_at": "2026-03-09T10:03:00Z",
				"runner_name":  "ubuntu-latest",
				"steps": []map[string]any{
					{
						"name":       "Checkout",
						"status":     "completed",
						"conclusion": "success",
						"number":     1,
					},
					{
						"name":       "Run tests",
						"status":     "completed",
						"conclusion": "failure",
						"number":     2,
					},
				},
			},
			{
				"id":          5002,
				"name":        "lint",
				"status":      "queued",
				"conclusion":  nil,
				"started_at":  nil,
				"runner_name": "",
				"steps":       []map[string]any{},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repos/owner/repo/actions/runs/123/jobs")
		json.NewEncoder(w).Encode(jobsJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	jobs, err := client.GetJobs(ctx, "owner/repo", 123)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	// First job
	assert.Equal(t, int64(5001), jobs[0].ID)
	assert.Equal(t, "test", jobs[0].Name)
	assert.Equal(t, "completed", jobs[0].Status)
	assert.Equal(t, "failure", jobs[0].Conclusion)
	assert.Equal(t, "ubuntu-latest", jobs[0].RunnerName)
	require.NotNil(t, jobs[0].StartedAt)
	assert.Equal(t, 2026, jobs[0].StartedAt.Year())
	require.NotNil(t, jobs[0].CompletedAt)

	// Steps
	require.Len(t, jobs[0].Steps, 2)
	assert.Equal(t, "Checkout", jobs[0].Steps[0].Name)
	assert.Equal(t, "success", jobs[0].Steps[0].Conclusion)
	assert.Equal(t, 1, jobs[0].Steps[0].Number)
	assert.Equal(t, "Run tests", jobs[0].Steps[1].Name)
	assert.Equal(t, "failure", jobs[0].Steps[1].Conclusion)

	// Second job — queued, nil conclusion
	assert.Equal(t, int64(5002), jobs[1].ID)
	assert.Equal(t, "queued", jobs[1].Status)
	assert.Equal(t, "", jobs[1].Conclusion)
	assert.Nil(t, jobs[1].StartedAt)
	assert.Empty(t, jobs[1].Steps)
}

// ---------- DetectAgent ----------

func TestDetectAgent_BotAuthor(t *testing.T) {
	pr := models.PullRequest{Author: "github-actions[bot]"}
	patterns := config.AgentPatternsConfig{
		BotAuthors: []string{"github-actions[bot]"},
	}
	DetectAgent(&pr, patterns, "some body text")
	assert.True(t, pr.IsAgent)
	assert.Equal(t, "bot:github-actions[bot]", pr.AgentSource)
}

func TestDetectAgent_BotAuthor_CaseInsensitive(t *testing.T) {
	pr := models.PullRequest{Author: "GitHub-Actions[bot]"}
	patterns := config.AgentPatternsConfig{
		BotAuthors: []string{"github-actions[bot]"},
	}
	DetectAgent(&pr, patterns, "")
	assert.True(t, pr.IsAgent)
}

func TestDetectAgent_PRBody(t *testing.T) {
	pr := models.PullRequest{Author: "human"}
	patterns := config.AgentPatternsConfig{
		PRBody:     "Generated with Claude Code",
		BotAuthors: []string{},
	}
	DetectAgent(&pr, patterns, "This PR was Generated with Claude Code for fun")
	assert.True(t, pr.IsAgent)
	assert.Equal(t, "body", pr.AgentSource)
}

func TestDetectAgent_CommitTrailer(t *testing.T) {
	pr := models.PullRequest{Author: "human"}
	patterns := config.AgentPatternsConfig{
		PRBody:        "not-in-body",
		CommitTrailer: "Co-Authored-By: Claude",
		BotAuthors:    []string{},
	}
	DetectAgent(&pr, patterns, "Some changes\n\nCo-Authored-By: Claude")
	assert.True(t, pr.IsAgent)
	assert.Equal(t, "trailer", pr.AgentSource)
}

func TestDetectAgent_NoMatch(t *testing.T) {
	pr := models.PullRequest{Author: "human"}
	patterns := config.AgentPatternsConfig{
		PRBody:        "Generated with Claude Code",
		CommitTrailer: "Co-Authored-By: Claude",
		BotAuthors:    []string{"dependabot[bot]"},
	}
	DetectAgent(&pr, patterns, "A normal PR body with no markers")
	assert.False(t, pr.IsAgent)
	assert.Equal(t, "", pr.AgentSource)
}

func TestDetectAgent_PriorityOrder(t *testing.T) {
	// Bot author takes precedence over body match
	pr := models.PullRequest{Author: "bot-user"}
	patterns := config.AgentPatternsConfig{
		PRBody:        "Generated with Claude Code",
		CommitTrailer: "Co-Authored-By: Claude",
		BotAuthors:    []string{"bot-user"},
	}
	DetectAgent(&pr, patterns, "Generated with Claude Code")
	assert.True(t, pr.IsAgent)
	assert.Equal(t, "bot:bot-user", pr.AgentSource)
}

// ---------- SearchPulls ----------

func TestSearchPulls(t *testing.T) {
	searchJSON := map[string]any{
		"items": []map[string]any{
			{
				"number":         42,
				"title":          "Fix the thing",
				"user":           map[string]any{"login": "dev"},
				"html_url":       "https://github.com/owner/repo/pull/42",
				"state":          "open",
				"created_at":     "2026-03-08T10:00:00Z",
				"updated_at":     "2026-03-09T10:00:00Z",
				"body":           "Fixes #100",
				"pull_request":   map[string]any{}, // non-nil → is a PR
				"repository_url": "https://api.github.com/repos/owner/repo",
			},
			{
				// This is an issue, not a PR
				"number":         99,
				"title":          "Bug report",
				"user":           map[string]any{"login": "user"},
				"html_url":       "https://github.com/owner/repo/issues/99",
				"state":          "open",
				"created_at":     "2026-03-01T10:00:00Z",
				"updated_at":     "2026-03-02T10:00:00Z",
				"body":           "Something is broken",
				"pull_request":   nil,
				"repository_url": "https://api.github.com/repos/owner/repo",
			},
			{
				"number":         7,
				"title":          "Another PR",
				"user":           map[string]any{"login": "contributor"},
				"html_url":       "https://github.com/other/lib/pull/7",
				"state":          "open",
				"created_at":     "2026-03-05T10:00:00Z",
				"updated_at":     "2026-03-06T10:00:00Z",
				"body":           "",
				"pull_request":   map[string]any{},
				"repository_url": "https://api.github.com/repos/other/lib",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/search/issues")
		assert.NotEmpty(t, r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(searchJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	pulls, err := client.SearchPulls(ctx, "is:open is:pr review-requested:@me")
	require.NoError(t, err)
	// Issue #99 should be filtered out
	require.Len(t, pulls, 2)

	assert.Equal(t, 42, pulls[0].Number)
	assert.Equal(t, "Fix the thing", pulls[0].Title)
	assert.Equal(t, "dev", pulls[0].Author)
	assert.Equal(t, "owner/repo", pulls[0].Repo)
	assert.Equal(t, "open", pulls[0].State)
	assert.Equal(t, "unknown", pulls[0].CIStatus)

	assert.Equal(t, 7, pulls[1].Number)
	assert.Equal(t, "other/lib", pulls[1].Repo)
}

// ---------- DiscoverWorkflows ----------

func TestDiscoverWorkflows(t *testing.T) {
	wfJSON := map[string]any{
		"workflows": []map[string]any{
			{"path": ".github/workflows/ci.yml", "state": "active"},
			{"path": ".github/workflows/release.yml", "state": "active"},
			{"path": ".github/workflows/old.yml", "state": "disabled_manually"},
			{"path": ".github/workflows/stale.yml", "state": "deleted"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repos/owner/repo/actions/workflows")
		json.NewEncoder(w).Encode(wfJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	files, err := client.DiscoverWorkflows(ctx, "owner/repo")
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, "ci.yml", files[0])
	assert.Equal(t, "release.yml", files[1])
}

func TestDiscoverWorkflows_NoActive(t *testing.T) {
	wfJSON := map[string]any{
		"workflows": []map[string]any{
			{"path": ".github/workflows/old.yml", "state": "disabled_manually"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(wfJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	files, err := client.DiscoverWorkflows(ctx, "owner/repo")
	require.NoError(t, err)
	assert.Empty(t, files)
}

// ---------- Actions ----------

func TestApprove(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
		body   map[string]string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.Approve(ctx, "owner/repo", 42)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/pulls/42/reviews", recorded.path)
	assert.Equal(t, "APPROVE", recorded.body["event"])
}

func TestMerge(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
		body   map[string]string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.Merge(ctx, "owner/repo", 42)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "PUT", recorded.method)
	assert.Equal(t, "/repos/owner/repo/pulls/42/merge", recorded.path)
	assert.Equal(t, "squash", recorded.body["merge_method"])
}

func TestRerun(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.Rerun(ctx, "owner/repo", 555)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/actions/runs/555/rerun", recorded.path)
}

func TestRerunFailed(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.RerunFailed(ctx, "owner/repo", 555)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/actions/runs/555/rerun-failed-jobs", recorded.path)
}

func TestCancel(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.Cancel(ctx, "owner/repo", 555)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/actions/runs/555/cancel", recorded.path)
}

func TestCreateTag(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
		body   map[string]string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.CreateTag(ctx, "owner/repo", "v1.0.0", "abc123")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/git/refs", recorded.path)
	assert.Equal(t, "refs/tags/v1.0.0", recorded.body["ref"])
	assert.Equal(t, "abc123", recorded.body["sha"])
}

func TestDispatchWorkflow(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
		body   map[string]string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.DispatchWorkflow(ctx, "owner/repo", "claude.yml", "main")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/actions/workflows/claude.yml/dispatches", recorded.path)
	assert.Equal(t, "main", recorded.body["ref"])
}

func TestCommentOnPR(t *testing.T) {
	var mu sync.Mutex
	var recorded struct {
		method string
		path   string
		body   map[string]string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		recorded.method = r.Method
		recorded.path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	err := client.CommentOnPR(ctx, "owner/repo", 42, "LGTM!")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "POST", recorded.method)
	assert.Equal(t, "/repos/owner/repo/issues/42/comments", recorded.path)
	assert.Equal(t, "LGTM!", recorded.body["body"])
}

// ---------- ListPulls ----------

func TestListPulls(t *testing.T) {
	pullsJSON := []map[string]any{
		{
			"number":     10,
			"title":      "Add feature",
			"user":       map[string]any{"login": "dev"},
			"html_url":   "https://github.com/owner/repo/pull/10",
			"state":      "open",
			"draft":      false,
			"created_at": "2026-03-08T10:00:00Z",
			"updated_at": "2026-03-09T10:00:00Z",
			"body":       "Implements feature X",
			"head":       map[string]any{"sha": "sha-aaa"},
			"additions":  50,
			"deletions":  10,
		},
		{
			"number":     11,
			"title":      "WIP: Draft PR",
			"user":       map[string]any{"login": "contributor"},
			"html_url":   "https://github.com/owner/repo/pull/11",
			"state":      "open",
			"draft":      true,
			"created_at": "2026-03-09T08:00:00Z",
			"updated_at": "2026-03-09T09:00:00Z",
			"body":       "",
			"head":       map[string]any{"sha": "sha-bbb"},
			"additions":  5,
			"deletions":  0,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repos/owner/repo/pulls")
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		json.NewEncoder(w).Encode(pullsJSON)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	pulls, err := client.ListPulls(ctx, "owner/repo")
	require.NoError(t, err)
	require.Len(t, pulls, 2)

	assert.Equal(t, 10, pulls[0].Number)
	assert.Equal(t, "Add feature", pulls[0].Title)
	assert.Equal(t, "dev", pulls[0].Author)
	assert.Equal(t, "owner/repo", pulls[0].Repo)
	assert.Equal(t, "sha-aaa", pulls[0].HeadSHA)
	assert.False(t, pulls[0].Draft)
	assert.Equal(t, 50, pulls[0].Additions)
	assert.Equal(t, 10, pulls[0].Deletions)
	assert.Equal(t, "unknown", pulls[0].CIStatus)

	assert.Equal(t, 11, pulls[1].Number)
	assert.True(t, pulls[1].Draft)
	assert.Equal(t, "sha-bbb", pulls[1].HeadSHA)
}

// ---------- GetPullDiff ----------

func TestGetPullDiff(t *testing.T) {
	diffText := `diff --git a/file.go b/file.go
index abc..def 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github.diff", r.Header.Get("Accept"))
		assert.Contains(t, r.URL.Path, "/repos/owner/repo/pulls/10")
		fmt.Fprint(w, diffText)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	diff, err := client.GetPullDiff(ctx, "owner/repo", 10)
	require.NoError(t, err)
	assert.Contains(t, diff, "diff --git")
	assert.Contains(t, diff, "+import")
}

// ---------- GetCombinedStatus ----------

func TestGetCombinedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repos/owner/repo/commits/abc123/status")
		fmt.Fprint(w, `{"state":"success"}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	status, err := client.GetCombinedStatus(ctx, "owner/repo", "abc123")
	require.NoError(t, err)
	assert.Equal(t, "success", status)
}

func TestGetCombinedStatus_Error_ReturnsUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	status, err := client.GetCombinedStatus(ctx, "owner/repo", "bad-sha")
	require.NoError(t, err) // non-fatal
	assert.Equal(t, "unknown", status)
}

// ---------- Authorization header ----------

func TestAuthorizationHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-secret-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewTestClient("my-secret-token", srv.URL)
	ctx := context.Background()

	_, _, err := client.get(ctx, "/test")
	require.NoError(t, err)
}

// ---------- Mutate error handling ----------

func TestMutateErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Validation failed"}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	_, err := client.post(ctx, "/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "POST")
	assert.Contains(t, err.Error(), "422")
}

// ---------- FetchPRStatuses ----------

func TestFetchPRStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond to combined status requests
		fmt.Fprint(w, `{"state":"success"}`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	pulls := []models.PullRequest{
		{Number: 1, HeadSHA: "sha1", CIStatus: "unknown"},
		{Number: 2, HeadSHA: "sha2", CIStatus: "success"}, // already has status
		{Number: 3, HeadSHA: "", CIStatus: "unknown"},      // no SHA
	}

	result := client.FetchPRStatuses(ctx, pulls, "owner/repo")
	require.Len(t, result, 3)
	assert.Equal(t, "success", result[0].CIStatus)        // fetched
	assert.Equal(t, "success", result[1].CIStatus)        // unchanged
	assert.Equal(t, "unknown", result[2].CIStatus)        // skipped (no SHA)
}

// ---------- Workflow path extraction edge cases ----------

func TestWorkflowPathExtraction_ShortPath(t *testing.T) {
	// Path shorter than ".github/workflows/" prefix
	r := ghRun{
		Path: "short",
	}
	model := r.toModel("owner/repo")
	assert.Equal(t, "short", model.WorkflowFile)
}

func TestWorkflowPathExtraction_ExactPrefix(t *testing.T) {
	// Path that is exactly the prefix — should not strip
	r := ghRun{
		Path: ".github/workflows/",
	}
	model := r.toModel("owner/repo")
	assert.Equal(t, ".github/workflows/", model.WorkflowFile)
}

// ---------- getJSON unmarshal error ----------

func TestGetJSON_UnmarshalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()

	client := NewTestClient("test-token", srv.URL)
	ctx := context.Background()

	var result struct{ Message string }
	_, err := client.getJSON(ctx, "/bad", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing /bad")
}
