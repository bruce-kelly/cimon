# Getting Started with CIMON

Step-by-step guide to installing, configuring, and using CIMON.

## Prerequisites

- [GitHub CLI](https://cli.github.com/) installed and authenticated (`gh auth login`)
- Terminal with Unicode support

## 1. Install

### Binary download

Grab the latest release for your platform from [Releases](https://github.com/bruce-kelly/cimon/releases).

### Homebrew

```bash
brew install bruce-kelly/tap/cimon
```

### From source

```bash
go install github.com/bruce-kelly/cimon/cmd/cimon@latest
```

## 2. Set Up Your Config

### Option A: Interactive wizard (recommended)

```bash
cimon init
```

The wizard will:
1. Verify your GitHub authentication
2. Detect your repository from the git remote (or ask you to enter one)
3. Detect the default branch
4. Discover active workflows via the GitHub API and categorize them (CI, release, agent)
5. Generate and write `.cimon.yml` to the current directory

### Option B: Manual config

Create `.cimon.yml` in your project root. Minimal example:

```yaml
repos:
  - repo: owner/repo-name
    branch: main
    groups:
      ci:
        label: "CI Pipeline"
        workflows: [ci.yml]
        expand_jobs: true
```

## 3. Launch

```bash
cimon
```

CIMON searches for `.cimon.yml` starting from the current directory, walking up to the filesystem root, then falls back to `~/.config/cimon/config.yml`.

## 4. Navigate Views

The current TUI is a four-view drill-down:

- **Compact view** — one line per repo, sorted by attention priority. Failed and active repos expand inline.
- **Detail view** — latest runs for one repo, grouped by workflow group, plus open PRs that need attention.
- **Run detail view** — one workflow run with jobs, step expansion, and failed-job logs.
- **PR detail view** — one pull request with changed files, CI/review state, and per-file diff jumps.

## 5. Use Keybindings

### Navigation

| Key | Action |
|-----|--------|
| `w` / `↑` | Move up |
| `s` / `↓` | Move down |
| `d` / `Enter` | Drill in / select |
| `a` / `Esc` | Back |
| `?` | Toggle help |
| `q` | Quit |

### Compact View

| Key | Action |
|-----|--------|
| `1` | Batch merge ready agent PRs |

### Detail View

| Key | Action |
|-----|--------|
| `1` | Rerun CI run / dispatch agent workflow / approve PR |
| `2` | Toggle recent attempts for a run, or view the selected PR diff |
| `3` | Dismiss PR |
| `/` | Filter runs and PRs |
| `e` | Toggle log pane |
| `r` | Open on GitHub |

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
| `d` | Jump to selected file diff |
| `1` | Approve PR |
| `2` | Merge PR |
| `3` | Dismiss PR |
| `e` | Toggle log pane |
| `r` | Open on GitHub |

You can override the global bindings in config:

```yaml
keybindings:
  up: [k]
  down: [j]
  filter: [f]
```

## 6. Understand Polling

CIMON polls GitHub on an adaptive schedule:

- **Idle** (default 30s) — nothing is running
- **Active** (default 5s) — a workflow is queued or in progress
- **Cooldown** — stays at active rate for a few extra ticks after completion

The status bar shows the current polling state. Rate limit usage appears when it drops below 200.

All intervals are configurable in `.cimon.yml`:

```yaml
polling:
  idle: 30
  active: 5
  cooldown: 3
```

## 7. Monitor Agent Workflows

If you have agent/bot workflows (Claude Code, Copilot, etc.), add them to an `agents` group:

```yaml
repos:
  - repo: owner/repo
    groups:
      agents:
        label: "Agents"
        workflows: [claude.yml, claude-pr-review.yml]
    agent_patterns:
      pr_body: "Generated with Claude Code"
      commit_trailer: "Co-Authored-By: Claude"
      bot_authors: ["github-actions[bot]"]
```

Agent workflow failures are treated separately from critical CI failures. In compact view they show as amber, and in detail view they expose a dispatch action instead of a rerun action.

## 8. Track PR Reviews

Review items are built from the open pull requests in your configured repos, plus optional review search queries:

```yaml
review_queue:
  auto_discover: true
  extra_filters:
    - "is:open is:pr team-review-requested:platform"
  escalation:
    amber: 24
    red: 48
```

The review queue prioritizes items by:
- Age (older = higher priority)
- CI status (failing CI = urgent)
- Agent source (agent PRs score higher)
- Review state (approved items sink)

Dismiss items with `3` from the detail or PR detail view. They stay dismissed across sessions.

If `auto_discover` is enabled, CIMON asks GitHub CLI for your username and adds a `review-requested:<you>` search query. `extra_filters` adds any other GitHub issue search queries you want to merge into the review queue.

## 9. Multi-Repo Setup

Add multiple repos to monitor everything from one dashboard:

```yaml
repos:
  - repo: owner/frontend
    branch: main
    groups:
      ci:
        label: "CI"
        workflows: [ci.yml]
        expand_jobs: true

  - repo: owner/backend
    branch: main
    groups:
      ci:
        label: "CI"
        workflows: [test.yml, lint.yml]
      release:
        label: "Deploy"
        workflows: [deploy.yml]
        auto_focus: true
      agents:
        label: "Agents"
        workflows: [claude.yml]
```

Each repo appears in compact view. Repos with failures, active runs, or ready PRs rise to the top automatically.

## Authentication

CIMON discovers your GitHub token in this order:

1. `GITHUB_TOKEN` environment variable
2. `GH_TOKEN` environment variable
3. `gh auth token` CLI command

The easiest path: install `gh` and run `gh auth login`.

For GitHub Enterprise Server, set `GH_HOST` to your hostname before running `cimon`, for example `GH_HOST=github.example.com cimon`.

## Troubleshooting

**"No .cimon.yml found"** — Run `cimon init` or create the file manually.

**"Auth error — check token"** — Run `gh auth status` to verify your token is valid and has the right scopes. On GitHub Enterprise Server, include the hostname in your auth flow and set `GH_HOST`.

**"Rate limit low"** — CIMON uses ETag caching to minimize API calls. If you're still hitting limits, increase the polling intervals.

**Nothing appears in the review section** — CIMON only shows open PRs from repos listed in your config.

**Agent PRs not flagged** — Check your `agent_patterns` config matches the patterns your agents use (PR body text, commit trailers, or bot usernames).
