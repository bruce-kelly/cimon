# Architecture Overview

CIMON is a Bubbletea TUI for monitoring GitHub Actions CI/CD pipelines across multiple repos. Config-driven via `.cimon.yml`. Tokyo Night theme. Written in Go.

## Package Map

```
cmd/cimon/
├── main.go              # Cobra root command, Bubbletea program launch
├── init.go              # interactive setup wizard
└── db.go                # `cimon db` subcommand

internal/
├── config/              # .cimon.yml parsing, zero-config detection
├── models/              # WorkflowRun, Job, PullRequest, PollResult
├── github/              # HTTP client, ETag caching, runs/pulls/actions/search
├── db/                  # SQLite persistence (WAL, 7 tables, embedded schema)
├── polling/             # adaptive cadence poller, state machine
├── agents/              # classify, profile, dispatch, autofix, scheduler, tracking
├── review/              # priority scoring, escalation, queue
├── confidence/          # 5-signal release confidence scoring
├── notify/              # desktop notifications (Linux/macOS)
└── ui/
    ├── app.go           # root Bubbletea model
    ├── keys.go          # KeyMap
    ├── theme.go         # Tokyo Night palette
    ├── screens/         # dashboard, timeline, release, metrics
    └── components/      # pipeline, selector, filterbar, sparkline, logpane, etc.
```

## Data Flow

```
.cimon.yml → CimonConfig → github.Client → Poller → PollResult → App → Screens → Components
```

1. **Startup.** `config.Load()` finds `.cimon.yml` (CWD walk-up), parses it. v1 configs auto-migrate to v2 format. App creates `github.Client` (discovers token via `GITHUB_TOKEN` env / `GH_TOKEN` env / `gh auth token`), `polling.Poller`, and initializes screen models.

2. **Poll cycle.** `Poller` runs in a goroutine, polls each repo, sends `PollResult` over a channel. A `tea.Cmd` reads from the channel and delivers it as a message to the Bubbletea update loop. Adaptive cadence: idle (30s) when nothing running, active (5s) when in-progress, cooldown (N idle ticks) before dropping back.

3. **Data distribution.** App's `Update()` receives `PollResultMsg`, persists runs/jobs/PRs to SQLite, builds review items and agent profiles, syncs agent status, evaluates auto-fix triggers, checks catch-up state. Routes data to the active screen model.

4. **Screen rendering.** Each screen model has a `Render()` method that returns a string. Components compose lipgloss-styled strings. The `Selector` pattern provides j/k navigation for list views.

5. **Actions.** User keybinding → confirm bar (if destructive) → action handler → GitHub API → flash result.

## Screen Architecture

- **Dashboard (`1`):** Three panels — review queue, active pipelines (per-repo), agent roster. Home screen.
- **Timeline (`2`):** Unified chronological feed across all repos. Color-coded by repo.
- **Release (`3`):** Per-repo release pipeline with job status, confidence scoring. Left/right to switch repos.
- **Metrics (`4`):** Historical CI health and agent task statistics from SQLite, per-repo.
- **Log Pane (`l`):** Toggleable from any screen. `l` cycles through closed/open/fullscreen. Diff highlighting for PR diffs. LIVE streaming for agent output.
- **Catch-up:** Overlay after idle > threshold summarizing CI/agent/PR changes while away.
- **Filter (`/`):** Case-insensitive multi-term filter across all list components. `Esc` clears.
- **Help (`?`):** Context-sensitive overlay showing available keybindings for current screen.

## Key Design Decisions

- **Bubbletea v2 Elm architecture.** Pure functional Model-Update-View. `View()` returns `tea.View` with `AltScreen = true`. No direct p.Send() — all communication via channels and tea.Cmd.
- **Pure Go SQLite.** `modernc.org/sqlite` (no CGO) enables cross-compilation. Dual writer/reader connections with WAL mode.
- **Multi-repo from the data layer up.** Config, client, models all support multiple repos. Views aggregate.
- **Process group isolation.** Agent subprocesses use `Setpgid: true` so entire process trees can be killed cleanly.
- **Backward-compatible config.** v1 `repo` key auto-migrates to v2 `repos` list.
- **ETag caching.** `sync.Map` for thread-safe caching. Conditional requests reduce API usage.
- **GoReleaser distribution.** Single binary for 6 platform targets. Homebrew tap for macOS.

## Dependencies

- `charm.land/bubbletea/v2` — TUI framework (Elm architecture)
- `charm.land/lipgloss/v2` — terminal styling
- `charm.land/bubbles/v2` — key bindings
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — config parsing
- `modernc.org/sqlite` — pure Go SQLite
- `github.com/robfig/cron/v3` — cron schedule computation
- `github.com/stretchr/testify` — test assertions

## Related Docs

- [Data Layer](data-layer.md) — config, models, client, polling, agents, persistence
- [Views](views.md) — screens, components, keybindings, theme
