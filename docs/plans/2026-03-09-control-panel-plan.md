# Control Panel Overhaul — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform cimon into a dense, Kojima-style live control panel with real-time job progress, richer visuals, and more information at a glance.

**Architecture:** New `progress.go` component renders job progress bars. Pulsing dot via `tickEven` bool toggled on each tick. Live log pane tracked via `liveRunID` field on App, refreshed on poll. All existing render functions enriched with more data fields. Dashboard panels get lipgloss rounded borders.

**Tech Stack:** Go, Bubbletea v2, Lipgloss v2

---

### Task 1: Job Progress Bar Component

**Files:**
- Create: `internal/ui/components/progress.go`
- Create: `internal/ui/components/progress_test.go`

**Step 1: Write the tests**

```go
// internal/ui/components/progress_test.go
package components

import (
	"testing"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRenderJobProgress_AllComplete(t *testing.T) {
	jobs := []models.Job{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "success"},
	}
	result := RenderJobProgress(jobs)
	assert.Contains(t, result, "[3/3]")
}

func TestRenderJobProgress_Mixed(t *testing.T) {
	jobs := []models.Job{
		{Status: "completed", Conclusion: "success"},
		{Status: "in_progress"},
		{Status: "queued"},
	}
	result := RenderJobProgress(jobs)
	assert.Contains(t, result, "[1/3]")
}

func TestRenderJobProgress_Empty(t *testing.T) {
	result := RenderJobProgress(nil)
	assert.Equal(t, "", result)
}

func TestRenderJobProgress_WithFailure(t *testing.T) {
	jobs := []models.Job{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "failure"},
		{Status: "completed", Conclusion: "success"},
	}
	result := RenderJobProgress(jobs)
	assert.Contains(t, result, "[3/3]")
}

func TestJobProgressColor_Success(t *testing.T) {
	jobs := []models.Job{{Status: "completed", Conclusion: "success"}}
	color := JobProgressColor(jobs)
	assert.Equal(t, "green", color)
}

func TestJobProgressColor_InProgress(t *testing.T) {
	jobs := []models.Job{{Status: "in_progress"}}
	color := JobProgressColor(jobs)
	assert.Equal(t, "amber", color)
}

func TestJobProgressColor_Failure(t *testing.T) {
	jobs := []models.Job{{Status: "completed", Conclusion: "failure"}}
	color := JobProgressColor(jobs)
	assert.Equal(t, "red", color)
}

func TestRenderMiniBar_Full(t *testing.T) {
	assert.Equal(t, "█████", RenderMiniBar(5, 5, 5))
}

func TestRenderMiniBar_Half(t *testing.T) {
	result := RenderMiniBar(3, 6, 6)
	assert.Equal(t, "███░░░", result)
}

func TestRenderMiniBar_Zero(t *testing.T) {
	assert.Equal(t, "░░░░░", RenderMiniBar(0, 5, 5))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/components/ -run TestRenderJobProgress -v`
Expected: FAIL — functions not defined

**Step 3: Write the implementation**

```go
// internal/ui/components/progress.go
package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
)

// RenderMiniBar renders a filled/empty bar like ███░░░
func RenderMiniBar(filled, total, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	f := filled * width / total
	if f > width {
		f = width
	}
	return strings.Repeat("█", f) + strings.Repeat("░", width-f)
}

// JobProgressColor returns "green", "amber", or "red" based on job states.
func JobProgressColor(jobs []models.Job) string {
	for _, j := range jobs {
		if j.Conclusion == "failure" {
			return "red"
		}
	}
	for _, j := range jobs {
		if j.Status == "in_progress" || j.Status == "queued" {
			return "amber"
		}
	}
	return "green"
}

// RenderJobProgress returns a compact job progress string like [3/5]██░░░
func RenderJobProgress(jobs []models.Job) string {
	if len(jobs) == 0 {
		return ""
	}
	done := 0
	for _, j := range jobs {
		if j.Status == "completed" {
			done++
		}
	}
	total := len(jobs)

	colorName := JobProgressColor(jobs)
	var barColor lipgloss.Color
	switch colorName {
	case "red":
		barColor = ui.ColorRed
	case "amber":
		barColor = ui.ColorAmber
	default:
		barColor = ui.ColorGreen
	}

	bar := RenderMiniBar(done, total, 5)
	return fmt.Sprintf("[%d/%d]%s",
		done, total,
		lipgloss.NewStyle().Foreground(barColor).Render(bar),
	)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/components/ -run "TestRenderJobProgress|TestJobProgressColor|TestRenderMiniBar" -v`
Expected: PASS

**Step 5: Commit**

```
feat: add job progress bar component
```

---

### Task 2: Pulsing Status Dot

**Files:**
- Modify: `internal/ui/theme.go` — add `PulsingDot(conclusion string, tickEven bool) string`
- Modify: `internal/app/app.go` — add `tickEven bool` field, toggle on agent tick
- Modify: `internal/ui/components/pipeline.go` — pass tickEven, use PulsingDot

**Step 1: Add PulsingDot to theme.go**

Add after existing `StatusDot` function in `internal/ui/theme.go`:

```go
// PulsingDot returns an alternating dot for in-progress runs.
func PulsingDot(conclusion string, tickEven bool) string {
	if conclusion == "" && tickEven {
		return "◌" // dim phase for in-progress
	}
	return StatusDot(conclusion)
}
```

**Step 2: Add tickEven to App struct**

In `internal/app/app.go`, add to the App struct (around line 48):

```go
tickEven bool
```

Toggle it in the `agentTickMsg` handler (around line 259):

```go
case agentTickMsg:
    a.tickEven = !a.tickEven
```

**Step 3: Thread tickEven through to pipeline render**

In `internal/ui/components/pipeline.go`, add field to PipelineView:

```go
type PipelineView struct {
    // ... existing fields
    TickEven bool
}
```

In `renderRun`, change:
```go
dot := ui.StatusDot(run.Conclusion)
```
to:
```go
dot := ui.PulsingDot(run.Conclusion, p.TickEven)
```

In `internal/app/app.go` `rebuildScreenData`, after `a.dashboard.Pipeline.SetRuns(allRuns)`:

```go
a.dashboard.Pipeline.TickEven = a.tickEven
```

**Step 4: Build and test**

Run: `go build ./cmd/cimon && go test ./... -count=1`
Expected: all pass, build succeeds

**Step 5: Commit**

```
feat: pulsing status dot for in-progress runs
```

---

### Task 3: Enrich Pipeline Run Rows

**Files:**
- Modify: `internal/ui/components/pipeline.go` — renderRun() adds SHA, job progress, failure hint

**Step 1: Update renderRun**

Replace the `renderRun` method in `internal/ui/components/pipeline.go`:

```go
func (p *PipelineView) renderRun(run models.WorkflowRun, selected bool, width int) string {
	dot := ui.PulsingDot(run.Conclusion, p.TickEven)
	dotColor := ui.StatusColor(run.Conclusion)

	elapsed := FormatDuration(run.Elapsed())
	ago := FormatTimeAgo(run.UpdatedAt)

	// Short SHA
	sha := run.HeadSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}

	// Job progress
	progress := RenderJobProgress(run.Jobs)

	// Failure hint: first failed step
	failHint := ""
	if run.Conclusion == "failure" {
		for _, j := range run.Jobs {
			if j.Conclusion == "failure" {
				for _, s := range j.Steps {
					if s.Conclusion == "failure" {
						failHint = lipgloss.NewStyle().Foreground(ui.ColorRed).Render(" ▸ " + s.Name)
						break
					}
				}
				if failHint == "" {
					failHint = lipgloss.NewStyle().Foreground(ui.ColorRed).Render(" ▸ " + j.Name)
				}
				break
			}
		}
	}

	line := fmt.Sprintf(" %s %s  %s  %s  %s  %s  %s  %s%s",
		lipgloss.NewStyle().Foreground(dotColor).Render(dot),
		run.Name,
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.HeadBranch),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(sha),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
		progress,
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
		failHint,
	)

	if selected {
		return lipgloss.NewStyle().
			Background(ui.ColorSelection).
			Width(width).
			Render(line)
	}
	return line
}
```

**Step 2: Build and test**

Run: `go build ./cmd/cimon && go test ./internal/ui/components/ -v`
Expected: PASS

**Step 3: Commit**

```
feat: enriched pipeline rows with SHA, job progress, failure hints
```

---

### Task 4: Enrich Timeline Rows

**Files:**
- Modify: `internal/ui/screens/timeline.go` — add job progress, shorten repo name

**Step 1: Update timeline Render()**

In `internal/ui/screens/timeline.go`, update the run row formatting inside `Render()`. Change the `line` construction to:

```go
		// Shorten repo to just name
		repoName := run.Repo
		if idx := strings.LastIndex(repoName, "/"); idx >= 0 {
			repoName = repoName[idx+1:]
		}
		repoPrefix := lipgloss.NewStyle().Foreground(repoColor).Render(repoName)

		progress := components.RenderJobProgress(run.Jobs)

		line := fmt.Sprintf(" %s  %s %s %s  %s  %s  %s",
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(absTime),
			lipgloss.NewStyle().Foreground(dotColor).Render(dot),
			repoPrefix,
			run.Name,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
			progress,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
		)
```

Also add `"strings"` to the import block if not present.

**Step 2: Build and test**

Run: `go build ./cmd/cimon && go test ./internal/ui/screens/ -v`
Expected: PASS

**Step 3: Commit**

```
feat: enriched timeline rows with job progress, shortened repo names
```

---

### Task 5: Enrich Release Screen Rows

**Files:**
- Modify: `internal/ui/screens/release.go` — add SHA, actor, job progress to run rows

**Step 1: Update release run row rendering**

In `internal/ui/screens/release.go` `Render()`, replace the run row block with:

```go
		for i, run := range runs {
			selected := i == r.Selector.Index()
			dot := ui.StatusDot(run.Conclusion)
			dotColor := ui.StatusColor(run.Conclusion)
			elapsed := components.FormatDuration(run.Elapsed())
			ago := components.FormatTimeAgo(run.UpdatedAt)

			sha := run.HeadSHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			progress := components.RenderJobProgress(run.Jobs)

			line := fmt.Sprintf(" %s %s  %s  %s  %s  %s  %s  %s",
				lipgloss.NewStyle().Foreground(dotColor).Render(dot),
				run.Name,
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.HeadBranch),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(sha),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
				progress,
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
			)

			// Inline job grid for selected run
			if selected && len(run.Jobs) > 0 {
				for _, job := range run.Jobs {
					jobDot := ui.StatusDot(job.Conclusion)
					jobColor := ui.StatusColor(job.Conclusion)
					jobElapsed := ""
					if job.StartedAt != nil && job.CompletedAt != nil {
						jobElapsed = "  " + components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
					} else if job.Status == "in_progress" && job.StartedAt != nil {
						jobElapsed = "  " + components.FormatDuration(time.Since(*job.StartedAt)) + "..."
					}
					line += fmt.Sprintf("\n   %s %s%s",
						lipgloss.NewStyle().Foreground(jobColor).Render(jobDot),
						job.Name,
						lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(jobElapsed),
					)
				}
			}

			if selected {
				sb.WriteString(lipgloss.NewStyle().Background(ui.ColorSelection).Width(r.Width).Render(line))
			} else {
				sb.WriteString(line)
			}
			sb.WriteString("\n")
		}
```

Add `"time"` to the import block.

**Step 2: Build and test**

Run: `go build ./cmd/cimon && go test ./internal/ui/screens/ -v`
Expected: PASS

**Step 3: Commit**

```
feat: enriched release rows with SHA, actor, job progress, inline job grid
```

---

### Task 6: Dashboard Box Borders

**Files:**
- Modify: `internal/ui/screens/dashboard.go` — wrap panels in lipgloss borders

**Step 1: Update dashboard Render()**

Replace the panel rendering section in `Render()`. The key change: wrap each panel's content in a bordered box with the header as the top label.

In the multi-column section (around line 111), change panel construction:

```go
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorBorder).
		Width(panelWidth - 2) // account for border chars

	// Pipeline panel
	pipeLabel := "Pipeline"
	if d.Focus == FocusPipeline {
		pipeLabel = "[ Pipeline ]"
	}
	pipeHeader := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render(pipeLabel)
	leftPanel := borderStyle.Render(pipeHeader + "\n" + pipeContent)

	// Review queue panel
	reviewLabel := "Review Queue"
	if d.Focus == FocusReview {
		reviewLabel = "[ Review Queue ]"
	}
	reviewHeader := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render(reviewLabel)
	centerPanel := borderStyle.Width(reviewWidth - 2).Render(reviewHeader + "\n" + reviewContent)
```

Apply the same pattern for the roster panel and single-column fallback.

**Step 2: Build and test**

Run: `go build ./cmd/cimon && go test ./internal/ui/screens/ -v`
Expected: PASS

**Step 3: Commit**

```
feat: dashboard panels with box-drawing borders
```

---

### Task 7: Screen Tab Badges

**Files:**
- Modify: `internal/app/app.go` — View() top bar rendering

**Step 1: Compute badge counts**

In `View()`, before building the screen list, compute counts:

```go
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
	for _, repos := range a.release.Runs {
		for _, r := range repos {
			if r.IsActive() {
				activeReleases++
			}
		}
	}
```

**Step 2: Update tab rendering**

Change the tab rendering loop to include badges:

```go
	for _, s := range screenList {
		badge := ""
		switch s.num {
		case "1":
			if failCount > 0 {
				badge = lipgloss.NewStyle().Foreground(ui.ColorRed).Render(fmt.Sprintf("(%d)", failCount))
			}
		case "3":
			if activeReleases > 0 {
				badge = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("●")
			}
		}
		if s.active {
			screenIndicator += lipgloss.NewStyle().Foreground(ui.ColorAccent).Render("["+s.num+"]"+s.name) + badge + " "
		} else {
			screenIndicator += lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(" "+s.num+" "+s.name) + badge + " "
		}
	}
```

**Step 3: Build and test**

Run: `go build ./cmd/cimon && go test ./internal/app/ -v`
Expected: PASS

**Step 4: Commit**

```
feat: screen tab badges for failures and active releases
```

---

### Task 8: Live Log Pane Refresh

**Files:**
- Modify: `internal/app/app.go` — add liveRunID/liveRunRepo fields, refresh on poll
- Modify: `internal/ui/components/logpane.go` — countdown header

**Step 1: Add live tracking fields to App**

In the App struct, add:

```go
	liveRunID   int64
	liveRunRepo string
```

**Step 2: Update handleReleaseKey Enter/v handler**

When opening the log pane for an active run, set the live tracking fields:

After the `go func()` block in the Enter/v handler (and similarly in dashboard ViewDiff for pipeline), add before the goroutine:

```go
if runCopy.IsActive() {
    a.liveRunID = runCopy.ID
    a.liveRunRepo = runCopy.Repo
} else {
    a.liveRunID = 0
    a.liveRunRepo = ""
}
```

**Step 3: Refresh on poll**

In `rebuildScreenData`, after all existing screen updates, add:

```go
	// Live log pane refresh
	if a.liveRunID != 0 {
		a.refreshLiveLogPane()
	}
```

Add the `refreshLiveLogPane` method:

```go
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

	// Re-render the log pane content
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Run #%d  %s\n", liveRun.ID, liveRun.HTMLURL))
	sb.WriteString(fmt.Sprintf("Status: %s  Conclusion: %s\n", liveRun.Status, liveRun.Conclusion))
	sha := liveRun.HeadSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}
	sb.WriteString(fmt.Sprintf("Branch: %s  SHA: %s  Actor: %s\n", liveRun.HeadBranch, sha, liveRun.Actor))
	sb.WriteString(fmt.Sprintf("Started: %s  Elapsed: %s\n\n",
		liveRun.CreatedAt.Format("15:04:05"),
		components.FormatDuration(liveRun.Elapsed()),
	))

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

	title := fmt.Sprintf("%s — %s", liveRun.Name, liveRun.HeadBranch)
	isStillLive := liveRun.IsActive()
	a.logPane.SetContent(title, sb.String(), isStillLive)

	if !isStillLive {
		a.liveRunID = 0
		a.liveRunRepo = ""
	}
}
```

**Step 4: Update log pane header with countdown**

In `internal/ui/components/logpane.go`, update `renderHeader()`:

```go
func (l *LogPane) renderHeader() string {
	title := l.Title
	if title == "" {
		title = "Log"
	}
	if l.IsLive {
		title += "  LIVE ●"
	}
	return title
}
```

**Step 5: Build and test**

Run: `go build ./cmd/cimon && go test ./... -count=1`
Expected: all pass

**Step 6: Commit**

```
feat: live log pane auto-refresh for in-progress runs
```

---

### Task 9: Final Polish Pass

**Step 1: Run all tests**

Run: `go test ./... -count=1 -v`
Expected: all pass

**Step 2: Build and smoke test**

Run: `go build -o cimon ./cmd/cimon && ./cimon`

Verify:
- Dashboard shows bordered panels
- Pipeline rows show SHA, actor, job progress bars
- In-progress runs pulse
- Tab badges show failure counts
- Press 3, navigate to augury, press Enter on a run — see details
- Timeline shows shortened repo names + progress
- Log pane shows LIVE and auto-updates for active runs

**Step 3: Commit any final tweaks**

```
chore: control panel overhaul polish
```
