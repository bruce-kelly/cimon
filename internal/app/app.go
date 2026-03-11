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
	mode        ui.ViewMode
	compactView *views.CompactView
	detailView  *views.DetailView
	repos       []views.RepoState

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
	releaseWorkflows map[string]map[string]bool

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

	releaseWorkflows := make(map[string]map[string]bool)
	for _, rc := range cfg.Repos {
		rw := make(map[string]bool)
		for _, g := range rc.Groups {
			for _, cat := range []string{"release", "deploy"} {
				if strings.Contains(strings.ToLower(g.Label), cat) {
					for _, wf := range g.Workflows {
						rw[wf] = true
					}
				}
			}
		}
		releaseWorkflows[rc.Repo] = rw
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
		allRuns:          make(map[string][]models.WorkflowRun),
		allPulls:         make(map[string][]models.PullRequest),
		releaseWorkflows: releaseWorkflows,
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

		inline := views.ComputeInlineStatus(runs)
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

		state := views.RepoState{
			RepoName:    shortName,
			FullName:    rc.Repo,
			Runs:        runs,
			PRs:         activePRs,
			ReviewItems: reviewItems,
			Inline:      inline,
			PRSummary:   views.ComputePRSummary(activePRs),
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
		a.tickEven = !a.tickEven
		return a, tickCmd()
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	}
	return a, nil
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
		a.confirmBar.HandleKey(msg.String())
		return a, nil
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
	case key.Matches(msg, ui.Keys.Enter):
		if r := a.compactView.SelectedRepo(); r != nil {
			a.compactView.AcknowledgeSelected()
			a.mode = ui.ViewDetail
			a.detailView = views.NewDetailView(*r)
		}
	case key.Matches(msg, ui.Keys.BatchMerge):
		return a.handleBatchMerge()
	}
	return a, nil
}

func (a App) handleDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, ui.Keys.Escape) {
		if a.logPane.Mode != components.LogPaneHidden {
			a.logPane.Mode = components.LogPaneHidden
			return a, nil
		}
		a.mode = ui.ViewCompact
		a.detailView = nil
		a.logPane.Clear()
		return a, nil
	}

	if key.Matches(msg, ui.Keys.LogCycle) {
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

	case key.Matches(msg, ui.Keys.Rerun):
		if run := a.detailView.SelectedRun(); run != nil {
			return a.handleRerun(run)
		}

	case key.Matches(msg, ui.Keys.Approve):
		if item := a.detailView.SelectedReviewItem(); item != nil {
			return a.handleApprove(&item.PR)
		}
	case key.Matches(msg, ui.Keys.Merge):
		if item := a.detailView.SelectedReviewItem(); item != nil {
			return a.handleMerge(&item.PR)
		}
	case key.Matches(msg, ui.Keys.Dismiss):
		if item := a.detailView.SelectedReviewItem(); item != nil {
			return a.handleDismiss(&item.PR)
		}

	case key.Matches(msg, ui.Keys.ViewDiff):
		return a.handleViewDiff()

	case key.Matches(msg, ui.Keys.Open):
		return a.handleOpen()
	}

	return a, nil
}

// --- Action handlers ---

func (a App) handleRerun(run *models.WorkflowRun) (tea.Model, tea.Cmd) {
	repo := a.detailView.Repo.FullName
	if run.Conclusion == "failure" {
		go func() {
			if err := a.client.RerunFailed(a.ctx, repo, run.ID); err != nil {
				a.flash.Show("Rerun failed: "+err.Error(), true)
			} else {
				a.flash.Show("Rerun triggered", false)
			}
		}()
	} else {
		go func() {
			if err := a.client.Rerun(a.ctx, repo, run.ID); err != nil {
				a.flash.Show("Rerun failed: "+err.Error(), true)
			} else {
				a.flash.Show("Rerun triggered", false)
			}
		}()
	}
	return a, nil
}

func (a App) handleApprove(pr *models.PullRequest) (tea.Model, tea.Cmd) {
	repo := a.detailView.Repo.FullName
	go func() {
		if err := a.client.Approve(a.ctx, repo, pr.Number); err != nil {
			a.flash.Show("Approve failed: "+err.Error(), true)
		} else {
			a.flash.Show(fmt.Sprintf("Approved PR #%d", pr.Number), false)
		}
	}()
	return a, nil
}

func (a App) handleMerge(pr *models.PullRequest) (tea.Model, tea.Cmd) {
	repo := a.detailView.Repo.FullName
	a.confirmBar.Show(
		fmt.Sprintf("Merge PR #%d? [y/n]", pr.Number),
		func() {
			go func() {
				if err := a.client.Merge(a.ctx, repo, pr.Number); err != nil {
					a.flash.Show("Merge failed: "+err.Error(), true)
				} else {
					a.flash.Show(fmt.Sprintf("Merged PR #%d", pr.Number), false)
				}
			}()
		},
		func() {},
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
	a.confirmBar.Show(
		fmt.Sprintf("Merge %d agent PRs? [y/n]", len(ready)),
		func() {
			go func() {
				merged := 0
				for _, item := range ready {
					if err := a.client.Merge(a.ctx, item.repo, item.pr.Number); err == nil {
						merged++
					}
				}
				a.flash.Show(fmt.Sprintf("Merged %d/%d agent PRs", merged, len(ready)), merged < len(ready))
			}()
		},
		func() {},
	)
	return a, nil
}

func (a App) handleViewDiff() (tea.Model, tea.Cmd) {
	if a.detailView == nil {
		return a, nil
	}
	repo := a.detailView.Repo.FullName

	if item := a.detailView.SelectedReviewItem(); item != nil {
		go func() {
			diff, err := a.client.GetPullDiff(a.ctx, repo, item.PR.Number)
			if err != nil {
				a.flash.Show("Failed to fetch diff: "+err.Error(), true)
				return
			}
			a.logPane.SetContent(fmt.Sprintf("PR #%d diff", item.PR.Number), diff, false)
			if a.logPane.Mode == components.LogPaneHidden {
				a.logPane.CycleMode()
			}
		}()
	} else if run := a.detailView.SelectedRun(); run != nil {
		go func() {
			var logContent strings.Builder
			for _, job := range run.Jobs {
				if job.Conclusion == "failure" && job.ID != 0 {
					logs, err := a.client.GetFailedLogs(a.ctx, repo, job.ID)
					if err == nil && logs != "" {
						logContent.WriteString(fmt.Sprintf("=== %s ===\n", job.Name))
						logContent.WriteString(logs)
						logContent.WriteString("\n\n")
					}
				}
			}
			if logContent.Len() > 0 {
				a.logPane.SetContent("Job logs", logContent.String(), false)
			} else {
				a.logPane.SetContent("Job logs", "No failed job logs available", false)
			}
			if a.logPane.Mode == components.LogPaneHidden {
				a.logPane.CycleMode()
			}
		}()
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
	}

	if a.help.Visible {
		viewName := a.mode.String()
		content = a.help.Render(viewName, a.width, contentHeight)
	}

	if a.logPane.Mode != components.LogPaneHidden && a.mode == ui.ViewDetail {
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

	if a.mode == ui.ViewDetail && a.detailView != nil {
		repoStyle := lipgloss.NewStyle().Foreground(ui.ColorPurple)
		left += " ─ " + repoStyle.Render(a.detailView.Repo.FullName)
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

	if a.mode == ui.ViewDetail {
		muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
		return muted.Render("[r]rerun [A]approve [m]merge [v]diff [o]browser [Esc]back")
	}

	return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(a.statusText)
}

// --- Kept from v1 ---

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
