package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Set by goreleaser ldflags.
var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "cimon",
	Short: "CI Monitor — GitHub Actions cockpit for your terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("cimon %s\n", version)
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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up cimon for your repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("init: not yet implemented")
		return nil
	},
}

func main() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
