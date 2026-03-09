package config

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var (
	sshPattern   = regexp.MustCompile(`git@github\.com:(.+/.+?)(?:\.git)?$`)
	httpsPattern = regexp.MustCompile(`https://github\.com/(.+/.+?)(?:\.git)?$`)
)

// DetectRepo extracts owner/repo from the current git directory's origin remote.
func DetectRepo() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repo or no origin remote")
	}
	return ParseGitHubRemote(strings.TrimSpace(string(out)))
}

// ParseGitHubRemote extracts owner/repo from a GitHub remote URL.
// Exported for testing.
func ParseGitHubRemote(url string) (string, error) {
	if m := sshPattern.FindStringSubmatch(url); len(m) == 2 {
		return m[1], nil
	}
	if m := httpsPattern.FindStringSubmatch(url); len(m) == 2 {
		return m[1], nil
	}
	return "", fmt.Errorf("not a GitHub remote: %s", url)
}

// DetectBranch returns the default branch name.
func DetectBranch() string {
	out, err := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err != nil {
		return "main"
	}
	ref := strings.TrimSpace(string(out))
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// CategorizeWorkflow assigns a workflow filename to a group (ci, release, or agents).
func CategorizeWorkflow(name string) string {
	lower := strings.ToLower(name)
	// Agent workflows
	agentPatterns := []string{"claude", "agent", "copilot", "ai-", "bot"}
	for _, p := range agentPatterns {
		if strings.Contains(lower, p) {
			return "agents"
		}
	}
	// Release workflows
	releasePatterns := []string{"release", "deploy", "publish", "cd"}
	for _, p := range releasePatterns {
		if strings.Contains(lower, p) {
			return "release"
		}
	}
	// Everything else is CI
	return "ci"
}

// BuildZeroConfig creates a CimonConfig from auto-detected repo + workflows.
func BuildZeroConfig(repo, branch string, workflows []string) *CimonConfig {
	groups := make(map[string]GroupConfig)
	for _, wf := range workflows {
		group := CategorizeWorkflow(wf)
		g := groups[group]
		g.Workflows = append(g.Workflows, wf)
		if g.Label == "" {
			labels := map[string]string{
				"ci":      "CI Pipeline",
				"release": "Release",
				"agents":  "Agents",
			}
			g.Label = labels[group]
		}
		if group == "ci" {
			g.ExpandJobs = true
		}
		groups[group] = g
	}

	cfg := &CimonConfig{
		Repos: []RepoConfig{{
			Repo:   repo,
			Branch: branch,
			Groups: groups,
		}},
		Source: "zero-config",
	}
	applyDefaults(cfg)
	return cfg
}
