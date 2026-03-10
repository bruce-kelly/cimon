# CIMON Control Panel Overhaul — Design

**Goal:** Transform cimon from a basic status list into a dense, Kojima-style live control panel. Real-time job progress, richer visual indicators, and more information at a glance.

---

## 1. Live Job Progress

### Log pane auto-refresh for in-progress runs

When the user presses Enter/v on an active run (any screen), the log pane:
- Opens with `LIVE ● 3s` header (countdown to next poll)
- Shows job grid updating in-place each poll cycle
- Tracks which run ID is being viewed; `rebuildScreenData` refreshes the content if that run is still active
- Clears LIVE indicator when the run completes

**Data flow:**
- `App` stores `liveRunID int64` + `liveRunRepo string`
- On each poll result, if `liveRunID != 0`, re-fetch jobs and re-render log pane content
- Log pane header shows `LIVE ● {countdown}s` using `time.Since(lastPoll)` and poll interval

### Job grid format in log pane
```
Run #12345  Release v0.4.0
Status: in_progress  Branch: v0.4.0  Actor: bfk
Started: 19:48:59  Elapsed: 2m15s

  ✓ build-linux     1m02s
  ● build-macos     0m45s...
  ○ build-windows   queued
  ○ publish          queued
  ○ create-release   queued

  [3/5 jobs]  ██████░░░░ 60%
```

---

## 2. Visual Polish

### Run rows everywhere (pipeline, timeline, release)

Enrich every run row with a compact info line:
```
● CI Pipeline  main  abc1234  bfk  [3/5] ██░░░ 1m30s  2m ago
```

Components: status dot, name, branch, short SHA, actor, job progress `[done/total]`, mini progress bar, elapsed, relative time.

### Job progress bar helper

New `RenderJobProgress(jobs []Job) string` in components:
- Counts completed/total jobs
- Returns `[3/5] ██████░░░░` with color (green if all pass, amber if in-progress, red if any failed)

### Pulsing status dot

In-progress runs show a pulsing dot — alternate between bright (`●`) and dim (`○`) on each render cycle. Track a `tickEven bool` on the App that toggles with each poll/tick.

### Box-drawing borders on dashboard panels

Wrap each panel in thin borders using `lipgloss.Border(lipgloss.RoundedBorder())` instead of just a text header. Panel header rendered inside the border top.

### Screen tab badges

Top bar shows failure/active counts per screen:
```
[1]dashboard  2 timeline  3 release●  4 metrics
```
- `●` after release if any release is in-progress
- Number badge if failures exist on that screen

---

## 3. Information Density

### Pipeline rows

Current: `● CI Pipeline  main  1m30s  2m ago`
New: `● CI Pipeline  main  abc1234  bfk  [3/5]██░░░  1m30s  2m ago`

Added: short SHA, actor, job progress.

### Timeline rows

Current: `19:48  ● bruce-kelly/augury  CI Pipeline  bfk  1m30s`
New: `19:48  ● augury  CI Pipeline  bfk  [3/5]██░░░  1m30s`

Added: job progress. Shortened repo to just name (owner visible in header/context).

### Release screen

Current: shows runs as simple list with status dot.
New: inline job grid for selected run (not just in log pane). Confidence bar always visible in header area.

### Dashboard failure inline

When a pipeline run has failed jobs, show the first failed step name inline:
```
✗ CI Pipeline  main  [4/5]  ▸ test-integration  2m30s
```

---

## Implementation scope

All changes are in the Go codebase at `/home/bfk/projects/cimon/`:

| Area | Files |
|------|-------|
| Job progress bar | `internal/ui/components/progress.go` (new) |
| Enriched run rows | `internal/ui/components/pipeline.go`, `internal/ui/screens/timeline.go`, `internal/ui/screens/release.go` |
| Live log pane refresh | `internal/app/app.go` (liveRunID tracking, poll handler) |
| Pulsing dot | `internal/app/app.go` (tickEven toggle), `internal/ui/theme.go` |
| Box borders | `internal/ui/screens/dashboard.go` |
| Tab badges | `internal/app/app.go` View() top bar |
| Failure inline | `internal/ui/components/pipeline.go` |
