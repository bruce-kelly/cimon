# Drill-Down Views Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Run Detail and PR Detail views as a third drill-down level, remap keybindings to left-hand WASD+numbered scheme, and fix the goroutine mutation bug.

**Architecture:** Three phases — (1) fix async action mutations via tea.Cmd/Msg pattern, (2) overhaul keybindings to WASD+1234ER, (3) add ViewRunDetail and ViewPRDetail view modes with new view structs. Each phase builds on the previous.

**Tech Stack:** Go, Bubbletea v2 (`charm.land/bubbletea/v2`), Lipgloss v2, testify/assert

**Spec:** `docs/superpowers/specs/2026-03-11-drill-down-views-design.md`

---

## Chunk 1: Fix Goroutine Mutation Bug (Phase 1)

### Task 1: Change ConfirmBar to return tea.Cmd

**Files:**
- Modify: `internal/ui/components/confirmbar.go`
- Modify: `internal/ui/components/components_test.go`

- [ ] **Step 1: Write the failing tests for new ConfirmBar signatures**

Add to `internal/ui/components/components_test.go`:

```go
func TestConfirmBar_ShowAndConfirmReturnsCmd(t *testing.T) {
	c := &ConfirmBar{}
	cmdCalled := false
	c.Show("Do it?",
		func() tea.Cmd { cmdCalled = true; return nil },
		func() tea.Cmd { return nil },
	)
	assert.True(t, c.Active)

	consumed, cmd := c.HandleKey("y")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cmdCalled)
	assert.Nil(t, cmd) // callback returned nil
}

func TestConfirmBar_CancelReturnsCmd(t *testing.T) {
	c := &ConfirmBar{}
	cancelCalled := false
	c.Show("Do it?",
		func() tea.Cmd { return nil },
		func() tea.Cmd { cancelCalled = true; return nil },
	)

	consumed, cmd := c.HandleKey("n")
	assert.True(t, consumed)
	assert.False(t, c.Active)
	assert.True(t, cancelCalled)
	assert.Nil(t, cmd)
}

func TestConfirmBar_HandleKeyReturnsNilCmdForOtherKeys(t *testing.T) {
	c := &ConfirmBar{}
	c.Show("Do it?",
		func() tea.Cmd { return nil },
		func() tea.Cmd { return nil },
	)

	consumed, cmd := c.HandleKey("x")
	assert.True(t, consumed)  // consumed while active
	assert.True(t, c.Active)  // still active
	assert.Nil(t, cmd)
}

func TestConfirmBar_InactiveReturnsNilCmd(t *testing.T) {
	c := &ConfirmBar{}
	consumed, cmd := c.HandleKey("y")
	assert.False(t, consumed)
	assert.Nil(t, cmd)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/components/ -run TestConfirmBar_ShowAndConfirmReturnsCmd -v -count=1`
Expected: Compilation error — `Show` and `HandleKey` signatures don't match.

- [ ] **Step 3: Update ConfirmBar signatures**

Replace the full contents of `internal/ui/components/confirmbar.go`:

```go
package components

import tea "charm.land/bubbletea/v2"

// ConfirmBar shows a y/n confirmation prompt.
type ConfirmBar struct {
	Active    bool
	Message   string
	OnConfirm func() tea.Cmd
	OnCancel  func() tea.Cmd
}

func (c *ConfirmBar) Show(message string, onConfirm, onCancel func() tea.Cmd) {
	c.Active = true
	c.Message = message
	c.OnConfirm = onConfirm
	c.OnCancel = onCancel
}

func (c *ConfirmBar) HandleKey(keyStr string) (bool, tea.Cmd) {
	if !c.Active {
		return false, nil
	}
	switch keyStr {
	case "y":
		c.Active = false
		if c.OnConfirm != nil {
			return true, c.OnConfirm()
		}
		return true, nil
	case "n", "esc":
		c.Active = false
		if c.OnCancel != nil {
			return true, c.OnCancel()
		}
		return true, nil
	}
	return true, nil // consume all keys while active
}

func (c *ConfirmBar) Render(width int) string {
	if !c.Active {
		return ""
	}
	return " " + c.Message + " [y/n/Esc]"
}
```

- [ ] **Step 4: Update old ConfirmBar tests to match new signatures**

Update the existing tests in `components_test.go` — change `func()` callbacks to `func() tea.Cmd`, and `c.HandleKey(...)` to `consumed, _ := c.HandleKey(...)`:

- `TestConfirmBar_ShowAndConfirm`: callback returns `func() tea.Cmd { confirmed = true; return nil }`, use `consumed, _ :=`
- `TestConfirmBar_ShowAndCancel`: same pattern
- `TestConfirmBar_EscCancels`: same pattern
- `TestConfirmBar_ConsumesOtherKeys`: use `consumed, _ :=`
- `TestConfirmBar_InactiveIgnoresKeys`: use `consumed, _ :=`
- `TestConfirmBar_RenderActive`: callback nil → `func() tea.Cmd { return nil }`
- `TestConfirmBar_RenderInactive`: no change needed

Add `tea "charm.land/bubbletea/v2"` to the import block.

- [ ] **Step 5: Run all component tests**

Run: `go test ./internal/ui/components/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/components/confirmbar.go internal/ui/components/components_test.go
git commit -m "refactor: change ConfirmBar callbacks to return tea.Cmd

HandleKey now returns (bool, tea.Cmd) so callers can propagate
async commands through Bubbletea's Update cycle."
```

### Task 2: Add ViewMode constants and stub view types

The remaining tasks in Chunks 1-3 reference `ui.ViewRunDetail`, `ui.ViewPRDetail`, `views.RunDetailView`, and `views.PRDetailView`. These must exist before any code can reference them.

**Files:**
- Modify: `internal/ui/app.go`
- Create: `internal/ui/views/rundetail.go` (stub)
- Create: `internal/ui/views/prdetail.go` (stub)

- [ ] **Step 1: Expand ViewMode enum**

Replace `internal/ui/app.go`:

```go
package ui

// ViewMode represents which view is active in the TUI.
type ViewMode int

const (
	ViewCompact ViewMode = iota
	ViewDetail
	ViewRunDetail
	ViewPRDetail
)

func (v ViewMode) String() string {
	switch v {
	case ViewCompact:
		return "compact"
	case ViewDetail:
		return "detail"
	case ViewRunDetail:
		return "run-detail"
	case ViewPRDetail:
		return "pr-detail"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 2: Create RunDetailView stub**

Create `internal/ui/views/rundetail.go` with minimal types so `app.go` can reference it:

```go
package views

import (
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// RunDetailView renders the drill-down into a single workflow run.
// Full implementation in a later task.
type RunDetailView struct {
	Run      models.WorkflowRun
	RepoName string
	Cursor   components.Selector
	expanded map[int]bool
}

// SetJobs populates jobs (after async fetch) and auto-expands failures.
func (rv *RunDetailView) SetJobs(jobs []models.Job) {
	rv.Run.Jobs = jobs
	rv.expanded = make(map[int]bool)
	rv.Cursor.SetCount(len(jobs))
	for i, job := range jobs {
		if job.Conclusion == "failure" {
			rv.expanded[i] = true
		}
	}
}
```

- [ ] **Step 3: Create PRDetailView stub**

Create `internal/ui/views/prdetail.go` with minimal types:

```go
package views

import (
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// PRDetailView renders the drill-down into a single pull request.
// Full implementation in a later task.
type PRDetailView struct {
	PR       models.PullRequest
	RepoName string
	Files    []DiffFile
	RawDiff  string
	Cursor   components.Selector
}

// SetFiles populates the file list (after diff fetch) and sets cursor count.
func (pv *PRDetailView) SetFiles(files []DiffFile) {
	pv.Files = files
	pv.Cursor.SetCount(len(files))
}
```

Note: `DiffFile` doesn't exist yet either. Add a minimal definition at the top of `prdetail.go` for now — it will be moved to `diffparse.go` in a later task:

```go
// DiffFile represents one file from a unified diff (stub — moved to diffparse.go later).
type DiffFile struct {
	Path      string
	Additions int
	Deletions int
	Offset    int
}
```

Actually, since `diffparse.go` will be in the same package, define `DiffFile` in a separate stub file to avoid redeclaration:

Create `internal/ui/views/diffparse.go` with just the type:

```go
package views

// DiffFile represents one file from a unified diff.
type DiffFile struct {
	Path      string
	Additions int
	Deletions int
	Offset    int // line offset into raw diff for log pane scroll
}
```

Then `prdetail.go` can reference `DiffFile` without defining it.

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: Compiles cleanly. All existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/views/rundetail.go internal/ui/views/prdetail.go internal/ui/views/diffparse.go
git commit -m "refactor: add ViewRunDetail/ViewPRDetail modes and stub view types

Unblocks forward references in upcoming tasks. Full implementations
follow in later tasks."
```

### Task 3: Add result message types to app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add message types after the existing pollErrorMsg**

Add after line 34 (`type pollErrorMsg struct { Err error }`) in `internal/app/app.go`:

```go
// actionResultMsg carries the result of an async action (rerun, approve, merge, etc).
type actionResultMsg struct {
	Message string
	IsError bool
}

// diffResultMsg carries fetched content for the log pane.
// Used for both PR diffs (from ViewDetail and ViewPRDetail) and
// job logs (from ViewDetail's handleViewDiff). The handler always
// populates the log pane; it also parses file list when in ViewPRDetail.
type diffResultMsg struct {
	Title   string
	Content string
	Err     error
}

// logsResultMsg carries fetched job logs.
type logsResultMsg struct {
	Title   string
	Content string
	Err     error
}

// jobsResultMsg carries fetched jobs for a run.
type jobsResultMsg struct {
	RunID int64
	Jobs  []models.Job
	Err   error
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/app/`
Expected: Compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "refactor: add result message types for async actions"
```

### Task 3: Convert action handlers to tea.Cmd pattern

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Write tests for actionResultMsg handling**

Add to `internal/app/app_test.go`:

```go
func TestUpdate_ActionResultMsg_ShowsFlash(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	msg := actionResultMsg{Message: "Rerun triggered", IsError: false}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "Rerun triggered", a.flash.Message)
	assert.False(t, a.flash.IsError)
}

func TestUpdate_ActionResultMsg_Error(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	msg := actionResultMsg{Message: "Rerun failed: 404", IsError: true}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "Rerun failed: 404", a.flash.Message)
	assert.True(t, a.flash.IsError)
}

func TestUpdate_DiffResultMsg_PopulatesLogPane(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	msg := diffResultMsg{Title: "PR #42 diff", Content: "+added line\n-removed line"}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "PR #42 diff", a.logPane.Title)
	assert.Contains(t, a.logPane.Content, "+added line")
}

func TestUpdate_DiffResultMsg_Error(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	msg := diffResultMsg{Err: fmt.Errorf("404 not found")}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Contains(t, a.flash.Message, "404 not found")
	assert.True(t, a.flash.IsError)
}

func TestUpdate_LogsResultMsg_IgnoredWhenNotInRunDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewCompact // not run detail

	msg := logsResultMsg{Title: "logs", Content: "some logs"}
	m, _ := a.Update(msg)
	a = m.(App)
	assert.Equal(t, "", a.logPane.Title) // not populated
}
```

Add `"fmt"` to the import block in `app_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run TestUpdate_ActionResultMsg -v -count=1`
Expected: FAIL — Update doesn't handle these message types yet.

- [ ] **Step 3: Add message handlers to Update()**

In `internal/app/app.go`, add cases to the `switch msg := msg.(type)` in `Update()`, after the `tea.KeyPressMsg` case:

```go
	case actionResultMsg:
		a.flash.Show(msg.Message, msg.IsError)
		return a, nil
	case diffResultMsg:
		if msg.Err != nil {
			a.flash.Show("Failed to fetch diff: "+msg.Err.Error(), true)
			return a, nil
		}
		a.logPane.SetContent(msg.Title, msg.Content, false)
		if a.logPane.Mode == components.LogPaneHidden {
			a.logPane.CycleMode()
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
```

The stub types from Task 2 (`views.RunDetailView`, `views.PRDetailView`) and `ui.ViewRunDetail` already exist. Add the fields and placeholder method to `App`:

In the `App` struct, add:
```go
	runDetailView  *views.RunDetailView
	prDetailView   *views.PRDetailView
```

Add a placeholder method:
```go
func (a App) fetchFailedLogs(jobs []models.Job) tea.Cmd {
	return nil // placeholder — implemented in Chunk 3
}
```

- [ ] **Step 4: Convert handleRerun to tea.Cmd**

Replace `handleRerun` in `internal/app/app.go`:

```go
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
```

- [ ] **Step 5: Convert handleApprove to tea.Cmd**

Replace `handleApprove`:

```go
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
```

- [ ] **Step 6: Convert handleMerge to tea.Cmd**

Replace `handleMerge`. The `ConfirmBar.OnConfirm` now returns `tea.Cmd`:

```go
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
```

- [ ] **Step 7: Convert handleBatchMerge to tea.Cmd**

Replace `handleBatchMerge`:

```go
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
```

- [ ] **Step 8: Convert handleViewDiff to tea.Cmd**

Replace `handleViewDiff`:

```go
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
```

- [ ] **Step 9: Update handleKey for ConfirmBar's new return type**

In `handleKey`, replace the confirmBar block:

```go
	if a.confirmBar.Active {
		handled, cmd := a.confirmBar.HandleKey(msg.String())
		if handled {
			return a, cmd
		}
	}
```

- [ ] **Step 10: Run all tests**

Run: `go test ./internal/app/ ./internal/ui/components/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 11: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "fix: convert async action handlers to tea.Cmd/Msg pattern

Goroutine mutations on value-receiver copies of App never propagated
back to the real model. Actions now return tea.Cmd that produce result
messages handled in Update(), fixing flash and log pane updates."
```

---

## Chunk 2: Keybinding Overhaul (Phase 2)

### Task 4: Replace KeyMap with WASD scheme

**Files:**
- Modify: `internal/ui/keys.go`

- [ ] **Step 1: Replace keys.go contents**

```go
package ui

import "charm.land/bubbles/v2/key"

// KeyMap defines all keybindings for WASD + numbered action scheme.
type KeyMap struct {
	// Navigation
	Up, Down, DrillIn, Back key.Binding

	// Context-sensitive actions
	Action1, Action2, Action3 key.Binding

	// Fixed actions
	Examine, Remote, Help, Quit key.Binding
}

// Keys is the global keybinding configuration.
var Keys = KeyMap{
	Up:      key.NewBinding(key.WithKeys("w", "up"), key.WithHelp("w/↑", "up")),
	Down:    key.NewBinding(key.WithKeys("s", "down"), key.WithHelp("s/↓", "down")),
	DrillIn: key.NewBinding(key.WithKeys("d", "enter"), key.WithHelp("d/enter", "select")),
	Back:    key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a/esc", "back")),
	Action1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "action 1")),
	Action2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "action 2")),
	Action3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "action 3")),
	Examine: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "examine")),
	Remote:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "github")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/ui/`
Expected: Compiles. (App references to old bindings will break — that's next.)

### Task 5: Update app.go key handlers for new bindings

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Update handleCompactKey**

Replace `handleCompactKey`:

```go
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
```

- [ ] **Step 2: Update handleDetailKey**

Replace `handleDetailKey`:

```go
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
```

- [ ] **Step 3: Add handleDetailDrillIn stub**

This navigates to run or PR detail. For now it's a stub that will be fully implemented in Chunk 3:

```go
func (a App) handleDetailDrillIn() (tea.Model, tea.Cmd) {
	// Implemented in Chunk 3 (drill-down views)
	return a, nil
}
```

- [ ] **Step 4: Update handleKey to use new binding names**

In `handleKey`, the `Help` and `Quit` bindings are the same names so no changes needed there. Just verify `handleKey` still references `ui.Keys.Help` and `ui.Keys.Quit` — these names haven't changed.

- [ ] **Step 5: Update app_test.go for new keybindings**

Update existing tests — `"s"` for down already works (unchanged). The `"enter"` test still works (DrillIn binding includes `"enter"`). The `"esc"` test still works (Back binding includes `"esc"`). Add a test for `"d"` and `"a"`:

```go
func TestHandleKey_DDrillsIn(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	dMsg := tea.KeyPressMsg{}
	dMsg.Text = "d"
	m, _ := a.handleKey(dMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.NotNil(t, a.detailView)
}

func TestHandleKey_AReturnsToCompact(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(views.RepoState{RepoName: "test", FullName: "owner/test"})

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewCompact, a.mode)
	assert.Nil(t, a.detailView)
}

func TestHandleKey_AAtCompactIsNoop(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	assert.Equal(t, ui.ViewCompact, a.mode)

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewCompact, a.mode) // still compact
}
```

- [ ] **Step 6: Run all app tests**

Run: `go test ./internal/app/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/keys.go internal/app/app.go internal/app/app_test.go
git commit -m "refactor: overhaul keybindings to WASD + numbered actions

Left-hand WASD navigation (w/s up/down, d drill-in, a back).
Context-sensitive 1/2/3 action keys, e examine, r github.
Removes j/k/h/l and single-letter action keys."
```

### Task 6: Update help overlay and footer for new bindings

**Files:**
- Modify: `internal/ui/components/help.go`
- Modify: `internal/app/app.go` (renderFooter)

- [ ] **Step 1: Update help.go bindings**

Replace the `bindings` slice in `HelpOverlay.Render()`:

```go
	var bindings []struct{ key, desc string }
	switch viewName {
	case "compact":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate up/down"},
			{"d/enter", "Drill into repo"},
			{"1", "Batch merge ready agent PRs"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	case "detail":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate up/down"},
			{"d/enter", "Drill into run/PR"},
			{"a/esc", "Back to repos"},
			{"1", "Rerun (run) / Approve (PR)"},
			{"2", "View diff / logs"},
			{"3", "Dismiss PR"},
			{"e", "Toggle log pane"},
			{"r", "Open on GitHub"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	case "run-detail":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate jobs"},
			{"d", "Expand/collapse job steps"},
			{"a/esc", "Back to repo"},
			{"1", "Rerun workflow"},
			{"2", "Rerun failed jobs"},
			{"e", "Toggle log pane"},
			{"r", "Open on GitHub"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	case "pr-detail":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate files"},
			{"d", "Jump to file diff"},
			{"a/esc", "Back to repo"},
			{"1", "Approve PR"},
			{"2", "Merge PR"},
			{"3", "Dismiss PR"},
			{"e", "Toggle log pane"},
			{"r", "Open on GitHub"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	default:
		bindings = []struct{ key, desc string }{
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	}
```

- [ ] **Step 2: Update renderFooter in app.go**

Replace `renderFooter`:

```go
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
		return muted.Render("[1]rerun/approve [2]diff/logs [3]dismiss [e]log [r]github [a]back")
	case ui.ViewRunDetail:
		return muted.Render("[1]rerun [2]rerun-failed [e]logs [r]github [a]back")
	case ui.ViewPRDetail:
		return muted.Render("[1]approve [2]merge [3]dismiss [e]diff [r]github [a]back")
	default:
		return muted.Render(a.statusText)
	}
}
```

- [ ] **Step 3: Run all tests**

Run: `go test ./internal/app/ ./internal/ui/components/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/components/help.go internal/app/app.go
git commit -m "feat: update help overlay and footer for WASD keybindings

Context-sensitive help per view mode. Footer shows available actions
with new key scheme."
```

---

## Chunk 3: Drill-Down Views (Phase 3)

### Task 7: (Already done in Task 2 — ViewMode enum and stubs)

This task was moved to Task 2 to unblock forward references. Skip to Task 8.

### Task 8: Implement diff parser

`DiffFile` type already exists in `diffparse.go` from Task 2's stub. This task adds `ParseDiffFiles` and tests.

**Files:**
- Modify: `internal/ui/views/diffparse.go` (add `ParseDiffFiles`)
- Create: `internal/ui/views/diffparse_test.go`

- [ ] **Step 1: Write diff parser tests**

Create `internal/ui/views/diffparse_test.go`:

```go
package views

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffFiles_SingleFile(t *testing.T) {
	raw := `diff --git a/hello.go b/hello.go
index 1234567..abcdef0 100644
--- a/hello.go
+++ b/hello.go
@@ -1,3 +1,5 @@
 package main
+import "fmt"
+
 func main() {
+	fmt.Println("hello")
 }
`
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "hello.go", files[0].Path)
	assert.Equal(t, 3, files[0].Additions)
	assert.Equal(t, 0, files[0].Deletions)
	assert.Equal(t, 0, files[0].Offset)
}

func TestParseDiffFiles_MultipleFiles(t *testing.T) {
	raw := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package foo
+func Foo() {}
 // end
diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -1,4 +1,3 @@
 package bar
-func Old() {}
+func New() {}
-// removed
`
	files := ParseDiffFiles(raw)
	require.Len(t, files, 2)
	assert.Equal(t, "foo.go", files[0].Path)
	assert.Equal(t, 1, files[0].Additions)
	assert.Equal(t, 0, files[0].Deletions)
	assert.Equal(t, "bar.go", files[1].Path)
	assert.Equal(t, 1, files[1].Additions)
	assert.Equal(t, 2, files[1].Deletions)
	assert.Greater(t, files[1].Offset, 0)
}

func TestParseDiffFiles_EmptyDiff(t *testing.T) {
	files := ParseDiffFiles("")
	assert.Empty(t, files)
}

func TestParseDiffFiles_BinaryFile(t *testing.T) {
	raw := `diff --git a/image.png b/image.png
Binary files /dev/null and b/image.png differ
`
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "image.png", files[0].Path)
	assert.Equal(t, 0, files[0].Additions)
	assert.Equal(t, 0, files[0].Deletions)
}

func TestParseDiffFiles_Rename(t *testing.T) {
	raw := `diff --git a/old.go b/new.go
similarity index 100%
rename from old.go
rename to new.go
`
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "new.go", files[0].Path)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/views/ -run TestParseDiffFiles -v -count=1`
Expected: Compilation error — `ParseDiffFiles` not defined.

- [ ] **Step 3: Implement diff parser**

Create `internal/ui/views/diffparse.go`:

```go
package views

import "strings"

// DiffFile represents one file from a unified diff.
type DiffFile struct {
	Path      string
	Additions int
	Deletions int
	Offset    int // line offset into raw diff for log pane scroll
}

// ParseDiffFiles extracts file entries from a unified diff string.
func ParseDiffFiles(raw string) []DiffFile {
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	var files []DiffFile
	var current *DiffFile

	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			// Flush previous
			if current != nil {
				files = append(files, *current)
			}
			current = &DiffFile{
				Path:   parseDiffPath(line),
				Offset: i,
			}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			current.Additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			current.Deletions++
		}
	}
	if current != nil {
		files = append(files, *current)
	}
	return files
}

// parseDiffPath extracts the file path from "diff --git a/path b/path".
func parseDiffPath(line string) string {
	// Format: "diff --git a/path b/path"
	parts := strings.SplitN(line, " b/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	// Fallback: try to extract from a/ prefix
	parts = strings.SplitN(line, " a/", 2)
	if len(parts) == 2 {
		// Get up to next space
		path := parts[1]
		if idx := strings.Index(path, " "); idx >= 0 {
			path = path[:idx]
		}
		return path
	}
	return line
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/views/ -run TestParseDiffFiles -v -count=1`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views/diffparse.go internal/ui/views/diffparse_test.go
git commit -m "feat: add unified diff parser for PR detail file list"
```

### Task 9: Implement RunDetailView

Replaces the stub from Task 2 with the full implementation.

**Files:**
- Modify: `internal/ui/views/rundetail.go` (replace stub with full implementation)
- Create: `internal/ui/views/rundetail_test.go`

- [ ] **Step 1: Write RunDetailView tests**

Create `internal/ui/views/rundetail_test.go`:

```go
package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRun() models.WorkflowRun {
	now := time.Now()
	return models.WorkflowRun{
		ID:           123,
		Name:         "CI",
		HeadBranch:   "main",
		HeadSHA:      "abc123def456",
		Status:       "completed",
		Conclusion:   "failure",
		Event:        "push",
		Actor:        "bfk",
		CreatedAt:    now.Add(-5 * time.Minute),
		UpdatedAt:    now,
		HTMLURL:      "https://github.com/owner/repo/actions/runs/123",
		Repo:         "owner/repo",
		Jobs: []models.Job{
			{ID: 1, Name: "build", Status: "completed", Conclusion: "success",
				StartedAt: timePtr(now.Add(-4 * time.Minute)), CompletedAt: timePtr(now.Add(-3 * time.Minute)),
				RunnerName: "ubuntu-latest"},
			{ID: 2, Name: "test", Status: "completed", Conclusion: "failure",
				StartedAt: timePtr(now.Add(-3 * time.Minute)), CompletedAt: timePtr(now),
				RunnerName: "ubuntu-latest",
				Steps: []models.Step{
					{Name: "Setup", Status: "completed", Conclusion: "success", Number: 1},
					{Name: "Run tests", Status: "completed", Conclusion: "failure", Number: 2},
					{Name: "Cleanup", Status: "completed", Conclusion: "success", Number: 3},
				}},
			{ID: 3, Name: "lint", Status: "completed", Conclusion: "success",
				StartedAt: timePtr(now.Add(-2 * time.Minute)), CompletedAt: timePtr(now.Add(-1 * time.Minute)),
				RunnerName: "ubuntu-latest"},
		},
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestRunDetailView_Render(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 30)

	assert.Contains(t, out, "main")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "push")
	assert.Contains(t, out, "bfk")
	assert.Contains(t, out, "Jobs")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "test")
	assert.Contains(t, out, "lint")
}

func TestRunDetailView_StatsLine(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 30)

	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "2/3")
}

func TestRunDetailView_CursorNavigatesJobs(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")

	assert.Equal(t, 0, rv.Cursor.Index())
	assert.Equal(t, 3, rv.Cursor.Count())

	rv.Cursor.Next()
	assert.Equal(t, 1, rv.Cursor.Index())

	job := rv.SelectedJob()
	require.NotNil(t, job)
	assert.Equal(t, "test", job.Name)
}

func TestRunDetailView_ExpandCollapseJob(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")

	rv.Cursor.Next() // select "test" job
	assert.False(t, rv.IsExpanded(1))

	rv.ToggleExpand()
	assert.True(t, rv.IsExpanded(1))

	rv.ToggleExpand()
	assert.False(t, rv.IsExpanded(1))
}

func TestRunDetailView_FailedJobsAutoExpand(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")

	// Job index 1 is "test" with conclusion "failure"
	assert.True(t, rv.IsExpanded(1), "failed jobs should auto-expand")
	assert.False(t, rv.IsExpanded(0), "passing jobs should not auto-expand")
}

func TestRunDetailView_ExpandedRender(t *testing.T) {
	run := testRun()
	rv := NewRunDetailView(run, "owner/repo")
	// Job 1 (test) auto-expanded because it failed
	out := rv.Render(80, 30)

	assert.Contains(t, out, "Setup")
	assert.Contains(t, out, "Run tests")
	assert.Contains(t, out, "Cleanup")
}

func TestRunDetailView_EmptyJobs(t *testing.T) {
	run := models.WorkflowRun{
		ID: 1, Name: "CI", HeadBranch: "main", HeadSHA: "abc123",
		Status: "completed", Conclusion: "success",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 20)
	assert.Contains(t, out, "Loading jobs")
	assert.Equal(t, 0, rv.Cursor.Count())
}

func TestRunDetailView_SetJobs(t *testing.T) {
	run := models.WorkflowRun{
		ID: 1, Name: "CI", HeadBranch: "main", HeadSHA: "abc123",
		Status: "completed", Conclusion: "success",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	rv := NewRunDetailView(run, "owner/repo")
	assert.Equal(t, 0, rv.Cursor.Count())

	jobs := []models.Job{
		{Name: "build", Conclusion: "success"},
		{Name: "test", Conclusion: "failure"},
	}
	rv.SetJobs(jobs)
	assert.Equal(t, 2, rv.Cursor.Count())
	assert.True(t, rv.IsExpanded(1)) // failed auto-expands
}

func TestRunDetailView_ActiveRun(t *testing.T) {
	run := models.WorkflowRun{
		ID: 1, Name: "CI", HeadBranch: "main", HeadSHA: "abc123",
		Status: "in_progress",
		CreatedAt: time.Now().Add(-2 * time.Minute), UpdatedAt: time.Now(),
	}
	rv := NewRunDetailView(run, "owner/repo")
	out := rv.Render(80, 20)
	assert.Contains(t, out, "in_progress")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/views/ -run TestRunDetailView -v -count=1`
Expected: Compilation error — `RunDetailView` not defined.

- [ ] **Step 3: Implement RunDetailView**

Create `internal/ui/views/rundetail.go`:

```go
package views

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// RunDetailView renders the drill-down into a single workflow run.
type RunDetailView struct {
	Run      models.WorkflowRun
	RepoName string
	Cursor   components.Selector
	expanded map[int]bool // job index → expanded
}

// NewRunDetailView creates a run detail view, auto-expanding failed jobs.
func NewRunDetailView(run models.WorkflowRun, repoName string) *RunDetailView {
	rv := &RunDetailView{
		Run:      run,
		RepoName: repoName,
		expanded: make(map[int]bool),
	}
	rv.Cursor.SetCount(len(run.Jobs))
	for i, job := range run.Jobs {
		if job.Conclusion == "failure" {
			rv.expanded[i] = true
		}
	}
	return rv
}

// SelectedJob returns the job at the cursor, or nil if no jobs.
func (rv *RunDetailView) SelectedJob() *models.Job {
	if len(rv.Run.Jobs) == 0 {
		return nil
	}
	return &rv.Run.Jobs[rv.Cursor.Index()]
}

// IsExpanded returns whether the job at index has its steps expanded.
func (rv *RunDetailView) IsExpanded(index int) bool {
	return rv.expanded[index]
}

// ToggleExpand toggles step expansion for the selected job.
func (rv *RunDetailView) ToggleExpand() {
	idx := rv.Cursor.Index()
	rv.expanded[idx] = !rv.expanded[idx]
}

// SetJobs populates jobs (after async fetch) and auto-expands failures.
func (rv *RunDetailView) SetJobs(jobs []models.Job) {
	rv.Run.Jobs = jobs
	rv.expanded = make(map[int]bool) // clear stale expansions
	rv.Cursor.SetCount(len(jobs))
	for i, job := range jobs {
		if job.Conclusion == "failure" {
			rv.expanded[i] = true
		}
	}
}

// Render draws the run detail view.
func (rv *RunDetailView) Render(width, height int) string {
	var lines []string

	// Run summary
	dot := runStatusDot(rv.Run)
	sha := rv.Run.HeadSHA
	if len(sha) > 6 {
		sha = sha[:6]
	}
	ago := components.FormatTimeAgo(rv.Run.UpdatedAt)
	summary := fmt.Sprintf("  %s %s %s  %s by %s", rv.Run.HeadBranch, dot, sha, rv.Run.Event, rv.Run.Actor)
	lines = append(lines, summary+lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  "+ago))

	// Stats line
	stats := rv.statsLine()
	lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  "+stats))
	lines = append(lines, "")

	// Jobs section
	jobsHeader := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  Jobs")
	lines = append(lines, jobsHeader)

	if len(rv.Run.Jobs) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  Loading jobs..."))
	}
	for i, job := range rv.Run.Jobs {
		selected := rv.Cursor.Index() == i
		lines = append(lines, rv.renderJobLine(job, selected, width))
		if rv.expanded[i] && len(job.Steps) > 0 {
			for _, step := range job.Steps {
				lines = append(lines, rv.renderStepLine(step))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (rv *RunDetailView) statsLine() string {
	status := rv.Run.Conclusion
	if rv.Run.IsActive() {
		status = rv.Run.Status
	}

	passed := 0
	total := len(rv.Run.Jobs)
	for _, j := range rv.Run.Jobs {
		if j.Conclusion == "success" {
			passed++
		}
	}

	elapsed := components.FormatDuration(rv.Run.Elapsed())

	if total == 0 {
		return fmt.Sprintf("%s · elapsed %s", status, elapsed)
	}
	return fmt.Sprintf("%s · %d/%d jobs passed · elapsed %s", status, passed, total, elapsed)
}

func (rv *RunDetailView) renderJobLine(job models.Job, selected bool, width int) string {
	dot := jobStatusDot(job)
	elapsed := ""
	if job.StartedAt != nil && job.CompletedAt != nil {
		elapsed = components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
	}

	runner := ""
	if job.RunnerName != "" {
		runner = lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  " + job.RunnerName)
	}

	line := fmt.Sprintf("  %s %s  %s%s", dot, job.Name, elapsed, runner)
	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func (rv *RunDetailView) renderStepLine(step models.Step) string {
	dot := stepStatusDot(step)
	return fmt.Sprintf("      %s %s", dot, step.Name)
}

func runStatusDot(run models.WorkflowRun) string {
	if run.IsActive() {
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●")
	}
	switch run.Conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	}
}

func jobStatusDot(job models.Job) string {
	if job.Status == "in_progress" {
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●")
	}
	switch job.Conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	}
}

func stepStatusDot(step models.Step) string {
	switch step.Conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	case "skipped":
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("⧗")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/views/ -run TestRunDetailView -v -count=1`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views/rundetail.go internal/ui/views/rundetail_test.go
git commit -m "feat: add RunDetailView for CI run drill-down

Shows run summary, job list with status/durations/runners, expandable
steps. Failed jobs auto-expand. Supports async job population via
SetJobs()."
```

### Task 10: Implement PRDetailView

Replaces the stub from Task 2 with the full implementation.

**Files:**
- Modify: `internal/ui/views/prdetail.go` (replace stub with full implementation)
- Create: `internal/ui/views/prdetail_test.go`

- [ ] **Step 1: Write PRDetailView tests**

Create `internal/ui/views/prdetail_test.go`:

```go
package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPR() models.PullRequest {
	return models.PullRequest{
		Number:      142,
		Title:       "Fix authentication timeout handling",
		Author:      "bfk",
		Repo:        "owner/repo",
		HTMLURL:     "https://github.com/owner/repo/pull/142",
		State:       "open",
		CIStatus:    "success",
		ReviewState: "approved",
		IsAgent:     true,
		AgentSource: "body",
		Additions:   47,
		Deletions:   12,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   time.Now().Add(-1 * time.Hour),
	}
}

func TestPRDetailView_Render(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)

	assert.Contains(t, out, "#142")
	assert.Contains(t, out, "Fix authentication timeout handling")
	assert.Contains(t, out, "bfk")
	assert.Contains(t, out, "+47")
	assert.Contains(t, out, "-12")
	assert.Contains(t, out, "agent")
	assert.Contains(t, out, "approved")
}

func TestPRDetailView_CIStatusRendered(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.Contains(t, out, "CI")
}

func TestPRDetailView_NoFilesBeforeDiff(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.Contains(t, out, "Loading diff")
	assert.Equal(t, 0, pv.Cursor.Count())
}

func TestPRDetailView_SetFiles(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")

	files := []DiffFile{
		{Path: "internal/auth/client.go", Additions: 31, Deletions: 8, Offset: 0},
		{Path: "internal/auth/client_test.go", Additions: 12, Deletions: 0, Offset: 20},
	}
	pv.SetFiles(files)
	assert.Equal(t, 2, pv.Cursor.Count())

	out := pv.Render(80, 30)
	assert.Contains(t, out, "Files Changed (2)")
	assert.Contains(t, out, "client.go")
	assert.Contains(t, out, "+31")
	assert.Contains(t, out, "-8")
}

func TestPRDetailView_CursorNavigatesFiles(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	pv.SetFiles([]DiffFile{
		{Path: "a.go", Additions: 1, Deletions: 0, Offset: 0},
		{Path: "b.go", Additions: 2, Deletions: 1, Offset: 10},
	})

	assert.Equal(t, 0, pv.Cursor.Index())

	pv.Cursor.Next()
	file := pv.SelectedFile()
	require.NotNil(t, file)
	assert.Equal(t, "b.go", file.Path)
	assert.Equal(t, 10, file.Offset)
}

func TestPRDetailView_SelectedFileNil(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	assert.Nil(t, pv.SelectedFile())
}

func TestPRDetailView_NonAgentPR(t *testing.T) {
	pr := testPR()
	pr.IsAgent = false
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.NotContains(t, out, "agent")
}

func TestPRDetailView_DraftPR(t *testing.T) {
	pr := testPR()
	pr.Draft = true
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.Contains(t, out, "draft")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/views/ -run TestPRDetailView -v -count=1`
Expected: Compilation error — `PRDetailView` not defined.

- [ ] **Step 3: Implement PRDetailView**

Replace the stub `internal/ui/views/prdetail.go` with the full implementation:

```go
package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// PRDetailView renders the drill-down into a single pull request.
type PRDetailView struct {
	PR       models.PullRequest
	RepoName string
	Files    []DiffFile
	RawDiff  string
	Cursor   components.Selector
}

// NewPRDetailView creates a PR detail view.
func NewPRDetailView(pr models.PullRequest, repoName string) *PRDetailView {
	return &PRDetailView{
		PR:       pr,
		RepoName: repoName,
	}
}

// SetFiles populates the file list (after diff fetch) and sets cursor count.
func (pv *PRDetailView) SetFiles(files []DiffFile) {
	pv.Files = files
	pv.Cursor.SetCount(len(files))
}

// SelectedFile returns the file at the cursor, or nil if no files.
func (pv *PRDetailView) SelectedFile() *DiffFile {
	if len(pv.Files) == 0 {
		return nil
	}
	return &pv.Files[pv.Cursor.Index()]
}

// Render draws the PR detail view.
func (pv *PRDetailView) Render(width, height int) string {
	var lines []string
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)

	// PR header
	num := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render(fmt.Sprintf("#%d", pv.PR.Number))
	lines = append(lines, fmt.Sprintf("  %s  %s", num, pv.PR.Title))

	// Meta line: author, age, size, agent badge
	age := detailFormatAge(time.Since(pv.PR.CreatedAt).Hours())
	size := fmt.Sprintf("+%d -%d (%d lines)", pv.PR.Additions, pv.PR.Deletions, pv.PR.Size())

	meta := fmt.Sprintf("  %s · %s · %s", pv.PR.Author, age, size)
	if pv.PR.IsAgent {
		meta += "   " + lipgloss.NewStyle().Foreground(ui.ColorPurple).Render("agent")
	}
	if pv.PR.Draft {
		meta += "   " + muted.Render("draft")
	}
	lines = append(lines, muted.Render(meta))

	// CI + review status
	ciStr := prCIStatus(pv.PR)
	reviewStr := prReviewStatus(pv.PR)
	lines = append(lines, fmt.Sprintf("  %s  %s", ciStr, reviewStr))
	lines = append(lines, "")

	// Files section
	if len(pv.Files) == 0 {
		lines = append(lines, muted.Render("  Loading diff..."))
	} else {
		filesHeader := muted.Render(fmt.Sprintf("  Files Changed (%d)", len(pv.Files)))
		lines = append(lines, filesHeader)
		for i, f := range pv.Files {
			selected := pv.Cursor.Index() == i
			lines = append(lines, pv.renderFileLine(f, selected, width))
		}
	}

	return strings.Join(lines, "\n")
}

func (pv *PRDetailView) renderFileLine(f DiffFile, selected bool, width int) string {
	adds := lipgloss.NewStyle().Foreground(ui.ColorGreen).Render(fmt.Sprintf("+%d", f.Additions))
	dels := lipgloss.NewStyle().Foreground(ui.ColorRed).Render(fmt.Sprintf("-%d", f.Deletions))

	line := fmt.Sprintf("  %s  %s %s", f.Path, adds, dels)
	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func prCIStatus(pr models.PullRequest) string {
	switch pr.CIStatus {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("CI✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("CI✗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("CI⧗")
	}
}

func prReviewStatus(pr models.PullRequest) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	switch pr.ReviewState {
	case "approved":
		return muted.Render("review: ") + lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("approved ✔")
	case "changes_requested":
		return muted.Render("review: ") + lipgloss.NewStyle().Foreground(ui.ColorRed).Render("changes requested")
	case "pending":
		return muted.Render("review: pending")
	default:
		return muted.Render("review: none")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/views/ -run TestPRDetailView -v -count=1`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views/prdetail.go internal/ui/views/prdetail_test.go
git commit -m "feat: add PRDetailView for pull request drill-down

Shows PR summary, CI/review status, file list from diff parsing.
Cursor navigates files. Supports async file population via SetFiles()."
```

### Task 11: Wire drill-down views into App

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Write navigation tests**

Add to `internal/app/app_test.go`:

```go
func TestHandleKey_DrillIntoRunDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	// Set up detail view with a run selected
	repo := views.RepoState{
		RepoName: "repo-a",
		FullName: "owner/repo-a",
		Runs: []models.WorkflowRun{
			{ID: 1, Name: "ci", HeadBranch: "main", HeadSHA: "abc123",
				Status: "completed", Conclusion: "failure",
				CreatedAt: now, UpdatedAt: now},
		},
	}
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(repo)

	// Press d to drill in
	dMsg := tea.KeyPressMsg{}
	dMsg.Text = "d"
	m, _ := a.handleKey(dMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewRunDetail, a.mode)
	assert.NotNil(t, a.runDetailView)
}

func TestHandleKey_DrillIntoPRDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	repo := views.RepoState{
		RepoName:    "repo-a",
		FullName:    "owner/repo-a",
		ReviewItems: []review.ReviewItem{
			{PR: models.PullRequest{Number: 42, Title: "Test PR", Repo: "owner/repo-a",
				CreatedAt: now, UpdatedAt: now}, Age: time.Hour},
		},
	}
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(repo)

	// Cursor to PR (after 0 runs)
	// Already on first item which is a PR since no runs

	dMsg := tea.KeyPressMsg{}
	dMsg.Text = "d"
	m, _ := a.handleKey(dMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewPRDetail, a.mode)
	assert.NotNil(t, a.prDetailView)
}

func TestHandleKey_BackFromRunDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewRunDetail
	run := models.WorkflowRun{ID: 1, CreatedAt: now, UpdatedAt: now}
	a.runDetailView = views.NewRunDetailView(run, "owner/repo-a")
	a.detailView = views.NewDetailView(views.RepoState{FullName: "owner/repo-a"})

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.Nil(t, a.runDetailView)
}

func TestHandleKey_BackFromPRDetail(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	a.mode = ui.ViewPRDetail
	pr := models.PullRequest{Number: 42, CreatedAt: now, UpdatedAt: now}
	a.prDetailView = views.NewPRDetailView(pr, "owner/repo-a")
	a.detailView = views.NewDetailView(views.RepoState{FullName: "owner/repo-a"})

	aMsg := tea.KeyPressMsg{}
	aMsg.Text = "a"
	m, _ := a.handleKey(aMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.Nil(t, a.prDetailView)
}
```

Add `"github.com/bruce-kelly/cimon/internal/review"` to the import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run TestHandleKey_DrillInto -v -count=1`
Expected: FAIL — drill-in stub returns immediately.

- [ ] **Step 3: Implement handleDetailDrillIn**

Replace the stub `handleDetailDrillIn` in `internal/app/app.go`:

```go
func (a App) handleDetailDrillIn() (tea.Model, tea.Cmd) {
	if a.detailView == nil {
		return a, nil
	}

	if run := a.detailView.SelectedRun(); run != nil {
		a.mode = ui.ViewRunDetail
		a.runDetailView = views.NewRunDetailView(*run, a.detailView.Repo.FullName)
		var cmds []tea.Cmd

		// Fetch jobs if empty
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
			// Jobs already loaded — fetch logs for failures
			cmds = append(cmds, a.fetchFailedLogs(run.Jobs))
		}
		return a, tea.Batch(cmds...)
	}

	if item := a.detailView.SelectedReviewItem(); item != nil {
		a.mode = ui.ViewPRDetail
		a.prDetailView = views.NewPRDetailView(item.PR, a.detailView.Repo.FullName)

		// Fetch diff
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
```

- [ ] **Step 4: Implement fetchFailedLogs**

Replace the placeholder `fetchFailedLogs`:

```go
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

	// Collect failed job IDs
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
```

- [ ] **Step 5: Add key handlers for run detail and PR detail views**

Add to `internal/app/app.go`:

```go
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

	// Arrow keys scroll log pane
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

	// Arrow keys scroll log pane
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
		// Scroll log pane to selected file
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
```

- [ ] **Step 6: Add action handlers for drill-down views**

```go
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
	// Navigate back to detail
	a.mode = ui.ViewDetail
	a.prDetailView = nil
	a.repos = a.buildRepoStates()
	a.compactView.UpdateRepos(a.repos)
	return a, nil
}
```

- [ ] **Step 7: Update handleKey to route to new views**

In `handleKey`, update the mode switch:

```go
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
```

- [ ] **Step 8: Update View() for new modes**

In the `View()` method, update the content switch and log pane condition:

```go
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
```

Update the log pane condition:

```go
	supportsLogPane := a.mode == ui.ViewDetail || a.mode == ui.ViewRunDetail || a.mode == ui.ViewPRDetail
	if a.logPane.Mode != components.LogPaneHidden && supportsLogPane {
```

- [ ] **Step 9: Update renderHeader with breadcrumbs**

Replace `renderHeader`:

```go
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
```

- [ ] **Step 10: Update diffResultMsg handler for PR detail file list**

In `Update()`, update the `diffResultMsg` handler to also parse files when in PR detail:

The handler from Task 3 already does this:
```go
	// If in PR detail view, also parse file list
	if a.mode == ui.ViewPRDetail && a.prDetailView != nil {
		a.prDetailView.SetFiles(views.ParseDiffFiles(msg.Content))
	}
```

Verify this is already present. If not, add it.

- [ ] **Step 11: Update handlePollResult for new views**

In `handlePollResult`, after the existing detail view cursor preservation block, add:

```go
	// Preserve run detail view on poll update
	if a.mode == ui.ViewRunDetail && a.runDetailView != nil {
		found := false
		for _, runs := range a.allRuns {
			for _, r := range runs {
				if r.ID == a.runDetailView.Run.ID {
					cursorPos := a.runDetailView.Cursor.Index()
					a.runDetailView.Run = r
					if len(r.Jobs) > 0 {
						a.runDetailView.SetJobs(r.Jobs)
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
```

- [ ] **Step 12: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests pass.

- [ ] **Step 13: Run go vet**

Run: `go vet ./...`
Expected: Clean.

- [ ] **Step 14: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go internal/ui/app.go
git commit -m "feat: wire RunDetail and PRDetail views into App

Drill-down from detail view: d on a run opens RunDetailView, d on a PR
opens PRDetailView. Full navigation with a/esc back, breadcrumb headers,
context-sensitive footers, async job/diff fetching, poll result handling."
```

---

## Chunk 4: Final Integration and Cleanup

### Task 12: Run full test suite and verify

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -count=1 -v`
Expected: All tests pass (should be 296 existing + new tests).

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: Clean.

- [ ] **Step 3: Build binary**

Run: `go build -o cimon ./cmd/cimon`
Expected: Compiles successfully.

- [ ] **Step 4: Verify test count**

Run: `go test ./... -count=1 -v 2>&1 | grep -c "^--- PASS"`
Expected: Higher than 296 (new tests added).

### Task 13: Update CLAUDE.md keybindings documentation

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update keybindings section**

Replace the `## Keybindings` section in `CLAUDE.md` with:

```markdown
## Keybindings

### Navigation (all views)
| Key | Action |
|-----|--------|
| `w` / `↑` | Cursor up |
| `s` / `↓` | Cursor down |
| `d` / `Enter` | Drill in / expand |
| `a` / `Esc` | Back / collapse (closes log pane first) |
| `?` | Help overlay |
| `q` | Quit |

### Compact View
| Key | Action |
|-----|--------|
| `1` | Batch merge all ready agent PRs (with confirm) |

### Detail View (per-repo)
| Key | Action |
|-----|--------|
| `1` | Rerun selected run / Approve selected PR |
| `2` | View diff (PR) or logs (run) |
| `3` | Dismiss selected PR |
| `e` | Toggle log pane (hidden → half → full) |
| `r` | Open in browser |

### Run Detail View
| Key | Action |
|-----|--------|
| `d` | Expand/collapse job steps |
| `1` | Rerun workflow |
| `2` | Rerun failed jobs |
| `e` | Toggle log pane |
| `r` | Open on GitHub |

### PR Detail View
| Key | Action |
|-----|--------|
| `d` | Jump log pane to selected file's diff |
| `1` | Approve PR |
| `2` | Merge PR (with confirm) |
| `3` | Dismiss PR |
| `e` | Toggle log pane |
| `r` | Open on GitHub |
```

- [ ] **Step 2: Update Architecture section**

In the architecture tree, add the new files:

```
    │   ├── views/
    │   │   ├── repostate.go     # ...
    │   │   ├── compact.go       # ...
    │   │   ├── detail.go        # ...
    │   │   ├── rundetail.go     # RunDetailView — single run drill-down, jobs/steps, cursor, expand/collapse
    │   │   ├── rundetail_test.go
    │   │   ├── prdetail.go      # PRDetailView — single PR drill-down, file list, CI/review status
    │   │   ├── prdetail_test.go
    │   │   ├── diffparse.go     # ParseDiffFiles — unified diff parser, file list with +/- counts and offsets
    │   │   └── diffparse_test.go
```

Update the Key Patterns section to add:
- **Four-view model:** CompactView → DetailView → RunDetailView / PRDetailView. WASD + numbered actions.
- **Async action pattern:** Actions return `tea.Cmd` producing result messages (`actionResultMsg`, `diffResultMsg`, `logsResultMsg`, `jobsResultMsg`). Handlers guard against stale responses.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for drill-down views and WASD keybindings"
```
