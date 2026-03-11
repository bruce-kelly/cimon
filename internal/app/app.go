package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/polling"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
	"github.com/bruce-kelly/cimon/internal/ui/views"
)

// pollResultMsg wraps a PollResult from the poller goroutine.
type pollResultMsg struct {
	Result models.PollResult
}

// pollErrorMsg indicates the poll channel was closed.
type pollErrorMsg struct {
	Err error
}

// actionResultMsg carries the outcome of an async action (rerun, approve, merge).
type actionResultMsg struct {
	Message string
	IsError bool
}

// diffResultMsg carries fetched diff content for the log pane.
type diffResultMsg struct {
	Title   string
	Content string
	Err     error
}

// logsResultMsg carries fetched job log content for the log pane.
type logsResultMsg struct {
	Title   string
	Content string
	Err     error
}

// jobsResultMsg carries fetched jobs for a workflow run.
type jobsResultMsg struct {
	RunID int64
	Jobs  []models.Job
	Err   error
}

// tickMsg fires periodically to expire NEW flags and refresh the clock.
type tickMsg struct{}

// App is the root Bubbletea model wired to poller, DB, and view models.
type App struct {
	// Infrastructure
	config   *config.CimonConfig
	client   *ghclient.Client
	db       *db.Database
	poller   *polling.Poller
	resultCh chan models.PollResult
	ctx      context.Context
	cancel   context.CancelFunc

	// v2 view models
	mode           ui.ViewMode
	compactView    *views.CompactView
	detailView     *views.DetailView
	runDetailView  *views.RunDetailView
	prDetailView   *views.PRDetailView
	repos          []views.RepoState

	// Overlays
	help       components.HelpOverlay
	flash      components.Flash
	confirmBar components.ConfirmBar
	logPane    components.LogPane

	// Dismissed PRs
	dismissed map[string]bool

	// Per-repo latest poll data
	allRuns  map[string][]models.WorkflowRun
	allPulls map[string][]models.PullRequest

	// Config-derived lookups
	releaseWorkflows   map[string]map[string]bool
	agentWorkflows     map[string]map[string]bool // repo → agent workflow files
	repoBranch         map[string]string           // repo → configured branch
	dispatchCooldowns  map[string]time.Time        // "repo/workflow" → last dispatch time

	// UI state
	width      int
	height     int
	quitting   bool
	statusText string
	rateLimit  int
	lastPoll   time.Time
	dbErrors int
}

// NewApp creates a fully wired App ready to run.
func NewApp(cfg *config.CimonConfig, client *ghclient.Client, database *db.Database) App {
	ctx, cancel := context.WithCancel(context.Background())

	releaseWorkflows := make(map[string]map[string]bool)
	agentWorkflows := make(map[string]map[string]bool)
	repoBranch := make(map[string]string)
	for _, rc := range cfg.Repos {
		rw := make(map[string]bool)
		aw := make(map[string]bool)
		repoBranch[rc.Repo] = rc.Branch
		for name, g := range rc.Groups {
			if strings.Contains(strings.ToLower(name), "agent") {
				for _, wf := range g.Workflows {
					aw[wf] = true
				}
			}
			for _, cat := range []string{"release", "deploy"} {
				if strings.Contains(strings.ToLower(g.Label), cat) {
					for _, wf := range g.Workflows {
						rw[wf] = true
					}
				}
			}
		}
		releaseWorkflows[rc.Repo] = rw
		agentWorkflows[rc.Repo] = aw
	}

	dismissed := make(map[string]bool)
	if database != nil {
		if loaded, err := database.LoadDismissed(); err == nil {
			dismissed = loaded
		}
	}

	resultCh := make(chan models.PollResult, len(cfg.Repos))
	p := polling.New(client, cfg, resultCh)

	a := App{
		config:           cfg,
		client:           client,
		db:               database,
		poller:           p,
		resultCh:         resultCh,
		ctx:              ctx,
		cancel:           cancel,
		mode:             ui.ViewCompact,
		dismissed:        dismissed,
		allRuns:           make(map[string][]models.WorkflowRun),
		allPulls:          make(map[string][]models.PullRequest),
		releaseWorkflows:  releaseWorkflows,
		agentWorkflows:    agentWorkflows,
		repoBranch:        repoBranch,
		dispatchCooldowns: make(map[string]time.Time),
	}

	a.repos = a.buildRepoStates()
	a.compactView = views.NewCompactView(a.repos)

	return a
}

func (a App) Init() tea.Cmd {
	a.poller.Start(a.ctx)
	return tea.Batch(waitForPoll(a.resultCh), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
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

// buildRepoStates constructs view-model RepoStates from current poll data.
func (a *App) buildRepoStates() []views.RepoState {
	states := make([]views.RepoState, 0, len(a.config.Repos))

	for _, rc := range a.config.Repos {
		runs := a.allRuns[rc.Repo]
		prs := a.allPulls[rc.Repo]

		var activePRs []models.PullRequest
		for _, pr := range prs {
			key := fmt.Sprintf("%s:%d", rc.Repo, pr.Number)
			if !a.dismissed[key] {
				activePRs = append(activePRs, pr)
			}
		}

		amberHours := 24
		redHours := 48
		if a.config.ReviewQueue.Escalation.Amber > 0 {
			amberHours = a.config.ReviewQueue.Escalation.Amber
		}
		if a.config.ReviewQueue.Escalation.Red > 0 {
			redHours = a.config.ReviewQueue.Escalation.Red
		}
		reviewItems := review.ReviewItemsFromPulls(activePRs, amberHours, redHours, nil)

		// Build critical workflow set: all groups except "agents"
		var criticalWorkflows map[string]bool
		for name, group := range rc.Groups {
			if strings.Contains(strings.ToLower(name), "agent") {
				continue
			}
			if criticalWorkflows == nil {
				criticalWorkflows = make(map[string]bool)
			}
			for _, wf := range group.Workflows {
				criticalWorkflows[wf] = true
			}
		}

		inline := views.ComputeInlineStatus(runs, criticalWorkflows)
		if rw, ok := a.releaseWorkflows[rc.Repo]; ok {
			for i := range inline.ActiveRuns {
				for _, r := range runs {
					if r.Name == inline.ActiveRuns[i].Name && rw[r.WorkflowFile] {
						inline.ActiveRuns[i].IsRelease = true
					}
				}
			}
		}

		shortName := rc.Repo
		if parts := strings.SplitN(rc.Repo, "/", 2); len(parts) == 2 {
			shortName = parts[1]
		}

		wfGroups := make(map[string]string)
		for _, group := range rc.Groups {
			for _, wf := range group.Workflows {
				wfGroups[wf] = group.Label
			}
		}

		state := views.RepoState{
			RepoName:       shortName,
			FullName:       rc.Repo,
			Runs:           runs,
			PRs:            activePRs,
			ReviewItems:    reviewItems,
			Inline:         inline,
			PRSummary:      views.ComputePRSummary(activePRs),
			WorkflowGroups: wfGroups,
		}

		states = append(states, state)
	}

	views.SortByAttention(states)
	return states
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pollResultMsg:
		return a.handlePollResult(msg)
	case pollErrorMsg:
		a.statusText = fmt.Sprintf("Poll error: %s", msg.Err)
		return a, waitForPoll(a.resultCh)
	case tickMsg:
		views.ClearExpiredNewFlags(a.repos, 30*time.Second)
		a.compactView.UpdateRepos(a.repos)
		return a, tickCmd()
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	case actionResultMsg:
		a.flash.Show(msg.Message, msg.IsError)
		return a, nil
	case diffResultMsg:
		supportsLogPane := a.mode == ui.ViewDetail || a.mode == ui.ViewRunDetail || a.mode == ui.ViewPRDetail
		if !supportsLogPane {
			return a, nil // navigated away, discard
		}
		if msg.Err != nil {
			a.flash.Show("Failed to fetch diff: "+msg.Err.Error(), true)
			return a, nil
		}
		a.logPane.SetContent(msg.Title, msg.Content, false)
		if a.logPane.Mode == components.LogPaneHidden {
			a.logPane.CycleMode()
		}
		if a.mode == ui.ViewPRDetail && a.prDetailView != nil {
			a.prDetailView.SetFiles(views.ParseDiffFiles(msg.Content))
			a.prDetailView.RawDiff = msg.Content
		}
		return a, nil
	case logsResultMsg:
		if a.mode != ui.ViewRunDetail {
			return a, nil
		}
		if msg.Err != nil {
			a.flash.Show("Failed to fetch logs: "+msg.Err.Error(), true)
			return a, nil
		}
		a.logPane.SetContent(msg.Title, msg.Content, false)
		if a.logPane.Mode == components.LogPaneHidden {
			a.logPane.CycleMode()
		}
		return a, nil
	case jobsResultMsg:
		if a.mode != ui.ViewRunDetail || a.runDetailView == nil {
			return a, nil
		}
		if msg.Err != nil {
			a.flash.Show("Failed to fetch jobs: "+msg.Err.Error(), true)
			return a, nil
		}
		a.runDetailView.SetJobs(msg.Jobs)
		return a, a.fetchFailedLogs(msg.Jobs)
	}
	return a, nil
}

func (a App) fetchFailedLogs(jobs []models.Job) tea.Cmd {
	client := a.client
	ctx := a.ctx
	repo := ""
	if a.detailView != nil {
		repo = a.detailView.Repo.FullName
	} else if a.runDetailView != nil {
		repo = a.runDetailView.RepoName
	}
	if repo == "" {
		return nil
	}

	type failedJob struct {
		id   int64
		name string
	}
	var failed []failedJob
	for _, j := range jobs {
		if j.Conclusion == "failure" && j.ID != 0 {
			failed = append(failed, failedJob{id: j.ID, name: j.Name})
		}
	}
	if len(failed) == 0 {
		return nil
	}

	return func() tea.Msg {
		var sb strings.Builder
		for _, fj := range failed {
			logs, err := client.GetFailedLogs(ctx, repo, fj.id)
			if err == nil && logs != "" {
				sb.WriteString(fmt.Sprintf("=== %s ===\n", fj.name))
				sb.WriteString(logs)
				sb.WriteString("\n\n")
			}
		}
		content := sb.String()
		if content == "" {
			content = "No failed job logs available"
		}
		return logsResultMsg{Title: "Job logs", Content: content}
	}
}

func (a App) handlePollResult(msg pollResultMsg) (tea.Model, tea.Cmd) {
	result := msg.Result

	if result.Error != nil {
		switch result.Error.(type) {
		case *ghclient.AuthError:
			a.statusText = "AUTH FAILED — check token"
			return a, waitForPoll(a.resultCh)
		case *ghclient.RateLimitError:
			a.statusText = "Rate limited — backing off"
			return a, waitForPoll(a.resultCh)
		}
	}

	a.allRuns[result.Repo] = result.Runs
	a.allPulls[result.Repo] = result.PullRequests
	a.rateLimit = result.RateLimitRemaining
	a.lastPoll = time.Now()

	if a.db != nil {
		for _, run := range result.Runs {
			if err := a.db.UpsertRun(run); err != nil {
				a.dbErrors++
			}
			if len(run.Jobs) > 0 {
				if err := a.db.UpsertJobs(run.ID, result.Repo, run.Jobs); err != nil {
					a.dbErrors++
				}
			}
		}
		for _, pr := range result.PullRequests {
			if err := a.db.UpsertPull(pr); err != nil {
				a.dbErrors++
			}
		}
		if a.dbErrors > 3 {
			a.flash.Show("DB write errors — check disk space", true)
			a.dbErrors = 0
		}
	}

	prevStates := a.repos
	a.repos = a.buildRepoStates()

	prevByName := make(map[string]views.RepoState)
	for _, r := range prevStates {
		prevByName[r.FullName] = r
	}
	for i := range a.repos {
		if prev, ok := prevByName[a.repos[i].FullName]; ok {
			if views.DetectNewFlag(prev, a.repos[i]) {
				a.repos[i].NewFlag = true
				a.repos[i].LastNotableChange = time.Now()
			} else if prev.NewFlag && !prev.UserAcknowledged {
				a.repos[i].NewFlag = prev.NewFlag
				a.repos[i].LastNotableChange = prev.LastNotableChange
				a.repos[i].UserAcknowledged = prev.UserAcknowledged
			}
		}
	}

	a.compactView.UpdateRepos(a.repos)

	if a.mode == ui.ViewDetail && a.detailView != nil {
		for _, r := range a.repos {
			if r.FullName == a.detailView.Repo.FullName {
				cursorPos := a.detailView.Cursor.Index()
				a.detailView = views.NewDetailView(r)
				for i := 0; i < cursorPos && i < a.detailView.Cursor.Count()-1; i++ {
					a.detailView.Cursor.Next()
				}
				break
			}
		}
	}

	// Preserve run detail view on poll update
	if a.mode == ui.ViewRunDetail && a.runDetailView != nil {
		found := false
		for _, runs := range a.allRuns {
			for _, r := range runs {
				if r.ID == a.runDetailView.Run.ID {
					cursorPos := a.runDetailView.Cursor.Index()
					// Poll results don't include jobs — preserve already-fetched jobs
					existingJobs := a.runDetailView.Run.Jobs
					a.runDetailView.Run = r
					if len(r.Jobs) > 0 {
						a.runDetailView.SetJobs(r.Jobs)
					} else if len(existingJobs) > 0 {
						a.runDetailView.Run.Jobs = existingJobs
					}
					for i := 0; i < cursorPos && i < a.runDetailView.Cursor.Count()-1; i++ {
						a.runDetailView.Cursor.Next()
					}
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			a.flash.Show("Run no longer available", true)
			a.mode = ui.ViewDetail
			a.runDetailView = nil
		}
	}

	// Preserve PR detail view on poll update
	if a.mode == ui.ViewPRDetail && a.prDetailView != nil {
		found := false
		for _, pulls := range a.allPulls {
			for _, p := range pulls {
				if p.Number == a.prDetailView.PR.Number && p.Repo == a.prDetailView.PR.Repo {
					a.prDetailView.PR = p
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			a.flash.Show(fmt.Sprintf("PR #%d was closed", a.prDetailView.PR.Number), true)
			a.mode = ui.ViewDetail
			a.prDetailView = nil
		}
	}

	activeRuns := 0
	for _, runs := range a.allRuns {
		for _, r := range runs {
			if r.IsActive() {
				activeRuns++
			}
		}
	}
	cadence := "idle"
	if activeRuns > 0 {
		cadence = "active"
	}
	interval := a.config.Polling.Idle
	if activeRuns > 0 {
		interval = a.config.Polling.Active
	}
	a.statusText = fmt.Sprintf("%s %ds  rl:%d", cadence, interval, a.rateLimit)

	return a, waitForPoll(a.resultCh)
}

func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.confirmBar.Active {
		handled, cmd := a.confirmBar.HandleKey(msg.String())
		if handled {
			return a, cmd
		}
	}

	if key.Matches(msg, ui.Keys.Help) {
		a.help.Toggle()
		return a, nil
	}
	if a.help.Visible {
		a.help.Visible = false
		return a, nil
	}

	if key.Matches(msg, ui.Keys.Quit) {
		a.quitting = true
		a.cancel()
		return a, tea.Quit
	}

	switch a.mode {
	case ui.ViewCompact:
		return a.handleCompactKey(msg)
	case ui.ViewDetail:
		return a.handleDetailKey(msg)
	case ui.ViewRunDetail:
		return a.handleRunDetailKey(msg)
	case ui.ViewPRDetail:
		return a.handlePRDetailKey(msg)
	}

	return a, nil
}

func (a App) handleCompactKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, ui.Keys.Down):
		a.compactView.Cursor.Next()
		a.compactView.AcknowledgeSelected()
	case key.Matches(msg, ui.Keys.Up):
		a.compactView.Cursor.Prev()
		a.compactView.AcknowledgeSelected()
	case key.Matches(msg, ui.Keys.DrillIn):
		if r := a.compactView.SelectedRepo(); r != nil {
			a.compactView.AcknowledgeSelected()
			a.mode = ui.ViewDetail
			a.detailView = views.NewDetailView(*r)
		}
	case key.Matches(msg, ui.Keys.Action1):
		return a.handleBatchMerge()
	}
	return a, nil
}

func (a App) handleDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, ui.Keys.Back) {
		if a.logPane.Mode != components.LogPaneHidden {
			a.logPane.Mode = components.LogPaneHidden
			return a, nil
		}
		a.mode = ui.ViewCompact
		a.detailView = nil
		a.logPane.Clear()
		return a, nil
	}

	if key.Matches(msg, ui.Keys.Examine) {
		a.logPane.CycleMode()
		return a, nil
	}

	// When log pane is visible, arrow keys scroll log
	if a.logPane.Mode != components.LogPaneHidden {
		if msg.String() == "up" {
			a.logPane.ScrollPos--
			if a.logPane.ScrollPos < 0 {
				a.logPane.ScrollPos = 0
			}
			return a, nil
		}
		if msg.String() == "down" {
			a.logPane.ScrollPos++
			return a, nil
		}
	}

	switch {
	case key.Matches(msg, ui.Keys.Down):
		a.detailView.Cursor.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.detailView.Cursor.Prev()

	case key.Matches(msg, ui.Keys.DrillIn):
		return a.handleDetailDrillIn()

	case key.Matches(msg, ui.Keys.Action1):
		if run := a.detailView.SelectedRun(); run != nil {
			repo := a.detailView.Repo.FullName
			if a.agentWorkflows[repo][run.WorkflowFile] {
				return a.handleDispatchAgent(repo, run)
			}
			return a.handleRerun(run)
		}
		if item := a.detailView.SelectedReviewItem(); item != nil {
			return a.handleApprove(&item.PR)
		}
	case key.Matches(msg, ui.Keys.Action2):
		return a.handleViewDiff()
	case key.Matches(msg, ui.Keys.Action3):
		if item := a.detailView.SelectedReviewItem(); item != nil {
			return a.handleDismiss(&item.PR)
		}

	case key.Matches(msg, ui.Keys.Remote):
		return a.handleOpen()
	}

	return a, nil
}

func (a App) handleDetailDrillIn() (tea.Model, tea.Cmd) {
	if a.detailView == nil {
		return a, nil
	}

	if run := a.detailView.SelectedRun(); run != nil {
		a.mode = ui.ViewRunDetail
		a.runDetailView = views.NewRunDetailView(*run, a.detailView.Repo.FullName)
		var cmds []tea.Cmd

		if len(run.Jobs) == 0 {
			client := a.client
			ctx := a.ctx
			repo := a.detailView.Repo.FullName
			runID := run.ID
			cmds = append(cmds, func() tea.Msg {
				jobs, err := client.GetJobs(ctx, repo, runID)
				if err != nil {
					return jobsResultMsg{RunID: runID, Err: err}
				}
				return jobsResultMsg{RunID: runID, Jobs: jobs}
			})
		} else {
			cmds = append(cmds, a.fetchFailedLogs(run.Jobs))
		}
		return a, tea.Batch(cmds...)
	}

	if item := a.detailView.SelectedReviewItem(); item != nil {
		a.mode = ui.ViewPRDetail
		a.prDetailView = views.NewPRDetailView(item.PR, a.detailView.Repo.FullName)

		client := a.client
		ctx := a.ctx
		repo := a.detailView.Repo.FullName
		number := item.PR.Number
		return a, func() tea.Msg {
			diff, err := client.GetPullDiff(ctx, repo, number)
			if err != nil {
				return diffResultMsg{Err: err}
			}
			return diffResultMsg{Title: fmt.Sprintf("PR #%d diff", number), Content: diff}
		}
	}

	return a, nil
}

func (a App) handleRunDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, ui.Keys.Back) {
		if a.logPane.Mode != components.LogPaneHidden {
			a.logPane.Mode = components.LogPaneHidden
			return a, nil
		}
		a.mode = ui.ViewDetail
		a.runDetailView = nil
		a.logPane.Clear()
		return a, nil
	}

	if key.Matches(msg, ui.Keys.Examine) {
		a.logPane.CycleMode()
		return a, nil
	}

	if a.logPane.Mode != components.LogPaneHidden {
		if msg.String() == "up" {
			a.logPane.ScrollPos--
			if a.logPane.ScrollPos < 0 {
				a.logPane.ScrollPos = 0
			}
			return a, nil
		}
		if msg.String() == "down" {
			a.logPane.ScrollPos++
			return a, nil
		}
	}

	switch {
	case key.Matches(msg, ui.Keys.Down):
		a.runDetailView.Cursor.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.runDetailView.Cursor.Prev()
	case key.Matches(msg, ui.Keys.DrillIn):
		a.runDetailView.ToggleExpand()

	case key.Matches(msg, ui.Keys.Action1):
		return a.handleRerunFromRunDetail()
	case key.Matches(msg, ui.Keys.Action2):
		return a.handleRerunFailedFromRunDetail()

	case key.Matches(msg, ui.Keys.Remote):
		if a.runDetailView.Run.HTMLURL != "" {
			go openBrowser(a.runDetailView.Run.HTMLURL)
		}
	}
	return a, nil
}

func (a App) handlePRDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, ui.Keys.Back) {
		if a.logPane.Mode != components.LogPaneHidden {
			a.logPane.Mode = components.LogPaneHidden
			return a, nil
		}
		a.mode = ui.ViewDetail
		a.prDetailView = nil
		a.logPane.Clear()
		return a, nil
	}

	if key.Matches(msg, ui.Keys.Examine) {
		a.logPane.CycleMode()
		return a, nil
	}

	if a.logPane.Mode != components.LogPaneHidden {
		if msg.String() == "up" {
			a.logPane.ScrollPos--
			if a.logPane.ScrollPos < 0 {
				a.logPane.ScrollPos = 0
			}
			return a, nil
		}
		if msg.String() == "down" {
			a.logPane.ScrollPos++
			return a, nil
		}
	}

	switch {
	case key.Matches(msg, ui.Keys.Down):
		a.prDetailView.Cursor.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.prDetailView.Cursor.Prev()
	case key.Matches(msg, ui.Keys.DrillIn):
		if f := a.prDetailView.SelectedFile(); f != nil {
			a.logPane.ScrollPos = f.Offset
			if a.logPane.Mode == components.LogPaneHidden {
				a.logPane.CycleMode()
			}
		}

	case key.Matches(msg, ui.Keys.Action1):
		return a.handleApproveFromPRDetail()
	case key.Matches(msg, ui.Keys.Action2):
		return a.handleMergeFromPRDetail()
	case key.Matches(msg, ui.Keys.Action3):
		return a.handleDismissFromPRDetail()

	case key.Matches(msg, ui.Keys.Remote):
		if a.prDetailView.PR.HTMLURL != "" {
			go openBrowser(a.prDetailView.PR.HTMLURL)
		}
	}
	return a, nil
}

// --- Drill-down action handlers ---

func (a App) handleRerunFromRunDetail() (tea.Model, tea.Cmd) {
	if a.runDetailView == nil {
		return a, nil
	}
	client := a.client
	ctx := a.ctx
	repo := a.runDetailView.RepoName
	runID := a.runDetailView.Run.ID
	return a, func() tea.Msg {
		if err := client.Rerun(ctx, repo, runID); err != nil {
			return actionResultMsg{Message: "Rerun failed: " + err.Error(), IsError: true}
		}
		return actionResultMsg{Message: "Rerun triggered", IsError: false}
	}
}

func (a App) handleRerunFailedFromRunDetail() (tea.Model, tea.Cmd) {
	if a.runDetailView == nil {
		return a, nil
	}
	client := a.client
	ctx := a.ctx
	repo := a.runDetailView.RepoName
	runID := a.runDetailView.Run.ID
	return a, func() tea.Msg {
		if err := client.RerunFailed(ctx, repo, runID); err != nil {
			return actionResultMsg{Message: "Rerun failed: " + err.Error(), IsError: true}
		}
		return actionResultMsg{Message: "Rerun failed jobs triggered", IsError: false}
	}
}

func (a App) handleApproveFromPRDetail() (tea.Model, tea.Cmd) {
	if a.prDetailView == nil {
		return a, nil
	}
	client := a.client
	ctx := a.ctx
	repo := a.prDetailView.RepoName
	number := a.prDetailView.PR.Number
	return a, func() tea.Msg {
		if err := client.Approve(ctx, repo, number); err != nil {
			return actionResultMsg{Message: "Approve failed: " + err.Error(), IsError: true}
		}
		return actionResultMsg{Message: fmt.Sprintf("Approved PR #%d", number), IsError: false}
	}
}

func (a App) handleMergeFromPRDetail() (tea.Model, tea.Cmd) {
	if a.prDetailView == nil {
		return a, nil
	}
	client := a.client
	ctx := a.ctx
	repo := a.prDetailView.RepoName
	number := a.prDetailView.PR.Number
	a.confirmBar.Show(
		fmt.Sprintf("Merge PR #%d? [y/n]", number),
		func() tea.Cmd {
			return func() tea.Msg {
				if err := client.Merge(ctx, repo, number); err != nil {
					return actionResultMsg{Message: "Merge failed: " + err.Error(), IsError: true}
				}
				return actionResultMsg{Message: fmt.Sprintf("Merged PR #%d", number), IsError: false}
			}
		},
		func() tea.Cmd { return nil },
	)
	return a, nil
}

func (a App) handleDismissFromPRDetail() (tea.Model, tea.Cmd) {
	if a.prDetailView == nil {
		return a, nil
	}
	repo := a.prDetailView.RepoName
	number := a.prDetailView.PR.Number
	dismissKey := fmt.Sprintf("%s:%d", repo, number)
	a.dismissed[dismissKey] = true
	if a.db != nil {
		a.db.AddDismissed(repo, number)
	}
	a.flash.Show(fmt.Sprintf("Dismissed PR #%d", number), false)
	a.mode = ui.ViewDetail
	a.prDetailView = nil
	a.repos = a.buildRepoStates()
	a.compactView.UpdateRepos(a.repos)
	return a, nil
}

// --- Action handlers ---

func (a App) handleDispatchAgent(repo string, run *models.WorkflowRun) (tea.Model, tea.Cmd) {
	wf := run.WorkflowFile
	cooldownKey := repo + "/" + wf

	// Guard: already running
	for _, r := range a.allRuns[repo] {
		if r.WorkflowFile == wf && r.IsActive() {
			a.flash.Show(fmt.Sprintf("%s already running", run.Name), false)
			return a, nil
		}
	}

	// Guard: cooldown (5 minutes)
	if last, ok := a.dispatchCooldowns[cooldownKey]; ok {
		elapsed := time.Since(last)
		if elapsed < 5*time.Minute {
			remaining := (5*time.Minute - elapsed).Round(time.Second)
			a.flash.Show(fmt.Sprintf("%s dispatched recently, wait %s", run.Name, remaining), false)
			return a, nil
		}
	}

	a.dispatchCooldowns[cooldownKey] = time.Now()
	branch := a.repoBranch[repo]
	if branch == "" {
		branch = "main"
	}
	client := a.client
	ctx := a.ctx
	name := run.Name
	return a, func() tea.Msg {
		if err := client.DispatchWorkflow(ctx, repo, wf, branch); err != nil {
			return actionResultMsg{Message: "Dispatch failed: " + err.Error(), IsError: true}
		}
		return actionResultMsg{Message: fmt.Sprintf("Dispatched %s", name), IsError: false}
	}
}

func (a App) handleRerun(run *models.WorkflowRun) (tea.Model, tea.Cmd) {
	client := a.client
	ctx := a.ctx
	repo := a.detailView.Repo.FullName
	runID := run.ID
	conclusion := run.Conclusion
	return a, func() tea.Msg {
		var err error
		if conclusion == "failure" {
			err = client.RerunFailed(ctx, repo, runID)
		} else {
			err = client.Rerun(ctx, repo, runID)
		}
		if err != nil {
			return actionResultMsg{Message: "Rerun failed: " + err.Error(), IsError: true}
		}
		return actionResultMsg{Message: "Rerun triggered", IsError: false}
	}
}

func (a App) handleApprove(pr *models.PullRequest) (tea.Model, tea.Cmd) {
	client := a.client
	ctx := a.ctx
	repo := a.detailView.Repo.FullName
	number := pr.Number
	return a, func() tea.Msg {
		if err := client.Approve(ctx, repo, number); err != nil {
			return actionResultMsg{Message: "Approve failed: " + err.Error(), IsError: true}
		}
		return actionResultMsg{Message: fmt.Sprintf("Approved PR #%d", number), IsError: false}
	}
}

func (a App) handleMerge(pr *models.PullRequest) (tea.Model, tea.Cmd) {
	client := a.client
	ctx := a.ctx
	repo := a.detailView.Repo.FullName
	number := pr.Number
	a.confirmBar.Show(
		fmt.Sprintf("Merge PR #%d? [y/n]", number),
		func() tea.Cmd {
			return func() tea.Msg {
				if err := client.Merge(ctx, repo, number); err != nil {
					return actionResultMsg{Message: "Merge failed: " + err.Error(), IsError: true}
				}
				return actionResultMsg{Message: fmt.Sprintf("Merged PR #%d", number), IsError: false}
			}
		},
		func() tea.Cmd { return nil },
	)
	return a, nil
}

func (a App) handleDismiss(pr *models.PullRequest) (tea.Model, tea.Cmd) {
	repo := a.detailView.Repo.FullName
	dismissKey := fmt.Sprintf("%s:%d", repo, pr.Number)
	a.dismissed[dismissKey] = true
	if a.db != nil {
		a.db.AddDismissed(repo, pr.Number)
	}
	a.flash.Show(fmt.Sprintf("Dismissed PR #%d", pr.Number), false)
	a.repos = a.buildRepoStates()
	a.compactView.UpdateRepos(a.repos)
	return a, nil
}

func (a App) handleBatchMerge() (tea.Model, tea.Cmd) {
	var ready []struct {
		repo string
		pr   models.PullRequest
	}
	for _, r := range a.repos {
		for _, pr := range r.PRs {
			if pr.IsAgent && pr.CIStatus == "success" && pr.ReviewState == "approved" && !pr.Draft {
				ready = append(ready, struct {
					repo string
					pr   models.PullRequest
				}{r.FullName, pr})
			}
		}
	}
	if len(ready) == 0 {
		a.flash.Show("No agent PRs ready to merge", false)
		return a, nil
	}
	client := a.client
	ctx := a.ctx
	readyCopy := ready
	a.confirmBar.Show(
		fmt.Sprintf("Merge %d agent PRs? [y/n]", len(readyCopy)),
		func() tea.Cmd {
			return func() tea.Msg {
				merged := 0
				for _, item := range readyCopy {
					if err := client.Merge(ctx, item.repo, item.pr.Number); err == nil {
						merged++
					}
				}
				isErr := merged < len(readyCopy)
				return actionResultMsg{
					Message: fmt.Sprintf("Merged %d/%d agent PRs", merged, len(readyCopy)),
					IsError: isErr,
				}
			}
		},
		func() tea.Cmd { return nil },
	)
	return a, nil
}

func (a App) handleViewDiff() (tea.Model, tea.Cmd) {
	if a.detailView == nil {
		return a, nil
	}
	client := a.client
	ctx := a.ctx
	repo := a.detailView.Repo.FullName

	if item := a.detailView.SelectedReviewItem(); item != nil {
		number := item.PR.Number
		return a, func() tea.Msg {
			diff, err := client.GetPullDiff(ctx, repo, number)
			if err != nil {
				return diffResultMsg{Err: err}
			}
			return diffResultMsg{Title: fmt.Sprintf("PR #%d diff", number), Content: diff}
		}
	}

	if run := a.detailView.SelectedRun(); run != nil {
		jobs := run.Jobs
		return a, func() tea.Msg {
			var logContent strings.Builder
			for _, job := range jobs {
				if job.Conclusion == "failure" && job.ID != 0 {
					logs, err := client.GetFailedLogs(ctx, repo, job.ID)
					if err == nil && logs != "" {
						logContent.WriteString(fmt.Sprintf("=== %s ===\n", job.Name))
						logContent.WriteString(logs)
						logContent.WriteString("\n\n")
					}
				}
			}
			content := logContent.String()
			if content == "" {
				content = "No failed job logs available"
			}
			return diffResultMsg{Title: "Job logs", Content: content}
		}
	}

	return a, nil
}

func (a App) handleOpen() (tea.Model, tea.Cmd) {
	if a.detailView == nil {
		return a, nil
	}
	var url string
	if item := a.detailView.SelectedReviewItem(); item != nil {
		url = item.PR.HTMLURL
	} else if run := a.detailView.SelectedRun(); run != nil {
		url = run.HTMLURL
	}
	if url != "" {
		go openBrowser(url)
	}
	return a, nil
}

// --- View ---

func (a App) View() tea.View {
	if a.quitting {
		return tea.NewView("")
	}

	if a.width < 36 || a.height < 10 {
		msg := fmt.Sprintf("Terminal too small (%d×%d). Need at least 36×10.", a.width, a.height)
		v := tea.NewView(msg)
		v.AltScreen = true
		return v
	}

	contentHeight := a.height - 2

	header := a.renderHeader()

	var content string
	switch a.mode {
	case ui.ViewCompact:
		content = a.compactView.Render(a.width, contentHeight)
	case ui.ViewDetail:
		if a.detailView != nil {
			content = a.detailView.Render(a.width, contentHeight)
		}
	case ui.ViewRunDetail:
		if a.runDetailView != nil {
			content = a.runDetailView.Render(a.width, contentHeight)
		}
	case ui.ViewPRDetail:
		if a.prDetailView != nil {
			content = a.prDetailView.Render(a.width, contentHeight)
		}
	}

	if a.help.Visible {
		viewName := a.mode.String()
		content = a.help.Render(viewName, a.width, contentHeight)
	}

	supportsLogPane := a.mode == ui.ViewDetail || a.mode == ui.ViewRunDetail || a.mode == ui.ViewPRDetail
	if a.logPane.Mode != components.LogPaneHidden && supportsLogPane {
		logHeight := contentHeight / 2
		if a.logPane.Mode == components.LogPaneFull {
			logHeight = contentHeight
		}
		mainHeight := contentHeight - logHeight

		contentLines := strings.Split(content, "\n")
		if len(contentLines) > mainHeight {
			contentLines = contentLines[:mainHeight]
		}
		content = strings.Join(contentLines, "\n")

		logRender := a.logPane.Render(a.width, logHeight)
		content = content + "\n" + logRender
	}

	contentLines := strings.Split(content, "\n")
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}

	footer := a.renderFooter()

	out := header + "\n" + strings.Join(contentLines, "\n") + "\n" + footer
	v := tea.NewView(out)
	v.AltScreen = true
	return v
}

func (a App) renderHeader() string {
	left := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render("CIMON")

	switch a.mode {
	case ui.ViewDetail:
		if a.detailView != nil {
			repoStyle := lipgloss.NewStyle().Foreground(ui.ColorPurple)
			left += " ─ " + repoStyle.Render(a.detailView.Repo.FullName)
		}
	case ui.ViewRunDetail:
		if a.runDetailView != nil {
			repoStyle := lipgloss.NewStyle().Foreground(ui.ColorPurple)
			sectionStyle := lipgloss.NewStyle().Foreground(ui.ColorBlue)
			left += " ─ " + repoStyle.Render(a.runDetailView.RepoName) + " ─ " + sectionStyle.Render("CI Pipeline")
		}
	case ui.ViewPRDetail:
		if a.prDetailView != nil {
			repoStyle := lipgloss.NewStyle().Foreground(ui.ColorPurple)
			sectionStyle := lipgloss.NewStyle().Foreground(ui.ColorBlue)
			left += " ─ " + repoStyle.Render(a.prDetailView.RepoName) + " ─ " + sectionStyle.Render("Pull Request")
		}
	}

	timeStr := time.Now().Format("15:04")
	right := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(timeStr)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	fill := a.width - leftW - rightW - 2
	if fill < 1 {
		fill = 1
	}
	mid := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(strings.Repeat("─", fill))

	return left + " " + mid + " " + right
}

func (a App) renderFooter() string {
	if a.confirmBar.Active {
		return a.confirmBar.Render(a.width)
	}
	if a.flash.Visible() {
		color := ui.ColorGreen
		if a.flash.IsError {
			color = ui.ColorRed
		}
		return lipgloss.NewStyle().Foreground(color).Render(a.flash.Message)
	}

	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	switch a.mode {
	case ui.ViewDetail:
		return muted.Render("[1]rerun/dispatch/approve [2]diff/logs [3]dismiss [e]log [r]github [a]back")
	case ui.ViewRunDetail:
		return muted.Render("[1]rerun [2]rerun-failed [e]logs [r]github [a]back")
	case ui.ViewPRDetail:
		return muted.Render("[1]approve [2]merge [3]dismiss [e]diff [r]github [a]back")
	default:
		return muted.Render(a.statusText)
	}
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
