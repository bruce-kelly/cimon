# Views Architecture

The UI layer: screens, components, keybindings, theme, and actions.

All rendering uses lipgloss-styled strings composed in Bubbletea models. Screens expose `SetSize(w, h)` and `Render(w, h)` for stateless re-rendering. Components follow the same pattern.

---

## App Root (`internal/app/app.go`)

`App` is the root Bubbletea model. It wires the poller, DB, and GitHub client to screen models. Lives in `internal/app/` (separate from `internal/ui/`) to avoid circular imports — screens import `ui` for theme colors.

**Startup:** `main.go` loads config, discovers token, opens DB, passes all to `app.NewApp()`. App creates poller, builds config-derived lookups (release/agent workflow maps), initializes screen models.

**Poll handling:** Each `PollResult` persists runs/PRs to DB via `UpsertRun`/`UpsertPull`, classifies errors (AuthError → status message, RateLimitError → retry countdown), tracks DB error count (flash after 3 consecutive failures), evaluates auto-fix triggers for failed runs, then `rebuildScreenData()` updates all screens: pipeline runs, review queue, agent profiles, timeline, release confidence, metrics stats.

**Agent integration:** App owns `Dispatcher`, `Scheduler`, and `AutoFixTracker`. A 10s `agentTickMsg` checks scheduled tasks (dispatches workflow or local agent) and agent lifetimes. Dispatcher feeds running agents to dashboard roster. Graceful shutdown on quit (SIGTERM → 5s → SIGKILL).

**Catchup overlay:** Tracks `lastInput` timestamp and pre-idle run/PR counts. When returning after idle threshold, shows summary of changes.

**Screen switching:** Dashboard (`1`), Timeline (`2`), Release (`3`), Metrics (`4`). Stored as `ui.Screen` enum (defined in `internal/ui/app.go`).

**View rendering:** `View()` returns `tea.View` with `v.AltScreen = true`. Minimum terminal size check (60x10). Two-bar layout: fixed top bar (screen tabs) + fixed bottom bar (confirmBar > flash > status). Content truncated to fit between them. LogPane splits content area when visible.

**Key dispatch:** `Update()` handles `tea.KeyPressMsg` — ActionMenu and ConfirmBar intercept when active. Filter mode intercepts when active. Screen-specific handlers for dashboard (Tab focus, j/k per panel, all actions), timeline (j/k), release (left/right repo switch).

**Actions:** All dashboard actions wired: rerun (r), approve (a), merge (m), batch merge (M), dismiss (x), view diff (v), open browser (o), dispatch agent (D), contextual action menu (Enter). Destructive actions show ConfirmBar; results show Flash.

---

## Screens (`internal/ui/screens/`)

Each is a struct with `SetSize(w, h)`, `Render(w, h) string`, and data-setting methods.

### Dashboard (`1`) — `dashboard.go`

Main home view with three panels.

**Panels:**
- **Pipeline** — `PipelineView` showing latest CI runs with job stages.
- **Review Queue** — Placeholder for review item list.
- **Agent Roster** — Placeholder for agent profiles.

**Focus cycling:** `FocusArea` enum (Pipeline, ReviewQueue, AgentRoster). `CycleFocus()` rotates focus.

`SetRuns(runs)` — distributes runs to pipeline view. `Render()` composes three panels with section headers.

### Timeline (`2`) — `timeline.go`

Cross-repo chronological feed. `SetRuns(runs)` sorts by `UpdatedAt` (newest first). Assigns stable repo colors via insertion-order map.

Each row: time (HH:MM) + status dot + repo name (colored) + workflow label + branch.

`Render()` composes the sorted, filtered feed.

### Release (`3`) — `release.go`

Per-repo release tracker. `SetRepos(repos)` / `SetRuns(runs)` / `SetConfidence(result)`.

Multi-repo navigation: `NextRepo()` / `PrevRepo()` cycle through repos.

Each row: run name + status + SHA + elapsed. Confidence score displayed with level color and signal breakdown.

### Metrics (`4`) — `metrics.go`

Historical statistics. `SetStats(run, task, effectiveness)` accepts stat maps.

`RenderBar(label, value, max, width)` — horizontal bar chart with percentage.

---

## Components (`internal/ui/components/`)

### Navigation

**`Selector`** (`selector.go`)
- `Next()` / `Prev()` — cursor movement with wrapping
- `SetCount(n)` — updates item count, clamps index
- `Index()` — current position
- Embedded in list views for j/k navigation

**`FilterBar`** (`filterbar.go`)
- `Activate()` / `Deactivate()` / `Clear()` — lifecycle
- `Matches(text)` — case-insensitive multi-term matching
- `HandleKey(msg)` — processes character input, backspace, enter, escape
- `IsActive()` / `Query()` — state queries

### Data Display

**`PipelineView`** (`pipeline.go`)
- Displays runs with job stages and status dots
- `SetRuns(runs)` / `FilteredRuns()` / `SelectedRun()`
- `SetKnownFailures(set)` — tags known failures
- `FormatDuration(d)` — human-readable elapsed time
- Embeds `Selector` and `FilterBar`

**`Sparkline`** (`sparkline.go`)
- `Render(values, width)` — Unicode bar chart (blocks mapped to 0-7 range)
- Uses `▁▂▃▄▅▆▇█` characters

### Overlays

**`LogPane`** (`logpane.go`)
- Three modes: Hidden, Half, Full
- `ClassifyLine(line)` — categorizes diff lines (add/remove/hunk/annotation/normal)
- `Render(width)` — styled content with diff highlighting
- `SetContent(content, streaming)` / `SetMode(mode)` / `CycleMode()`
- LIVE indicator for streaming agent output

**`HelpOverlay`** (`help.go`)
- `Render(width, height, screenName)` — context-sensitive keybinding display
- Shows global keys + screen-specific keys
- Bordered box with title

**`CatchupOverlay`** (`catchup.go`)
- `Render(width, height, summary)` — idle summary display
- `BuildSummary(newRuns, newTasks, changedPulls)` — constructs summary text

### Interactive

**`ConfirmBar`** (`confirmbar.go`)
- `Show(message)` / `HandleKey(msg)` — y/n/Esc prompt
- `IsActive()` / `Confirmed()` — state queries
- `Render()` — styled prompt line

**`Flash`** (`flash.go`)
- `ShowSuccess(msg)` / `ShowError(msg)` — timed messages
- `Visible()` — checks if message still within display duration
- `Render()` — colored message line

**`ActionMenu`** (`actionmenu.go`)
- `Show(items)` / `Hide()` — lifecycle
- `HandleKey(msg)` — j/k navigation, enter selection, escape close
- `Render(width)` — bordered menu with highlighted selection
- `Selected()` — returns selected action item

---

## Keybindings (`internal/ui/keys.go`)

Defined in `KeyMap` struct with `key.Binding` fields from `charm.land/bubbles/v2/key`.

### Global

| Key | Field | Action |
|-----|-------|--------|
| `1` | `Screen1` | Dashboard |
| `2` | `Screen2` | Timeline |
| `3` | `Screen3` | Release |
| `4` | `Screen4` | Metrics |
| `q`, `ctrl+c` | `Quit` | Quit |
| `?` | `Help` | Toggle help |
| `/` | `Filter` | Open filter |
| `l` | `LogCycle` | Cycle log pane |
| `Esc` | `Escape` | Back/close |

### Dashboard

| Key | Field | Action |
|-----|-------|--------|
| `j`/`k`, `w`/`s`, arrows | `Down`/`Up` | Navigate items |
| `Tab` | `Tab` | Cycle focus |
| `Enter` | `Enter` | Action menu |
| `r` | `Rerun` | Smart rerun |
| `a` | `Approve` | Approve PR |
| `m` | `Merge` | Merge PR |
| `M` | `BatchMerge` | Batch merge agent PRs |
| `v` | `ViewDiff` | View diff/output |
| `x` | `Dismiss` | Dismiss item |
| `o` | `Open` | Open in browser |
| `D` | `Dispatch` | Dispatch agent |

---

## Theme (`internal/ui/theme.go`)

Tokyo Night palette.

### Colors

| Variable | Hex | Usage |
|----------|-----|-------|
| `ColorBg` | `#1a1b26` | Screen backgrounds |
| `ColorFg` | `#c0caf5` | Default text |
| `ColorMuted` | `#565f89` | Secondary text, elapsed times |
| `ColorAccent` | `#e0af68` | SHA text, headers, active highlights |
| `ColorGreen` | `#9ece6a` | Success states |
| `ColorRed` | `#f7768e` | Failures, errors |
| `ColorAmber` | `#ff9e64` | In-progress, running |
| `ColorBlue` | `#7aa2f7` | Repo names, section headers |
| `ColorPurple` | `#bb9af7` | Second repo color |
| `ColorBorder` | `#3b4261` | Separators, borders |
| `ColorSurface` | `#24283b` | Elevated surfaces (status bar) |
| `ColorSelection` | `#364a82` | Selected item background |

### Helper Functions

- `StatusColor(conclusion)` — returns `color.Color` for a conclusion string
- `StatusDot(conclusion)` — returns Unicode dot character for status
- `RepoColor(index)` — stable color from 5-color rotation
- `RepoColors` — `[]color.Color{Blue, Purple, Green, Amber, Accent}`
