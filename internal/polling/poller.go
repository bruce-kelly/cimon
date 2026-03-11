package polling

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
)

// Poller runs a background loop that fetches workflow runs and PRs for all
// configured repos, sending results on a channel. Poll frequency adapts
// based on whether any runs are currently active.
type Poller struct {
	client   *github.Client
	config   *config.CimonConfig
	state    *PollState
	resultCh chan<- models.PollResult
	cancel   context.CancelFunc
	notFound map[string]bool // "repo/workflow" keys that returned 404
}

// New creates a Poller wired to the given client and config. Results are
// sent on resultCh after each per-repo fetch.
func New(client *github.Client, cfg *config.CimonConfig, resultCh chan<- models.PollResult) *Poller {
	return &Poller{
		client:   client,
		config:   cfg,
		state:    NewPollState(cfg.Polling.Idle, cfg.Polling.Active, cfg.Polling.Cooldown),
		resultCh: resultCh,
		notFound: make(map[string]bool),
	}
}

// State returns the adaptive cadence state.
func (p *Poller) State() *PollState { return p.state }

// Start launches the background poll loop. Cancel the parent context or
// call Stop to shut it down.
func (p *Poller) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)
	go p.loop(ctx)
}

// Stop cancels the background poll loop.
func (p *Poller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *Poller) loop(ctx context.Context) {
	p.pollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.state.Interval()):
			p.pollOnce(ctx)
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	hasActive := false
	for _, repo := range p.config.Repos {
		result := p.pollRepo(ctx, &repo)
		for _, run := range result.Runs {
			if run.IsActive() {
				hasActive = true
			}
		}
		select {
		case p.resultCh <- result:
		case <-ctx.Done():
			return
		}
	}
	p.state.Update(hasActive)
}

func (p *Poller) pollRepo(ctx context.Context, repo *config.RepoConfig) models.PollResult {
	result := models.PollResult{Repo: repo.Repo}

	seen := make(map[string]bool) // dedupe workflows across groups
	for name, group := range repo.Groups {
		for _, wf := range group.Workflows {
			if seen[wf] {
				continue
			}
			seen[wf] = true
			nfKey := repo.Repo + "/" + wf
			if p.notFound[nfKey] {
				continue
			}
			var runs []models.WorkflowRun
			var err error
			if name == "ci" {
				runs, err = p.client.ListRuns(ctx, repo.Repo, wf, repo.Branch)
			} else {
				// Release/agent workflows aren't branch-scoped (tags, dispatch, schedule)
				runs, err = p.client.ListRunsUnscoped(ctx, repo.Repo, wf)
			}
			if err != nil {
				var nfErr *github.NotFoundError
				if errors.As(err, &nfErr) {
					slog.Warn("workflow not found, skipping future polls", "repo", repo.Repo, "workflow", wf)
					p.notFound[nfKey] = true
					continue
				}
				slog.Error("list runs failed", "repo", repo.Repo, "workflow", wf, "err", err)
				result.Error = err
				continue
			}
			result.Runs = append(result.Runs, runs...)
		}
	}

	pulls, err := p.client.ListPulls(ctx, repo.Repo)
	if err != nil {
		slog.Error("list pulls failed", "repo", repo.Repo, "err", err)
		result.Error = err
	} else {
		for i := range pulls {
			github.DetectAgent(&pulls[i], repo.AgentPatterns, "")
		}
		result.PullRequests = pulls
	}

	rl := p.client.GetRateLimit()
	result.RateLimitRemaining = rl.Remaining

	return result
}
