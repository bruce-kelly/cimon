package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bruce-kelly/cimon/internal/config"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up cimon for your repositories",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("cimon init — interactive setup")
	fmt.Println()

	// Step 1: Check token
	token, err := ghclient.DiscoverToken()
	if err != nil {
		return fmt.Errorf("GitHub token not found. Run `gh auth login` first")
	}
	fmt.Println("✓ GitHub token found")

	// Step 2: Detect repo from current directory
	repo, err := config.DetectRepo()
	if err != nil {
		fmt.Printf("Enter repository (owner/repo): ")
		input, _ := reader.ReadString('\n')
		repo = strings.TrimSpace(input)
	} else {
		fmt.Printf("Detected repo: %s — use this? [Y/n] ", repo)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "n" {
			fmt.Printf("Enter repository (owner/repo): ")
			input, _ = reader.ReadString('\n')
			repo = strings.TrimSpace(input)
		}
	}

	if repo == "" {
		return fmt.Errorf("no repository specified")
	}

	// Step 3: Detect branch
	branch := config.DetectBranch()
	fmt.Printf("Default branch: %s\n", branch)

	// Step 4: Discover workflows
	client := ghclient.NewClient(token)
	fmt.Printf("Discovering workflows for %s...\n", repo)
	ctx := cmd.Context()
	workflows, err := client.DiscoverWorkflows(ctx, repo)
	if err != nil {
		fmt.Printf("Warning: could not discover workflows: %v\n", err)
		workflows = []string{}
	}

	if len(workflows) > 0 {
		fmt.Printf("Found %d workflows:\n", len(workflows))
		for _, wf := range workflows {
			category := config.CategorizeWorkflow(wf)
			fmt.Printf("  %s (%s)\n", wf, category)
		}
	}

	// Step 5: Generate config
	cfg := config.BuildZeroConfig(repo, branch, workflows)

	// Step 6: Write .cimon.yml
	yamlContent := generateYAML(repo, branch, workflows)
	fmt.Println()
	fmt.Println("Generated .cimon.yml:")
	fmt.Println("---")
	fmt.Println(yamlContent)
	fmt.Println("---")

	fmt.Printf("Write .cimon.yml? [Y/n] ")
	input, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(input)) != "n" {
		if err := os.WriteFile(".cimon.yml", []byte(yamlContent), 0o644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		fmt.Println("✓ .cimon.yml written")
	}

	fmt.Println()
	fmt.Printf("Run `cimon` to start monitoring %s\n", repo)
	_ = cfg // used for validation
	return nil
}

func generateYAML(repo, branch string, workflows []string) string {
	var sb strings.Builder
	sb.WriteString("repos:\n")
	sb.WriteString(fmt.Sprintf("  - repo: %s\n", repo))
	sb.WriteString(fmt.Sprintf("    branch: %s\n", branch))
	sb.WriteString("    groups:\n")

	// Group workflows by category
	groups := make(map[string][]string)
	for _, wf := range workflows {
		cat := config.CategorizeWorkflow(wf)
		groups[cat] = append(groups[cat], wf)
	}

	labels := map[string]string{
		"ci":      "CI Pipeline",
		"release": "Release",
		"agents":  "Agents",
	}

	for _, group := range []string{"ci", "release", "agents"} {
		wfs, ok := groups[group]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("      %s:\n", group))
		sb.WriteString(fmt.Sprintf("        label: \"%s\"\n", labels[group]))
		sb.WriteString("        workflows:\n")
		for _, wf := range wfs {
			sb.WriteString(fmt.Sprintf("          - %s\n", wf))
		}
		if group == "ci" {
			sb.WriteString("        expand_jobs: true\n")
		}
	}

	return sb.String()
}
