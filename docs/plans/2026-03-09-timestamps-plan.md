# Timestamps Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add relative timestamps ("3h ago") across all screens and absolute timestamps ("14:23") on the timeline screen.

**Architecture:** Two new formatting functions in `internal/ui/components/pipeline.go` (alongside existing `FormatDuration`). Each screen render method gets a one-line addition to include the timestamp. No model, DB, or config changes.

**Tech Stack:** Go stdlib `time` and `fmt` only. Tests use `testify/assert`.

---

### Task 1: FormatTimeAgo function + tests

**Files:**
- Modify: `internal/ui/components/pipeline.go:127-136` (add after FormatDuration)
- Modify: `internal/ui/components/components_test.go` (add tests after FormatDuration tests)

**Step 1: Write the failing tests**

Add to `internal/ui/components/components_test.go` after `TestFormatDuration_Zero`:

```go
// --- FormatTimeAgo ---

func TestFormatTimeAgo_Now(t *testing.T) {
	assert.Equal(t, "now", FormatTimeAgo(time.Now()))
}

func TestFormatTimeAgo_Seconds(t *testing.T) {
	assert.Equal(t, "now", FormatTimeAgo(time.Now().Add(-30*time.Second)))
}

func TestFormatTimeAgo_Minutes(t *testing.T) {
	assert.Equal(t, "3m ago", FormatTimeAgo(time.Now().Add(-3*time.Minute)))
}

func TestFormatTimeAgo_OneMinute(t *testing.T) {
	assert.Equal(t, "1m ago", FormatTimeAgo(time.Now().Add(-1*time.Minute)))
}

func TestFormatTimeAgo_Hours(t *testing.T) {
	assert.Equal(t, "2h ago", FormatTimeAgo(time.Now().Add(-2*time.Hour)))
}

func TestFormatTimeAgo_OneHour(t *testing.T) {
	assert.Equal(t, "1h ago", FormatTimeAgo(time.Now().Add(-1*time.Hour)))
}

func TestFormatTimeAgo_Days(t *testing.T) {
	assert.Equal(t, "3d ago", FormatTimeAgo(time.Now().Add(-3*24*time.Hour)))
}

func TestFormatTimeAgo_Weeks(t *testing.T) {
	assert.Equal(t, "2w ago", FormatTimeAgo(time.Now().Add(-14*24*time.Hour)))
}

func TestFormatTimeAgo_Months(t *testing.T) {
	assert.Equal(t, "2mo ago", FormatTimeAgo(time.Now().Add(-60*24*time.Hour)))
}

func TestFormatTimeAgo_ZeroTime(t *testing.T) {
	assert.Equal(t, "", FormatTimeAgo(time.Time{}))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/components/ -run "TestFormatTimeAgo" -v -count=1`
Expected: FAIL — `FormatTimeAgo` undefined

**Step 3: Write minimal implementation**

Add to `internal/ui/components/pipeline.go` after the `FormatDuration` function:

```go
// FormatTimeAgo renders a time as a relative "ago" string.
func FormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/components/ -run "TestFormatTimeAgo" -v -count=1`
Expected: PASS (all 10 tests)

**Step 5: Commit**

```
feat: add FormatTimeAgo relative timestamp formatter
```

---

### Task 2: FormatTimeAbsolute function + tests

**Files:**
- Modify: `internal/ui/components/pipeline.go` (add after FormatTimeAgo)
- Modify: `internal/ui/components/components_test.go` (add tests)

**Step 1: Write the failing tests**

Add to `internal/ui/components/components_test.go`:

```go
// --- FormatTimeAbsolute ---

func TestFormatTimeAbsolute_Today(t *testing.T) {
	now := time.Now()
	ts := time.Date(now.Year(), now.Month(), now.Day(), 14, 23, 0, 0, now.Location())
	assert.Equal(t, "14:23", FormatTimeAbsolute(ts))
}

func TestFormatTimeAbsolute_ThisYear(t *testing.T) {
	now := time.Now()
	// Pick a date that's definitely not today but in the same year
	ts := time.Date(now.Year(), 1, 1, 9, 5, 0, 0, now.Location())
	if now.Month() == 1 && now.Day() == 1 {
		ts = time.Date(now.Year(), 2, 1, 9, 5, 0, 0, now.Location())
	}
	result := FormatTimeAbsolute(ts)
	assert.Contains(t, result, "09:05")
	assert.Contains(t, result, "Jan") // or Feb if Jan 1
}

func TestFormatTimeAbsolute_OlderYear(t *testing.T) {
	ts := time.Date(2024, 6, 15, 8, 30, 0, 0, time.Local)
	assert.Equal(t, "2024-06-15 08:30", FormatTimeAbsolute(ts))
}

func TestFormatTimeAbsolute_ZeroTime(t *testing.T) {
	assert.Equal(t, "", FormatTimeAbsolute(time.Time{}))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/components/ -run "TestFormatTimeAbsolute" -v -count=1`
Expected: FAIL — `FormatTimeAbsolute` undefined

**Step 3: Write minimal implementation**

Add to `internal/ui/components/pipeline.go`:

```go
// FormatTimeAbsolute renders a time as a wall-clock string.
func FormatTimeAbsolute(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return t.Format("15:04")
	}
	if y1 == y2 {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("2006-01-02 15:04")
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/components/ -run "TestFormatTimeAbsolute" -v -count=1`
Expected: PASS (all 4 tests)

**Step 5: Commit**

```
feat: add FormatTimeAbsolute wall-clock timestamp formatter
```

---

### Task 3: Add timestamps to pipeline runs

**Files:**
- Modify: `internal/ui/components/pipeline.go:88-109` (renderRun method)

**Step 1: Modify renderRun to include relative timestamp**

In `internal/ui/components/pipeline.go`, change the `renderRun` method. Replace lines 92-99:

```go
	elapsed := FormatDuration(run.Elapsed())
	ago := FormatTimeAgo(run.UpdatedAt)

	line := fmt.Sprintf(" %s %s  %s  %s  %s  %s",
		lipgloss.NewStyle().Foreground(dotColor).Render(dot),
		run.Name,
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.HeadBranch),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
	)
```

**Step 2: Run all tests**

Run: `go test ./internal/ui/components/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```
feat: show relative timestamps on pipeline runs
```

---

### Task 4: Add timestamps to review queue

**Files:**
- Modify: `internal/ui/screens/dashboard.go:141-174` (renderReviewItem method)

**Step 1: Add `time` import and modify renderReviewItem**

Add `"time"` to the imports and `"github.com/bruce-kelly/cimon/internal/ui/components"` if not already present (it is — used for Selector).

Wait — `screens` already imports `components`. But it doesn't import `time`. Add `"time"` to the import block.

In `renderReviewItem`, after building the line (line 163-168), add the age:

```go
	age := components.FormatTimeAgo(pr.CreatedAt)

	line := fmt.Sprintf(" %s #%d %s%s  %s",
		lipgloss.NewStyle().Foreground(ciColor).Render(ciDot),
		pr.Number,
		lipgloss.NewStyle().Foreground(escColor).Render(pr.Title),
		agentBadge,
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(age),
	)
```

**Step 2: Run tests**

Run: `go test ./internal/ui/screens/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```
feat: show PR age in review queue
```

---

### Task 5: Add timestamps to agent roster

**Files:**
- Modify: `internal/ui/screens/dashboard.go:176-233` (renderAgentRoster method)

**Step 1: Add timestamp to agent profiles**

In `renderAgentRoster`, modify the profile line (around line 189):

```go
		ago := components.FormatTimeAgo(profile.LastRunAt)

		line := fmt.Sprintf(" %s %s  %s  %.0f%%  %s",
			sparkline,
			profile.WorkflowFile,
			lipgloss.NewStyle().Foreground(outcomeColor).Render(profile.LastOutcome.String()),
			profile.SuccessRate*100,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
		)
```

**Step 2: Add timestamp to dispatched agents**

In the dispatched agents section (around line 220):

```go
			elapsed := components.FormatTimeAgo(agent.StartedAt)

			line := fmt.Sprintf("  %s %s%s  %s",
				lipgloss.NewStyle().Foreground(statusColor).Render(agent.Status),
				agent.Task,
				lipgloss.NewStyle().Foreground(ui.ColorBlue).Render(prLink),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
			)
```

**Step 3: Run tests**

Run: `go test ./internal/ui/screens/ -v -count=1`
Expected: PASS

**Step 4: Commit**

```
feat: show timestamps on agent roster and dispatched agents
```

---

### Task 6: Add absolute timestamps to timeline

**Files:**
- Modify: `internal/ui/screens/timeline.go:73-99` (Render method)

**Step 1: Modify timeline Render to prefix absolute timestamp**

In the `Render` method, change the line construction (around line 83-91):

```go
		elapsed := components.FormatDuration(run.Elapsed())
		absTime := components.FormatTimeAbsolute(run.UpdatedAt)

		line := fmt.Sprintf(" %s  %s %s %s  %s  %s",
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(absTime),
			lipgloss.NewStyle().Foreground(dotColor).Render(dot),
			repoPrefix,
			run.Name,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
		)
```

**Step 2: Run tests**

Run: `go test ./internal/ui/screens/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```
feat: show absolute timestamps on timeline
```

---

### Task 7: Add timestamps to release runs

**Files:**
- Modify: `internal/ui/screens/release.go:73-100` (Render method, release runs section)

**Step 1: Modify release run rendering**

In the release `Render` method, change the run line construction (around line 77-83):

```go
			elapsed := components.FormatDuration(run.Elapsed())
			ago := components.FormatTimeAgo(run.UpdatedAt)

			line := fmt.Sprintf(" %s %s  %s  %s  %s",
				lipgloss.NewStyle().Foreground(dotColor).Render(dot),
				run.Name,
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.HeadBranch),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
			)
```

**Step 2: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS (all 266+ tests)

**Step 3: Commit**

```
feat: show relative timestamps on release runs
```
