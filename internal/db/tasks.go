package db

import (
	"fmt"
	"time"
)

// AgentTask represents a dispatched agent task record.
type AgentTask struct {
	ID          string
	Repo        string
	Task        string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
	ExitCode    *int
	PRNumber    *int
}

// InsertTask records a new agent task.
func (d *Database) InsertTask(id, repo, task string, startedAt time.Time) error {
	_, err := d.writer.Exec(`
		INSERT INTO agent_tasks (id, repo, task, status, started_at)
		VALUES (?, ?, ?, 'running', ?)`,
		id, repo, task, startedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting task %s: %w", id, err)
	}
	return nil
}

// UpdateTaskStatus updates status, exit code, and completed_at for a task.
func (d *Database) UpdateTaskStatus(id, status string, exitCode *int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.writer.Exec(`
		UPDATE agent_tasks
		SET status = ?, exit_code = ?, completed_at = ?
		WHERE id = ?`,
		status, exitCode, now, id,
	)
	if err != nil {
		return fmt.Errorf("updating task %s: %w", id, err)
	}
	return nil
}

// LinkTaskToPR associates a completed agent task with the PR it created.
func (d *Database) LinkTaskToPR(id string, prNumber int) error {
	_, err := d.writer.Exec(`
		UPDATE agent_tasks SET pr_number = ? WHERE id = ?`,
		prNumber, id,
	)
	if err != nil {
		return fmt.Errorf("linking task %s to PR %d: %w", id, prNumber, err)
	}
	return nil
}

// QueryTasks returns agent tasks filtered by repo and/or status.
// Pass empty string to skip a filter.
func (d *Database) QueryTasks(repo string, status string) ([]AgentTask, error) {
	query := `SELECT id, repo, task, status, started_at, completed_at, exit_code, pr_number
		FROM agent_tasks WHERE 1=1`
	var args []any

	if repo != "" {
		query += ` AND repo = ?`
		args = append(args, repo)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY started_at DESC`

	rows, err := d.reader.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying tasks: %w", err)
	}
	defer rows.Close()

	var tasks []AgentTask
	for rows.Next() {
		var t AgentTask
		var startedStr string
		var completedStr *string

		if err := rows.Scan(
			&t.ID, &t.Repo, &t.Task, &t.Status,
			&startedStr, &completedStr, &t.ExitCode, &t.PRNumber,
		); err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}

		t.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		if completedStr != nil {
			ct, _ := time.Parse(time.RFC3339, *completedStr)
			t.CompletedAt = &ct
		}

		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// TasksSince returns the count of tasks started since the given time.
func (d *Database) TasksSince(repo string, since time.Time) (int, error) {
	var count int
	err := d.reader.QueryRow(`
		SELECT COUNT(*) FROM agent_tasks
		WHERE repo = ? AND started_at >= ?`,
		repo, since.UTC().Format(time.RFC3339),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting tasks since: %w", err)
	}
	return count, nil
}

// MarkOrphanedTasks marks running tasks as failed if their ID is not
// in the active set. Returns the number of tasks marked.
func (d *Database) MarkOrphanedTasks(activePIDs map[string]bool) (int64, error) {
	// Get all running tasks
	rows, err := d.reader.Query(`SELECT id FROM agent_tasks WHERE status = 'running'`)
	if err != nil {
		return 0, fmt.Errorf("querying running tasks: %w", err)
	}
	defer rows.Close()

	var orphanIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scanning task id: %w", err)
		}
		if !activePIDs[id] {
			orphanIDs = append(orphanIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(orphanIDs) == 0 {
		return 0, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var count int64
	for _, id := range orphanIDs {
		result, err := d.writer.Exec(`
			UPDATE agent_tasks SET status = 'failed', completed_at = ?
			WHERE id = ? AND status = 'running'`,
			now, id,
		)
		if err != nil {
			return count, fmt.Errorf("marking orphan %s: %w", id, err)
		}
		n, _ := result.RowsAffected()
		count += n
	}
	return count, nil
}
