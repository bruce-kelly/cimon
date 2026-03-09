package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/bruce-kelly/cimon/internal/app"
	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	"github.com/bruce-kelly/cimon/internal/github"
)

// Set by goreleaser ldflags.
var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "cimon",
	Short: "CI Monitor — GitHub Actions cockpit for your terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if cfg == nil {
			return fmt.Errorf("no .cimon.yml found — run `cimon init` to create one")
		}

		token, err := github.DiscoverToken()
		if err != nil {
			return err
		}

		client := github.NewClient(token)

		database, err := db.Open(cfg.Database.Path)
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		a := app.NewApp(cfg, client, database)
		p := tea.NewProgram(a)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print cimon version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("cimon %s\n", version)
	},
}

func main() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(dbCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
