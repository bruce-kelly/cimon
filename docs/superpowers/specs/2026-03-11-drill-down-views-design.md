# Drill-Down Views: Run Detail & PR Detail

**Date:** 2026-03-11
**Status:** Design

## Overview

Add two new view modes to CIMON's TUI — `ViewRunDetail` and `ViewPRDetail` — providing a third level of drill-down from the existing detail view. Simultaneously remap all keybindings to a left-hand WASD + numbered action scheme, and fix the goroutine mutation bug that prevents async actions (log pane, flash messages) from working.

## Implementation Phases

This spec covers three coupled changes, implemented in order:

1. **Phase 1: Fix goroutine mutation bug** — prerequisite for all async actions working correctly. Convert `go func()` patterns in action handlers and `ConfirmBar.OnConfirm` callbacks to `tea.Cmd` → `tea.Msg` pattern.
2. **Phase 2: Keybinding overhaul** — WASD navigation + 1/2/3/E/R action scheme. Independent of new views but shipped together for a clean transition.
3. **Phase 3: Drill-down views** — `ViewRunDetail` and `ViewPRDetail` view modes.

## Navigation Model

```
Compact ──D/Enter──▸ Detail (repo) ──D/Enter──▸ RunDetail or PRDetail
                                                  ↑ depends on cursor
Compact ◂──A/Esc─── Detail ◂──A/Esc──── RunDetail / PRDetail
```

- A/Esc always goes back one level
- D/Enter always drills into the selected item
- A at compact level is a no-op (already at top)
- Returning to detail view preserves cursor position
- Log pane clears on A/Esc back

## Keybinding Scheme

Left hand stays on WASD home position. Right hand is free.

All WASD keys are **lowercase**. `a`/`d`/`w`/`s` — no Shift required.

### Navigation (all views)

| Key | Action |
|-----|--------|
| `w` / `↑` | Cursor up |
| `s` / `↓` | Cursor down |
| `d` / `Enter` | Drill in / expand |
| `a` / `Esc` | Back / collapse |

`a` at compact level is a no-op (already at top). `j`/`k`/`h`/`l` are removed — WASD is the sole letter-key navigation.

**Log pane scrolling:** When the log pane is visible, `↑`/`↓` (arrow keys only) scroll the log pane content. `w`/`s` still navigate the cursor (items/jobs/files). This matches the current behavior — arrow keys are "scroll what's under focus", letter keys are "navigate the list."

### New KeyMap Struct

```go
var Keys = KeyMap{
    Up:       key.NewBinding(key.WithKeys("w", "up"), key.WithHelp("w/↑", "up")),
    Down:     key.NewBinding(key.WithKeys("s", "down"), key.WithHelp("s/↓", "down")),
    DrillIn:  key.NewBinding(key.WithKeys("d", "enter"), key.WithHelp("d/enter", "select")),
    Back:     key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a/esc", "back")),
    Action1:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "")),     // context-sensitive
    Action2:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "")),
    Action3:  key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "")),
    Examine:  key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "log cycle")),
    Remote:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "github")),
    Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
    Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
```

Bindings removed: `Enter` (now `DrillIn`), `Escape` (now `Back`), `Rerun`, `Approve`, `Merge`, `Dismiss`, `ViewDiff`, `Open`, `LogCycle`, `BatchMerge`.

### Actions (context-sensitive per view)

| Key | Compact | Detail | Run Detail | PR Detail |
|-----|---------|--------|------------|-----------|
| 1 | batch merge | rerun (run) / approve (PR) | rerun | approve |
| 2 | — | view diff/logs | rerun failed | merge |
| 3 | — | dismiss (PR only) | — | dismiss |
| E | — | log cycle | log cycle | log cycle |
| R | — | open github | open github | open github |
| Q | quit | quit | quit | quit |
| ? | help | help | help | help |

### Removed bindings

- `j`/`k` — replaced by `s`/`w` (already existed as aliases, now primary)
- `h`/`l` — not used
- `r` (rerun), `A` (uppercase approve), `m` (merge), `x` (dismiss), `v` (view), `o` (open), `M` (batch merge) — all replaced by numbered/lettered scheme above
- Note: old `A` (uppercase) was approve. New `a` (lowercase) is back. No conflict.

## View Layouts

### Run Detail View

```
CIMON ── owner/repo ── CI Pipeline ─────────────────── 15:04

  main ✗ abc123  push by bfk                        3m ago
  failure · 2/3 jobs passed · elapsed 4m12s

  Jobs
▸ ✓ build          1m03s   ubuntu-latest
  ✗ test           2m44s   ubuntu-latest
    ▸ Setup         ✓
    ▸ Run tests     ✗
    ▸ Cleanup       ✓
  ✓ lint           0m25s   ubuntu-latest

───────────────── logs ──────────────────────────────────
  === test ===
  FAIL TestFoo: expected 3, got 4
  ...

[1]rerun [2]rerun-failed [E]logs [R]github [A]back
```

**Content:**
- Run summary: branch, status dot, SHA (6 char), event, actor, age
- Stats line: conclusion, job pass/total, elapsed duration
- Jobs list with status dots, duration, runner name
- Cursor (W/S) navigates jobs
- D on a job expands/collapses its steps inline
- Failed jobs auto-expand steps on entry
- Log pane auto-opens (half) with failed job logs on entry

**Data source:** `WorkflowRun.Jobs` (already fetched by poller if configured), `GetJobs()` on-demand if jobs are empty, `GetFailedLogs()` for log pane content.

### PR Detail View

```
CIMON ── owner/repo ── Pull Request ────────────────── 15:04

  #142  Fix authentication timeout handling
  bfk · 2d ago · +47 -12 (59 lines)   agent
  CI✓  review: approved ✔

  Files Changed (4)
▸ internal/auth/client.go          +31 -8
  internal/auth/client_test.go     +12 -0
  internal/config/config.go         +3 -3
  internal/config/config_test.go    +1 -1

───────────────── diff ──────────────────────────────────
  diff --git a/internal/auth/client.go ...
  @@ -45,8 +45,12 @@
  +    ctx, cancel := context.WithTimeout(...)
  ...

[1]approve [2]merge [3]dismiss [E]diff [R]github [A]back
```

**Content:**
- PR summary: number, title (full), author, age, size (+/-), agent badge
- Status line: CI status, review state
- File list parsed from diff (filename + per-file +/- counts)
- Cursor (W/S) navigates file list
- D on a file scrolls log pane to that file's hunk
- Diff auto-loads into log pane (half) on entry
- E cycles log pane (hidden → half → full → hidden)

**Data source:** `GetPullDiff()` for diff content and file list parsing. Diff is fetched once on entry via `tea.Cmd`, parsed into file list for cursor navigation.

## Prerequisite: Fix Goroutine Mutation Bug

### Problem

Action handlers (`handleViewDiff`, `handleRerun`, `handleApprove`, `handleMerge`, `handleOpen`) spawn goroutines that mutate `a.flash` and `a.logPane` on a value-receiver copy of `App`. The mutations never propagate back to the actual model.

### Solution

Convert async operations to the `tea.Cmd` → `tea.Msg` pattern:

1. Define result message types:
   ```go
   type actionResultMsg struct {
       Message string
       IsError bool
   }
   type diffResultMsg struct {
       Title   string
       Content string
       Err     error
   }
   type logsResultMsg struct {
       Title   string
       Content string
       Err     error
   }
   type jobsResultMsg struct {
       RunID int64
       Jobs  []models.Job
       Err   error
   }
   ```

2. Action handlers extract all needed values to local variables, then return `tea.Cmd`. The closure must NOT reference `a` (value-receiver copy):
   ```go
   func (a App) handleRerun(run *WorkflowRun) (tea.Model, tea.Cmd) {
       client := a.client  // pointer — survives copy
       ctx := a.ctx        // interface — survives copy
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

   **Also applies to `ConfirmBar.OnConfirm` callbacks.** The merge/batch-merge confirm handlers currently spawn goroutines inside `OnConfirm` that mutate `a.flash` on a copy. Fix:

   - Change `ConfirmBar.OnConfirm` callback signature from `func()` to `func() tea.Cmd`
   - Change `ConfirmBar.OnCancel` from `func()` to `func() tea.Cmd` (for consistency)
   - Change `ConfirmBar.HandleKey` return type from `bool` to `(bool, tea.Cmd)` — it returns the cmd from `OnConfirm`/`OnCancel` when the user presses `y`/`n`
   - Update call site in `handleKey` (`app.go:332-335`):
     ```go
     if a.confirmBar.Active {
         handled, cmd := a.confirmBar.HandleKey(msg.String())
         if handled {
             return a, cmd
         }
     }
     ```

3. `Update()` handles result messages and mutates the real model. **Guard against stale responses:** if the user navigated away before the async result arrives, ignore the result (except `actionResultMsg` which always shows flash regardless of view).

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
       // If in PR detail view, also parse file list
       if a.mode == ui.ViewPRDetail && a.prDetailView != nil {
           a.prDetailView.SetFiles(views.ParseDiffFiles(msg.Content))
       }
       return a, nil

   case logsResultMsg:
       if a.mode != ui.ViewRunDetail {
           return a, nil // navigated away, discard
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
           return a, nil // navigated away, discard
       }
       if msg.Err != nil {
           a.flash.Show("Failed to fetch jobs: "+msg.Err.Error(), true)
           return a, nil
       }
       a.runDetailView.SetJobs(msg.Jobs)
       // Auto-fetch logs for failed jobs
       return a, a.fetchFailedLogs(msg.Jobs)
   ```

   The `fetchFailedLogs` helper returns a `tea.Cmd` that loops over failed jobs, calls `GetFailedLogs` for each, concatenates the output, and returns a `logsResultMsg`.

This pattern is already used by the poller (`pollResultMsg`). Same approach, applied to user-triggered actions.

## New Files

| File | Contents |
|------|----------|
| `internal/ui/views/rundetail.go` | `RunDetailView` struct, `Render()`, job/step rendering, cursor over jobs |
| `internal/ui/views/rundetail_test.go` | Render output, cursor, step expansion, empty states |
| `internal/ui/views/prdetail.go` | `PRDetailView` struct, `Render()`, file list, diff parsing |
| `internal/ui/views/prdetail_test.go` | Render output, cursor, file list parsing, empty states |
| `internal/ui/views/diffparse.go` | `ParseDiffFiles()` — extract file names and per-file +/- counts from raw unified diff |
| `internal/ui/views/diffparse_test.go` | Diff parsing tests |

## Modified Files

| File | Changes |
|------|---------|
| `internal/ui/app.go` | Add `ViewRunDetail`, `ViewPRDetail` to ViewMode enum + String() |
| `internal/ui/keys.go` | Replace KeyMap with WASD + 1/2/3/E/R/Q scheme |
| `internal/app/app.go` | Add message types, `handleRunDetailKey()`, `handlePRDetailKey()`, fix goroutine mutation bug, add `runDetailView`/`prDetailView` fields, update `handleKey` switch (add `ViewRunDetail`/`ViewPRDetail` cases), update `handlePollResult` for new views, update `View()` and `renderHeader()`/`renderFooter()`, add `fetchFailedLogs` helper |
| `internal/app/app_test.go` | Add tests for new navigation, key handlers, message handling |
| `internal/ui/components/confirmbar.go` | Change `OnConfirm`/`OnCancel` signature to `func() tea.Cmd`, change `HandleKey` return to `(bool, tea.Cmd)` |
| `internal/ui/components/help.go` | Update help overlay with new keybindings per view |

## Diff Parser

Simple parser for unified diff format. Extracts from `GetPullDiff()` output:

```go
type DiffFile struct {
    Path      string
    Additions int
    Deletions int
    Offset    int // line offset into raw diff for log pane scroll-to-file
}

func ParseDiffFiles(raw string) []DiffFile
```

Parses `diff --git a/path b/path` headers and counts `+`/`-` lines per file. Records line offset so the log pane can jump to a file's hunk when `d` is pressed on a file in the file list.

Lives in `internal/ui/views/diffparse.go` (not a separate package — only used by `PRDetailView`).

## View()-Level Log Pane Condition

Current code (`app.go:645`) only renders the log pane when `a.mode == ui.ViewDetail`. This must be extended:

```go
supportsLogPane := a.mode == ui.ViewDetail || a.mode == ui.ViewRunDetail || a.mode == ui.ViewPRDetail
if a.logPane.Mode != components.LogPaneHidden && supportsLogPane {
```

## Header Breadcrumbs (4 variants)

| ViewMode | Header |
|----------|--------|
| `ViewCompact` | `CIMON ─── 15:04` |
| `ViewDetail` | `CIMON ── owner/repo ─── 15:04` |
| `ViewRunDetail` | `CIMON ── owner/repo ── CI Pipeline ─── 15:04` |
| `ViewPRDetail` | `CIMON ── owner/repo ── Pull Request ─── 15:04` |

## Footer Action Hints (4 variants)

| ViewMode | Footer |
|----------|--------|
| `ViewCompact` | `[1]batch-merge [?]help [Q]quit` + status text |
| `ViewDetail` | `[1]rerun/approve [2]diff/logs [3]dismiss [E]log [R]github [A]back` |
| `ViewRunDetail` | `[1]rerun [2]rerun-failed [E]logs [R]github [A]back` |
| `ViewPRDetail` | `[1]approve [2]merge [3]dismiss [E]diff [R]github [A]back` |

Flash messages and confirm bar override the footer as they do today.

## Poll Result Handling for New Views

When a `pollResultMsg` arrives while `a.mode == ViewRunDetail` or `ViewPRDetail`:

- **Run detail:** Find the matching run by ID in the updated `allRuns`. If found, update `RunDetailView.Run` in place (preserves cursor on jobs). If the run disappeared (e.g., deleted), flash "Run no longer available" and navigate back to detail.
- **PR detail:** Find the matching PR by number in updated `allPulls`. If found, update `PRDetailView.PR` in place. If merged/closed, flash "PR #N was closed" and navigate back to detail.

This follows the same pattern as the existing detail view cursor preservation in `handlePollResult` (`app.go:297-308`).

## Loading and Empty States

### Run Detail — jobs loading

When entering a run with empty `Jobs`:
- Render `"Loading jobs..."` in muted text where the jobs list would be
- Fire `tea.Cmd` to `GetJobs()` → `jobsResultMsg`
- On receipt, populate `RunDetailView.Jobs`, set cursor count, auto-expand failed

### Run Detail — active run

When the selected run has `Status == "in_progress"` or `"queued"`:
- Show `● in_progress` or `◌ queued` status dot (blue) instead of ✗/✓
- Jobs may still be updating — poll results will refresh them in place
- No auto-log-fetch (no failed logs yet)

### PR Detail — diff loading

When entering PR detail:
- Render `"Loading diff..."` in muted text where the file list would be
- Fire `tea.Cmd` to `GetPullDiff()` → `diffResultMsg`
- On receipt, parse file list, populate `PRDetailView.Files`, set cursor count, load diff into log pane

## Job Step Expansion and Cursor

The `RunDetailView` uses a flat `Selector` cursor over jobs. When `d` expands a job's steps:

- The expanded steps are **not** cursor-navigable — they display as indented detail lines under the job
- `d` toggles expand/collapse on the selected job
- `w`/`s` still moves between jobs (steps are visual-only, like the current inline expansion in `detail.go:79-83`)
- Selector count = number of jobs (does not change on expand/collapse)

This keeps the cursor model simple. Step-level selection is out of scope.

## Relationship to Existing Components

- **`RunDetailView` does NOT reuse `PipelineView`** — `PipelineView` (`components/pipeline.go`) renders multiple runs with selector/filter; `RunDetailView` renders a single run's jobs/steps with a different layout. New rendering code in `rundetail.go`.
- **`PRDetailView` reuses `LogPane`** for diff rendering — the existing diff highlighting (`ClassifyLine`) works as-is.
- Both views use the existing `Selector` component for cursor navigation.
- Both views use the existing `Flash` and `ConfirmBar` components (via App, not directly).

## Entry Behavior

### Entering Run Detail

1. App sets `mode = ViewRunDetail`, creates `RunDetailView` from selected run
2. If `run.Jobs` is empty, fire `tea.Cmd` to `GetJobs()` → `jobsResultMsg` → populate jobs
3. If any job has `conclusion == "failure"`, auto-expand its steps
4. Fire `tea.Cmd` to `GetFailedLogs()` for failed jobs → `logsResultMsg` → populate log pane (half mode)

### Entering PR Detail

1. App sets `mode = ViewPRDetail`, creates `PRDetailView` from selected review item
2. Fire `tea.Cmd` to `GetPullDiff()` → `diffResultMsg` → parse file list + populate log pane (half mode)

### Exiting (A/Esc)

1. Clear log pane
2. Set `mode = ViewDetail`
3. Detail view cursor position preserved (stored before drill-in)

## Testing Strategy

- **View rendering tests:** Each view struct tested with mock data, assert output contains expected elements (status dots, job names, file paths, etc.)
- **Diff parser tests:** Various diff formats (single file, multi-file, binary files, renames, empty diffs)
- **Navigation tests in app_test.go:** D from detail → run/PR detail, A/Esc back, cursor preservation
- **Message handling tests:** `jobsResultMsg`, `diffResultMsg`, `logsResultMsg`, `actionResultMsg` all update correct state
- **Keybinding tests:** Verify 1/2/3/E/R dispatch correct actions per view mode

All tests use in-memory data and mock HTTP (httptest). No network calls.
