package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bruce-kelly/cimon/internal/db"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management commands",
}

var dbStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show database statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.Open(defaultDBPath())
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		runStats, err := database.RunStats(30)
		if err != nil {
			return fmt.Errorf("querying run stats: %w", err)
		}
		taskStats, err := database.TaskStats(30)
		if err != nil {
			return fmt.Errorf("querying task stats: %w", err)
		}
		effectiveness, err := database.AgentEffectivenessStats(30)
		if err != nil {
			return fmt.Errorf("querying effectiveness: %w", err)
		}

		fmt.Println("CI Runs (30 days):")
		fmt.Printf("  Total: %d  Success: %d  Failure: %d\n",
			runStats.Total, runStats.Success, runStats.Failure)
		fmt.Println()
		fmt.Println("Agent Tasks (30 days):")
		fmt.Printf("  Total: %d  Completed: %d  Failed: %d\n",
			taskStats.Total, taskStats.Completed, taskStats.Failed)
		fmt.Println()
		fmt.Println("Agent Effectiveness (30 days):")
		fmt.Printf("  Dispatched: %d  Created PRs: %d  Failure Rate: %.1f%%\n",
			effectiveness.Dispatched, effectiveness.CreatedPR, effectiveness.FailureRate*100)
		return nil
	},
}

var dbExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export database to JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.Open(defaultDBPath())
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		runs, err := database.QueryAllRuns(1000)
		if err != nil {
			return fmt.Errorf("querying runs: %w", err)
		}
		data := map[string]any{
			"runs": runs,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	},
}

var pruneDays int

var dbPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete old data from database",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.Open(defaultDBPath())
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		count, err := database.PruneRuns(pruneDays)
		if err != nil {
			return fmt.Errorf("pruning: %w", err)
		}
		fmt.Printf("Pruned %d runs older than %d days\n", count, pruneDays)
		return nil
	},
}

func init() {
	dbPruneCmd.Flags().IntVar(&pruneDays, "days", 90, "Delete runs older than N days")
	dbCmd.AddCommand(dbStatsCmd)
	dbCmd.AddCommand(dbExportCmd)
	dbCmd.AddCommand(dbPruneCmd)
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".local/share/cimon/cimon.db"
	}
	return home + "/.local/share/cimon/cimon.db"
}
