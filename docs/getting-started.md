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

### Option C: Auto-prompt

Just run `cimon` with no config file — it will offer to run the setup wizard.

## 3. Launch

```bash
cimon
```

CIMON searches for `.cimon.yml` starting from the current directory, walking up to the filesystem root.

## 4. Navigate Screens

CIMON has four main screens, switched with number keys:

### Dashboard (`1`) — Home screen

Three sections:

- **Review Queue** — PRs needing your attention, sorted by priority. Escalation coloring shows age (green < 24h, amber 24-48h, red > 48h).
- **Pipelines** — Latest CI runs per repo with job stages, SHAs, and elapsed times.
- **Agent Roster** — Sparkline history and status for each agent workflow, plus any locally dispatched agents.

### Timeline (`2`)

Chronological feed of all workflow runs across repos. Color-coded by repository. Useful for seeing the big picture of what's happening.

### Release Tracker (`3`)

Focused view on release workflows. Shows current job status, confidence scoring, and previous release history. Use `Left`/`Right` to switch between repos.

### Metrics (`4`)

Historical CI health and agent task statistics from SQLite. Per-repo breakdown of success rates, agent effectiveness, and dispatch stats.

## 5. Use Keybindings

### Navigation

| Key | Action |
|-----|--------|
| `1`/`2`/`3`/`4` | Switch screen |
| `j`/`k` | Move up/down in lists |
| `Tab` | Cycle focus between dashboard widgets |

### Quick Actions

| Key | Action |
|-----|--------|
| `r` | Smart rerun — reruns failed jobs if any, otherwise full rerun |
| `a` | Approve the selected PR |
| `m` | Merge the selected PR |
| `v` | View PR diff in the log pane |
| `x` | Dismiss item from review queue |
| `o` | Open selected item in browser |
| `D` | Dispatch a Claude Code agent |

### More Actions via Menu

Press `Enter` on any selected item to open the **action menu**. This shows context-appropriate actions:

- **On a PR:** comment, close, create tag
- **On a run:** cancel, create tag
- **On an agent:** dispatch workflow

Navigate with `j`/`k`, select with `Enter`, close with `Esc`.

### Log Pane

| Key | Action |
|-----|--------|
| `l` | Cycle log pane: closed -> open (30%) -> fullscreen -> closed |
| `Esc` | Close log pane |

### Help

Press `?` at any time to see context-sensitive keybinding help for the current screen.

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

The agent roster shows:
- **Sparkline** — recent run history (green = success, red = failure)
- **State** — ALERT (failed), running, or idle
- **Timing** — next scheduled run or time since last run

Agent-created PRs are flagged in the review queue and score higher priority.

## 8. Dispatch Local Agents

Press `D` to dispatch a Claude Code agent:
1. Enter a task prompt
2. Confirm the dispatch
3. The agent appears in the roster with live status tracking

Dispatched agents run as local subprocesses (`claude -p <task>`). They're automatically terminated on CIMON exit.

## 9. Track PR Reviews

Enable cross-repo review tracking to see PRs that need your attention:

```yaml
review_queue:
  auto_discover: true
  extra_filters:
    - "is:open is:pr review-requested:@me"
  escalation:
    amber: 24
    red: 48
```

The review queue prioritizes items by:
- Age (older = higher priority)
- CI status (failing CI = urgent)
- Agent source (agent PRs score higher)
- Review state (approved items sink)

Dismiss items with `x` — they stay dismissed across sessions.

## 10. Multi-Repo Setup

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

Each repo gets its own pipeline on the dashboard. The timeline merges all repos chronologically. The release tracker lets you switch between repos with arrow keys.

## Authentication

CIMON discovers your GitHub token in this order:

1. `GITHUB_TOKEN` environment variable
2. `GH_TOKEN` environment variable
3. `gh auth token` CLI command

The easiest path: install `gh` and run `gh auth login`.

## Troubleshooting

**"No .cimon.yml found"** — Run `cimon init` or create the file manually.

**"Auth error — check token"** — Run `gh auth status` to verify your token is valid and has the right scopes.

**"Rate limit low"** — CIMON uses ETag caching to minimize API calls. If you're still hitting limits, increase the polling intervals.

**Nothing appears in review queue** — Make sure `auto_discover: true` is set and your GitHub username is detectable via `gh api user --jq .login`.

**Agent PRs not flagged** — Check your `agent_patterns` config matches the patterns your agents use (PR body text, commit trailers, or bot usernames).
