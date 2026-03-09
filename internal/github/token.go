package github

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DiscoverToken tries: GITHUB_TOKEN env, GH_TOKEN env, `gh auth token`.
func DiscoverToken() (string, error) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t, nil
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("no GitHub token found: set GITHUB_TOKEN, GH_TOKEN, or run `gh auth login`")
	}
	return strings.TrimSpace(string(out)), nil
}

// DiscoverUsername returns the authenticated GitHub username.
func DiscoverUsername() string {
	out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
