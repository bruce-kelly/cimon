# CLAUDE.md — CIMON

## What This Is

CIMON (CI Monitor) is a Bubbletea TUI for monitoring GitHub Actions CI/CD pipelines and Claude Code agent activity across multiple repositories. Tracks agent-created PRs, surfaces what needs human attention, and dispatches new agent tasks. Config-driven, Tokyo Night color scheme. Written in Go.

## Commands

```bash
go build -o cimon ./cmd/cimon       # build binary
go test ./... -count=1              # run tests
go vet ./...                        # lint
./cimon                             # run (needs .cimon.yml in CWD or parent)
./cimon init                        # interactive setup wizard
./cimon version                     # print version
./cimon db stats                    # show SQLite stats
./cimon db export                   # export runs as JSON
./cimon db prune --days 90          # prune old data
```

## Architecture

```
cmd/cimon/
├── main.go              # Cobra root command, Bubbletea program launch
├── init.go              # interactive setup wizard (token, repo, workflow discovery)
└── db.go                # `cimon db` subcommand: stats, export, prune

internal/
├── config/
│   ├── config.go        # .cimon.yml v2 parser, v1 auto-migration, all config structs, Load/LoadFromPath
│   ├── config_test.go   # v2 parsing, v1 migration, defaults, validation, ConfigError
│   ├── detect.go        # DetectRepo (git remote), DetectBranch, CategorizeWorkflow, BuildZeroConfig
│   └── detect_test.go   # remote parsing, workflow categorization
├── models/
│   ├── run.go           # WorkflowRun, Job (with Elapsed), Step
│   ├── pr.go            # PullRequest (with Size)
│   ├── poll.go          # PollResult
│   └── models_test.go   # IsActive, Elapsed, Size, zero values
├── github/
│   ├── client.go        # Client — net/http, ETag caching (sync.Map), rate limit tracking, get/post/put/patch
│   ├── client_test.go   # ETags, rate limits, 404/403 handling, auth header, unmarshal errors
│   ├── runs.go          # ListRuns, GetJobs — workflow run + job fetching
│   ├── pulls.go         # ListPulls, GetPullDiff, GetCombinedStatus, FetchPRStatuses, DetectAgent
│   ├── actions.go       # Approve, Merge, Rerun, RerunFailed, Cancel, CreateTag, DispatchWorkflow, CommentOnPR
│   ├── search.go        # SearchPulls, DiscoverWorkflows
│   └── token.go         # DiscoverToken — GITHUB_TOKEN > GH_TOKEN > gh auth token
├── db/
│   ├── db.go            # Database — dual conn (writer/reader), WAL mode, embedded schema, Open/OpenMemory
│   ├── schema.sql       # SQLite schema v2 (7 tables: workflow_runs, pull_requests, jobs, agent_tasks, review_events, dismissed_items, schema_version)
│   ├── runs.go          # UpsertRun, UpsertJobs, QueryRuns, QueryAllRuns, IsKnownFailure
│   ├── pulls.go         # UpsertPull, QueryPulls
│   ├── tasks.go         # InsertTask, UpdateTaskStatus, LinkTaskToPR, MarkOrphanedTasks, QueryTasks
│   ├── stats.go         # RunStats, TaskStats, AgentEffectivenessStats
│   ├── dismissed.go     # AddDismissed, RemoveDismissed, IsDismissed, LoadDismissed
│   └── db_test.go       # all CRUD, known failures, stats, catchup queries, dismissed ops
├── polling/
│   ├── poller.go        # Poller — multi-repo poll loop, channel-based result delivery
│   ├── state.go         # PollState — 3-tier adaptive cadence (idle/active/cooldown)
│   └── polling_test.go  # cadence transitions, interval values
├── agents/
│   ├── classify.go      # OutcomeBucket, TriggerType enums, ClassifyOutcome, ClassifyTrigger
│   ├── profile.go       # AgentProfile, RunSnapshot, BuildAgentProfiles
│   ├── dispatch.go      # DispatchedAgent, Dispatcher — subprocess lifecycle, output capture, process group isolation
│   ├── autofix.go       # AutoFixTracker — Evaluate (cooldown/concurrency/known failure gating), BuildFixPrompt
│   ├── scheduler.go     # Scheduler — cron-based task scheduling with double-fire prevention
│   ├── tracking.go      # FindPRForTask — link completed agent tasks to PRs
│   ├── agents_test.go   # classify, profiles, dispatch, scheduler tests
│   └── autofix_test.go  # evaluate, cooldown, known failure, prompt tests
├── review/
│   ├── review.go        # ReviewItem, EscalationLevel, ScorePR, ReviewItemsFromPulls
│   ├── queue.go         # Queue — thread-safe review item management
│   └── review_test.go   # scoring, escalation, filtering, sorting
├── confidence/
│   ├── confidence.go    # ComputeConfidence — 5-signal scoring, ConfidenceResult, Level
│   └── confidence_test.go
├── notify/
│   ├── notify.go        # CanNotify, Send — Linux notify-send, macOS osascript
│   └── notify_test.go
└── ui/
    ├── app.go           # App — root Bubbletea model, screen switching, key dispatch
    ├── keys.go          # KeyMap — all keybindings via bubbles/v2/key
    ├── theme.go         # Tokyo Night palette, StatusColor, StatusDot, RepoColor
    ├── screens/
    │   ├── dashboard.go     # DashboardModel — 3-panel layout, focus cycling, pipeline/review/roster
    │   ├── timeline.go      # TimelineModel — cross-repo chronological feed, repo color mapping
    │   ├── release.go       # ReleaseModel — per-repo release tracker, confidence display, repo switching
    │   ├── metrics.go       # MetricsModel — CI health + agent stats from DB
    │   └── screens_test.go  # dashboard, timeline, release, metrics unit tests
    └── components/
        ├── pipeline.go      # PipelineView — job stages, known failure tags, selector + filter
        ├── selector.go      # Selector — j/k cursor navigation with wrapping
        ├── filterbar.go     # FilterBar — case-insensitive multi-term filter input
        ├── sparkline.go     # Sparkline — Unicode bar chart rendering
        ├── confirmbar.go    # ConfirmBar — y/n/Esc confirmation prompt
        ├── flash.go         # Flash — timed success/error messages
        ├── actionmenu.go    # ActionMenu — contextual action popup
        ├── logpane.go       # LogPane — diff highlighting, streaming LIVE mode
        ├── help.go          # HelpOverlay — context-sensitive keybinding display
        ├── catchup.go       # CatchupOverlay — idle summary
        └── components_test.go # selector, filter, sparkline, confirm, flash, action menu, pipeline tests
```

Full architecture docs: `docs/architecture/` (overview, data-layer, views).

## Key Patterns

- **Bubbletea v2 Elm architecture:** Model-Update-View pattern. `View()` returns `tea.View` (not string). `tea.NewView(str)` with `.AltScreen = true`.
- **Multi-repo config:** `.cimon.yml` v2 uses `repos` list. v1 `repo` key auto-migrates.
- **Poller→TUI communication:** Channel + `tea.Cmd` pattern (goroutine sends to channel, tea.Cmd reads from channel). Avoids `p.Send()` deadlock.
- **ETag caching:** `sync.Map` keyed by URL path. Conditional requests with `If-None-Match`. 304s return cached body.
- **Pure Go SQLite:** `modernc.org/sqlite` (no CGO). Dual connections: writer (MaxOpenConns=1) + reader pool (MaxOpenConns=4). WAL mode.
- **Process group isolation:** Agent subprocesses get `Setpgid: true`. Kill with `syscall.Kill(-pid, SIGTERM)`.
- **Selector pattern:** `Selector` struct (Next/Prev/SetCount/Index) embedded in list views for j/k navigation.
- **Filter bar:** `FilterBar.Matches(text)` does case-insensitive multi-term matching. Components expose `FilteredRuns()` etc.
- **Agent detection:** Configurable patterns (pr_body, commit_trailer, bot_authors) in `AgentPatternsConfig`.
- **Review queue:** `ReviewItemsFromPulls()` converts PRs into prioritized ReviewItems. Escalation thresholds from config.
- **Auto-fix copilot:** `AutoFixTracker.Evaluate()` checks cooldown/concurrency/known failures before dispatching.
- **Confidence scoring:** `ComputeConfidence()` — 5 signals (CI rate, new failures, agent PRs, review queue) → 0-100 score.
- **Token discovery:** `GITHUB_TOKEN` env → `GH_TOKEN` env → `gh auth token` subprocess.
- **Zero-config:** `DetectRepo()` parses git remote, `BuildZeroConfig()` creates config from discovered workflows.
- **GoReleaser:** Cross-platform binaries (linux/darwin/windows × amd64/arm64), Homebrew tap.

## Testing

```bash
go test ./... -count=1 -v
```

139 tests across 11 test packages:
- `config_test.go` — v2 parsing, v1 migration, defaults, validation, ConfigError, detect/categorize
- `client_test.go` — ETags, rate limits, 404/403, auth, runs, jobs, pulls, diff, agent detection, search, actions
- `models_test.go` — IsActive, Elapsed, Size, zero values, PollResult
- `db_test.go` — schema v2, CRUD, known failures, stats, catchup, dismissed
- `agents_test.go` — outcome/trigger classification, profiles, dispatch, scheduler
- `autofix_test.go` — evaluate, cooldown, known failure gating, prompt building
- `review_test.go` — scoring, escalation, filtering, sorting, queue
- `confidence_test.go` — signals, levels, edge cases
- `polling_test.go` — cadence transitions, intervals
- `notify_test.go` — CanNotify, Send
- `components_test.go` — selector, filter, sparkline, confirm, flash, action menu, pipeline
- `screens_test.go` — dashboard, timeline, release, metrics

Tests use `httptest` servers and in-memory SQLite. No network calls.

## Dependencies

Runtime: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`, `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`, `github.com/robfig/cron/v3`
Test: `github.com/stretchr/testify`

## Config Format (.cimon.yml)

### v2 (multi-repo)

```yaml
repos:
  - repo: owner/repo-a
    branch: main
    agent_patterns:
      pr_body: "Generated with Claude Code"
      commit_trailer: "Co-Authored-By: Claude"
      bot_authors: ["github-actions[bot]"]
    groups:
      ci:
        label: "CI Pipeline"
        workflows: [ci.yml]
        expand_jobs: true
        auto_fix: true
        auto_fix_cooldown: 300
      release:
        label: "Release"
        workflows: [release.yml]
        auto_focus: true
      agents:
        label: "Agents"
        workflows: [claude.yml, claude-pr-review.yml]
    secrets: [SECRET_NAME]

  - repo: owner/repo-b
    branch: main
    groups:
      ci:
        label: "CI"
        workflows: [ci.yml]

review_queue:
  auto_discover: true
  extra_filters:
    - "is:open is:pr review-requested:@me"
  escalation:
    amber: 24
    red: 48

polling:
  idle: 30
  active: 5
  cooldown: 3

database:
  path: ~/.local/share/cimon/cimon.db
  retention_days: 90

agents:
  scheduled:
    - name: "daily-review"
      cron: "0 9 * * *"
      workflow: claude-pr-review.yml
    - name: "nightly-fix"
      cron: "0 2 * * *"
      prompt: "Fix failing tests"
      repo: owner/repo-a
  max_concurrent: 2
  max_lifetime: 1800
  capture_output: true

notifications: false

catchup:
  enabled: true
  idle_threshold: 900

theme:
  palette: tokyo-night
```

### v1 (single repo — auto-migrates)

```yaml
repo: owner/repo
branch: main
groups:
  ci:
    label: "CI"
    workflows: [ci.yml]
```

Only `repo` (v1) or `repos` (v2) is required. Everything else has defaults.

## Keybindings

**Global:** `1`/`2`/`3`/`4` switch screen, `l` cycle log pane, `/` filter, `?` help, `Esc` back/close, `q` quit.

**Dashboard:** `j`/`k` (`w`/`s`) navigate, `Tab` cycle widget focus, `Enter` action menu, `r` smart rerun, `a` approve PR, `m` merge PR, `M` batch merge ready agent PRs, `v` view diff/agent output, `x` dismiss, `o` browser, `D` dispatch agent.

**Release:** `Left`/`Right` switch repos, `r` rerun, `o` browser.

## Theme

Tokyo Night palette. Colors in `internal/ui/theme.go`: bg `#1a1b26`, fg `#c0caf5`, accent `#e0af68`, green `#9ece6a`, red `#f7768e`, amber `#ff9e64`, blue `#7aa2f7`, purple `#bb9af7`. `StatusColor()`, `StatusDot()`, `RepoColor()` helpers.
