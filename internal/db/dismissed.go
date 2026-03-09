package db

import (
	"fmt"
	"time"
)

// AddDismissed marks a PR as dismissed.
func (d *Database) AddDismissed(repo string, number int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.writer.Exec(`
		INSERT OR REPLACE INTO dismissed_items (repo, number, dismissed_at)
		VALUES (?, ?, ?)`, repo, number, now,
	)
	if err != nil {
		return fmt.Errorf("dismissing %s#%d: %w", repo, number, err)
	}
	return nil
}

// RemoveDismissed un-dismisses a PR.
func (d *Database) RemoveDismissed(repo string, number int) error {
	_, err := d.writer.Exec(`
		DELETE FROM dismissed_items WHERE repo = ? AND number = ?`,
		repo, number,
	)
	if err != nil {
		return fmt.Errorf("removing dismissed %s#%d: %w", repo, number, err)
	}
	return nil
}

// IsDismissed checks whether a PR has been dismissed.
func (d *Database) IsDismissed(repo string, number int) (bool, error) {
	var count int
	err := d.reader.QueryRow(`
		SELECT COUNT(*) FROM dismissed_items WHERE repo = ? AND number = ?`,
		repo, number,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking dismissed %s#%d: %w", repo, number, err)
	}
	return count > 0, nil
}

// LoadDismissed returns all dismissed items as a map of "repo:number" → true.
func (d *Database) LoadDismissed() (map[string]bool, error) {
	rows, err := d.reader.Query(`SELECT repo, number FROM dismissed_items`)
	if err != nil {
		return nil, fmt.Errorf("loading dismissed: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var repo string
		var number int
		if err := rows.Scan(&repo, &number); err != nil {
			return nil, fmt.Errorf("scanning dismissed: %w", err)
		}
		key := fmt.Sprintf("%s:%d", repo, number)
		result[key] = true
	}
	return result, rows.Err()
}
