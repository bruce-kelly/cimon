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

	// UI state
	width      int
	height     int
	quitting   bool
	statusText string
	rateLimit  int
	lastPoll   time.Time
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

	dashboard := screens.NewDashboard()
	dashboard.Pipeline.ExpandJobs = expandJobs

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
	return waitForPoll(a.resultCh)
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
	for _, run := range msg.Result.Runs {
		if err := a.db.UpsertRun(run); err != nil {
			slog.Error("upsert run", "err", err)
		}
	}
	for _, pr := range msg.Result.PullRequests {
		if err := a.db.UpsertPull(pr); err != nil {
			slog.Error("upsert pull", "err", err)
		}
	}

	a.rebuildScreenData()

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
	a.dashboard.Pipeline.SetRuns(allRuns)

	// Dashboard — review queue
	a.dashboard.ReviewItems = review.ReviewItemsFromPulls(
		allPulls,
		a.config.ReviewQueue.Escalation.Amber,
		a.config.ReviewQueue.Escalation.Red,
		a.dismissed,
	)

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
	case key.Matches(msg, ui.Keys.Tab):
		a.dashboard.CycleFocus()
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
				a.logPane.SetContent(
					fmt.Sprintf("%s — %s", run.Name, run.HTMLURL),
					fmt.Sprintf("Run #%d\nStatus: %s\nConclusion: %s\nBranch: %s\nActor: %s\n",
						run.ID, run.Status, run.Conclusion, run.HeadBranch, run.Actor),
					false,
				)
				if a.logPane.Mode == components.LogPaneHidden {
					a.logPane.CycleMode()
				}
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
						Label: "Approve", Key: "a",
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
							a.db.AddDismissed(pr.Repo, pr.Number)
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
	s := msg.String()
	switch {
	case s == "left" || s == "h":
		a.release.PrevRepo()
	case s == "right" || s == "l":
		a.release.NextRepo()
	case key.Matches(msg, ui.Keys.Down):
		a.release.Selector.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.release.Selector.Prev()
	}
	return a, nil
}

func (a App) View() tea.View {
	if a.quitting {
		return tea.NewView("")
	}

	barStyle := lipgloss.NewStyle().
		Background(ui.ColorSurface).
		Foreground(ui.ColorMuted).
		Padding(0, 1).
		Width(a.width)

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
		if s.active {
			screenIndicator += lipgloss.NewStyle().Foreground(ui.ColorAccent).Render("["+s.num+"]"+s.name) + " "
		} else {
			screenIndicator += lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(" "+s.num+" "+s.name) + " "
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
