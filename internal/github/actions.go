package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// Approve submits an approval review on a PR.
func (c *Client) Approve(ctx context.Context, repo string, number int) error {
	path := fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number)
	body, err := json.Marshal(map[string]string{"event": "APPROVE"})
	if err != nil {
		return fmt.Errorf("marshaling approve body: %w", err)
	}
	_, err = c.post(ctx, path, bytes.NewReader(body))
	return err
}

// Merge merges a PR via squash.
func (c *Client) Merge(ctx context.Context, repo string, number int) error {
	path := fmt.Sprintf("/repos/%s/pulls/%d/merge", repo, number)
	body, err := json.Marshal(map[string]string{"merge_method": "squash"})
	if err != nil {
		return fmt.Errorf("marshaling merge body: %w", err)
	}
	_, err = c.put(ctx, path, bytes.NewReader(body))
	return err
}

// Rerun re-runs all jobs in a workflow run.
func (c *Client) Rerun(ctx context.Context, repo string, runID int64) error {
	path := fmt.Sprintf("/repos/%s/actions/runs/%d/rerun", repo, runID)
	_, err := c.post(ctx, path, nil)
	return err
}

// RerunFailed re-runs only failed jobs in a workflow run.
func (c *Client) RerunFailed(ctx context.Context, repo string, runID int64) error {
	path := fmt.Sprintf("/repos/%s/actions/runs/%d/rerun-failed-jobs", repo, runID)
	_, err := c.post(ctx, path, nil)
	return err
}

// Cancel cancels a workflow run.
func (c *Client) Cancel(ctx context.Context, repo string, runID int64) error {
	path := fmt.Sprintf("/repos/%s/actions/runs/%d/cancel", repo, runID)
	_, err := c.post(ctx, path, nil)
	return err
}

// CreateTag creates a git tag pointing at the given SHA.
func (c *Client) CreateTag(ctx context.Context, repo, tag, sha string) error {
	path := fmt.Sprintf("/repos/%s/git/refs", repo)
	body, err := json.Marshal(map[string]string{
		"ref": "refs/tags/" + tag,
		"sha": sha,
	})
	if err != nil {
		return fmt.Errorf("marshaling tag body: %w", err)
	}
	_, err = c.post(ctx, path, bytes.NewReader(body))
	return err
}

// DispatchWorkflow triggers a workflow_dispatch event.
func (c *Client) DispatchWorkflow(ctx context.Context, repo, workflowFile, ref string) error {
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches", repo, workflowFile)
	body, err := json.Marshal(map[string]string{"ref": ref})
	if err != nil {
		return fmt.Errorf("marshaling dispatch body: %w", err)
	}
	_, err = c.post(ctx, path, bytes.NewReader(body))
	return err
}

// CommentOnPR adds a comment to a PR (via the issues API).
func (c *Client) CommentOnPR(ctx context.Context, repo string, number int, comment string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	body, err := json.Marshal(map[string]string{"body": comment})
	if err != nil {
		return fmt.Errorf("marshaling comment body: %w", err)
	}
	_, err = c.post(ctx, path, bytes.NewReader(body))
	return err
}
