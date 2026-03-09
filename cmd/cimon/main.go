package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/bruce-kelly/cimon/internal/ui"
)

// Set by goreleaser ldflags.
var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "cimon",
	Short: "CI Monitor — GitHub Actions cockpit for your terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		app := ui.NewApp()
		p := tea.NewProgram(app)
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
