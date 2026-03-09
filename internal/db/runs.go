package db

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
)

// UpsertRun inserts or replaces a workflow run record.
func (d *Database) UpsertRun(run models.WorkflowRun) error {
	_, err := d.writer.Exec(`
		INSERT OR REPLACE INTO workflow_runs
			(id, repo, name, workflow_file, head_branch, head_sha,
			 status, conclusion, event, actor, created_at, updated_at, html_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.Repo, run.Name, run.WorkflowFile,
		run.HeadBranch, run.HeadSHA,
		run.Status, run.Conclusion, run.Event, run.Actor,
		run.CreatedAt.UTC().Format(time.RFC3339),
		run.UpdatedAt.UTC().Format(time.RFC3339),
		run.HTMLURL,
	)
	if err != nil {
		return fmt.Errorf("upserting run %d: %w", run.ID, err)
	}
	return nil
}

// UpsertJobs inserts or replaces job records for a given run.
func (d *Database) UpsertJobs(runID int64, repo string, jobs []models.Job) error {
	tx, err := d.writer.Begin()
	if err != nil {
		return fmt.Errorf("beginning job upsert tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO jobs
			(id, run_id, repo, name, conclusion, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing job upsert: %w", err)
	}
	defer stmt.Close()

	for _, j := range jobs {
		var startedAt, completedAt *string
		if j.StartedAt != nil {
			s := j.StartedAt.UTC().Format(time.RFC3339)
			startedAt = &s
		}
		if j.CompletedAt != nil {
			s := j.CompletedAt.UTC().Format(time.RFC3339)
			completedAt = &s
		}
		if _, err := stmt.Exec(j.ID, runID, repo, j.Name, j.Conclusion, startedAt, completedAt); err != nil {
			return fmt.Errorf("upserting job %d: %w", j.ID, err)
		}
	}

	return tx.Commit()
}

// QueryRuns returns the most recent workflow runs for a repo, ordered by updated_at DESC.
func (d *Database) QueryRuns(repo string, limit int) ([]models.WorkflowRun, error) {
	rows, err := d.reader.Query(`
		SELECT id, repo, name, workflow_file, head_branch, head_sha,
		       status, conclusion, event, actor, created_at, updated_at, html_url
		FROM workflow_runs
		WHERE repo = ?
		ORDER BY updated_at DESC
		LIMIT ?`, repo, limit)
	if err != nil {
		return nil, fmt.Errorf("querying runs: %w", err)
	}
	defer rows.Close()

	var runs []models.WorkflowRun
	for rows.Next() {
		var r models.WorkflowRun
		var createdStr, updatedStr string
		var headBranch, headSHA, status, conclusion, event, actor, htmlURL *string

		if err := rows.Scan(
			&r.ID, &r.Repo, &r.Name, &r.WorkflowFile,
			&headBranch, &headSHA, &status, &conclusion,
			&event, &actor, &createdStr, &updatedStr, &htmlURL,
		); err != nil {
			return nil, fmt.Errorf("scanning run: %w", err)
		}

		if headBranch != nil {
			r.HeadBranch = *headBranch
		}
		if headSHA != nil {
			r.HeadSHA = *headSHA
		}
		if status != nil {
			r.Status = *status
		}
		if conclusion != nil {
			r.Conclusion = *conclusion
		}
		if event != nil {
			r.Event = *event
		}
		if actor != nil {
			r.Actor = *actor
		}
		if htmlURL != nil {
			r.HTMLURL = *htmlURL
		}

		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			r.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, updatedStr); err == nil {
			r.UpdatedAt = t
		}

		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// QueryAllRuns returns the most recent workflow runs across all repos.
func (d *Database) QueryAllRuns(limit int) ([]models.WorkflowRun, error) {
	rows, err := d.reader.Query(`
		SELECT id, repo, name, workflow_file, head_branch, head_sha,
		       status, conclusion, event, actor, created_at, updated_at, html_url
		FROM workflow_runs
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying all runs: %w", err)
	}
	defer rows.Close()

	var runs []models.WorkflowRun
	for rows.Next() {
		var r models.WorkflowRun
		var createdStr, updatedStr string
		var headBranch, headSHA, status, conclusion, event, actor, htmlURL *string

		if err := rows.Scan(
			&r.ID, &r.Repo, &r.Name, &r.WorkflowFile,
			&headBranch, &headSHA, &status, &conclusion,
			&event, &actor, &createdStr, &updatedStr, &htmlURL,
		); err != nil {
			return nil, fmt.Errorf("scanning run: %w", err)
		}

		if headBranch != nil {
			r.HeadBranch = *headBranch
		}
		if headSHA != nil {
			r.HeadSHA = *headSHA
		}
		if status != nil {
			r.Status = *status
		}
		if conclusion != nil {
			r.Conclusion = *conclusion
		}
		if event != nil {
			r.Event = *event
		}
		if actor != nil {
			r.Actor = *actor
		}
		if htmlURL != nil {
			r.HTMLURL = *htmlURL
		}

		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			r.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, updatedStr); err == nil {
			r.UpdatedAt = t
		}

		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// RunsSince returns the count of runs updated since the given time.
func (d *Database) RunsSince(repo string, since time.Time) (int, error) {
	var count int
	err := d.reader.QueryRow(`
		SELECT COUNT(*) FROM workflow_runs
		WHERE repo = ? AND updated_at >= ?`,
		repo, since.UTC().Format(time.RFC3339),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting runs since: %w", err)
	}
	return count, nil
}

// IsKnownFailure checks whether a job name has failed 3+ times on the main
// branch within the last N hours — indicating a recurring/known failure.
func (d *Database) IsKnownFailure(repo, jobName string, hours int) (bool, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339)
	var count int
	err := d.reader.QueryRow(`
		SELECT COUNT(*) FROM jobs j
		JOIN workflow_runs r ON j.run_id = r.id
		WHERE j.repo = ? AND j.name = ? AND j.conclusion = 'failure'
		  AND r.head_branch = 'main'
		  AND j.completed_at >= ?`,
		repo, jobName, cutoff,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking known failure: %w", err)
	}
	return count >= 3, nil
}

// PruneRuns deletes workflow runs older than retentionDays and their
// associated jobs. Returns the number of runs deleted.
func (d *Database) PruneRuns(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC().Format(time.RFC3339)

	tx, err := d.writer.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning prune tx: %w", err)
	}
	defer tx.Rollback()

	// Delete orphaned jobs first
	if _, err := tx.Exec(`
		DELETE FROM jobs WHERE run_id IN (
			SELECT id FROM workflow_runs WHERE updated_at < ?
		)`, cutoff); err != nil {
		return 0, fmt.Errorf("pruning jobs: %w", err)
	}

	result, err := tx.Exec(`DELETE FROM workflow_runs WHERE updated_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning runs: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting prune count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing prune: %w", err)
	}

	// Reclaim WAL space after bulk delete
	if _, err := d.writer.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		slog.Warn("WAL checkpoint after prune failed", "err", err)
	}

	return count, nil
}
