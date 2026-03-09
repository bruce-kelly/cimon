package db

import (
	"fmt"
	"time"
)

// RunStatsResult holds aggregated workflow run statistics.
type RunStatsResult struct {
	Total   int
	Success int
	Failure int
}

// TaskStatsResult holds aggregated agent task statistics.
type TaskStatsResult struct {
	Total     int
	Completed int
	Failed    int
}

// EffectivenessResult holds agent effectiveness metrics.
type EffectivenessResult struct {
	Dispatched  int
	CreatedPR   int
	FailureRate float64
}

// RunStats returns aggregated run stats for the last N days.
func (d *Database) RunStats(days int) (RunStatsResult, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	var r RunStatsResult

	err := d.reader.QueryRow(`
		SELECT
			COUNT(*),
			SUM(CASE WHEN conclusion = 'success' THEN 1 ELSE 0 END),
			SUM(CASE WHEN conclusion = 'failure' THEN 1 ELSE 0 END)
		FROM workflow_runs
		WHERE updated_at >= ?`, cutoff,
	).Scan(&r.Total, &r.Success, &r.Failure)
	if err != nil {
		return r, fmt.Errorf("computing run stats: %w", err)
	}
	return r, nil
}

// TaskStats returns aggregated task stats for the last N days.
func (d *Database) TaskStats(days int) (TaskStatsResult, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	var r TaskStatsResult

	err := d.reader.QueryRow(`
		SELECT
			COUNT(*),
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)
		FROM agent_tasks
		WHERE started_at >= ?`, cutoff,
	).Scan(&r.Total, &r.Completed, &r.Failed)
	if err != nil {
		return r, fmt.Errorf("computing task stats: %w", err)
	}
	return r, nil
}

// AgentEffectivenessStats returns how effective agent dispatches have been
// over the last N days: total dispatched, how many created PRs, and failure rate.
func (d *Database) AgentEffectivenessStats(days int) (EffectivenessResult, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	var r EffectivenessResult
	var failed int

	err := d.reader.QueryRow(`
		SELECT
			COUNT(*),
			SUM(CASE WHEN pr_number IS NOT NULL THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)
		FROM agent_tasks
		WHERE started_at >= ?`, cutoff,
	).Scan(&r.Dispatched, &r.CreatedPR, &failed)
	if err != nil {
		return r, fmt.Errorf("computing effectiveness stats: %w", err)
	}

	if r.Dispatched > 0 {
		r.FailureRate = float64(failed) / float64(r.Dispatched)
	}

	return r, nil
}
