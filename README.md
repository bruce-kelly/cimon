# CIMON — CI Monitor TUI for GitHub Actions

A compact terminal control plane for monitoring GitHub Actions pipelines and managing PR reviews across multiple repositories. Designed to run as a side pane alongside your editor.

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
5. **Navigate** — `j`/`k` to move, `Enter` to drill in, `?` for help

CIMON searches for `.cimon.yml` starting from the current directory, walking up to the root. If no config is found, it offers to run the setup wizard automatically.

## How It Works

CIMON has two views:

**Compact View** — One line per repo with inline status. Repos with CI failures or active runs auto-expand to show detail. Sorted by attention priority (failures first). A `NEW` flag appears when something changes.

```
CIMON ──────────────── 12:47

■ repo-a    ✓  2 PRs (2 ready)
■ repo-b    ✓  1 PR  (CI ⧗)
■ repo-c    ✗  3 PRs (1 ready)  NEW
  ci: build ✗  test ✗  4m ago
■ repo-d    ● releasing
  deploy ███████░░░ 7/10  1:22

────────────── active 5s  rl:4830
```

**Detail View** — Press `Enter` to drill into a repo. See recent CI runs with job expansion, open PRs sorted by review priority. Take action directly: rerun, approve, merge, dismiss, view diff.

## Features

- **Multi-repo monitoring** — Watch CI pipelines across all your repositories from one compact view
- **Inline expansion** — Failed and active repos auto-expand to show job details and progress bars
- **Review queue** — Priority-sorted PRs needing your attention, with escalation coloring by age
- **Agent PR detection** — Identifies agent-created PRs (Claude Code, Copilot, etc.) with configurable patterns
- **Batch merge** — Merge all ready agent PRs across repos with one keypress (`M`)
- **Change detection** — `NEW` flag on repo lines when CI breaks, PRs become merge-ready, or releases start
- **Attention sorting** — Repos ordered by priority: failures → active → ready PRs → all-green
- **Log pane** — View PR diffs and failed job logs without leaving the terminal
- **Cross-repo PR discovery** — Auto-discover PRs requesting your review across GitHub
- **Adaptive polling** — Automatic rate adjustment: idle (30s), active (5s), cooldown
- **SQLite persistence** — Workflow runs, PRs, and review events stored locally
- **Desktop notifications** — Opt-in alerts for CI failures (Linux/macOS)
- **ETag caching** — Conditional requests minimize API usage; 304s don't count against rate limits
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

### Compact View

| Key | Action |
|-----|--------|
| `j`/`k` (`w`/`s`) | Navigate repos |
| `Enter` | Drill into selected repo |
| `M` | Batch merge ready agent PRs |
| `?` | Help |
| `q` | Quit |

### Detail View

| Key | Action |
|-----|--------|
| `j`/`k` (`w`/`s`) | Navigate runs and PRs |
| `r` | Rerun (smart: failed jobs or full) |
| `A` | Approve PR |
| `m` | Merge PR (with confirm) |
| `x` | Dismiss PR |
| `v` | View diff / job logs |
| `o` | Open in browser |
| `l` | Toggle log pane |
| `Esc` | Back to compact view |

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
