# CIMON — CI Monitor TUI for GitHub Actions

Cimon is a terminal UI for watching GitHub Actions and open pull requests across your repositories. It is designed to sit in a split pane while you work.

It is useful if you already spend most of the day in a terminal and want repo status nearby. It is not trying to replace GitHub; it is a lightweight monitor with a drill-down view when you need more detail.

<!-- screenshot placeholder -->

## Install

### Binary (recommended)

Download from [Releases](https://github.com/bruce-kelly/cimon/releases) or via Homebrew:

```bash
brew install bruce-kelly/tap/cimon
```

### From source

```bash
go install github.com/bruce-kelly/cimon/cmd/cimon@latest
```

## Quickstart

1. **Install** — download binary or `go install`
2. **Set up config** — `cimon init` (interactive wizard discovers your repos and workflows)
3. **Edit config** — Tweak `.cimon.yml` if needed (see [Config Reference](#config-reference))
4. **Run** — `cimon`
5. **Navigate** — `w`/`s` to move, `d`/`Enter` to drill in, `?` for help

Cimon searches for `.cimon.yml` starting from the current directory, walking up to the root. If no config is found, it offers to run the setup wizard automatically.

## Overview

The main flow starts in compact mode and drills down from there:

**Compact View** — One line per repo with inline status. Repos with CI failures or active runs auto-expand to show detail. Repos are sorted by attention priority so the noisy or broken ones rise to the top. A `NEW` flag appears when something changes. Agent workflow failures show as amber (⚠) separately from critical CI failures (red ✗).

```
CIMON ──────────────── 12:47

■ repo-a    ✓  2 PRs (2 ready)
■ repo-b    ✓  1 PR  (CI ⧗)
■ repo-c    ✗  3 PRs (1 ready)  NEW
  ci: build ✗  test ✗  4m ago
■ repo-d    ⚠  2 agent workflows failing
■ repo-e    ● releasing
  deploy ███████░░░ 7/10  1:22

────────────── active 5s  rl:4830
```

**Detail View** — Press `d`/`Enter` to drill into a repo. Runs are grouped by config group (CI Pipeline, Release, Agents, etc.) with section headers, deduplicated to the latest per workflow. From here you can rerun CI, dispatch agent workflows, approve, merge, dismiss, or open diffs and logs.

**Run Detail View** — Drill into a specific workflow run. See all jobs with expand/collapse for steps. Failed job logs are fetched automatically.

**PR Detail View** — Drill into a specific PR. See the changed files, CI and review status, and agent badges. View diffs per file.

## Features

- **Repo overview in one place** — Watch CI pipelines and open PRs across multiple repositories from one compact view
- **Drill-down navigation** — Start from repo status, then open a repo, a run, or a PR for more detail
- **Inline expansion** — Failed and active repos auto-expand to show job details and progress bars
- **Agent-aware severity** — Agent workflow failures show as amber, while CI/build/release failures stay red
- **Direct actions** — Rerun workflows, dispatch agent workflows, approve PRs, merge, dismiss, and open GitHub from the TUI
- **Grouped detail view** — Runs are organized by config group (CI, builds, release, agents) with section headers
- **Review queue** — Open PRs are summarized and sorted by attention
- **Agent PR detection** — Agent-created PRs can be identified with configurable patterns
- **Change detection** — `NEW` flags appear when CI breaks, PRs become merge-ready, or releases start
- **Resilient polling** — Deleted or renamed workflows return 404 once, then are skipped for the session
- **Log pane** — View PR diffs and failed job logs without leaving the terminal
- **Adaptive polling** — Polling automatically speeds up when work is active and slows down when things are quiet
- **SQLite persistence** — Workflow runs, PRs, and review events are stored locally
- **Desktop notifications** — Optional CI failure alerts on Linux and macOS
- **ETag caching** — Conditional requests help reduce API usage
- **Cross-platform** — Single binary for Linux, macOS, and Windows (amd64/arm64)

## Config Reference

Minimal `.cimon.yml`:

```yaml
repos:
  - repo: owner/repo-name
    branch: main
    groups:
      ci:
        label: "CI Pipeline"
        workflows: [ci.yml]
```

Multi-repo with agents and review queue:

```yaml
repos:
  - repo: owner/frontend
    branch: main
    groups:
      ci:
        label: "CI"
        workflows: [ci.yml]
      agents:
        label: "Agents"
        workflows: [claude.yml]
    agent_patterns:
      pr_body: "Generated with Claude Code"
      commit_trailer: "Co-Authored-By: Claude"

  - repo: owner/backend
    branch: main
    groups:
      ci:
        label: "CI"
        workflows: [test.yml]
      release:
        label: "Deploy"
        workflows: [deploy.yml]

review_queue:
  auto_discover: true
  escalation:
    amber: 24
    red: 48

polling:
  idle: 30
  active: 5
  cooldown: 3
```

Only `repos` is required. Everything else has defaults. Old single-repo configs (`repo:` key) auto-migrate.

## Keybindings

### Navigation (all views)

| Key | Action |
|-----|--------|
| `w` / `↑` | Cursor up |
| `s` / `↓` | Cursor down |
| `d` / `Enter` | Drill in / expand |
| `a` / `Esc` | Back (closes log pane first) |
| `?` | Help overlay |
| `q` | Quit |

### Compact View

| Key | Action |
|-----|--------|
| `1` | Batch merge ready agent PRs |

### Detail View

| Key | Action |
|-----|--------|
| `1` | Rerun (CI run) / Dispatch (agent run) / Approve (PR) |
| `2` | View diff / logs |
| `3` | Dismiss PR |
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
| `d` | Jump to file diff |
| `1` | Approve PR |
| `2` | Merge PR (with confirm) |
| `3` | Dismiss PR |
| `e` | Toggle log pane |
| `r` | Open on GitHub |

## Requirements

- GitHub token via `GITHUB_TOKEN` env, `GH_TOKEN` env, or `gh auth token`
- Terminal with Unicode support

## Development

```bash
# Build
go build -o cimon ./cmd/cimon

# Run tests
go test ./... -count=1

# Lint
go vet ./...

# Release (requires goreleaser)
goreleaser release --snapshot --clean
```

## License

MIT
