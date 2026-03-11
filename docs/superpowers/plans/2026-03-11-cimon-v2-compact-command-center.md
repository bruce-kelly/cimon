# CIMON v2: Compact Command Center — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace CIMON's four-screen TUI with a two-view model (compact status board + per-repo drill-in) designed for side-pane use alongside Claude Code.

**Architecture:** New `internal/ui/views/` package contains CompactView and DetailView models with pure rendering functions. `internal/app/app.go` is rewritten to wire these views instead of the four screen models. The data layer (polling, GitHub client, DB, review scoring) is unchanged. Reusable components (Selector, LogPane, ConfirmBar, Flash) stay in `internal/ui/components/`.

**Tech Stack:** Go, Bubbletea v2, Lipgloss v2, testify/assert

**Spec:** `docs/superpowers/specs/2026-03-11-cimon-v2-design.md`

---

## File Structure

### New Files

| File | Purpose |
|------|---------|
| `internal/ui/views/repostate.go` | RepoState, InlineStatus, PRSummary types; ComputeRepoStates, ComputeInlineStatus, ComputePRSummary, SortByAttention, DetectNewFlag functions |
| `internal/ui/views/repostate_test.go` | Tests for all computation functions |
| `internal/ui/views/compact.go` | CompactView model (repo list + inline expansion + cursor), Render method |
| `internal/ui/views/compact_test.go` | Render tests for all compact states (quiet, active, all-green, zero repos, single repo, NEW flag) |
| `internal/ui/views/detail.go` | DetailView model (runs + PRs + linear cursor + action dispatch), Render method |
| `internal/ui/views/detail_test.go` | Render tests for detail sections, cursor navigation, action key filtering |

### Modified Files

| File | Changes |
|------|---------|
| `internal/ui/app.go` | Replace `Screen` enum with `ViewMode` enum (Compact, Detail) |
| `internal/ui/keys.go` | Simplified keybindings for two-view model (remove screen-switch keys, add approve-uppercase) |
| `internal/ui/components/help.go` | Update help text for v2 keybindings |
| `internal/app/app.go` | Major rewrite: replace 4 screen models with compact/detail views, new key dispatch, new View() |
| `internal/app/app_test.go` | Rewrite tests for v2 behavior |

### Deleted Files (cleanup step)

| File | Reason |
|------|--------|
| `internal/ui/screens/dashboard.go` | Replaced by compact + detail views |
| `internal/ui/screens/timeline.go` | Removed from TUI |
| `internal/ui/screens/release.go` | Removed from TUI |
| `internal/ui/screens/metrics.go` | Removed from TUI |
| `internal/ui/screens/screens_test.go` | Tests for deleted screens |
| `internal/ui/components/actionmenu.go` | No longer used (direct key actions in detail view) |
| `internal/ui/components/catchup.go` | Replaced by NEW flag |
| `internal/ui/components/sparkline.go` | Only used by agent roster (removed) |
| `internal/ui/components/filterbar.go` | Removed from v2 |

### Unchanged Files

Everything in `internal/github/`, `internal/db/`, `internal/polling/`, `internal/config/`, `internal/models/`, `internal/review/`, `internal/confidence/`, `internal/agents/`, `internal/notify/`, `cmd/cimon/`. Components: `selector.go`, `logpane.go`, `confirmbar.go`, `flash.go`, `pipeline.go` (kept for reference, rendering helpers reused).

---

## Chunk 1: Foundation Types

### Task 1: ViewMode Enum

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Replace Screen enum with ViewMode enum**

Replace the entire contents of `internal/ui/app.go`:

```go
package ui

// ViewMode represents which view is active in the TUI.
type ViewMode int

const (
	ViewCompact ViewMode = iota
	ViewDetail
)

func (v ViewMode) String() string {
	switch v {
	case ViewCompact:
		return "compact"
	case ViewDetail:
		return "detail"
	default:
		return "unknown"
	}
}
```

Keep the old `Screen` type temporarily so the build doesn't break until Chunk 4:

```go
// Deprecated: Screen is kept for v1 compatibility during migration.
// Remove in Chunk 5 cleanup.
type Screen = int

const (
	ScreenDashboard Screen = iota
	ScreenTimeline
	ScreenRelease
	ScreenMetrics
)
```

- [ ] **Step 2: Verify the full project still compiles**

Run: `go build ./...`
Expected: PASS (old Screen type still available for v1 code)

- [ ] **Step 3: Commit**

```bash
git add internal/ui/app.go
git commit -m "refactor: replace Screen enum with ViewMode enum for v2"
```

### Task 2: RepoState Types

**Files:**
- Create: `internal/ui/views/repostate.go`
- Create: `internal/ui/views/repostate_test.go`

- [ ] **Step 1: Write tests for InlineStatus computation**

```go
// internal/ui/views/repostate_test.go
package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestComputeInlineStatus_AllPassing(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
	}
	status := ComputeInlineStatus(runs)
	assert.Equal(t, StatusPassing, status.Worst)
	assert.Empty(t, status.FailedJobs)
	assert.Empty(t, status.ActiveRuns)
}

func TestComputeInlineStatus_HasFailure(t *testing.T) {
	now := time.Now()
	runs := []models.WorkflowRun{
		{
			Status:     "completed",
			Conclusion: "failure",
			Name:       "ci",
			UpdatedAt:  now.Add(-4 * time.Minute),
			Jobs: []models.Job{
				{Name: "build", Conclusion: "failure"},
				{Name: "test", Conclusion: "failure"},
				{Name: "lint", Conclusion: "success"},
			},
		},
	}
	status := ComputeInlineStatus(runs)
	assert.Equal(t, StatusFailed, status.Worst)
	assert.Equal(t, []string{"build", "test"}, status.FailedJobs)
	assert.Equal(t, "ci", status.FailedWorkflow)
}

func TestComputeInlineStatus_HasActive(t *testing.T) {
	runs := []models.WorkflowRun{
		{
			Status: "in_progress",
			Name:   "deploy",
			Jobs: []models.Job{
				{Name: "build", Conclusion: "success"},
				{Name: "test", Conclusion: ""},
				{Name: "deploy", Conclusion: ""},
			},
		},
	}
	status := ComputeInlineStatus(runs)
	assert.Equal(t, StatusActive, status.Worst)
	assert.Len(t, status.ActiveRuns, 1)
	assert.Equal(t, "deploy", status.ActiveRuns[0].Name)
	assert.Equal(t, 1, status.ActiveRuns[0].CompletedJobs)
	assert.Equal(t, 3, status.ActiveRuns[0].TotalJobs)
}

func TestComputeInlineStatus_FailureTrumpsActive(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "failure", Name: "ci"},
		{Status: "in_progress", Name: "deploy"},
	}
	status := ComputeInlineStatus(runs)
	assert.Equal(t, StatusFailed, status.Worst)
}
```

- [ ] **Step 2: Write tests for PRSummary computation**

Append to the same test file:

```go
func TestComputePRSummary_Empty(t *testing.T) {
	summary := ComputePRSummary(nil)
	assert.Equal(t, 0, summary.Total)
	assert.Equal(t, 0, summary.Ready)
	assert.False(t, summary.CIPending)
}

func TestComputePRSummary_MixedStates(t *testing.T) {
	prs := []models.PullRequest{
		{CIStatus: "success", ReviewState: "approved", Draft: false},   // ready
		{CIStatus: "success", ReviewState: "approved", Draft: false},   // ready
		{CIStatus: "pending", ReviewState: "", Draft: false},           // CI pending
		{CIStatus: "success", ReviewState: "", Draft: true},            // draft
	}
	summary := ComputePRSummary(prs)
	assert.Equal(t, 4, summary.Total)
	assert.Equal(t, 2, summary.Ready)
	assert.True(t, summary.CIPending)
}

func TestComputePRSummary_AgentPRs(t *testing.T) {
	prs := []models.PullRequest{
		{CIStatus: "success", ReviewState: "approved", IsAgent: true},
		{CIStatus: "failure", IsAgent: true},
	}
	summary := ComputePRSummary(prs)
	assert.Equal(t, 1, summary.Ready)
}
```

- [ ] **Step 3: Write tests for SortByAttention**

```go
func TestSortByAttention_FailuresFirst(t *testing.T) {
	states := []RepoState{
		{RepoName: "green", Inline: InlineStatus{Worst: StatusPassing}},
		{RepoName: "failed", Inline: InlineStatus{Worst: StatusFailed}},
		{RepoName: "active", Inline: InlineStatus{Worst: StatusActive}},
	}
	SortByAttention(states)
	assert.Equal(t, "failed", states[0].RepoName)
	assert.Equal(t, "active", states[1].RepoName)
	assert.Equal(t, "green", states[2].RepoName)
}

func TestSortByAttention_ReadyPRsBeforeQuiet(t *testing.T) {
	states := []RepoState{
		{RepoName: "quiet", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 0}},
		{RepoName: "has-ready", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 2, Ready: 1}},
	}
	SortByAttention(states)
	assert.Equal(t, "has-ready", states[0].RepoName)
	assert.Equal(t, "quiet", states[1].RepoName)
}
```

- [ ] **Step 4: Write tests for DetectNewFlag**

```go
func TestDetectNewFlag_NewFailure(t *testing.T) {
	prev := RepoState{Inline: InlineStatus{Worst: StatusPassing}}
	curr := RepoState{Inline: InlineStatus{Worst: StatusFailed}}
	assert.True(t, DetectNewFlag(prev, curr))
}

func TestDetectNewFlag_NoChange(t *testing.T) {
	prev := RepoState{Inline: InlineStatus{Worst: StatusPassing}, PRs: PRSummary{Ready: 1}}
	curr := RepoState{Inline: InlineStatus{Worst: StatusPassing}, PRs: PRSummary{Ready: 1}}
	assert.False(t, DetectNewFlag(prev, curr))
}

func TestDetectNewFlag_NewReadyPR(t *testing.T) {
	prev := RepoState{PRSummary: PRSummary{Ready: 0}}
	curr := RepoState{PRSummary: PRSummary{Ready: 1}}
	assert.True(t, DetectNewFlag(prev, curr))
}

func TestDetectNewFlag_ReleaseStarted(t *testing.T) {
	prev := RepoState{Inline: InlineStatus{Releasing: false}}
	curr := RepoState{Inline: InlineStatus{Releasing: true}}
	assert.True(t, DetectNewFlag(prev, curr))
}
```

- [ ] **Step 5: Run tests to verify they fail**

Run: `go test ./internal/ui/views/ -v -count=1`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 6: Implement repostate.go**

```go
// internal/ui/views/repostate.go
package views

import (
	"sort"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
)

// RepoStatus represents the worst CI state for a repo.
type RepoStatus int

const (
	StatusPassing RepoStatus = iota
	StatusPending
	StatusActive
	StatusFailed
)

// ActiveRunInfo summarizes a running workflow for inline display.
type ActiveRunInfo struct {
	Name          string
	CompletedJobs int
	TotalJobs     int
	Elapsed       time.Duration
	IsRelease     bool
}

// InlineStatus is the computed CI state for inline expansion in compact view.
type InlineStatus struct {
	Worst          RepoStatus
	FailedWorkflow string
	FailedJobs     []string
	FailedAt       time.Time
	ActiveRuns     []ActiveRunInfo
	Releasing      bool
}

// PRSummary is the computed PR state for the repo summary line.
type PRSummary struct {
	Total     int
	Ready     int // approved + CI passing + not draft
	CIPending bool
}

// RepoState holds all display state for one repo in the compact view.
type RepoState struct {
	RepoName    string // short name (repo portion only, no owner/)
	FullName    string // owner/repo
	Runs        []models.WorkflowRun
	PRs         []models.PullRequest
	ReviewItems []review.ReviewItem
	Inline      InlineStatus
	PRSummary   PRSummary

	// NEW flag state
	NewFlag             bool
	LastNotableChange   time.Time
	UserAcknowledged    bool
}

// ComputeInlineStatus derives the inline display state from a repo's runs.
func ComputeInlineStatus(runs []models.WorkflowRun) InlineStatus {
	var status InlineStatus
	status.Worst = StatusPassing

	for i := range runs {
		r := &runs[i]
		if r.IsActive() {
			if status.Worst < StatusActive {
				status.Worst = StatusActive
			}
			completed := 0
			total := len(r.Jobs)
			for _, j := range r.Jobs {
				if j.Conclusion != "" {
					completed++
				}
			}
			status.ActiveRuns = append(status.ActiveRuns, ActiveRunInfo{
				Name:          r.Name,
				CompletedJobs: completed,
				TotalJobs:     total,
				Elapsed:       r.Elapsed(),
			})
		} else if r.Conclusion == "failure" {
			status.Worst = StatusFailed
			if status.FailedWorkflow == "" {
				status.FailedWorkflow = r.Name
				status.FailedAt = r.UpdatedAt
				for _, j := range r.Jobs {
					if j.Conclusion == "failure" {
						status.FailedJobs = append(status.FailedJobs, j.Name)
					}
				}
			}
		}
	}

	// Check if any active run is a release (caller sets IsRelease on ActiveRunInfo)
	for i := range status.ActiveRuns {
		if status.ActiveRuns[i].IsRelease {
			status.Releasing = true
			break
		}
	}

	return status
}

// ComputePRSummary derives the PR summary from a repo's pull requests.
func ComputePRSummary(prs []models.PullRequest) PRSummary {
	var s PRSummary
	s.Total = len(prs)
	for _, pr := range prs {
		if pr.CIStatus == "pending" {
			s.CIPending = true
		}
		if pr.CIStatus == "success" && pr.ReviewState == "approved" && !pr.Draft {
			s.Ready++
		}
	}
	return s
}

// SortByAttention sorts repo states by attention priority:
// 1. Failures (newest first)
// 2. Active runs
// 3. Repos with ready-to-merge PRs
// 4. All green
func SortByAttention(states []RepoState) {
	sort.SliceStable(states, func(i, j int) bool {
		si, sj := states[i], states[j]
		// Failed repos first
		if (si.Inline.Worst == StatusFailed) != (sj.Inline.Worst == StatusFailed) {
			return si.Inline.Worst == StatusFailed
		}
		// Active repos next
		if (si.Inline.Worst == StatusActive) != (sj.Inline.Worst == StatusActive) {
			return si.Inline.Worst == StatusActive
		}
		// Repos with ready PRs next
		if (si.PRSummary.Ready > 0) != (sj.PRSummary.Ready > 0) {
			return si.PRSummary.Ready > 0
		}
		return false
	})
}

// DetectNewFlag returns true if the repo state changed in a notable way.
func DetectNewFlag(prev, curr RepoState) bool {
	// Passing → failed
	if prev.Inline.Worst != StatusFailed && curr.Inline.Worst == StatusFailed {
		return true
	}
	// New ready PRs
	if curr.PRSummary.Ready > prev.PRSummary.Ready {
		return true
	}
	// Release started
	if !prev.Inline.Releasing && curr.Inline.Releasing {
		return true
	}
	return false
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/ui/views/ -v -count=1`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/ui/views/repostate.go internal/ui/views/repostate_test.go
git commit -m "feat(v2): add RepoState types and computation functions"
```

---

## Chunk 2: Compact View

### Task 3: Compact View Model and Rendering

**Files:**
- Create: `internal/ui/views/compact.go`
- Create: `internal/ui/views/compact_test.go`

- [ ] **Step 1: Write test for compact view with no repos**

```go
// internal/ui/views/compact_test.go
package views

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompactView_NoRepos(t *testing.T) {
	cv := NewCompactView(nil)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "No repos configured")
	assert.Contains(t, out, "cimon init")
}
```

- [ ] **Step 2: Write test for compact view all-green state**

```go
func TestCompactView_AllGreen(t *testing.T) {
	repos := []RepoState{
		{RepoName: "repo-a", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 0}},
		{RepoName: "repo-b", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 0}},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "repo-a")
	assert.Contains(t, out, "repo-b")
	assert.Contains(t, out, "all passing")
}
```

- [ ] **Step 3: Write test for compact view with failure and inline expansion**

```go
func TestCompactView_FailureInlineExpands(t *testing.T) {
	repos := []RepoState{
		{
			RepoName: "repo-c",
			Inline: InlineStatus{
				Worst:          StatusFailed,
				FailedWorkflow: "ci",
				FailedJobs:     []string{"build", "test"},
				FailedAt:       time.Now().Add(-4 * time.Minute),
			},
			PRSummary: PRSummary{Total: 3, Ready: 1},
		},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "repo-c")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "test")
}
```

- [ ] **Step 4: Write test for compact view with active run and progress**

```go
func TestCompactView_ActiveRunShowsProgress(t *testing.T) {
	repos := []RepoState{
		{
			RepoName: "repo-d",
			Inline: InlineStatus{
				Worst: StatusActive,
				ActiveRuns: []ActiveRunInfo{
					{Name: "deploy", CompletedJobs: 7, TotalJobs: 10, Elapsed: 82 * time.Second},
				},
				Releasing: true,
			},
		},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "repo-d")
	assert.Contains(t, out, "deploy")
	assert.Contains(t, out, "7/10")
}
```

- [ ] **Step 5: Write test for cursor navigation**

```go
func TestCompactView_CursorNavigation(t *testing.T) {
	repos := []RepoState{
		{RepoName: "repo-a", Inline: InlineStatus{Worst: StatusPassing}},
		{RepoName: "repo-b", Inline: InlineStatus{Worst: StatusPassing}},
		{RepoName: "repo-c", Inline: InlineStatus{Worst: StatusPassing}},
	}
	cv := NewCompactView(repos)
	assert.Equal(t, 0, cv.Cursor.Index())

	cv.Cursor.Next()
	assert.Equal(t, 1, cv.Cursor.Index())

	cv.Cursor.Next()
	assert.Equal(t, 2, cv.Cursor.Index())

	cv.Cursor.Next() // wraps
	assert.Equal(t, 0, cv.Cursor.Index())
}
```

- [ ] **Step 6: Write test for NEW flag rendering**

```go
func TestCompactView_NewFlagShows(t *testing.T) {
	repos := []RepoState{
		{
			RepoName: "repo-a",
			Inline:   InlineStatus{Worst: StatusFailed},
			NewFlag:  true,
		},
	}
	cv := NewCompactView(repos)
	out := cv.Render(40, 20)
	assert.Contains(t, out, "NEW")
}
```

- [ ] **Step 7: Write test for PR summary rendering**

```go
func TestCompactView_PRSummaryRendering(t *testing.T) {
	repos := []RepoState{
		{RepoName: "repo-a", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 2, Ready: 2}},
		{RepoName: "repo-b", Inline: InlineStatus{Worst: StatusPassing}, PRSummary: PRSummary{Total: 1, CIPending: true}},
	}
	cv := NewCompactView(repos)
	out := cv.Render(50, 20)
	assert.Contains(t, out, "2 PRs")
	assert.Contains(t, out, "2 ready")
	assert.Contains(t, out, "1 PR")
}
```

- [ ] **Step 8: Run tests to verify they fail**

Run: `go test ./internal/ui/views/ -v -count=1 -run TestCompact`
Expected: FAIL — CompactView not implemented

- [ ] **Step 9: Implement compact.go**

```go
// internal/ui/views/compact.go
package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// CompactView renders the compact repo list with inline expansion.
type CompactView struct {
	Repos  []RepoState
	Cursor components.Selector
}

// NewCompactView creates a compact view from repo states.
func NewCompactView(repos []RepoState) *CompactView {
	cv := &CompactView{Repos: repos}
	cv.Cursor.SetCount(len(repos))
	return cv
}

// UpdateRepos replaces the repo list, preserving cursor position.
func (cv *CompactView) UpdateRepos(repos []RepoState) {
	cv.Repos = repos
	cv.Cursor.SetCount(len(repos))
}

// SelectedRepo returns the currently selected repo state, or nil if empty.
func (cv *CompactView) SelectedRepo() *RepoState {
	if len(cv.Repos) == 0 {
		return nil
	}
	return &cv.Repos[cv.Cursor.Index()]
}

// AcknowledgeSelected clears the NEW flag on the selected repo.
func (cv *CompactView) AcknowledgeSelected() {
	if r := cv.SelectedRepo(); r != nil {
		r.NewFlag = false
		r.UserAcknowledged = true
	}
}

// Render draws the compact view. Returns a plain string (caller wraps in tea.View).
func (cv *CompactView) Render(width, height int) string {
	if len(cv.Repos) == 0 {
		return renderEmpty(width, height)
	}

	var lines []string

	for i, repo := range cv.Repos {
		selected := i == cv.Cursor.Index()
		lines = append(lines, renderRepoLine(repo, selected, width))

		// Inline expansion: only for failures and active runs
		if repo.Inline.Worst == StatusFailed && repo.Inline.FailedWorkflow != "" {
			lines = append(lines, renderFailedInline(repo.Inline, width))
		}
		for _, ar := range repo.Inline.ActiveRuns {
			lines = append(lines, renderActiveInline(ar, width))
		}
	}

	// All-green message if every repo is passing and no active runs
	allQuiet := true
	for _, r := range cv.Repos {
		if r.Inline.Worst != StatusPassing {
			allQuiet = false
			break
		}
	}
	if allQuiet {
		// Pad to fill space, then add message
		for len(lines) < height-3 {
			lines = append(lines, "")
		}
		style := lipgloss.NewStyle().Foreground(ui.ColorGreen)
		lines = append(lines, style.Render("all passing"))
	}

	return strings.Join(lines, "\n")
}

func renderEmpty(width, height int) string {
	msg := "No repos configured. Run `cimon init`."
	style := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	return style.Render(msg)
}

func renderRepoLine(repo RepoState, selected bool, width int) string {
	// Status dot
	dot := statusDot(repo.Inline.Worst)

	// Status icon
	icon := statusIcon(repo.Inline)

	// PR summary
	prText := renderPRSummary(repo.PRSummary)

	// NEW flag
	newFlag := ""
	if repo.NewFlag {
		newFlag = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true).Render(" NEW")
	}

	line := fmt.Sprintf("%s %s  %s  %s%s", dot, repo.RepoName, icon, prText, newFlag)

	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}

	return line
}

func renderPRSummary(s PRSummary) string {
	if s.Total == 0 {
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("—")
	}
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)

	noun := "PRs"
	if s.Total == 1 {
		noun = "PR"
	}
	text := fmt.Sprintf("%d %s", s.Total, noun)

	if s.Ready > 0 {
		text += muted.Render(fmt.Sprintf(" (%d ready)", s.Ready))
	} else if s.CIPending {
		text += muted.Render(" (CI ⧗)")
	}
	return text
}

func renderFailedInline(status InlineStatus, width int) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	red := lipgloss.NewStyle().Foreground(ui.ColorRed)

	var jobs []string
	for _, j := range status.FailedJobs {
		jobs = append(jobs, j+" "+red.Render("✗"))
	}
	ago := components.FormatTimeAgo(status.FailedAt)
	return muted.Render("  "+status.FailedWorkflow+": ") + strings.Join(jobs, "  ") + muted.Render("  "+ago)
}

func renderActiveInline(ar ActiveRunInfo, width int) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	green := lipgloss.NewStyle().Foreground(ui.ColorGreen)

	bar := progressBar(ar.CompletedJobs, ar.TotalJobs, 10)
	elapsed := components.FormatDuration(ar.Elapsed)
	progress := fmt.Sprintf("%d/%d", ar.CompletedJobs, ar.TotalJobs)

	return muted.Render("  "+ar.Name+" ") + green.Render(bar) + muted.Render(" "+progress+"  "+elapsed)
}

func statusDot(worst RepoStatus) string {
	var c lipgloss.Color
	switch worst {
	case StatusFailed:
		c = ui.ColorRed
	case StatusActive:
		c = ui.ColorBlue
	case StatusPending:
		c = ui.ColorAmber
	default:
		c = ui.ColorGreen
	}
	return lipgloss.NewStyle().Foreground(c).Render("■")
}

func statusIcon(status InlineStatus) string {
	switch status.Worst {
	case StatusFailed:
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	case StatusActive:
		if status.Releasing {
			return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●") +
				" " + lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("releasing")
		}
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●")
	case StatusPending:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("⧗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	}
}

func progressBar(completed, total, barWidth int) string {
	if total == 0 {
		return strings.Repeat("░", barWidth)
	}
	filled := barWidth * completed / total
	if filled > barWidth {
		filled = barWidth
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
}

func padRight(s string, width int) string {
	// Note: lipgloss.Width accounts for ANSI escape sequences
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// ClearExpiredNewFlags clears NEW flags older than the given duration.
func ClearExpiredNewFlags(repos []RepoState, maxAge time.Duration) {
	now := time.Now()
	for i := range repos {
		if repos[i].NewFlag && !repos[i].LastNotableChange.IsZero() {
			if now.Sub(repos[i].LastNotableChange) > maxAge {
				repos[i].NewFlag = false
			}
		}
	}
}
```

Note: This references `ui.ColorGreen`, `ui.ColorRed`, etc. from `internal/ui/theme.go` and `components.FormatTimeAgo`, `components.FormatDuration` from `internal/ui/components/pipeline.go`. These already exist.

- [ ] **Step 10: Run tests to verify they pass**

Run: `go test ./internal/ui/views/ -v -count=1`
Expected: All PASS

- [ ] **Step 11: Commit**

```bash
git add internal/ui/views/compact.go internal/ui/views/compact_test.go
git commit -m "feat(v2): add CompactView model and renderer"
```

---

## Chunk 3: Detail View

### Task 4: Detail View Model and Rendering

**Files:**
- Create: `internal/ui/views/detail.go`
- Create: `internal/ui/views/detail_test.go`

- [ ] **Step 1: Write tests for detail view rendering**

```go
// internal/ui/views/detail_test.go
package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/stretchr/testify/assert"
)

func TestDetailView_RenderCIPipeline(t *testing.T) {
	repo := RepoState{
		RepoName: "repo-c",
		FullName: "owner/repo-c",
		Runs: []models.WorkflowRun{
			{
				Name:       "ci",
				HeadBranch: "main",
				HeadSHA:    "a1b2c3d4",
				Conclusion: "failure",
				Status:     "completed",
				UpdatedAt:  time.Now().Add(-4 * time.Minute),
				Jobs: []models.Job{
					{Name: "build", Conclusion: "failure"},
					{Name: "test", Conclusion: "failure"},
					{Name: "lint", Conclusion: "success"},
				},
			},
			{
				Name:       "ci",
				HeadBranch: "main",
				HeadSHA:    "f4e5d6a7",
				Conclusion: "success",
				Status:     "completed",
				UpdatedAt:  time.Now().Add(-2 * time.Hour),
			},
		},
	}
	dv := NewDetailView(repo)
	out := dv.Render(50, 30)
	assert.Contains(t, out, "CI Pipeline")
	assert.Contains(t, out, "main")
	assert.Contains(t, out, "a1b2c3")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "Pull Requests")
}

func TestDetailView_RenderPullRequests(t *testing.T) {
	repo := RepoState{
		RepoName: "repo-c",
		FullName: "owner/repo-c",
		ReviewItems: []review.ReviewItem{
			{
				PR:  models.PullRequest{Number: 201, Title: "Fix auth middleware", CIStatus: "failure", IsAgent: true},
				Age: 3 * time.Hour,
			},
			{
				PR:  models.PullRequest{Number: 198, Title: "Add cache layer", CIStatus: "success", ReviewState: "approved"},
				Age: 24 * time.Hour,
			},
		},
	}
	dv := NewDetailView(repo)
	out := dv.Render(50, 30)
	assert.Contains(t, out, "#201")
	assert.Contains(t, out, "Fix auth")
	assert.Contains(t, out, "agent")
	assert.Contains(t, out, "#198")
}
```

- [ ] **Step 2: Write tests for linear cursor navigation**

```go
func TestDetailView_LinearCursor(t *testing.T) {
	repo := RepoState{
		RepoName: "repo-c",
		FullName: "owner/repo-c",
		Runs: []models.WorkflowRun{
			{Name: "ci", HeadBranch: "main", Status: "completed"},
			{Name: "ci", HeadBranch: "main", Status: "completed"},
		},
		ReviewItems: []review.ReviewItem{
			{PR: models.PullRequest{Number: 201}},
			{PR: models.PullRequest{Number: 198}},
		},
	}
	dv := NewDetailView(repo)

	assert.Equal(t, 0, dv.Cursor.Index())
	assert.True(t, dv.IsRunSelected(), "cursor 0 should be a run")

	dv.Cursor.Next()
	assert.Equal(t, 1, dv.Cursor.Index())
	assert.True(t, dv.IsRunSelected(), "cursor 1 should be a run")

	dv.Cursor.Next()
	assert.Equal(t, 2, dv.Cursor.Index())
	assert.False(t, dv.IsRunSelected(), "cursor 2 should be a PR")

	dv.Cursor.Next()
	assert.Equal(t, 3, dv.Cursor.Index())
	assert.False(t, dv.IsRunSelected(), "cursor 3 should be a PR")

	dv.Cursor.Next() // wraps
	assert.Equal(t, 0, dv.Cursor.Index())
}
```

- [ ] **Step 3: Write tests for action applicability**

```go
func TestDetailView_SelectedRun(t *testing.T) {
	repo := RepoState{
		Runs:        []models.WorkflowRun{{ID: 100}},
		ReviewItems: []review.ReviewItem{{PR: models.PullRequest{Number: 42}}},
	}
	dv := NewDetailView(repo)
	assert.NotNil(t, dv.SelectedRun())
	assert.Nil(t, dv.SelectedReviewItem())

	dv.Cursor.Next()
	assert.Nil(t, dv.SelectedRun())
	assert.NotNil(t, dv.SelectedReviewItem())
}

func TestDetailView_EmptyRepo(t *testing.T) {
	dv := NewDetailView(RepoState{RepoName: "empty", FullName: "owner/empty"})
	out := dv.Render(50, 20)
	assert.Contains(t, out, "empty")
	assert.Nil(t, dv.SelectedRun())
	assert.Nil(t, dv.SelectedReviewItem())
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/ui/views/ -v -count=1 -run TestDetail`
Expected: FAIL — DetailView not implemented

- [ ] **Step 5: Implement detail.go**

```go
// internal/ui/views/detail.go
package views

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// DetailView renders the per-repo drill-in with runs and PRs.
type DetailView struct {
	Repo     RepoState
	Cursor   components.Selector
	RunCount int // boundary: cursor < RunCount means run selected
}

// NewDetailView creates a detail view for the given repo.
func NewDetailView(repo RepoState) *DetailView {
	dv := &DetailView{
		Repo:     repo,
		RunCount: len(repo.Runs),
	}
	total := len(repo.Runs) + len(repo.ReviewItems)
	dv.Cursor.SetCount(total)
	return dv
}

// IsRunSelected returns true if the cursor is on a run (not a PR).
func (dv *DetailView) IsRunSelected() bool {
	return dv.Cursor.Index() < dv.RunCount
}

// SelectedRun returns the run at cursor, or nil if cursor is on a PR.
func (dv *DetailView) SelectedRun() *models.WorkflowRun {
	idx := dv.Cursor.Index()
	if idx >= dv.RunCount || dv.RunCount == 0 {
		return nil
	}
	return &dv.Repo.Runs[idx]
}

// SelectedReviewItem returns the review item at cursor, or nil if cursor is on a run.
func (dv *DetailView) SelectedReviewItem() *review.ReviewItem {
	idx := dv.Cursor.Index()
	if idx < dv.RunCount {
		return nil
	}
	prIdx := idx - dv.RunCount
	if prIdx >= len(dv.Repo.ReviewItems) {
		return nil
	}
	return &dv.Repo.ReviewItems[prIdx]
}

// Render draws the detail view for one repo.
func (dv *DetailView) Render(width, height int) string {
	var lines []string

	// CI Pipeline section
	header := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("CI Pipeline")
	lines = append(lines, header)

	if len(dv.Repo.Runs) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No recent runs"))
	}
	for i, run := range dv.Repo.Runs {
		selected := dv.Cursor.Index() == i
		lines = append(lines, renderRunLine(run, selected, width))
		// Expand jobs for selected run
		if selected && len(run.Jobs) > 0 {
			for _, job := range run.Jobs {
				lines = append(lines, renderJobLine(job, width))
			}
		}
	}

	lines = append(lines, "")

	// Pull Requests section
	prHeader := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("Pull Requests")
	lines = append(lines, prHeader)

	if len(dv.Repo.ReviewItems) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No open PRs"))
	}
	for i, item := range dv.Repo.ReviewItems {
		cursorIdx := dv.RunCount + i
		selected := dv.Cursor.Index() == cursorIdx
		lines = append(lines, renderPRLine(item, selected, width))
	}

	return strings.Join(lines, "\n")
}

func renderRunLine(run models.WorkflowRun, selected bool, width int) string {
	dot := runStatusDot(run)
	sha := run.HeadSHA
	if len(sha) > 6 {
		sha = sha[:6]
	}
	ago := components.FormatTimeAgo(run.UpdatedAt)

	line := fmt.Sprintf("  %s %s %s  %s", run.HeadBranch, dot, sha, ago)

	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
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

func renderJobLine(job models.Job, width int) string {
	var dot string
	switch job.Conclusion {
	case "success":
		dot = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		dot = lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	default:
		dot = lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	}

	elapsed := ""
	// Job.StartedAt and CompletedAt are *time.Time (pointers) — nil-check before use
	if job.StartedAt != nil && job.CompletedAt != nil {
		elapsed = components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
	}

	return fmt.Sprintf("    %s %s  %s", dot, job.Name, elapsed)
}

func renderPRLine(item review.ReviewItem, selected bool, width int) string {
	pr := item.PR

	// CI dot
	var ciDot string
	switch pr.CIStatus {
	case "success":
		ciDot = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("CI✓")
	case "failure":
		ciDot = lipgloss.NewStyle().Foreground(ui.ColorRed).Render("CI✗")
	default:
		ciDot = lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("CI⧗")
	}

	// Agent badge
	agent := ""
	if pr.IsAgent {
		agent = lipgloss.NewStyle().Foreground(ui.ColorPurple).Render("agent") + "  "
	}

	// Approved checkmark
	approved := ""
	if pr.ReviewState == "approved" {
		approved = " " + lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✔")
	}

	// Title (truncated)
	title := pr.Title
	maxTitle := width - 30 // leave room for #num, age, CI, agent
	if maxTitle < 10 {
		maxTitle = 10
	}
	if len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	age := formatAge(item.Age.Hours())
	num := fmt.Sprintf("#%d", pr.Number)

	line := fmt.Sprintf("  %s  %s  %s%s %s%s", num, title, agent, age, ciDot, approved)

	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func formatAge(hours float64) string {
	if hours < 1 {
		return "<1h"
	}
	if hours < 24 {
		return fmt.Sprintf("%.0fh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.0fd", days)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui/views/ -v -count=1`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ui/views/detail.go internal/ui/views/detail_test.go
git commit -m "feat(v2): add DetailView model and renderer"
```

---

## Chunk 4: App Integration

### Task 5: Update Keybindings and Help

**Files:**
- Modify: `internal/ui/keys.go`
- Modify: `internal/ui/components/help.go`

- [ ] **Step 1: Simplify keybindings for two-view model**

Replace `internal/ui/keys.go`:

```go
package ui

import "charm.land/bubbles/v2/key"

// KeyMap defines all keybindings for v2's two-view model.
type KeyMap struct {
	// Navigation
	Up, Down, Enter, Escape key.Binding

	// Compact view
	BatchMerge key.Binding

	// Detail view actions
	Rerun, Approve, Merge, Dismiss key.Binding
	ViewDiff, Open                 key.Binding

	// Shared
	LogCycle, Help, Quit key.Binding

	// Shared
	LogCycle, Help, Quit key.Binding
}

// Keys is the global keybinding configuration.
// Note: log pane scrolling uses Up/Down (arrow keys only, not w/s/j/k).
// When the log pane is open, handleDetailKey routes arrow-up/arrow-down
// to scroll the log pane, while w/s/j/k continue to navigate the item list.
var Keys = KeyMap{
	Up:         key.NewBinding(key.WithKeys("w", "k", "up"), key.WithHelp("w/k/↑", "up")),
	Down:       key.NewBinding(key.WithKeys("s", "j", "down"), key.WithHelp("s/j/↓", "down")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	BatchMerge: key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "batch merge")),
	Rerun:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun")),
	Approve:    key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "approve")),
	Merge:      key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge")),
	Dismiss:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "dismiss")),
	ViewDiff:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view diff/logs")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
	LogCycle:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "toggle log pane")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
```

- [ ] **Step 2: Update HelpOverlay for v2 keybindings**

Replace the `Render` method in `internal/ui/components/help.go` — update the bindings list to match v2:

```go
func (h *HelpOverlay) Render(viewName string, width, height int) string {
	if !h.Visible {
		return ""
	}

	bindings := []struct{ key, desc string }{
		{"w/k", "Navigate up"},
		{"s/j", "Navigate down"},
		{"Enter", "Drill in / select"},
		{"Esc", "Back / close log pane"},
		{"r", "Rerun (on runs)"},
		{"A", "Approve PR"},
		{"m", "Merge PR"},
		{"M", "Batch merge ready agent PRs"},
		{"x", "Dismiss PR"},
		{"v", "View diff / logs"},
		{"o", "Open in browser"},
		{"l", "Toggle log pane"},
		{"?", "Toggle help"},
		{"q", "Quit"},
	}

	// Same rendering logic as existing — build table with key/desc columns
	// ... (keep existing table formatting code, just update bindings slice)
```

Keep the existing border/centering rendering code from help.go lines 40-62, only replace the bindings data.

- [ ] **Step 3: Verify ui package compiles**

Run: `go build ./internal/ui/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ui/keys.go internal/ui/components/help.go
git commit -m "refactor(v2): simplify keybindings and help for two-view model"
```

### Task 6: Rewrite App Struct and NewApp

**Files:**
- Modify: `internal/app/app.go`

This is the largest change. We replace the App struct fields, NewApp, and Init. The old screen models, agent dispatch, scheduler, auto-fix, catchup, and action menu are removed. The data layer (client, db, poller, polling) stays.

- [ ] **Step 1: Replace App struct**

Replace the App struct definition (lines ~44-103) with:

```go
type App struct {
	// Infrastructure (unchanged)
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

	// Overlays (kept from v1)
	help       components.HelpOverlay
	flash      components.Flash
	confirmBar components.ConfirmBar
	logPane    components.LogPane

	// Dismissed PRs (unchanged)
	dismissed map[string]bool

	// Per-repo latest poll data (unchanged)
	allRuns  map[string][]models.WorkflowRun
	allPulls map[string][]models.PullRequest

	// Config-derived lookups (unchanged)
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
```

Removed fields: `screen`, `dashboard`, `timeline`, `release`, `metrics`, `actionMenu`, `catchup`, `lastInput`, `preIdleRuns`, `preIdlePulls`, `liveRunID`, `liveRunRepo`, `agentWorkflows`, `dispatcher`, `scheduler`, `autoFix`.

- [ ] **Step 2: Replace NewApp**

Replace the NewApp function. Key changes: no screen model initialization, no agent infrastructure, builds empty repos slice:

```go
func NewApp(cfg *config.CimonConfig, client *ghclient.Client, database *db.Database) App {
	ctx, cancel := context.WithCancel(context.Background())

	// Build release workflow lookup (for inline expansion)
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

	// Load dismissed items (LoadDismissed returns map[string]bool directly)
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
```

- [ ] **Step 3: Replace Init**

Simpler Init — just poller + tick for NEW flag expiry:

```go
type tickMsg struct{}

func (a App) Init() tea.Cmd {
	a.poller.Start(a.ctx) // Start already launches a goroutine internally
	return tea.Batch(waitForPoll(a.resultCh), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}
```

- [ ] **Step 4: Add buildRepoStates helper**

This replaces `rebuildScreenData`. It builds `[]views.RepoState` from polled data:

```go
func (a *App) buildRepoStates() []views.RepoState {
	states := make([]views.RepoState, 0, len(a.config.Repos))

	for _, rc := range a.config.Repos {
		runs := a.allRuns[rc.Repo]
		prs := a.allPulls[rc.Repo]

		// Filter dismissed PRs
		var activePRs []models.PullRequest
		for _, pr := range prs {
			key := fmt.Sprintf("%s:%d", rc.Repo, pr.Number)
			if !a.dismissed[key] {
				activePRs = append(activePRs, pr)
			}
		}

		// Compute review items
		// ReviewItemsFromPulls(pulls, amberHours int, redHours int, dismissed map[string]bool)
		// We already filtered dismissed above, so pass nil for dismissed
		amberHours := 24
		redHours := 48
		if a.config.ReviewQueue.Escalation.Amber > 0 {
			amberHours = a.config.ReviewQueue.Escalation.Amber
		}
		if a.config.ReviewQueue.Escalation.Red > 0 {
			redHours = a.config.ReviewQueue.Escalation.Red
		}
		reviewItems := review.ReviewItemsFromPulls(activePRs, amberHours, redHours, nil)

		// Compute inline status
		inline := views.ComputeInlineStatus(runs)
		// Mark release runs
		if rw, ok := a.releaseWorkflows[rc.Repo]; ok {
			for i := range inline.ActiveRuns {
				for _, r := range runs {
					if r.Name == inline.ActiveRuns[i].Name && rw[r.WorkflowFile] {
						inline.ActiveRuns[i].IsRelease = true
					}
				}
			}
		}

		// Short name (repo portion only)
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
```

- [ ] **Step 5: Verify partial build (app package won't compile yet — old methods still reference removed fields)**

Note: The app package won't compile at this point because Update/View/handleKey still reference the old screen models. This is expected — we'll replace those methods in the next tasks. Verify the new types are correct by building the views package:

Run: `go build ./internal/ui/views/`
Expected: PASS

- [ ] **Step 6: Commit work in progress**

```bash
git add internal/app/app.go
git commit -m "wip(v2): replace App struct, NewApp, Init with v2 models"
```

### Task 7: Rewrite Update and Key Handling

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace Update method**

```go
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
```

- [ ] **Step 2: Replace handlePollResult**

Simplified: persist data, rebuild repo states, detect NEW flags, update status bar:

```go
func (a App) handlePollResult(msg pollResultMsg) (tea.Model, tea.Cmd) {
	result := msg.Result

	// Handle errors
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

	// Store poll data
	a.allRuns[result.Repo] = result.Runs
	a.allPulls[result.Repo] = result.PullRequests
	a.rateLimit = result.RateLimitRemaining
	a.lastPoll = time.Now()

	// Persist to DB (UpsertRun/UpsertPull take single model args — repo is a field on the model)
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

	// Rebuild repo states and detect NEW flags
	prevStates := a.repos
	a.repos = a.buildRepoStates()

	// Detect NEW flags by comparing prev and curr
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
				// Preserve existing NEW flag
				a.repos[i].NewFlag = prev.NewFlag
				a.repos[i].LastNotableChange = prev.LastNotableChange
				a.repos[i].UserAcknowledged = prev.UserAcknowledged
			}
		}
	}

	a.compactView.UpdateRepos(a.repos)

	// Update detail view if active
	if a.mode == ui.ViewDetail && a.detailView != nil {
		for _, r := range a.repos {
			if r.FullName == a.detailView.Repo.FullName {
				cursorPos := a.detailView.Cursor.Index()
				a.detailView = views.NewDetailView(r)
				// Restore cursor position (clamped by new count)
				for i := 0; i < cursorPos && i < a.detailView.Cursor.Count()-1; i++ {
					a.detailView.Cursor.Next()
				}
				break
			}
		}
	}

	// Build status text
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

	// waitForPoll is a free function taking the channel
	return a, waitForPoll(a.resultCh)
}
```

- [ ] **Step 3: Replace handleKey for two-view model**

```go
func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// ConfirmBar intercepts all keys when active
	if a.confirmBar.Active {
		if a.confirmBar.HandleKey(keyStr) {
			return a, nil
		}
	}

	// Help overlay: toggle
	if key.Matches(msg, ui.Keys.Help) {
		a.help.Toggle()
		return a, nil
	}
	// Dismiss help on any key if visible
	if a.help.Visible {
		a.help.Visible = false
		return a, nil
	}

	// Quit
	if key.Matches(msg, ui.Keys.Quit) {
		a.quitting = true
		a.cancel()
		return a, tea.Quit
	}

	// Route to view-specific handler
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
	// Log pane: Esc closes it first, second Esc goes back
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

	// Log pane toggle
	if key.Matches(msg, ui.Keys.LogCycle) {
		a.logPane.CycleMode()
		return a, nil
	}

	// When log pane is open, arrow keys scroll the log, w/s/j/k navigate items
	if a.logPane.Mode != components.LogPaneHidden {
		// Check for arrow keys specifically (not w/s/j/k)
		if msg.Text == "up" || msg.String() == "up" {
			a.logPane.ScrollPos--
			if a.logPane.ScrollPos < 0 {
				a.logPane.ScrollPos = 0
			}
			return a, nil
		}
		if msg.Text == "down" || msg.String() == "down" {
			a.logPane.ScrollPos++
			return a, nil
		}
	}

	// Navigation (w/s/j/k + arrow keys when log pane is hidden)
	switch {
	case key.Matches(msg, ui.Keys.Down):
		a.detailView.Cursor.Next()
	case key.Matches(msg, ui.Keys.Up):
		a.detailView.Cursor.Prev()

	// Run actions
	case key.Matches(msg, ui.Keys.Rerun):
		if run := a.detailView.SelectedRun(); run != nil {
			return a.handleRerun(run)
		}

	// PR actions
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

	// View diff/logs
	case key.Matches(msg, ui.Keys.ViewDiff):
		return a.handleViewDiff()

	// Open in browser
	case key.Matches(msg, ui.Keys.Open):
		return a.handleOpen()
	}

	return a, nil
}
```

- [ ] **Step 4: Add action handler stubs**

These reuse logic from v1 but simplified (no action menu, direct key dispatch). Keep existing `handleRerun`, `handleApprove`, `handleMerge`, `handleDismiss` logic from v1's `handleDashboardKey` but extract into standalone methods. Use goroutines + flash for async results (same pattern as v1, known mutation issue preserved):

```go
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
	key := fmt.Sprintf("%s:%d", repo, pr.Number)
	a.dismissed[key] = true
	if a.db != nil {
		a.db.AddDismissed(repo, pr.Number)
	}
	a.flash.Show(fmt.Sprintf("Dismissed PR #%d", pr.Number), false)
	// Rebuild to remove from view
	a.repos = a.buildRepoStates()
	a.compactView.UpdateRepos(a.repos)
	return a, nil
}

func (a App) handleBatchMerge() (tea.Model, tea.Cmd) {
	// Find all ready agent PRs across all repos
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
		// Fetch PR diff
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
		// Show run logs — GetFailedLogs takes (ctx, repo, jobID int64)
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
```

Note: `openBrowser` already exists in v1's app.go — keep it. Also keep `waitForPoll` and `pollResultMsg`/`pollErrorMsg` types unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "wip(v2): rewrite Update, handleKey, and action handlers for two-view model"
```

### Task 8: Rewrite View Method

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace View method**

```go
func (a App) View() tea.View {
	if a.quitting {
		return tea.NewView("")
	}

	// Minimum terminal size
	if a.width < 36 || a.height < 10 {
		msg := fmt.Sprintf("Terminal too small (%d×%d). Need at least 36×10.", a.width, a.height)
		v := tea.NewView(msg)
		v.AltScreen = true
		return v
	}

	contentHeight := a.height - 2 // header + footer

	// Header
	header := a.renderHeader()

	// Content
	var content string
	switch a.mode {
	case ui.ViewCompact:
		content = a.compactView.Render(a.width, contentHeight)
	case ui.ViewDetail:
		if a.detailView != nil {
			content = a.detailView.Render(a.width, contentHeight)
		}
	}

	// Help overlay replaces content
	if a.help.Visible {
		viewName := a.mode.String()
		content = a.help.Render(viewName, a.width, contentHeight)
	}

	// Log pane split
	if a.logPane.Mode != components.LogPaneHidden && a.mode == ui.ViewDetail {
		logHeight := contentHeight / 2
		if a.logPane.Mode == components.LogPaneFull {
			logHeight = contentHeight
		}
		mainHeight := contentHeight - logHeight

		// Truncate content to mainHeight lines
		contentLines := strings.Split(content, "\n")
		if len(contentLines) > mainHeight {
			contentLines = contentLines[:mainHeight]
		}
		content = strings.Join(contentLines, "\n")

		logRender := a.logPane.Render(a.width, logHeight)
		content = content + "\n" + logRender
	}

	// Truncate content to fit
	contentLines := strings.Split(content, "\n")
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	// Pad to fill
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}

	// Footer
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

	// Fill middle with ─
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
	// Priority: ConfirmBar > Flash > status/action hints
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
```

- [ ] **Step 2: Remove old screen rendering code, old handleKey handlers, old rebuildScreenData**

Delete: `rebuildScreenData`, `handleDashboardKey`, `handleTimelineKey`, `handleReleaseKey`, `refreshLiveLogPane`, and all v1-specific helper functions that reference removed screen models.

Keep: `waitForPoll`, `pollResultMsg`, `pollErrorMsg`, `openBrowser`, `findRepoConfig`.

- [ ] **Step 3: Remove old imports**

Remove imports for `internal/ui/screens`, `internal/agents` (dispatcher, scheduler, autofix), `internal/ui/components` items no longer used (ActionMenu, CatchupOverlay, FilterBar, Sparkline).

- [ ] **Step 4: Build and fix compile errors**

Run: `go build ./internal/app/`
Fix any remaining compilation issues (missing imports, unused variables, type mismatches).

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(v2): rewrite View and complete App integration for two-view model"
```

### Task 9: Rewrite App Tests

**Files:**
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Replace app tests for v2 behavior**

```go
package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/views"
	"github.com/stretchr/testify/assert"
)

func testApp(t *testing.T) App {
	t.Helper()
	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{Repo: "owner/repo-a", Branch: "main"},
			{Repo: "owner/repo-b", Branch: "main"},
		},
		Polling:     config.PollingConfig{Idle: 30, Active: 5, Cooldown: 3},
		ReviewQueue: config.ReviewQueueConfig{Escalation: config.EscalationConfig{Amber: 24, Red: 48}},
	}
	testDB, err := db.OpenMemory()
	assert.NoError(t, err)
	client := ghclient.NewTestClient("test-token", "http://localhost:1")
	return NewApp(cfg, client, testDB)
}

func TestNewApp_InitializesCompactMode(t *testing.T) {
	a := testApp(t)
	assert.Equal(t, ui.ViewCompact, a.mode)
	assert.NotNil(t, a.compactView)
	assert.Nil(t, a.detailView)
	assert.Len(t, a.repos, 2)
	assert.False(t, a.quitting)
}

func TestHandlePollResult_UpdatesRepoStates(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	result := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{
			{ID: 1, Name: "ci", Status: "completed", Conclusion: "success",
				Repo: "owner/repo-a", WorkflowFile: "ci.yml",
				CreatedAt: now, UpdatedAt: now},
		},
		PullRequests:       []models.PullRequest{{Number: 42, Title: "Test PR", Repo: "owner/repo-a", State: "open", CreatedAt: now, UpdatedAt: now}},
		RateLimitRemaining: 4500,
	}

	msg := pollResultMsg{Result: result}
	m, _ := a.handlePollResult(msg)
	a = m.(App)

	assert.Equal(t, 4500, a.rateLimit)
	assert.Len(t, a.allRuns["owner/repo-a"], 1)
	assert.Len(t, a.allPulls["owner/repo-a"], 1)
}

func TestHandlePollResult_AuthError(t *testing.T) {
	a := testApp(t)
	result := models.PollResult{
		Repo:  "owner/repo-a",
		Error: &ghclient.AuthError{StatusCode: 401, Message: "bad credentials"},
	}
	msg := pollResultMsg{Result: result}
	m, _ := a.handlePollResult(msg)
	a = m.(App)
	assert.Contains(t, a.statusText, "AUTH FAILED")
}

func TestHandleKey_CompactNavigation(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	// Down moves cursor — use Text field as per Bubbletea v2 API
	downMsg := tea.KeyPressMsg{}
	downMsg.Text = "s"
	m, _ := a.handleKey(downMsg)
	a = m.(App)
	assert.Equal(t, 1, a.compactView.Cursor.Index())
}

func TestHandleKey_EnterDrillsIn(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40

	enterMsg := tea.KeyPressMsg{}
	enterMsg.Text = "enter"
	m, _ := a.handleKey(enterMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewDetail, a.mode)
	assert.NotNil(t, a.detailView)
}

func TestHandleKey_EscReturnsToCompact(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	a.mode = ui.ViewDetail
	a.detailView = views.NewDetailView(views.RepoState{RepoName: "test", FullName: "owner/test"})

	escMsg := tea.KeyPressMsg{}
	escMsg.Text = "esc"
	m, _ := a.handleKey(escMsg)
	a = m.(App)
	assert.Equal(t, ui.ViewCompact, a.mode)
	assert.Nil(t, a.detailView)
}

func TestHandlePollResult_DetectsNewFlag(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	now := time.Now()

	// First poll: all passing
	result1 := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{{ID: 1, Status: "completed", Conclusion: "success",
			Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now}},
	}
	m, _ := a.handlePollResult(pollResultMsg{Result: result1})
	a = m.(App)

	// Second poll: failure
	result2 := models.PollResult{
		Repo: "owner/repo-a",
		Runs: []models.WorkflowRun{{ID: 2, Status: "completed", Conclusion: "failure",
			Repo: "owner/repo-a", CreatedAt: now, UpdatedAt: now}},
	}
	m, _ = a.handlePollResult(pollResultMsg{Result: result2})
	a = m.(App)

	// Find repo-a in states and check NEW flag
	found := false
	for _, r := range a.repos {
		if r.FullName == "owner/repo-a" {
			assert.True(t, r.NewFlag, "repo-a should have NEW flag after failure")
			found = true
		}
	}
	assert.True(t, found)
}

func TestView_ReturnsAltScreen(t *testing.T) {
	a := testApp(t)
	a.width = 80
	a.height = 40
	v := a.View()
	assert.True(t, v.AltScreen)
}

func TestView_TooSmall(t *testing.T) {
	a := testApp(t)
	a.width = 30
	a.height = 5
	v := a.View()
	// tea.View doesn't expose Body as a public field — just verify it doesn't panic
	// and returns AltScreen
	assert.True(t, v.AltScreen)
}

func TestView_Quitting(t *testing.T) {
	a := testApp(t)
	a.quitting = true
	v := a.View()
	assert.NotNil(t, v)
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/app/ -v -count=1`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/app/app_test.go
git commit -m "test(v2): rewrite App tests for two-view model"
```

---

## Chunk 5: Cleanup

### Task 10: Remove Old Screens and Unused Components

**Files:**
- Delete: `internal/ui/screens/dashboard.go`
- Delete: `internal/ui/screens/timeline.go`
- Delete: `internal/ui/screens/release.go`
- Delete: `internal/ui/screens/metrics.go`
- Delete: `internal/ui/screens/screens_test.go`
- Delete: `internal/ui/components/actionmenu.go`
- Delete: `internal/ui/components/catchup.go`
- Delete: `internal/ui/components/sparkline.go`
- Delete: `internal/ui/components/filterbar.go`

- [ ] **Step 1: Delete old screen files**

```bash
rm internal/ui/screens/dashboard.go
rm internal/ui/screens/timeline.go
rm internal/ui/screens/release.go
rm internal/ui/screens/metrics.go
rm internal/ui/screens/screens_test.go
```

- [ ] **Step 2: Delete unused component files**

```bash
rm internal/ui/components/actionmenu.go
rm internal/ui/components/catchup.go
rm internal/ui/components/sparkline.go
rm internal/ui/components/filterbar.go
```

- [ ] **Step 3: Remove references from components_test.go**

Edit `internal/ui/components/components_test.go` to remove tests for ActionMenu, CatchupOverlay, Sparkline, and FilterBar. Keep tests for Selector, ConfirmBar, Flash, Pipeline, LogPane, and HelpOverlay.

- [ ] **Step 4: Verify full build and test suite**

Run: `go build ./... && go test ./... -count=1`
Expected: All packages build, all tests pass.

- [ ] **Step 5: Run go vet**

Run: `go vet ./...`
Expected: Clean

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore(v2): remove old screens and unused components"
```

### Task 11: Final Verification

- [ ] **Step 1: Run full test suite with verbose output**

Run: `go test ./... -count=1 -v 2>&1 | tail -30`
Expected: All packages PASS

- [ ] **Step 2: Build binary**

Run: `go build -o cimon ./cmd/cimon`
Expected: Binary compiles successfully

- [ ] **Step 3: Verify binary runs (smoke test)**

Run: `./cimon version`
Expected: Prints version info without error

- [ ] **Step 4: Commit any remaining fixes**

If any issues were found and fixed in steps 1-3:

```bash
git add -A
git commit -m "fix(v2): address issues found in final verification"
```

- [ ] **Step 5: Squash WIP commits (optional)**

The WIP commits from Tasks 6-8 can be squashed into clean feature commits if desired:

```bash
git rebase -i HEAD~N  # where N is the number of commits to clean up
```

---

## Implementation Notes

### Preserved Known Issues

The **goroutine flash mutation** issue from v1 is preserved in v2. Action handlers (rerun, approve, merge) spawn goroutines that call `a.flash.Show()` on a value-receiver copy. The mutation doesn't propagate. This is a known issue flagged in MEMORY.md. A proper fix requires a `tea.Cmd`/`tea.Msg` pattern for async action results — worth doing but out of scope for this UI reshape.

### Color Constants

The views package references color constants from `internal/ui/theme.go`. All of these already exist:
- `ui.ColorGreen`, `ui.ColorRed`, `ui.ColorBlue`, `ui.ColorAmber`, `ui.ColorPurple`
- `ui.ColorMuted`, `ui.ColorAccent`
- `ui.ColorSelection` (`#364a82` — already defined in theme.go)

### Component Format Helpers

The views package uses `components.FormatTimeAgo`, `components.FormatDuration`, and `components.FormatTimeAbsolute` from `internal/ui/components/pipeline.go`. These are public functions and remain available after the v2 changes.

### Mouse Support

Mouse click support (`tea.MouseClickMsg`) is listed in the spec but not included in this plan to keep scope focused. It can be added as a follow-up: enable `tea.EnableMouseCellMotion` in Init, handle `tea.MouseClickMsg` in `handleCompactKey` by computing which repo line was clicked based on Y coordinate.
