# Views Architecture

The UI layer is built around Bubbletea view models in `internal/app/`, `internal/ui/views/`, and `internal/ui/components/`.

All rendering is string-based and styled with lipgloss. The root `App.View()` wraps the rendered string in a `tea.View` and enables the alternate screen.

---

## App Root (`internal/app/app.go`)

`App` is the root Bubbletea model. It owns:

- config, GitHub client, SQLite database, and poller
- current `ui.ViewMode`
- the four view models (`CompactView`, `DetailView`, `RunDetailView`, `PRDetailView`)
- overlays (`HelpOverlay`, `Flash`, `ConfirmBar`, `LogPane`)
- per-repo poll caches used to rebuild `RepoState`

### Startup

`NewApp()` derives workflow metadata from config, loads dismissed PRs from SQLite, creates the poller result channel, builds initial repo state, and initializes the compact view.

### Poll handling

Each `PollResult`:

- updates `allRuns` and `allPulls`
- persists runs, jobs, and PRs to SQLite
- rebuilds `RepoState` values and re-sorts repos by attention
- detects `NEW` flags by comparing current and previous repo state
- refreshes the active detail/run-detail/pr-detail view when possible
- updates footer status text with cadence and current rate limit

### Rendering

`View()` renders a two-bar layout:

- fixed header with `CIMON`, the active mode label, and the current time
- content area containing the active view, optionally split with the log pane
- fixed footer showing either a confirm prompt, flash message, or status/help text

The app truncates content to fit between the header and footer because lipgloss height padding does not truncate.

### Key dispatch

`Update()` routes `tea.KeyPressMsg` by active mode:

- `compact` handles repo navigation, drill-in, and batch merge
- `detail` handles run/PR selection, drill-in, rerun/dispatch/approve, diff viewing, dismissal, and remote open
- `run-detail` handles job navigation, step expansion, rerun, rerun-failed, and remote open
- `pr-detail` handles file navigation, diff jump, approve, merge, dismiss, and remote open

The help overlay and confirm bar intercept keys before view-specific handlers.

---

## Views (`internal/ui/views/`)

Each view is a focused struct with a `Render(width, height)` method plus a small amount of cursor or expansion state.

### Compact View (`compact.go`)

The compact view renders one summary line per repo using `RepoState`.

- Uses `Selector` for repo navigation
- Shows repo health, PR summary, and `NEW` badges
- Auto-expands critical failures with failed jobs and age
- Shows an inline note for agent-only failures
- Shows inline progress bars for active runs
- Clears the selected repo's `NEW` flag on navigation or drill-in

### Detail View (`detail.go`)

The detail view renders one repo at a time.

- Deduplicates runs to the latest run per workflow file
- Sorts runs by workflow group priority: CI, builds, release/deploy, agents, then other
- Renders runs first, then review items for open PRs
- Uses a single linear cursor across both runs and PRs
- Expands selected run rows with job summaries when jobs are already available

### Run Detail View (`rundetail.go`)

The run detail view renders a single workflow run.

- Shows run summary, elapsed time, actor, and event
- Lists jobs with per-job status and runner name
- Auto-expands failed jobs
- Allows toggling step visibility per job
- Starts in a loading state if jobs have not been fetched yet

### PR Detail View (`prdetail.go`)

The PR detail view renders a single pull request.

- Shows title, author, age, size, CI state, review state, and agent/draft badges
- Parses the raw unified diff into a file list with additions/deletions
- Uses a cursor across changed files
- Starts in a loading state until diff content arrives

### Repo State (`repostate.go`)

`RepoState` is the bridge between poll data and the views.

It combines:

- raw runs and active PRs for one repo
- computed inline repo status
- PR summary counts
- review queue items
- workflow-file-to-group labels
- `NEW`-flag metadata

`ComputeInlineStatus()` classifies critical failures separately from agent workflow failures and only considers the latest completed run per workflow file.

---

## Components (`internal/ui/components/`)

### `Selector`

Shared cursor helper used by the views.

- `Next()` / `Prev()` wrap around
- `SetCount(n)` clamps the cursor when item count changes
- `Index()` returns the active row

### `PipelineView`

Pipeline-oriented formatter used for run/job presentation.

- renders job stages with status dots
- formats durations and time-ago strings
- tags known failures when supplied

### `ConfirmBar`

Bottom-bar confirmation prompt for destructive actions.

- activated by merge and dismiss flows
- consumes `y`, `n`, and `Esc`
- returns a `tea.Cmd` when confirmation should continue the action

### `Flash`

Short-lived bottom-bar feedback for success and error results.

### `LogPane`

Context pane for diffs and failed-job logs.

- modes: hidden, half-height, full-height
- diff-aware line highlighting
- scrollable with arrow keys when visible
- can be populated from PR diffs or fetched workflow logs

### `HelpOverlay`

Context-sensitive keybinding overlay keyed off `ViewMode.String()`.

---

## Keybindings (`internal/ui/keys.go`)

`KeyMap` defines the actual bindings used by the app:

| Key | Binding | Meaning |
|---|---|---|
| `w`, `↑` | `Up` | Move selection up |
| `s`, `↓` | `Down` | Move selection down |
| `d`, `Enter` | `DrillIn` | Select / drill in / expand |
| `a`, `Esc` | `Back` | Back out / close log pane |
| `1` | `Action1` | Context-sensitive primary action |
| `2` | `Action2` | Context-sensitive secondary action |
| `3` | `Action3` | Context-sensitive tertiary action |
| `e` | `Examine` | Toggle log pane |
| `r` | `Remote` | Open selected item on GitHub |
| `?` | `Help` | Toggle help |
| `q`, `ctrl+c` | `Quit` | Quit |

---

## Theme (`internal/ui/theme.go`)

The app uses a Tokyo Night-inspired palette exposed from `internal/ui/theme.go`.

The most important semantic colors are:

- green for success
- red for critical failures
- amber for agent failures and pending states
- blue for active/running states
- muted gray for secondary text
- selection background for the active row
