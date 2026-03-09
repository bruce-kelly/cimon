# CIMON — CI Monitor TUI for GitHub Actions

A terminal control plane for monitoring GitHub Actions pipelines, tracking agent activity, and managing PR reviews across multiple repositories.

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
5. **Navigate** — `j`/`k` to move, `Enter` for actions, `?` for help

CIMON searches for `.cimon.yml` starting from the current directory, walking up to the root. If no config is found, it offers to run the setup wizard automatically.

## Features

- **Multi-repo monitoring** — Watch CI pipelines across all your repositories from one dashboard
- **Review queue** — Priority-sorted PRs needing your attention, with escalation coloring by age
- **Agent tracking** — Sparkline history, outcome states, and schedule info for agent workflows (Claude Code, Copilot, etc.)
- **Agent dispatch** — Spawn Claude Code agents directly from the TUI and track their lifecycle
- **Auto-fix copilot** — Automatically dispatch fix agents on new CI failures with cooldown and known failure detection
- **Agent lifecycle** — Track agent tasks from dispatch to PR creation, with batch merge for ready agent PRs
- **Release confidence** — 5-signal scoring (CI rate, failures, agent PRs, review queue) on the release screen
- **Catch-up overlay** — Summarizes CI/agent/PR changes after idle periods
- **Cross-repo PR discovery** — Auto-discover PRs requesting your review across GitHub
- **Adaptive polling** — Automatic rate adjustment: idle (30s), active (5s), cooldown
- **Release tracker** — Focused view on release workflows with job status and history
- **Timeline feed** — Chronological cross-repo event stream with color coding
- **Metrics screen** — Historical CI health and agent task statistics from SQLite
- **Filter bar** — `/` to filter any list by multi-term case-insensitive search
- **SQLite persistence** — Workflow runs, PRs, agent tasks, and review events stored locally
- **Agent scheduling** — Cron-based task dispatch with double-fire prevention
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
        expand_jobs: true
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
        expand_jobs: true
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
        auto_focus: true

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

See [`docs/getting-started.md`](docs/getting-started.md) for the full setup guide.

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `1`/`2`/`3`/`4` | Switch screen (dashboard / timeline / release / metrics) |
| `l` | Cycle log pane (off / docked / fullscreen) |
| `/` | Open filter bar |
| `?` | Toggle help overlay |
| `Esc` | Back / close overlay / clear filter |
| `q` | Quit |

### Dashboard

| Key | Action |
|-----|--------|
| `j`/`k` (`w`/`s`) | Navigate items |
| `Tab` | Cycle widget focus |
| `Enter` | Open action menu for selected item |
| `r` | Smart rerun (failed jobs or full rerun) |
| `a` | Approve PR |
| `m` | Merge PR |
| `v` | View PR diff / agent output |
| `x` | Dismiss item |
| `o` | Open in browser |
| `D` | Dispatch Claude agent |
| `M` | Batch merge ready agent PRs |

### Release

| Key | Action |
|-----|--------|
| `Left`/`Right` | Switch repos |
| `r` | Rerun |
| `o` | Open in browser |

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
