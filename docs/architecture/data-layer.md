# Data Layer Architecture

The data layer handles configuration loading, GitHub API communication, adaptive polling, agent intelligence classification, review queue management, and local state persistence.

## Config System (`internal/config/`)

### Format versions

**v2** (current) uses a `repos` list, each entry a `RepoConfig`:

```yaml
repos:
  - repo: owner/repo
    branch: main
    groups: { ... }
    secrets: [SECRET_A]
  - repo: owner/other
    branch: develop
```

**v1** (legacy) uses a single top-level `repo` key. Auto-migrated at parse time: `migrateV1()` detects `repo` key without `repos` and wraps it into a single-element `repos` list.

### Discovery

`findConfig()` walks from CWD up to filesystem root looking for `.cimon.yml`.

### Zero-config detection (`detect.go`)

- `DetectRepo()` — parses git remote origin URL (SSH and HTTPS formats) via `ParseGitHubRemote()`
- `DetectBranch()` — reads `git symbolic-ref HEAD`
- `CategorizeWorkflow(name)` — maps workflow filenames to categories (ci, release, agent) based on patterns
- `BuildZeroConfig(repo, branch, workflows)` — creates a `CimonConfig` from discovered data without a YAML file

### Structs

| Struct | Key fields |
|---|---|
| `GroupConfig` | `Label`, `Workflows []string`, `ExpandJobs`, `AutoFocus`, `AutoFix`, `AutoFixCooldown` |
| `PollingConfig` | `Idle=30`, `Active=5`, `Cooldown=3` (seconds/ticks) |
| `ThemeConfig` | `Palette="tokyo-night"` |
| `EscalationConfig` | `Amber=24`, `Red=48` (hours) |
| `ReviewQueueConfig` | `AutoDiscover`, `ExtraFilters []string`, `Escalation` |
| `AgentPatternsConfig` | `PRBody`, `CommitTrailer`, `BotAuthors` (all have defaults) |
| `RepoConfig` | `Repo`, `Branch="main"`, `Groups`, `Secrets`, `AgentPatterns` |
| `DatabaseConfig` | `Path` (default `~/.local/share/cimon/cimon.db`), `RetentionDays=90` |
| `ScheduledAgentConfig` | `Name`, `Cron`, `Workflow` or `Prompt`, `Repo` |
| `AgentsConfig` | `Scheduled []ScheduledAgentConfig`, `MaxConcurrent=2`, `MaxLifetime=1800`, `CaptureOutput` |
| `CatchupConfig` | `Enabled`, `IdleThreshold=900` (seconds) |
| `CimonConfig` | `Repos []RepoConfig`, `Polling`, `Theme`, `ReviewQueue`, `Database`, `Agents`, `Notifications`, `Catchup` |

Only `Repo` (inside a `RepoConfig`) is required. Everything else has defaults.

---

## Core Models (`internal/models/`)

### `WorkflowRun` (`run.go`)

Enriched workflow run. Fields: `ID`, `Name`, `Status`, `Conclusion`, `WorkflowFile`, `HeadBranch`, `HeadSHA`, `Event`, `Repo`, `Actor`, `HTMLURL`, `RunStartedAt`, `UpdatedAt`, `Jobs []Job`.

Computed methods:
- `IsActive()` — `Status == "queued" || Status == "in_progress"`
- `Elapsed()` — duration from start to completion (or now if active)
- `ShortSHA()` — first 7 chars of `HeadSHA`

### `Job`

Workflow job. Fields: `ID`, `Name`, `Status`, `Conclusion`, `StartedAt`, `CompletedAt`, `Steps []Step`.

`Elapsed()` — duration, measured against `time.Now()` if still running.

### `PullRequest` (`pr.go`)

Fields: `Repo`, `Number`, `Title`, `State`, `Draft`, `HTMLURL`, `Author`, `CreatedAt`, `UpdatedAt`, `Additions`, `Deletions`, `Body`, `IsAgent`, `AgentSource`, `CIStatus`, `ReviewState`, `Approved`.

`Size()` — `Additions + Deletions`.

### `PollResult` (`poll.go`)

One polling cycle's data: `Runs []WorkflowRun`, `Pulls []PullRequest`, `Error string`, `Repo string`, `RateLimitRemaining int`.

---

## GitHub Client (`internal/github/`)

### `Client` (`client.go`)

HTTP client with ETag caching and rate limit tracking.

**Token discovery** (`token.go`): `GITHUB_TOKEN` env → `GH_TOKEN` env → `gh auth token` subprocess.

**ETag caching** (`cache.go`): Bounded LRU cache (5000 entries) using `container/list` + `sync.Mutex`. `getJSON()` sends `If-None-Match` header when a cached ETag exists. On HTTP 304, returns cached body. On success, stores response ETag and body. Evicts oldest entry when at capacity.

**Error classification** (`errors.go`): HTTP 401/403 → `AuthError`, HTTP 429 → `RateLimitError` with `RetryAfter` duration parsed from `Retry-After` header. These typed errors propagate through the poller to the App for differentiated handling.

**Rate limit tracking**: Every response passes through header parsing for `X-RateLimit-Remaining` and `X-RateLimit-Reset`. Parse errors are logged via slog instead of silently ignored.

### Methods

| Method | File | Notes |
|---|---|---|
| `ListRuns(repo, workflowFile, branch, perPage)` | `runs.go` | Returns `[]WorkflowRun`, handles both scoped and unscoped queries |
| `GetJobs(repo, runID)` | `runs.go` | Returns `[]Job` |
| `ListPulls(repo)` | `pulls.go` | Returns `[]PullRequest` |
| `GetPullDiff(repo, number)` | `pulls.go` | Returns raw diff string |
| `GetCombinedStatus(repo, ref)` | `pulls.go` | Returns status string |
| `FetchPRStatuses(repo, pulls)` | `pulls.go` | Concurrent status enrichment |
| `DetectAgent(pr, patterns)` | `pulls.go` | Checks body/trailer/author patterns |
| `Approve(repo, number)` | `actions.go` | POST review with APPROVE |
| `Merge(repo, number)` | `actions.go` | PUT merge (squash) |
| `Rerun(repo, runID)` | `actions.go` | POST rerun |
| `RerunFailed(repo, runID)` | `actions.go` | POST rerun-failed-jobs |
| `Cancel(repo, runID)` | `actions.go` | POST cancel |
| `CreateTag(repo, tag, sha)` | `actions.go` | POST git ref |
| `DispatchWorkflow(repo, file, ref)` | `actions.go` | POST workflow dispatch |
| `CommentOnPR(repo, number, body)` | `actions.go` | POST issue comment |
| `SearchPulls(query)` | `search.go` | Search API for PRs |
| `DiscoverWorkflows(repo)` | `search.go` | List repo workflows |

---

## Polling Engine (`internal/polling/`)

### `PollState` — adaptive cadence state machine (`state.go`)

Three-tier cadence: **idle** (default 30s) → **active** (default 5s) → **cooldown** → **idle**.

```
has_active=true  →  cadence = Active, idleTicks = 0
has_active=false →  idleTicks++
                    if idleTicks > cooldown → cadence = Idle
```

The cooldown tier prevents immediate drop to idle when a run completes.

### `Poller` (`poller.go`)

Runs in a goroutine, polls each repo, sends `PollResult` over a channel. The Bubbletea app reads from the channel via a `tea.Cmd`.

---

## Agent Intelligence (`internal/agents/`)

### Outcome classification (`classify.go`)

`OutcomeBucket` enum: `Silent`, `Acted`, `Alert`.

`ClassifyOutcome(conclusion, hasArtifacts)`:
- failure → Alert
- success + artifacts → Acted
- success + no artifacts → Silent

`ClassifyTrigger(event)`:
- `"schedule"` → Cron
- `"workflow_dispatch"` → Manual
- everything else → Event

### Agent profiles (`profile.go`)

`BuildAgentProfiles(runs, agentWorkflows)` — groups runs by workflow, builds history/sparklines/success rates. Returns `map[string][]AgentProfile` keyed by repo.

### Agent dispatch (`dispatch.go`)

`Dispatcher` manages subprocess lifecycle:
- `Dispatch(repo, task)` — spawns `claude -p <task>`. Process group isolation via `Setpgid: true`.
- `CheckAll()` — polls running agents, reaps zombies, kills agents exceeding `MaxLifetime`.
- `Shutdown()` — graceful termination: SIGTERM to all process groups, 5s wait, then SIGKILL for stragglers.
- `GetOutput(agentID, tailLines)` — reads tail of agent's output log.

### Auto-fix (`autofix.go`)

`AutoFixTracker.Evaluate()`:
- Checks: `AutoFix` enabled on group, max concurrent not reached, cooldown not active, failures are new (not known).
- Returns `AutoFixDecision` with `ShouldFix`, `Reason`, `FailingJobs`.

`BuildFixPrompt(repo, run, failingJobs)` — constructs prompt for fix agent.

### Scheduling (`scheduler.go`)

`Scheduler` — cron-based tasks with double-fire prevention via `lastFired` map.

### Lifecycle tracking (`tracking.go`)

`FindPRForTask(task, pulls)` — links completed agent tasks to PRs they created. Matching: same repo, agent PR, created after task started.

---

## Review Queue (`internal/review/`)

### `ReviewItem` and scoring (`review.go`)

Priority scoring formula:
```
score = 100.0
score += ageHours * 2        // older = more urgent
score += 30 if CI failing
score += 20 if agent PR
score += 10 if type is PR
score -= 50 if approved
score -= 20 if draft
```

`EscalationLevel`: Normal (< amber hours), Amber (amber-red hours), Red (> red hours).

`ReviewItemsFromPulls(pulls, dismissed, escalation)` — filters, scores, sorts descending.

### `Queue` (`queue.go`)

Thread-safe wrapper: `Update(items)`, `Items()`, `Len()`.

---

## SQLite Persistence (`internal/db/`)

### `Database` (`db.go`)

SQLite storage. Pure Go driver (`modernc.org/sqlite`). WAL mode. Dual connections: writer (`MaxOpenConns=1`) + reader pool (`MaxOpenConns=4`).

Schema embedded via `go:embed schema.sql`. `OpenMemory()` for tests.

**Tables:**

| Table | Primary Key | Purpose |
|---|---|---|
| `workflow_runs` | `id` | Run history with upsert |
| `pull_requests` | `(repo, number)` | PR state with upsert |
| `jobs` | `id` | Per-job outcomes, FK to workflow_runs |
| `agent_tasks` | `id` | Dispatched agent lifecycle |
| `review_events` | `id` (autoincrement) | Event log |
| `dismissed_items` | `(repo, number)` | Dismissed review items |
| `schema_version` | — | Single-row version tracker (v2) |

**Key methods:**

- `UpsertRun()` / `UpsertJobs()` / `UpsertPull()` — persist from poll results
- `IsKnownFailure(repo, jobName)` — checks if job has failed on main recently
- `InsertTask()` / `UpdateTaskStatus()` / `LinkTaskToPR()` — agent task lifecycle
- `MarkOrphanedTasks()` — detects running tasks with dead PIDs
- `RunStats()` / `TaskStats()` / `AgentEffectivenessStats()` — aggregate stats
- `RunsSince()` / `TasksSince()` / `PullsChangedSince()` — catch-up queries
- `PruneRuns()` — delete old data

---

## Release Confidence (`internal/confidence/`)

### `ComputeConfidence(input)`

Scores 0-100 from 5 signals:
- CI passing rate (0-40 points)
- No new failures (0-20 points, -5 penalty if new failures)
- No failing agent PRs (0-15 points)
- All agent PRs resolved (0-15 points)
- Review queue clear (0-10 points)

Returns `ConfidenceResult` with `Score`, `Level` (High/Medium/Low), and `Signals` breakdown.

---

## Notifications (`internal/notify/`)

`CanNotify()` — checks for `notify-send` (Linux) or `osascript` (macOS).

`Send(title, body)` — fires desktop notification via available tool.
