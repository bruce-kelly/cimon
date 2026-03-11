# CIMON v2 Design: Compact Command Center

## Summary

CIMON v2 replaces the four-screen TUI (dashboard, timeline, release, metrics) with a two-view model designed to run as a side pane alongside Claude Code. One compact view shows all repos with inline status. Drill into any repo for detail and actions. The data layer is unchanged — polling, SQLite, GitHub client, review scoring all stay. This is a UI reshape, not a rewrite.

## Problem

v1 is organized by data type (runs, PRs, releases, metrics) across four separate screens. Users context-switch between screens to build a mental picture. The real usage pattern is: glance at CI status, see if anything needs attention, act on it, go back to coding. v1 makes that a multi-step process. v2 makes it a glance.

## Identity

CIMON is a cross-repo CI/CD triage layer. It watches everything, surfaces what needs a human, and lets you handle quick actions (rerun, approve, merge) without leaving the terminal. It runs alongside Claude Code as a persistent side pane. For anything requiring thought (debugging, code changes), it points you to the right repo.

## Views

### Compact View (default)

The repo list IS the entire view. One line per repo. Inline detail expands under repos that have active or broken state. Collapses when things resolve.

```
CIMON ──────────────── 12:47

■ repo-a    ✓  2 PRs (2 ready)
■ repo-b    ✓  1 PR  (CI ⧗)
■ repo-c    ✗  3 PRs (1 ready)  NEW
  ci: build ✗  test ✗  4m ago
■ repo-d    ● releasing
  deploy ███████░░░ 7/10  1:22



────────────── active 5s  rl:4830
```

**Repo line format:** `[status dot] repo-name [status icon] [PR summary] [NEW flag]`

- Status dot: colored square matching repo's worst state (green=passing, red=failure, blue=active, amber=warning)
- Status icon: `✓` all passing, `✗` failures, `●` in-progress, `⧗` pending
- PR summary: count of open PRs with readiness breakdown — `(N ready)` means approved + CI passing, `(CI ⧗)` means CI still running
- `NEW` flag: appears when repo state changed since last user interaction (green→red, new PR, PR became merge-ready). Clears when user selects the repo or after a timeout (30s).

**Inline expansion:** Repos with CI failures or in-progress runs auto-expand to show detail lines beneath:

- Failed runs: job names with status dots, time since failure
- Active runs: workflow name, job progress bar (filled/total), elapsed time
- Releasing: same as active but visually distinct (blue dot on the repo line)

PR-only states (ready to merge, pending review) do NOT trigger inline expansion — the PR count on the repo line is sufficient. Inline expansion is reserved for CI activity that demands attention.

Quiet repos show just the single summary line. The view breathes — grows when things are happening, contracts when they resolve.

**Repo ordering:** Sorted by attention priority:
1. Repos with failures (newest failure first)
2. Repos with active runs
3. Repos with ready-to-merge PRs
4. Repos that are all-green

**All-green state:** When every repo is passing and no runs are active, show a minimal confirmation:

```
CIMON ──────────────── 12:47

■ repo-a    ✓  2 PRs (2 ready)
■ repo-b    ✓  1 PR
■ repo-c    ✓  —
■ repo-d    ✓  —



all passing

────────────── idle 30s  rl:4832
```

**Navigation:**
- `w`/`s` (or `j`/`k`): move cursor between repo lines (inline detail is part of the repo block, not separately selectable)
- `Enter`: drill into selected repo
- `M`: batch merge all ready agent PRs across all repos (shows ConfirmBar before executing — this is a global operation that doesn't belong to any single repo)
- `?`: help overlay
- `q`: quit

**Edge cases:**
- **Zero repos:** Show message "No repos configured. Run `cimon init`." in place of repo list.
- **Single repo:** Show compact view normally (one line). User still presses Enter to drill in — auto-entering detail would break the consistent navigation model.

### Detail View (per-repo drill-in)

Entered via `Enter` on a repo in compact view. Same terminal pane, replaces compact content. Shows full context for one repo.

```
CIMON ─ repo-c ──────────── 12:47

CI Pipeline
  main ✗ a1b2c3  4m ago
    build  ✗ 1:42  exit 1
    test   ✗ 0:38  2 failed
    lint   ✓ 0:12
  main ✓ f4e5d6  2h ago
  main ✓ 8a9b0c  5h ago

Pull Requests
  #201  Fix auth   agent  3h CI✗
  #198  Add cache         1d CI✓
  #195  Bump deps  agent  2d CI✓ ✔

[r]rerun [A]approve [m]merge
[v]diff  [o]browser [Esc]back
────────────── active 5s  rl:4830
```

**Sections:**

1. **CI Pipeline** — Recent runs for this repo, newest first. Each run shows: branch, status icon, short SHA, time ago. Selected run expands to show jobs with conclusion, elapsed time, and failure detail (failed step name or exit code). Runs are grouped by workflow group if the config defines groups, otherwise flat.

2. **Pull Requests** — Open PRs sorted by review queue score (same algorithm as v1). Each shows: number, title (truncated), agent badge if detected, age, CI status, approval checkmark. Dismissed PRs hidden.

**Navigation:**
- `w`/`s` (or `j`/`k`): move cursor linearly through all items — runs first, then PRs. The cursor flows across sections without a section-switch key. A visual separator (the "Pull Requests" heading) makes the boundary clear.
- `Esc`: back to compact view (but if log pane is open, first Esc closes the log pane; second Esc returns to compact)
- `l`: toggle log pane (horizontal split)

**Actions (on selected item):**
- `r`: rerun (smart — reruns failed jobs if the run failed, full rerun otherwise)
- `A`: approve PR (uppercase — lowercase `a` reserved for WASD left)
- `m`: merge PR (squash, shows ConfirmBar before executing)
- `x`: dismiss PR from review queue
- `v`: view diff (PR) or job logs (run) in log pane
- `o`: open in browser

Action keys that don't apply to the selected item type are silently ignored (e.g., `r` on a PR, `A` on a run).

### Log Pane

Toggled with `l` in detail view only. Splits the pane horizontally. Shows content relevant to the selected item:

- **Run selected:** job logs for failed jobs (last 4000 chars, same as v1)
- **PR selected:** unified diff with syntax highlighting (added/removed lines, same as v1 LogPane)

Press `l` to cycle: hidden → half-split → full-pane → hidden. `Esc` closes the log pane (does not exit detail view — a second `Esc` does that).

When the log pane has focus, `↑`/`↓` scroll the log content. `l` cycles the pane size. All other keys pass through to the detail view beneath.

## Header and Footer

**Header:** `CIMON` left-aligned, current time right-aligned. In detail view, repo name appears after CIMON. Minimal — one line.

**Footer:** Status line showing poll state (`idle`/`active` + interval), rate limit remaining (prefixed `rl:`). In detail view, action key hints replace the status line. One line.

## Change Detection (NEW flag)

The `NEW` flag on a repo line serves as a peripheral-vision tap on the shoulder. It fires when:

- A run transitions from passing to failing (new failure)
- A PR becomes merge-ready (CI passes + approved)
- A new agent PR is opened
- A release workflow starts

It does NOT fire for:
- Known flakes that fail again
- Runs completing successfully (that's normal, not noteworthy)
- PR updates that don't change readiness

The flag clears when the user moves the cursor to that repo (acknowledging it) or after 30 seconds.

Implementation: each repo model tracks a `lastNotableChange` timestamp and a `userAcknowledged` flag. The compact view checks these to render the NEW badge. The 30-second expiry requires a periodic `tea.Tick` command — piggyback on a 10-second tick (same cadence as the existing agent tick in v1) that checks for stale NEW flags each cycle.

## Data Layer

**No changes to:**
- `internal/github/` — client, caching, rate limits, all API calls
- `internal/db/` — schema, queries, persistence
- `internal/polling/` — poller, adaptive cadence
- `internal/config/` — config parsing, detection
- `internal/models/` — data structures
- `internal/review/` — scoring, escalation
- `internal/confidence/` — scoring (still computed, just not displayed on a screen)

**Changes to `internal/app/app.go`:**
- Replace screen switching (4 screens) with view switching (compact ↔ detail)
- Replace screen models with CompactViewModel and DetailViewModel
- `handlePollResult` still persists data and rebuilds state, but populates the new view models
- Key dispatch simplified: compact keys vs detail keys vs log pane keys

**Removed from TUI (kept in data layer):**
- Timeline screen — chronological cross-repo feed. Data still in DB, no TUI screen.
- Release screen — confidence scoring. Data still computed, accessible via `cimon release` CLI if desired.
- Metrics screen — historical stats. Data still in DB, accessible via `cimon stats` CLI.

## What Gets Cut from v1

| v1 Feature | v2 Status |
|---|---|
| Dashboard screen (3-panel) | Replaced by compact view + detail drill-in |
| Timeline screen | Removed from TUI. Data in DB. |
| Release screen | Removed from TUI. Confidence still computed internally. |
| Metrics screen | Removed from TUI. Stats accessible via CLI. |
| Screen tab bar (1/2/3/4) | Removed. No screens to switch. |
| FilterBar (/ search) | Removed from compact. Could add to detail view later if needed. |
| ActionMenu (Enter popup) | Removed. Actions are direct key presses in detail view. |
| ConfirmBar (y/n) | Kept for destructive actions (merge). |
| CatchupOverlay | Removed. The NEW flag replaces idle-return catchup. |
| HelpOverlay (?) | Kept, simplified for two-view model. |
| Agent dispatch (D key) | Removed from v2 initial. Can be re-added to detail view later. |
| Agent roster panel | Removed. Agent PRs visible in PR lists. Running agents not tracked in TUI. |
| Auto-fix copilot | Removed from TUI. Could run as daemon feature later. |
| Cron scheduler | Removed from TUI. Could run as daemon feature later. |
| Sparkline component | Removed (was used for agent roster). |
| Flash messages | Kept for action feedback in detail view. |

## What Gets Added

| Feature | Description |
|---|---|
| Compact view | Repo list with inline expansion. The new default. |
| Detail view | Per-repo drill-in replacing the dashboard. |
| NEW flag | Change detection indicator on repo lines. |
| Repo ordering | Attention-priority sorting (failures first). |
| Inline expansion | Auto-expanding detail under active/failing repos. |
| Batch merge (M) | Kept from v1, works from compact view globally. |
| Mouse click | Click repo to drill in (Bubbletea v2 mouse support). Requires `tea.EnableMouseCellMotion` and handling `tea.MouseClickMsg`. |

## Internal Model Structure

```
App
├── mode: Compact | Detail
├── selectedRepo: int (cursor position in compact)
├── repos: []RepoState
│   ├── config: RepoConfig
│   ├── runs: []WorkflowRun (from last poll)
│   ├── prs: []PullRequest (from last poll)
│   ├── reviewItems: []ReviewItem (scored)
│   ├── inlineStatus: InlineStatus (computed: worst state, active runs, failed jobs)
│   ├── prSummary: PRSummary (computed: total, ready count)
│   ├── newFlag: bool
│   └── lastNotableChange: time.Time
├── detailView: DetailModel (active when mode=Detail)
│   ├── repoIndex: int
│   ├── cursor: int (linear index across runs then PRs)
│   ├── runCount: int (boundary: cursor < runCount means run selected)
│   └── logPane: LogPane
├── flash: Flash
├── help: HelpOverlay
├── confirm: ConfirmBar
├── poller: *Poller
├── db: *Database
└── client: *github.Client
```

Each `RepoState` is recomputed on every poll result (all repos rebuilt when any single repo's poll completes — same as v1's `rebuildScreenData`). This is O(repos × runs) but negligible for < 10 repos. The compact view reads `repos` directly. The detail view reads `repos[detailView.repoIndex]` for its data.

## Keybinding Summary

### Compact View
| Key | Action |
|---|---|
| `w`/`k` | Cursor up (previous repo) |
| `s`/`j` | Cursor down (next repo) |
| `Enter` | Drill into selected repo |
| `M` | Batch merge all ready agent PRs (with confirm) |
| `?` | Help overlay |
| `q` | Quit |

### Detail View
| Key | Action |
|---|---|
| `w`/`k` | Cursor up (previous item) |
| `s`/`j` | Cursor down (next item) |
| `Esc` | Close log pane (if open), else back to compact |
| `r` | Rerun selected run (no-op on PRs) |
| `A` | Approve selected PR (no-op on runs) |
| `m` | Merge selected PR with confirm (no-op on runs) |
| `x` | Dismiss selected PR (no-op on runs) |
| `v` | View diff/logs in log pane |
| `o` | Open in browser |
| `l` | Toggle log pane (hidden → half → full → hidden) |
| `↑`/`↓` | Scroll log pane content (when log pane open) |
| `?` | Help overlay |
| `q` | Quit |

## Terminal Size

- **Minimum width:** 36 columns (shortest repo line: `■ repo-x  ✓  —`). Repo names show only the repo portion (not `owner/`) in compact view to save width. Full `owner/repo` shown in the detail view header.
- **Minimum height:** 10 rows (header + 4 repos + footer + padding)
- **Optimal width:** 40-50 columns for compact, 60+ for detail view with log pane
- **Designed for side-pane use:** works at half a standard terminal (80÷2 = 40 cols)

## Config Changes

No config format changes. All existing `.cimon.yml` fields continue to work. The following config fields become unused by the TUI but are not removed (they still feed the data layer):

- `catchup.enabled`, `catchup.idle_threshold` — catchup overlay removed
- `agents.scheduled` — scheduler not in TUI (could be daemon feature later)
- `notifications` — desktop notifications not wired in v2 TUI (could fire on NEW flag events in daemon mode later)
- `theme.palette` — still used, Tokyo Night remains default

## Migration

This is a breaking UI change. There is no v1→v2 transition mode. Users upgrading get the new two-view model. Since CIMON is pre-release (no tagged version yet), this is acceptable.

## Future Extensions (not in v2 scope)

These are enabled by the architecture but not built:

- **Daemon mode:** `cimon --daemon` runs headless, polls, fires webhooks. `cimon attach` connects TUI.
- **Webhook notifier:** POST to Slack/Discord/ntfy on NEW-flag events.
- **Agent fleet status:** Add agent section to detail view when agents are running.
- **Agent dispatch:** Re-add `D` key in detail view to dispatch fix agent for a failed run.
- **Filter bar:** Re-add `/` search in detail view for repos with many runs/PRs.
- **CLI commands:** `cimon stats`, `cimon timeline`, `cimon release` for data that left the TUI.
