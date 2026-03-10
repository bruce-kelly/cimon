package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/agents"
	"github.com/bruce-kelly/cimon/internal/confidence"
	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/polling"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
	"github.com/bruce-kelly/cimon/internal/ui/screens"
)

// pollResultMsg wraps a PollResult from the poller goroutine.
type pollResultMsg struct {
	Result models.PollResult
}

// pollErrorMsg indicates the poll channel was closed.
type pollErrorMsg struct {
	Err error
}

// agentTickMsg fires periodically to check scheduled tasks and agent lifetimes.
type agentTickMsg struct{}

// App is the root Bubbletea model wired to poller, DB, and screen models.
type App struct {
	// Infrastructure
	config   *config.CimonConfig
	client   *ghclient.Client
	db       *db.Database
	poller   *polling.Poller
	resultCh chan models.PollResult
	ctx      context.Context
	cancel   context.CancelFunc

	// Screen models
	screen    ui.Screen
	dashboard screens.DashboardModel
	timeline  screens.TimelineModel
	release   screens.ReleaseModel
	metrics   screens.MetricsModel

	// Overlays
	help       components.HelpOverlay
	flash      components.Flash
	confirmBar components.ConfirmBar
	logPane    components.LogPane
	actionMenu components.ActionMenu

	// Dismissed PRs
	dismissed map[string]bool

	// Per-repo latest poll data
	allRuns  map[string][]models.WorkflowRun
	allPulls map[string][]models.PullRequest

	// Config-derived lookups
	releaseWorkflows map[string]map[string]bool // repo → release workflow files
	agentWorkflows   map[string]map[string]bool // repo → agent workflow files

	// Agent infrastructure
	dispatcher *agents.Dispatcher
	scheduler  *agents.Scheduler
	autoFix    *agents.AutoFixTracker

	// Catchup overlay
	catchup      components.CatchupOverlay
	lastInput    time.Time
	preIdleRuns  int
	preIdlePulls int

	// Live log pane tracking
	liveRunID   int64
	liveRunRepo string

	// UI state
	width      int
	height     int
	quitting   bool
	statusText string
	rateLimit  int
	lastPoll   time.Time
	dbErrors   int
	tickEven   bool
}

// NewApp creates a fully wired App ready to run.
func NewApp(cfg *config.CimonConfig, client *ghclient.Client, database *db.Database) App {
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan models.PollResult, len(cfg.Repos))
	poller := polling.New(client, cfg, resultCh)

	// Build config-derived lookups
	releaseWorkflows := make(map[string]map[string]bool)
	agentWorkflows := make(map[string]map[string]bool)
	expandJobs := false
	var releaseRepos []string

	for _, repo := range cfg.Repos {
		for _, group := range repo.Groups {
			if group.ExpandJobs {
				expandJobs = true
			}
			for _, wf := range group.Workflows {
				if isReleaseGroup(group.Label) {
					if releaseWorkflows[repo.Repo] == nil {
						releaseWorkflows[repo.Repo] = make(map[string]bool)
					}
					releaseWorkflows[repo.Repo][wf] = true
				}
				if isAgentGroup(group.Label) {
					if agentWorkflows[repo.Repo] == nil {
						agentWorkflows[repo.Repo] = make(map[string]bool)
					}
					agentWorkflows[repo.Repo][wf] = true
				}
			}
		}
		if releaseWorkflows[repo.Repo] != nil {
			releaseRepos = append(releaseRepos, repo.Repo)
		}
	}

	dismissed, err := database.LoadDismissed()
	if err != nil {
		slog.Error("loading dismissed items", "err", err)
		dismissed = make(map[string]bool)
	}

	// Agent infrastructure
	maxConcurrent := cfg.Agents.MaxConcurrent
	if maxConcurrent == 0 {
		maxConcurrent = 2
	}
	maxLifetime := cfg.Agents.MaxLifetime
	if maxLifetime == 0 {
		maxLifetime = 1800
	}
	dispatcher := agents.NewDispatcher(maxConcurrent, maxLifetime, cfg.Agents.CaptureOutput)

	var scheduler *agents.Scheduler
	if len(cfg.Agents.Scheduled) > 0 {
		scheduler = agents.NewScheduler(cfg.Agents.Scheduled)
	}

	autoFix := agents.NewAutoFixTracker(maxConcurrent)
	for _, repo := range cfg.Repos {
		for groupName, group := range repo.Groups {
			if group.AutoFix {
				cooldown := group.AutoFixCooldown
				if cooldown == 0 {
					cooldown = 300
				}
				autoFix.SetCooldown(repo.Repo, groupName, cooldown)
			}
		}
	}

	// Check for agent workflows to show roster
	hasAgents := false
	for _, wfs := range agentWorkflows {
		if len(wfs) > 0 {
			hasAgents = true
			break
		}
	}

	dashboard := screens.NewDashboard()
	dashboard.Pipeline.ExpandJobs = expandJobs
	dashboard.ShowRoster = hasAgents

	release := screens.NewRelease()
	release.Repos = releaseRepos

	return App{
		config:           cfg,
		client:           client,
		db:               database,
		poller:           poller,
		resultCh:         resultCh,
		ctx:              ctx,
		cancel:           cancel,
		screen:           ui.ScreenDashboard,
		dashboard:        dashboard,
		timeline:         screens.NewTimeline(),
		release:          release,
		metrics:          screens.NewMetrics(),
		allRuns:          make(map[string][]models.WorkflowRun),
		allPulls:         make(map[string][]models.PullRequest),
		releaseWorkflows: releaseWorkflows,
		agentWorkflows:   agentWorkflows,
		dismissed:        dismissed,
		dispatcher:       dispatcher,
		scheduler:        scheduler,
		autoFix:          autoFix,
		lastInput:        time.Now(),
		statusText:       "cimon — connecting...",
	}
}

func isReleaseGroup(label string) bool {
	l := strings.ToLower(label)
	return strings.Contains(l, "release") || strings.Contains(l, "deploy")
}

func isAgentGroup(label string) bool {
	l := strings.ToLower(label)
	return strings.Contains(l, "agent")
}

func (a App) Init() tea.Cmd {
	a.poller.Start(a.ctx)
	cmds := []tea.Cmd{waitForPoll(a.resultCh)}
	if a.scheduler != nil || a.dispatcher != nil {
		cmds = append(cmds, agentTick())
	}
	return tea.Batch(cmds...)
}

func agentTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return agentTickMsg{}
	})
}

// waitForPoll returns a tea.Cmd that blocks until a PollResult arrives.
func waitForPoll(ch <-chan models.PollResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return pollErrorMsg{Err: fmt.Errorf("poll channel closed")}
		}
		return pollResultMsg{Result: result}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pollResultMsg:
		return a.handlePollResult(msg)

	case pollErrorMsg:
		a.statusText = fmt.Sprintf("Poll error: %v", msg.Err)
		return a, nil

	case agentTickMsg:
		a.tickEven = !a.tickEven
		// Check scheduled tasks
		if a.scheduler != nil && a.dispatcher != nil {
			for _, task := range a.scheduler.DueTasks(time.Now()) {
				repo := task.Config.Repo
				prompt := task.Config.Prompt
				if prompt == "" {
					wf := task.Config.Workflow
					go func(r, w string) {
						if err := a.client.DispatchWorkflow(a.ctx, r, w, "main"); err != nil {
							slog.Error("scheduled dispatch failed", "name", task.Config.Name, "err", err)
						}
					}(repo, wf)
				} else {
					go func(r, p string) {
						if _, err := a.dispatcher.Dispatch(r, p); err != nil {
							slog.Error("scheduled agent dispatch failed", "name", task.Config.Name, "err", err)
						}
					}(repo, prompt)
				}
			}
		}
		// Check agent lifetimes
		if a.dispatcher != nil {
			a.dispatcher.CheckAll()
			a.dashboard.Dispatched = a.dispatcher.AllAgents()
		}
		return a, agentTick()

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		contentHeight := a.height - 2 // top bar + bottom bar
		a.dashboard.SetSize(a.width, contentHeight)
		a.timeline.Width = a.width
		a.timeline.Height = contentHeight
		a.release.Width = a.width
		a.release.Height = contentHeight
		a.metrics.Width = a.width
		a.metrics.Height = contentHeight
		return a, nil

	case tea.KeyPressMsg:
		// ActionMenu intercepts all keys when active
		if a.actionMenu.Active {
			handled := a.actionMenu.HandleKey(msg.String())
			if handled {
				return a, nil
			}
		}
		// ConfirmBar intercepts all keys when active
		if a.confirmBar.Active {
			handled := a.confirmBar.HandleKey(msg.String())
			if handled {
				return a, nil
			}
		}
		return a.handleKey(msg)
	}

	return a, nil
}

func (a App) handlePollResult(msg pollResultMsg) (tea.Model, tea.Cmd) {
	// Check for classified errors
	if msg.Result.Error != nil {
		var authErr *ghclient.AuthError
		var rlErr *ghclient.RateLimitError
		switch {
		case errors.As(msg.Result.Error, &authErr):
			a.statusText = "AUTH FAILED — token expired or revoked. Restart with valid token."
			return a, waitForPoll(a.resultCh)
		case errors.As(msg.Result.Error, &rlErr):
			a.statusText = fmt.Sprintf("Rate limited — retrying at %s",
				rlErr.ResetAt.Format("15:04:05"))
			return a, waitForPoll(a.resultCh)
		}
	}

	repo := msg.Result.Repo
	a.allRuns[repo] = msg.Result.Runs
	a.allPulls[repo] = msg.Result.PullRequests
	a.lastPoll = time.Now()
	a.rateLimit = msg.Result.RateLimitRemaining

	// Persist to DB
	dbFailed := false
	for _, run := range msg.Result.Runs {
		if err := a.db.UpsertRun(run); err != nil {
			slog.Error("upsert run", "err", err)
			dbFailed = true
		}
	}
	for _, pr := range msg.Result.PullRequests {
		if err := a.db.UpsertPull(pr); err != nil {
			slog.Error("upsert pull", "err", err)
			dbFailed = true
		}
	}
	if dbFailed {
		a.dbErrors++
		if a.dbErrors >= 3 {
			a.flash.Show("DB writes failing — data not persisted", true)
		}
	} else {
		a.dbErrors = 0
	}

	// Auto-fix evaluation
	if a.autoFix != nil && a.dispatcher != nil {
		for _, run := range msg.Result.Runs {
			if run.Conclusion != "failure" {
				continue
			}
			repoConfig := a.findRepoConfig(run.Repo)
			if repoConfig == nil {
				continue
			}
			autoFixEnabled := false
			wfKey := ""
			for _, group := range repoConfig.Groups {
				for _, wf := range group.Workflows {
					if wf == run.WorkflowFile && group.AutoFix {
						autoFixEnabled = true
						wfKey = wf
					}
				}
			}
			if !autoFixEnabled {
				continue
			}

			var failedJobs []models.Job
			for _, j := range run.Jobs {
				if j.Conclusion == "failure" {
					failedJobs = append(failedJobs, j)
				}
			}

			isKnown := func(repo, jobName string) bool {
				known, err := a.db.IsKnownFailure(repo, jobName, 48)
				if err != nil {
					slog.Error("checking known failure", "err", err)
					return false
				}
				return known
			}
			decision := a.autoFix.Evaluate(
				run.Repo, wfKey, failedJobs,
				len(a.dispatcher.RunningAgents()),
				isKnown,
			)
			if decision.ShouldDispatch {
				prompt := agents.BuildFixPrompt(decision.Repo, decision.WorkflowFile, decision.FailingJobs)
				go func(r, p string) {
					id, err := a.dispatcher.Dispatch(r, p)
					if err != nil {
						slog.Error("auto-fix dispatch failed", "repo", r, "err", err)
					} else {
						slog.Info("auto-fix dispatched", "repo", r, "agent", id)
						a.flash.Show("Auto-fix agent dispatched", false)
					}
				}(decision.Repo, prompt)
			}
		}
	}

	a.rebuildScreenData()

	// Feed dispatched agents to dashboard
	if a.dispatcher != nil {
		a.dashboard.Dispatched = a.dispatcher.AllAgents()
	}

	// Snapshot for catchup detection
	totalRunsForSnapshot := 0
	for _, runs := range a.allRuns {
		totalRunsForSnapshot += len(runs)
	}
	totalPullsForSnapshot := 0
	for _, pulls := range a.allPulls {
		totalPullsForSnapshot += len(pulls)
	}
	if time.Since(a.lastInput) < 60*time.Second {
		a.preIdleRuns = totalRunsForSnapshot
		a.preIdlePulls = totalPullsForSnapshot
	}

	// Update status bar
	totalRuns := 0
	active := 0
	for _, runs := range a.allRuns {
		totalRuns += len(runs)
		for _, r := range runs {
			if r.IsActive() {
				active++
			}
		}
	}
	cadence := a.poller.State().Interval()
	if active > 0 {
		a.statusText = fmt.Sprintf("cimon — %d runs (%d active) | rate: %d | %s",
			totalRuns, active, a.rateLimit, formatInterval(cadence))
	} else {
		a.statusText = fmt.Sprintf("cimon — %d runs | rate: %d | %s",
			totalRuns, a.rateLimit, formatInterval(cadence))
	}

	return a, waitForPoll(a.resultCh)
}

func formatInterval(d time.Duration) string {
	return fmt.Sprintf("%.0fs", d.Seconds())
}

func (a *App) rebuildScreenData() {
	// Flatten all runs and pulls across repos
	var allRuns []models.WorkflowRun
	var allPulls []models.PullRequest
	for _, runs := range a.allRuns {
		allRuns = append(allRuns, runs...)
	}
	for _, pulls := range a.allPulls {
		allPulls = append(allPulls, pulls...)
	}

	// Dashboard — pipeline
	a.dashboard.Pipeline.TickEven = a.tickEven
	a.dashboard.Pipeline.SetRuns(allRuns)

	// Dashboard — review queue
	a.dashboard.ReviewItems = review.ReviewItemsFromPulls(
		allPulls,
		a.config.ReviewQueue.Escalation.Amber,
		a.config.ReviewQueue.Escalation.Red,
		a.dismissed,
	)
	a.dashboard.ReviewSel.SetCount(len(a.dashboard.ReviewItems))

	// Dashboard — agent profiles
	agentWFs := make(map[string]bool)
	var agentRuns []models.WorkflowRun
	for repo, runs := range a.allRuns {
		if awf, ok := a.agentWorkflows[repo]; ok {
			for _, r := range runs {
				if awf[r.WorkflowFile] {
					agentRuns = append(agentRuns, r)
					agentWFs[r.WorkflowFile] = true
				}
			}
		}
	}
	a.dashboard.AgentProfiles = agents.BuildAgentProfiles(agentRuns, agentWFs)
	a.dashboard.RosterSel.SetCount(len(a.dashboard.AgentProfiles))

	// Timeline — all runs chronologically
	a.timeline.SetRuns(allRuns)

	// Release — per-repo release runs + confidence
	for repo, runs := range a.allRuns {
		rwf, ok := a.releaseWorkflows[repo]
		if !ok {
			continue
		}
		var releaseRuns []models.WorkflowRun
		for _, r := range runs {
			if rwf[r.WorkflowFile] {
				releaseRuns = append(releaseRuns, r)
			}
		}
		a.release.Runs[repo] = releaseRuns

		repoPulls := a.allPulls[repo]
		reviewItems := review.ReviewItemsFromPulls(
			repoPulls,
			a.config.ReviewQueue.Escalation.Amber,
			a.config.ReviewQueue.Escalation.Red,
			nil,
		)
		newFailures := 0
		for _, r := range runs {
			if r.Conclusion == "failure" && time.Since(r.UpdatedAt) < 24*time.Hour {
				newFailures++
			}
		}
		conf := confidence.ComputeConfidence(runs, repoPulls, len(reviewItems), newFailures)
		a.release.Confidence[repo] = conf
	}

	// Update release selector count for current repo
	if len(a.release.Repos) > 0 {
		curRepo := a.release.Repos[a.release.CurrentRepo]
		a.release.Selector.SetCount(len(a.release.Runs[curRepo]))
	}

	// Live log pane refresh
	if a.liveRunID != 0 {
		a.refreshLiveLogPane()
	}

	// Metrics — refresh from DB
	if runStats, err := a.db.RunStats(30); err == nil {
		a.metrics.RunStats = &runStats
	}
	if taskStats, err := a.db.TaskStats(30); err == nil {
		a.metrics.TaskStats = &taskStats
	}
	if eff, err := a.db.AgentEffectivenessStats(30); err == nil {
		a.metrics.Effectiveness = &eff
	}
}

func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	now := time.Now()

	// Catchup overlay — dismiss on any key
	if a.catchup.Visible {
		a.catchup.Dismiss()
		a.lastInput = now
		return a, nil
	}

	// Check if returning from idle
	if a.config.Catchup.Enabled {
		threshold := time.Duration(a.config.Catchup.IdleThreshold) * time.Second
		if threshold == 0 {
			threshold = 15 * time.Minute
		}
		if now.Sub(a.lastInput) > threshold {
			totalRuns := 0
			totalPulls := 0
			for _, runs := range a.allRuns {
				totalRuns += len(runs)
			}
			for _, pulls := range a.allPulls {
				totalPulls += len(pulls)
			}
			newRuns := totalRuns - a.preIdleRuns
			changedPRs := totalPulls - a.preIdlePulls
			if newRuns > 0 || changedPRs > 0 {
				a.catchup.Show(newRuns, 0, changedPRs)
				a.lastInput = now
				return a, nil
			}
		}
	}

	a.lastInput = now

	// Help overlay intercepts all keys when visible
	if a.help.Visible {
		a.help.Toggle()
		return a, nil
	}

	// Filter mode intercepts when active
	if a.screen == ui.ScreenDashboard && a.dashboard.Pipeline.Filter.Active {
		if key.Matches(msg, ui.Keys.Escape) {
			a.dashboard.Pipeline.Filter.Clear()
		} else {
			a.dashboard.Pipeline.Filter.HandleKey(msg.String())
		}
		return a, nil
	}
	if a.screen == ui.ScreenTimeline && a.timeline.Filter.Active {
		if key.Matches(msg, ui.Keys.Escape) {
			a.timeline.Filter.Clear()
		} else {
			a.timeline.Filter.HandleKey(msg.String())
		}
		return a, nil
	}

	// Global keys
	switch {
	case key.Matches(msg, ui.Keys.Quit):
		a.quitting = true
		if a.dispatcher != nil {
			a.dispatcher.Shutdown()
		}
		a.poller.Stop()
		a.cancel()
		return a, tea.Quit

	case key.Matches(msg, ui.Keys.Help):
		a.help.Toggle()
		return a, nil

	case key.Matches(msg, ui.Keys.Screen1):
		a.screen = ui.ScreenDashboard
		return a, nil
	case key.Matches(msg, ui.Keys.Screen2):
		a.screen = ui.ScreenTimeline
		return a, nil
	case key.Matches(msg, ui.Keys.Screen3):
		a.screen = ui.ScreenRelease
		return a, nil
	case key.Matches(msg, ui.Keys.Screen4):
		a.screen = ui.ScreenMetrics
		return a, nil

	case key.Matches(msg, ui.Keys.LogCycle):
		a.logPane.CycleMode()
		return a, nil

	case key.Matches(msg, ui.Keys.Filter):
		switch a.screen {
		case ui.ScreenDashboard:
			a.dashboard.Pipeline.Filter.Active = true
		case ui.ScreenTimeline:
			a.timeline.Filter.Active = true
		}
		return a, nil
	}

	// Screen-specific keys
	switch a.screen {
	case ui.ScreenDashboard:
		return a.handleDashboardKey(msg)
	case ui.ScreenTimeline:
		return a.handleTimelineKey(msg)
	case ui.ScreenRelease:
		return a.handleReleaseKey(msg)
	}

	return a, nil
}

func (a App) handleDashboardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, ui.Keys.Tab), key.Matches(msg, ui.Keys.Right):
		a.dashboard.CycleFocus()
	case key.Matches(msg, ui.Keys.Left):
		a.dashboard.CycleFocusReverse()
	case key.Matches(msg, ui.Keys.Down):
		switch a.dashboard.Focus {
		case screens.FocusPipeline:
			a.dashboard.Pipeline.Selector.Next()
		case screens.FocusReview:
			a.dashboard.ReviewSel.Next()
		case screens.FocusRoster:
			a.dashboard.RosterSel.Next()
		}
	case key.Matches(msg, ui.Keys.Up):
		switch a.dashboard.Focus {
		case screens.FocusPipeline:
			a.dashboard.Pipeline.Selector.Prev()
		case screens.FocusReview:
			a.dashboard.ReviewSel.Prev()
		case screens.FocusRoster:
			a.dashboard.RosterSel.Prev()
		}

	case key.Matches(msg, ui.Keys.Rerun):
		if a.dashboard.Focus == screens.FocusPipeline {
			if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
				runCopy := *run
				a.confirmBar.Show(
					fmt.Sprintf("Rerun %s #%d?", runCopy.Name, runCopy.ID),
					func() {
						go func() {
							var err error
							if runCopy.Conclusion == "failure" {
								err = a.client.RerunFailed(a.ctx, runCopy.Repo, runCopy.ID)
							} else {
								err = a.client.Rerun(a.ctx, runCopy.Repo, runCopy.ID)
							}
							if err != nil {
								a.flash.Show("Rerun failed: "+err.Error(), true)
							} else {
								a.flash.Show("Rerun triggered", false)
							}
						}()
					},
					func() {},
				)
			}
		}

	case key.Matches(msg, ui.Keys.Open):
		var url string
		switch a.dashboard.Focus {
		case screens.FocusPipeline:
			if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
				url = run.HTMLURL
			}
		case screens.FocusReview:
			idx := a.dashboard.ReviewSel.Index()
			if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
				url = a.dashboard.ReviewItems[idx].PR.HTMLURL
			}
		}
		if url != "" {
			go openBrowser(url)
		}

	case key.Matches(msg, ui.Keys.Approve):
		if a.dashboard.Focus == screens.FocusReview {
			idx := a.dashboard.ReviewSel.Index()
			if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
				pr := a.dashboard.ReviewItems[idx].PR
				a.confirmBar.Show(
					fmt.Sprintf("Approve %s#%d?", pr.Repo, pr.Number),
					func() {
						go func() {
							if err := a.client.Approve(a.ctx, pr.Repo, pr.Number); err != nil {
								a.flash.Show("Approve failed: "+err.Error(), true)
							} else {
								a.flash.Show(fmt.Sprintf("Approved %s#%d", pr.Repo, pr.Number), false)
							}
						}()
					},
					func() {},
				)
			}
		}

	case key.Matches(msg, ui.Keys.Merge):
		if a.dashboard.Focus == screens.FocusReview {
			idx := a.dashboard.ReviewSel.Index()
			if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
				pr := a.dashboard.ReviewItems[idx].PR
				a.confirmBar.Show(
					fmt.Sprintf("Merge %s#%d?", pr.Repo, pr.Number),
					func() {
						go func() {
							if err := a.client.Merge(a.ctx, pr.Repo, pr.Number); err != nil {
								a.flash.Show("Merge failed: "+err.Error(), true)
							} else {
								a.flash.Show(fmt.Sprintf("Merged %s#%d", pr.Repo, pr.Number), false)
							}
						}()
					},
					func() {},
				)
			}
		}

	case key.Matches(msg, ui.Keys.BatchMerge):
		var agentReady []models.PullRequest
		for _, item := range a.dashboard.ReviewItems {
			if item.PR.IsAgent && item.PR.CIStatus == "success" && item.PR.ReviewState == "approved" {
				agentReady = append(agentReady, item.PR)
			}
		}
		if len(agentReady) > 0 {
			a.confirmBar.Show(
				fmt.Sprintf("Batch merge %d agent PRs?", len(agentReady)),
				func() {
					go func() {
						merged := 0
						for _, pr := range agentReady {
							if err := a.client.Merge(a.ctx, pr.Repo, pr.Number); err != nil {
								slog.Error("batch merge failed", "pr", pr.Number, "err", err)
							} else {
								merged++
							}
						}
						a.flash.Show(fmt.Sprintf("Merged %d/%d agent PRs", merged, len(agentReady)), merged < len(agentReady))
					}()
				},
				func() {},
			)
		}

	case key.Matches(msg, ui.Keys.Dismiss):
		if a.dashboard.Focus == screens.FocusReview {
			idx := a.dashboard.ReviewSel.Index()
			if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
				pr := a.dashboard.ReviewItems[idx].PR
				dismissKey := fmt.Sprintf("%s:%d", pr.Repo, pr.Number)
				if err := a.db.AddDismissed(pr.Repo, pr.Number); err != nil {
					a.flash.Show("Dismiss failed: "+err.Error(), true)
				} else {
					a.dismissed[dismissKey] = true
					a.rebuildScreenData()
					a.flash.Show(fmt.Sprintf("Dismissed %s#%d", pr.Repo, pr.Number), false)
				}
			}
		}

	case key.Matches(msg, ui.Keys.ViewDiff):
		switch a.dashboard.Focus {
		case screens.FocusReview:
			idx := a.dashboard.ReviewSel.Index()
			if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
				pr := a.dashboard.ReviewItems[idx].PR
				go func() {
					diff, err := a.client.GetPullDiff(a.ctx, pr.Repo, pr.Number)
					if err != nil {
						a.flash.Show("Failed to fetch diff: "+err.Error(), true)
						return
					}
					a.logPane.SetContent(
						fmt.Sprintf("%s#%d — %s", pr.Repo, pr.Number, pr.Title),
						diff,
						false,
					)
					if a.logPane.Mode == components.LogPaneHidden {
						a.logPane.CycleMode()
					}
				}()
			}
		case screens.FocusPipeline:
			if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
				if run.IsActive() {
					a.liveRunID = run.ID
					a.liveRunRepo = run.Repo
				} else {
					a.liveRunID = 0
					a.liveRunRepo = ""
				}
				a.logPane.SetContent(
					fmt.Sprintf("%s \u2014 %s", run.Name, run.HTMLURL),
					fmt.Sprintf("Run #%d\nStatus: %s\nConclusion: %s\nBranch: %s\nActor: %s\n",
						run.ID, run.Status, run.Conclusion, run.HeadBranch, run.Actor),
					run.IsActive(),
				)
				if a.logPane.Mode == components.LogPaneHidden {
					a.logPane.CycleMode()
				}
			}
		}

	case key.Matches(msg, ui.Keys.Dispatch):
		if a.dashboard.Focus == screens.FocusPipeline && a.dispatcher != nil {
			if run := a.dashboard.Pipeline.SelectedRun(); run != nil && run.Conclusion == "failure" {
				runCopy := *run
				prompt := fmt.Sprintf("Fix the failing CI in %s. The workflow %s failed.", runCopy.Repo, runCopy.Name)
				a.confirmBar.Show(
					fmt.Sprintf("Dispatch fix agent for %s?", runCopy.Name),
					func() {
						go func() {
							id, err := a.dispatcher.Dispatch(runCopy.Repo, prompt)
							if err != nil {
								a.flash.Show("Dispatch failed: "+err.Error(), true)
							} else {
								a.flash.Show(fmt.Sprintf("Agent dispatched: %s", id), false)
							}
						}()
					},
					func() {},
				)
			}
		}

	case key.Matches(msg, ui.Keys.Enter):
		var items []components.ActionMenuItem

		switch a.dashboard.Focus {
		case screens.FocusPipeline:
			if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
				runCopy := *run
				items = append(items, components.ActionMenuItem{
					Label: "Rerun all jobs", Key: "r",
					Action: func() {
						go func() {
							if err := a.client.Rerun(a.ctx, runCopy.Repo, runCopy.ID); err != nil {
								a.flash.Show("Rerun failed: "+err.Error(), true)
							} else {
								a.flash.Show("Rerun triggered", false)
							}
						}()
					},
				})
				if runCopy.Conclusion == "failure" {
					items = append(items, components.ActionMenuItem{
						Label: "Rerun failed jobs", Key: "f",
						Action: func() {
							go func() {
								if err := a.client.RerunFailed(a.ctx, runCopy.Repo, runCopy.ID); err != nil {
									a.flash.Show("Rerun failed: "+err.Error(), true)
								} else {
									a.flash.Show("Rerun (failed only) triggered", false)
								}
							}()
						},
					})
				}
				if runCopy.IsActive() {
					items = append(items, components.ActionMenuItem{
						Label: "Cancel", Key: "c",
						Action: func() {
							go func() {
								if err := a.client.Cancel(a.ctx, runCopy.Repo, runCopy.ID); err != nil {
									a.flash.Show("Cancel failed: "+err.Error(), true)
								} else {
									a.flash.Show("Run cancelled", false)
								}
							}()
						},
					})
				}
				items = append(items, components.ActionMenuItem{
					Label: "Open in browser", Key: "o",
					Action: func() { go openBrowser(runCopy.HTMLURL) },
				})
			}

		case screens.FocusReview:
			idx := a.dashboard.ReviewSel.Index()
			if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
				pr := a.dashboard.ReviewItems[idx].PR
				items = append(items,
					components.ActionMenuItem{
						Label: "Approve", Key: "A",
						Action: func() {
							go func() {
								if err := a.client.Approve(a.ctx, pr.Repo, pr.Number); err != nil {
									a.flash.Show("Approve failed: "+err.Error(), true)
								} else {
									a.flash.Show(fmt.Sprintf("Approved %s#%d", pr.Repo, pr.Number), false)
								}
							}()
						},
					},
					components.ActionMenuItem{
						Label: "Merge", Key: "m",
						Action: func() {
							go func() {
								if err := a.client.Merge(a.ctx, pr.Repo, pr.Number); err != nil {
									a.flash.Show("Merge failed: "+err.Error(), true)
								} else {
									a.flash.Show(fmt.Sprintf("Merged %s#%d", pr.Repo, pr.Number), false)
								}
							}()
						},
					},
					components.ActionMenuItem{
						Label: "View diff", Key: "v",
						Action: func() {
							go func() {
								diff, err := a.client.GetPullDiff(a.ctx, pr.Repo, pr.Number)
								if err != nil {
									a.flash.Show("Diff fetch failed: "+err.Error(), true)
									return
								}
								a.logPane.SetContent(
									fmt.Sprintf("%s#%d", pr.Repo, pr.Number),
									diff, false,
								)
								if a.logPane.Mode == components.LogPaneHidden {
									a.logPane.CycleMode()
								}
							}()
						},
					},
					components.ActionMenuItem{
						Label: "Open in browser", Key: "o",
						Action: func() { go openBrowser(pr.HTMLURL) },
					},
					components.ActionMenuItem{
						Label: "Dismiss", Key: "x",
						Action: func() {
							dismissKey := fmt.Sprintf("%s:%d", pr.Repo, pr.Number)
							if err := a.db.AddDismissed(pr.Repo, pr.Number); err != nil {
								slog.Error("dismiss failed", "pr", pr.Number, "err", err)
								return
							}
							a.dismissed[dismissKey] = true
							a.rebuildScreenData()
						},
					},
				)
			}
		}

		if len(items) > 0 {
			a.actionMenu.Show(items)
		}
	}
	return a, nil
}

func (a App) handleTimelineKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, ui.Keys.Down):
		a.timeline.Selector.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.timeline.Selector.Prev()
	}
	return a, nil
}

func (a App) handleReleaseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, ui.Keys.Left):
		a.release.PrevRepo()
		if len(a.release.Repos) > 0 {
			repo := a.release.Repos[a.release.CurrentRepo]
			a.release.Selector.SetCount(len(a.release.Runs[repo]))
		}
	case key.Matches(msg, ui.Keys.Right):
		a.release.NextRepo()
		if len(a.release.Repos) > 0 {
			repo := a.release.Repos[a.release.CurrentRepo]
			a.release.Selector.SetCount(len(a.release.Runs[repo]))
		}
	case key.Matches(msg, ui.Keys.Down):
		a.release.Selector.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.release.Selector.Prev()

	case key.Matches(msg, ui.Keys.Enter), key.Matches(msg, ui.Keys.ViewDiff):
		if run := a.release.SelectedRun(); run != nil {
			runCopy := *run
			if runCopy.IsActive() {
				a.liveRunID = runCopy.ID
				a.liveRunRepo = runCopy.Repo
			} else {
				a.liveRunID = 0
				a.liveRunRepo = ""
			}
			go func() {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("Run #%d  %s\n", runCopy.ID, runCopy.HTMLURL))
				sb.WriteString(fmt.Sprintf("Status: %s  Conclusion: %s\n", runCopy.Status, runCopy.Conclusion))
				sb.WriteString(fmt.Sprintf("Branch: %s  SHA: %s  Event: %s\n", runCopy.HeadBranch, runCopy.HeadSHA[:min(7, len(runCopy.HeadSHA))], runCopy.Event))
				sb.WriteString(fmt.Sprintf("Actor: %s\n", runCopy.Actor))
				if !runCopy.CreatedAt.IsZero() {
					sb.WriteString(fmt.Sprintf("Started: %s  Elapsed: %s\n", runCopy.CreatedAt.Format("15:04:05"), components.FormatDuration(runCopy.Elapsed())))
				}
				sb.WriteString("\n")

				// Fetch jobs if not already present
				jobs := runCopy.Jobs
				if len(jobs) == 0 {
					fetched, err := a.client.GetJobs(a.ctx, runCopy.Repo, runCopy.ID)
					if err != nil {
						sb.WriteString(fmt.Sprintf("Failed to fetch jobs: %s\n", err))
					} else {
						jobs = fetched
					}
				}

				for _, job := range jobs {
					icon := "●"
					switch job.Conclusion {
					case "success":
						icon = "✓"
					case "failure":
						icon = "✗"
					case "cancelled":
						icon = "○"
					}
					elapsed := ""
					if job.StartedAt != nil && job.CompletedAt != nil {
						elapsed = " " + components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
					}
					sb.WriteString(fmt.Sprintf("  %s %s%s\n", icon, job.Name, elapsed))

					// Show steps for failed jobs
					if job.Conclusion == "failure" {
						for _, step := range job.Steps {
							if step.Conclusion == "failure" {
								sb.WriteString(fmt.Sprintf("      ✗ %s\n", step.Name))
							}
						}
					}
				}

				// Fetch logs for failed jobs
				for _, job := range jobs {
					if job.Conclusion == "failure" {
						sb.WriteString(fmt.Sprintf("\n── %s (failed) ──\n", job.Name))
						logs, err := a.client.GetFailedLogs(a.ctx, runCopy.Repo, job.ID)
						if err != nil {
							sb.WriteString(fmt.Sprintf("  Failed to fetch logs: %s\n", err))
						} else if logs == "" {
							sb.WriteString("  (no logs available)\n")
						} else {
							// Truncate long logs
							if len(logs) > 4000 {
								logs = logs[len(logs)-4000:]
							}
							sb.WriteString(logs)
							sb.WriteString("\n")
						}
					}
				}

				title := fmt.Sprintf("%s — %s", runCopy.Name, runCopy.HeadBranch)
				a.logPane.SetContent(title, sb.String(), false)
				if a.logPane.Mode == components.LogPaneHidden {
					a.logPane.CycleMode()
				}
			}()
		}

	case key.Matches(msg, ui.Keys.Open):
		if run := a.release.SelectedRun(); run != nil {
			go openBrowser(run.HTMLURL)
		}

	case key.Matches(msg, ui.Keys.Rerun):
		if run := a.release.SelectedRun(); run != nil {
			runCopy := *run
			a.confirmBar.Show(
				fmt.Sprintf("Rerun %s #%d?", runCopy.Name, runCopy.ID),
				func() {
					go func() {
						var err error
						if runCopy.Conclusion == "failure" {
							err = a.client.RerunFailed(a.ctx, runCopy.Repo, runCopy.ID)
						} else {
							err = a.client.Rerun(a.ctx, runCopy.Repo, runCopy.ID)
						}
						if err != nil {
							a.flash.Show("Rerun failed: "+err.Error(), true)
						} else {
							a.flash.Show("Rerun triggered", false)
						}
					}()
				},
				func() {},
			)
		}
	}
	return a, nil
}

func (a App) View() tea.View {
	if a.quitting {
		return tea.NewView("")
	}

	if a.width < 60 || a.height < 10 {
		msg := fmt.Sprintf("Terminal too small: %d×%d\nMinimum: 60×10\nPlease resize.", a.width, a.height)
		v := tea.NewView(msg)
		v.AltScreen = true
		return v
	}

	barStyle := lipgloss.NewStyle().
		Background(ui.ColorSurface).
		Foreground(ui.ColorMuted).
		Padding(0, 1).
		Width(a.width)

	// Badge counts for screen tabs
	failCount := 0
	activeReleases := 0
	for _, runs := range a.allRuns {
		for _, r := range runs {
			if r.Conclusion == "failure" {
				failCount++
			}
		}
	}
	for _, runs := range a.release.Runs {
		for _, r := range runs {
			if r.IsActive() {
				activeReleases++
			}
		}
	}

	// Top bar: screen tabs
	screenIndicator := ""
	screenList := []struct {
		num    string
		name   string
		active bool
	}{
		{"1", "dashboard", a.screen == ui.ScreenDashboard},
		{"2", "timeline", a.screen == ui.ScreenTimeline},
		{"3", "release", a.screen == ui.ScreenRelease},
		{"4", "metrics", a.screen == ui.ScreenMetrics},
	}
	for _, s := range screenList {
		badge := ""
		switch s.num {
		case "1":
			if failCount > 0 {
				badge = lipgloss.NewStyle().Foreground(ui.ColorRed).Render(fmt.Sprintf("(%d)", failCount))
			}
		case "3":
			if activeReleases > 0 {
				badge = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("\u25cf")
			}
		}
		if s.active {
			screenIndicator += lipgloss.NewStyle().Foreground(ui.ColorAccent).Render("["+s.num+"]"+s.name) + badge + " "
		} else {
			screenIndicator += lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(" "+s.num+" "+s.name) + badge + " "
		}
	}
	topBar := barStyle.Render(screenIndicator)

	// Bottom bar: confirmBar > flash > status
	var bottomContent string
	if a.confirmBar.Active {
		bottomContent = a.confirmBar.Render(a.width)
	} else if a.flash.Visible() {
		if a.flash.IsError {
			bottomContent = lipgloss.NewStyle().Foreground(ui.ColorRed).Width(a.width).Padding(0, 1).Render(a.flash.Message)
		} else {
			bottomContent = lipgloss.NewStyle().Foreground(ui.ColorGreen).Width(a.width).Padding(0, 1).Render(a.flash.Message)
		}
	} else {
		bottomContent = a.statusText
	}
	bottomBar := barStyle.Render(bottomContent)

	// Screen content
	var content string
	switch a.screen {
	case ui.ScreenDashboard:
		content = a.dashboard.Render()
	case ui.ScreenTimeline:
		content = a.timeline.Render()
	case ui.ScreenRelease:
		content = a.release.Render()
	case ui.ScreenMetrics:
		content = a.metrics.Render()
	}

	// Catchup overlay replaces content
	if a.catchup.Visible {
		contentHeight := a.height - 2
		if contentHeight < 0 {
			contentHeight = 0
		}
		content = a.catchup.Render(a.width, contentHeight)
	}

	// Help overlay replaces content
	if a.help.Visible {
		content = a.help.Render(a.screen.String(), a.width, a.height)
	}

	// ActionMenu overlay
	if a.actionMenu.Active {
		content = content + "\n" + a.actionMenu.Render()
	}

	// Truncate content to fit between the two bars
	contentHeight := a.height - 2
	if contentHeight < 0 {
		contentHeight = 0
	}

	// LogPane splits the content area
	logContent := ""
	if a.logPane.Mode != components.LogPaneHidden {
		logContent = a.logPane.Render(a.width, contentHeight)
	}

	var rendered string
	if logContent != "" {
		logHeight := contentHeight / 2
		if a.logPane.Mode == components.LogPaneFull {
			logHeight = contentHeight
		}
		screenHeight := contentHeight - logHeight

		screenLines := strings.Split(content, "\n")
		if len(screenLines) > screenHeight {
			screenLines = screenLines[:screenHeight]
		}
		content = strings.Join(screenLines, "\n")

		contentStyle := lipgloss.NewStyle().
			Width(a.width).
			Height(screenHeight).
			Background(ui.ColorBg)
		rendered = topBar + "\n" + contentStyle.Render(content) + "\n" + logContent + "\n" + bottomBar
	} else {
		lines := strings.Split(content, "\n")
		if len(lines) > contentHeight {
			lines = lines[:contentHeight]
		}
		content = strings.Join(lines, "\n")

		contentStyle := lipgloss.NewStyle().
			Width(a.width).
			Height(contentHeight).
			Background(ui.ColorBg)
		rendered = topBar + "\n" + contentStyle.Render(content) + "\n" + bottomBar
	}

	v := tea.NewView(rendered)
	v.AltScreen = true
	return v
}

func (a *App) refreshLiveLogPane() {
	var liveRun *models.WorkflowRun
	for _, runs := range a.allRuns {
		for i := range runs {
			if runs[i].ID == a.liveRunID {
				liveRun = &runs[i]
				break
			}
		}
	}
	if liveRun == nil {
		a.liveRunID = 0
		a.liveRunRepo = ""
		a.logPane.IsLive = false
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Run #%d  %s\n", liveRun.ID, liveRun.HTMLURL))
	sb.WriteString(fmt.Sprintf("Status: %s  Conclusion: %s\n", liveRun.Status, liveRun.Conclusion))
	sha := liveRun.HeadSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}
	sb.WriteString(fmt.Sprintf("Branch: %s  SHA: %s  Actor: %s\n", liveRun.HeadBranch, sha, liveRun.Actor))
	if !liveRun.CreatedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Started: %s  Elapsed: %s\n",
			liveRun.CreatedAt.Format("15:04:05"),
			components.FormatDuration(liveRun.Elapsed()),
		))
	}
	sb.WriteString("\n")

	for _, job := range liveRun.Jobs {
		dot := ui.StatusDot(job.Conclusion)
		if job.Status == "in_progress" && job.Conclusion == "" {
			dot = ui.PulsingDot("", a.tickEven)
		}
		dotColor := ui.StatusColor(job.Conclusion)
		elapsed := ""
		if job.StartedAt != nil {
			if job.CompletedAt != nil {
				elapsed = components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
			} else {
				elapsed = components.FormatDuration(time.Since(*job.StartedAt)) + "..."
			}
		} else if job.Status == "queued" {
			elapsed = "queued"
		}
		sb.WriteString(fmt.Sprintf("  %s %s  %s\n",
			lipgloss.NewStyle().Foreground(dotColor).Render(dot),
			job.Name,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
		))
	}

	sb.WriteString("\n")
	progress := components.RenderJobProgress(liveRun.Jobs)
	if progress != "" {
		sb.WriteString("  " + progress + "\n")
	}

	title := fmt.Sprintf("%s \u2014 %s", liveRun.Name, liveRun.HeadBranch)
	isStillLive := liveRun.IsActive()
	a.logPane.SetContent(title, sb.String(), isStillLive)

	if !isStillLive {
		a.liveRunID = 0
		a.liveRunRepo = ""
	}
}

func (a App) findRepoConfig(repo string) *config.RepoConfig {
	for i := range a.config.Repos {
		if a.config.Repos[i].Repo == repo {
			return &a.config.Repos[i]
		}
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Run()
	}
}
