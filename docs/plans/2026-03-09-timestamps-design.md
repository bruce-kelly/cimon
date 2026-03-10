# Timestamps in the UI — Design

**Goal:** Show when things happened — relative timestamps ("3h ago") on all screens, absolute timestamps ("14:23") on the timeline.

**Architecture:** Add two formatting functions to `components` package. Wire them into existing render methods. No model or DB changes — all timestamp fields already exist.

---

## Formatting Functions

**`FormatTimeAgo(t time.Time) string`** — relative:
- <1m → `"now"`, 1-59m → `"3m ago"`, 1-23h → `"2h ago"`, 1-6d → `"1d ago"`, 7-29d → `"2w ago"`, 30+d → `"3mo ago"`

**`FormatTimeAbsolute(t time.Time) string`** — for timeline:
- Today → `"14:23"`, this year → `"Mar 9 14:23"`, older → `"2025-03-09 14:23"`

## Where Timestamps Appear

| Location | Source | Format | Render change |
|---|---|---|---|
| Pipeline runs | `run.UpdatedAt` | relative | Append after elapsed |
| Review queue | `pr.CreatedAt` | relative | Append after title/badge |
| Agent roster | `profile.LastRunAt` | relative | Append after success rate |
| Dispatched agents | `agent.StartedAt` | relative | Append after status |
| Timeline | `run.UpdatedAt` | absolute | Prefix before status dot |
| Release runs | `run.UpdatedAt` | relative | Append after elapsed |
