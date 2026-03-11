# Architecture Overview

CIMON is a Bubbletea TUI for monitoring GitHub Actions pipelines and pull requests across multiple repositories. It is config-driven via `.cimon.yml`, uses a local SQLite database for persistence, and renders a four-view terminal UI.

## Package Map

```
cmd/cimon/
├── main.go              # Cobra root command, config/token/DB init, launches App
├── init.go              # interactive setup wizard
└── db.go                # `cimon db` subcommand

internal/
├── app/                 # root Bubbletea model — wires poller→DB→views
├── config/              # .cimon.yml parsing, v1 migration, zero-config detection
├── models/              # WorkflowRun, Job, PullRequest, PollResult
├── github/              # HTTP client, ETag caching, runs/pulls/actions/search
├── db/                  # SQLite persistence (WAL, embedded schema)
├── polling/             # adaptive cadence poller, state machine
├── agents/              # classification, profiles, dispatch, autofix, scheduler
├── review/              # PR scoring and review queue
├── confidence/          # release confidence scoring
├── notify/              # desktop notifications
└── ui/
    ├── app.go           # ViewMode enum
    ├── keys.go          # WASD + numbered action keymap
    ├── theme.go         # Tokyo Night palette
    ├── views/           # compact, detail, run-detail, pr-detail
    └── components/      # selector, confirm bar, flash, log pane, help, pipeline
```

## Data Flow

```
.cimon.yml → CimonConfig → github.Client → Poller → PollResult → App → Views → Components
```

1. `main.go` loads config, discovers a GitHub token, opens SQLite, and constructs `app.NewApp()`.
2. `App.Init()` starts the poller and registers `tea.Cmd` handlers that wait on the poller result channel and the 10-second UI tick.
3. `Poller` fetches workflow runs and pull requests per repo, applies adaptive polling cadence, and emits `models.PollResult`.
4. `App.Update()` persists runs and PRs, rebuilds per-repo view state, preserves active drill-down views, and updates status text, flash messages, and overlays.
5. `App.View()` renders a fixed-header, fixed-footer layout with one of four views and an optional log pane/help overlay.

## View Model

The TUI is a drill-down flow, not a multi-tab dashboard:

- `compact` — one line per repo with inline failure and active-run expansion
- `detail` — per-repo runs and review items, grouped by configured workflow groups
- `run-detail` — one workflow run with jobs and expandable steps
- `pr-detail` — one pull request with diff-derived file list and status badges

Navigation is consistent across views: `w`/`s` move, `d`/`Enter` drills in, `a`/`Esc` backs out, `1`-`3` trigger context-sensitive actions, `e` toggles the log pane, `r` opens GitHub, `?` shows help, and `q` quits.

## Key Design Decisions

- Bubbletea v2 Elm architecture. Poller-to-TUI communication goes through a channel read by `tea.Cmd`; the app does not call `Program.Send`.
- `internal/app` is separate from `internal/ui` so views can depend on shared UI theme/types without creating circular imports.
- The compact view derives repo attention from the latest completed run per workflow file, avoiding stale failures after a newer success.
- Agent workflows are treated differently from critical CI/build/release workflows, both in severity and in available actions.
- SQLite uses a single writer connection plus a small read pool under WAL mode for simple cross-platform persistence.

## Related Docs

- [Data Layer](data-layer.md) — config, models, client, polling, persistence
- [Views](views.md) — app rendering, views, components, keybindings
