# CLAUDE.md — CIMON

## What This Is

CIMON (CI Monitor) is a Bubbletea TUI for monitoring GitHub Actions CI/CD pipelines and agent-created PRs across multiple repositories. Four-view model: compact status board → per-repo detail → run detail / PR detail drill-down. WASD navigation + numbered actions. Designed for side-pane use alongside Claude Code. Config-driven, Tokyo Night color scheme. Written in Go.

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
├── main.go              # Cobra root command, config/token/DB init, launches App
├── init.go              # interactive setup wizard (token, repo, workflow discovery)
└── db.go                # `cimon db` subcommand: stats, export, prune

internal/
├── app/
│   ├── app.go           # App — root Bubbletea model, wires poller→DB→views, key dispatch, all actions
│   └── app_test.go      # NewApp, handlePollResult, error classification, compact/detail navigation, View
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
│   ├── client.go        # Client — net/http, bounded LRU ETag cache, rate limit tracking, get/post/put/patch
│   ├── client_test.go   # ETags, rate limits, 401/403/429 handling, auth header, unmarshal errors
│   ├── cache.go         # etagCache — bounded LRU cache (container/list + sync.Mutex)
│   ├── cache_test.go    # store/load, eviction, LRU ordering, delete
│   ├── errors.go        # AuthError (401/403), RateLimitError (429) with RetryAfter
│   ├── errors_test.go   # error type assertions, retry-after parsing
│   ├── runs.go          # ListRuns, GetJobs, GetFailedLogs — workflow run + job fetching
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
    ├── app.go           # ViewMode enum (ViewCompact, ViewDetail, ViewRunDetail, ViewPRDetail)
    ├── keys.go          # KeyMap — WASD + numbered action keybindings via bubbles/v2/key
    ├── theme.go         # Tokyo Night palette, StatusColor, StatusDot, RepoColor
    ├── views/
    │   ├── repostate.go     # RepoState, InlineStatus, PRSummary, ComputeInlineStatus, SortByAttention, DetectNewFlag
    │   ├── repostate_test.go # inline status, PR summary, sorting, NEW flag detection
    │   ├── compact.go       # CompactView — repo list with inline expansion, cursor, NEW flag, PR summaries
    │   ├── compact_test.go  # empty, all-green, failure expansion, active progress, cursor, NEW flag, PR summary
    │   ├── detail.go        # DetailView — CI pipeline + PR sections, linear cursor, action applicability
    │   ├── detail_test.go   # pipeline render, PR render, linear cursor, selected run/PR, empty repo
    │   ├── rundetail.go     # RunDetailView — single run drill-down, jobs/steps, cursor, expand/collapse
    │   ├── rundetail_test.go # render, cursor, step expansion, empty/loading states
    │   ├── prdetail.go      # PRDetailView — single PR drill-down, file list, CI/review status
    │   ├── prdetail_test.go # render, cursor, file list, loading states
    │   ├── diffparse.go     # ParseDiffFiles — unified diff parser, file list with +/- counts and offsets
    │   └── diffparse_test.go # single/multi/binary/rename diff parsing
    └── components/
        ├── pipeline.go      # PipelineView — job stages, known failure tags, selector + filter, format helpers
        ├── selector.go      # Selector — w/s cursor navigation with wrapping
        ├── confirmbar.go    # ConfirmBar — y/n/Esc confirmation prompt, returns tea.Cmd
        ├── flash.go         # Flash — timed success/error messages
        ├── logpane.go       # LogPane — diff highlighting, streaming LIVE mode
        ├── help.go          # HelpOverlay — context-sensitive keybinding display per view mode
        └── components_test.go # selector, confirm, flash, pipeline, format helpers tests
```

Full architecture docs: `docs/architecture/` (overview, data-layer, views).

## Key Patterns

- **Bubbletea v2 Elm architecture:** Model-Update-View pattern. `View()` returns `tea.View` (not string). `tea.NewView(str)` with `.AltScreen = true`.
- **Four-view model:** CompactView → DetailView → RunDetailView / PRDetailView. WASD + numbered actions. `d`/`Enter` drills in, `a`/`Esc` backs out.
- **App wiring:** `internal/app/app.go` owns config, GitHub client, DB, poller, and view models. Separate from `internal/ui/` to avoid circular imports (views import `ui` for theme).
- **Multi-repo config:** `.cimon.yml` v2 uses `repos` list. v1 `repo` key auto-migrates.
- **Poller→TUI communication:** Channel + `tea.Cmd` pattern (goroutine sends to channel, tea.Cmd reads from channel). Avoids `p.Send()` deadlock.
- **Poll result handling:** Each PollResult persists runs/PRs to DB, rebuilds RepoStates (inline status, PR summaries, review items), detects NEW flags, updates views.
- **Two-bar layout:** Fixed header (CIMON + clock) + fixed footer (status or action hints). Content truncated to fit between them.
- **NEW flag:** Change detection indicator on repo lines. Fires on: passing→failed, new ready PR, release started. Clears on cursor selection or 30s timeout via 10s tick.
- **Attention sorting:** Repos sorted by priority: failures → active runs → ready PRs → all-green.
- **Inline expansion:** Repos with CI failures or in-progress runs auto-expand detail lines beneath the repo summary.
- **ETag caching:** Bounded LRU cache (5000 entries) keyed by URL. Conditional requests with `If-None-Match`. 304s return cached body.
- **Pure Go SQLite:** `modernc.org/sqlite` (no CGO). Dual connections: writer (MaxOpenConns=1) + reader pool (MaxOpenConns=4). WAL mode.
- **Async action pattern:** Actions return `tea.Cmd` producing result messages (`actionResultMsg`, `diffResultMsg`, `logsResultMsg`, `jobsResultMsg`). Handlers guard against stale responses (e.g., navigated away before result arrives).
- **Selector pattern:** `Selector` struct (Next/Prev/SetCount/Index) embedded in views for w/s navigation.
- **Agent detection:** Configurable patterns (pr_body, commit_trailer, bot_authors) in `AgentPatternsConfig`.
- **Review queue:** `ReviewItemsFromPulls()` converts PRs into prioritized ReviewItems. Escalation thresholds from config.
- **Confidence scoring:** `ComputeConfidence()` — 5 signals (CI rate, new failures, agent PRs, review queue) → 0-100 score. Computed internally, not displayed in TUI.
- **Token discovery:** `GITHUB_TOKEN` env → `GH_TOKEN` env → `gh auth token` subprocess.
- **Zero-config:** `DetectRepo()` parses git remote, `BuildZeroConfig()` creates config from discovered workflows.
- **GoReleaser:** Cross-platform binaries (linux/darwin/windows × amd64/arm64), Homebrew tap.

## Testing

```bash
go test ./... -count=1 -v
```

320 tests across 15 test packages:
- `app_test.go` — NewApp, handlePollResult, AuthError/RateLimitError, compact/detail/run-detail/PR-detail navigation, d drill-in, a/esc back, NEW flag detection, View, async message handling
- `config_test.go` — v2 parsing, v1 migration, defaults, validation, ConfigError, detect/categorize
- `client_test.go` — ETags, rate limits, 401/403/429 handling, auth header, unmarshal errors
- `cache_test.go` — store/load, eviction, LRU ordering, delete, len
- `errors_test.go` — AuthError/RateLimitError type assertions
- `models_test.go` — IsActive, Elapsed, Size, zero values, PollResult
- `db_test.go` — schema v2, CRUD, known failures, stats, catchup, dismissed
- `agents_test.go` — outcome/trigger classification, profiles, dispatch, scheduler
- `autofix_test.go` — evaluate, cooldown, known failure gating, prompt building
- `review_test.go` — scoring, escalation, filtering, sorting, queue
- `confidence_test.go` — signals, levels, edge cases
- `polling_test.go` — cadence transitions, intervals, poller integration with httptest
- `notify_test.go` — CanNotify, Send
- `components_test.go` — selector, confirm (tea.Cmd return), flash, pipeline, format helpers
- `repostate_test.go` — inline status computation, PR summary, attention sorting, NEW flag detection
- `compact_test.go` — empty, all-green, failure expansion, active progress, cursor, NEW flag, PR summary
- `detail_test.go` — pipeline render, PR render, linear cursor, selected run/PR, empty repo
- `rundetail_test.go` — render, cursor navigation, step expand/collapse, auto-expand failures, loading state, SetJobs
- `prdetail_test.go` — render, cursor navigation, file list, CI/review status, agent/draft badges, loading state
- `diffparse_test.go` — single/multi-file, binary, rename, empty diff parsing

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

### Navigation (all views)
| Key | Action |
|-----|--------|
| `w` / `↑` | Cursor up |
| `s` / `↓` | Cursor down |
| `d` / `Enter` | Drill in / expand |
| `a` / `Esc` | Back / collapse (closes log pane first) |
| `?` | Help overlay |
| `q` | Quit |

### Compact View
| Key | Action |
|-----|--------|
| `1` | Batch merge all ready agent PRs (with confirm) |

### Detail View (per-repo)
| Key | Action |
|-----|--------|
| `1` | Rerun selected run / Approve selected PR |
| `2` | View diff (PR) or logs (run) |
| `3` | Dismiss selected PR |
| `e` | Toggle log pane (hidden → half → full) |
| `r` | Open in browser |

### Run Detail View
| Key | Action |
|-----|--------|
| `d` | Expand/collapse job steps |
| `1` | Rerun workflow |
| `2` | Rerun failed jobs |
| `e` | Toggle log pane |
| `r` | Open on GitHub |

### PR Detail View
| Key | Action |
|-----|--------|
| `d` | Jump log pane to selected file's diff |
| `1` | Approve PR |
| `2` | Merge PR (with confirm) |
| `3` | Dismiss PR |
| `e` | Toggle log pane |
| `r` | Open on GitHub |

## Theme

Tokyo Night palette. Colors in `internal/ui/theme.go`: bg `#1a1b26`, fg `#c0caf5`, accent `#e0af68`, green `#9ece6a`, red `#f7768e`, amber `#ff9e64`, blue `#7aa2f7`, purple `#bb9af7`. `StatusColor()`, `StatusDot()`, `RepoColor()` helpers.
