package config

import (
	"testing"
)

func TestParseGitHubRemote_SSH(t *testing.T) {
	got, err := ParseGitHubRemote("git@github.com:owner/repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/repo" {
		t.Errorf("got %q, want %q", got, "owner/repo")
	}
}

func TestParseGitHubRemote_SSHNoGit(t *testing.T) {
	got, err := ParseGitHubRemote("git@github.com:owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/repo" {
		t.Errorf("got %q, want %q", got, "owner/repo")
	}
}

func TestParseGitHubRemote_HTTPS(t *testing.T) {
	got, err := ParseGitHubRemote("https://github.com/owner/repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/repo" {
		t.Errorf("got %q, want %q", got, "owner/repo")
	}
}

func TestParseGitHubRemote_HTTPSNoGit(t *testing.T) {
	got, err := ParseGitHubRemote("https://github.com/owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/repo" {
		t.Errorf("got %q, want %q", got, "owner/repo")
	}
}

func TestParseGitHubRemote_NonGitHub(t *testing.T) {
	_, err := ParseGitHubRemote("https://gitlab.com/owner/repo")
	if err == nil {
		t.Fatal("expected error for non-GitHub URL")
	}
}

func TestParseGitHubRemote_Empty(t *testing.T) {
	_, err := ParseGitHubRemote("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestCategorizeWorkflow(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"ci.yml", "ci"},
		{"release.yml", "release"},
		{"claude.yml", "agents"},
		{"deploy.yml", "release"},
		{"ai-review.yml", "agents"},
		{"test.yml", "ci"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeWorkflow(tt.name)
			if got != tt.want {
				t.Errorf("CategorizeWorkflow(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestBuildZeroConfig(t *testing.T) {
	cfg := BuildZeroConfig("owner/repo", "develop", []string{"ci.yml", "release.yml", "claude.yml"})

	if cfg.Source != "zero-config" {
		t.Errorf("Source = %q, want %q", cfg.Source, "zero-config")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(cfg.Repos))
	}

	r := cfg.Repos[0]
	if r.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", r.Repo, "owner/repo")
	}
	if r.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", r.Branch, "develop")
	}

	// CI group
	ci, ok := r.Groups["ci"]
	if !ok {
		t.Fatal("missing ci group")
	}
	if ci.Label != "CI Pipeline" {
		t.Errorf("ci Label = %q, want %q", ci.Label, "CI Pipeline")
	}
	if len(ci.Workflows) != 1 || ci.Workflows[0] != "ci.yml" {
		t.Errorf("ci Workflows = %v, want [ci.yml]", ci.Workflows)
	}
	if !ci.ExpandJobs {
		t.Error("ci ExpandJobs should be true")
	}

	// Release group
	rel, ok := r.Groups["release"]
	if !ok {
		t.Fatal("missing release group")
	}
	if rel.Label != "Release" {
		t.Errorf("release Label = %q, want %q", rel.Label, "Release")
	}
	if len(rel.Workflows) != 1 || rel.Workflows[0] != "release.yml" {
		t.Errorf("release Workflows = %v, want [release.yml]", rel.Workflows)
	}

	// Agents group
	agents, ok := r.Groups["agents"]
	if !ok {
		t.Fatal("missing agents group")
	}
	if agents.Label != "Agents" {
		t.Errorf("agents Label = %q, want %q", agents.Label, "Agents")
	}
	if len(agents.Workflows) != 1 || agents.Workflows[0] != "claude.yml" {
		t.Errorf("agents Workflows = %v, want [claude.yml]", agents.Workflows)
	}

	// Defaults applied
	if cfg.Polling.Idle != 30 {
		t.Errorf("Polling.Idle = %d, want 30", cfg.Polling.Idle)
	}
	if cfg.Polling.Active != 5 {
		t.Errorf("Polling.Active = %d, want 5", cfg.Polling.Active)
	}
	if cfg.Database.RetentionDays != 90 {
		t.Errorf("Database.RetentionDays = %d, want 90", cfg.Database.RetentionDays)
	}
}

func TestBuildZeroConfig_EmptyWorkflows(t *testing.T) {
	cfg := BuildZeroConfig("owner/repo", "main", []string{})

	if cfg.Source != "zero-config" {
		t.Errorf("Source = %q, want %q", cfg.Source, "zero-config")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(cfg.Repos))
	}

	r := cfg.Repos[0]
	if len(r.Groups) != 0 {
		t.Errorf("got %d groups, want 0", len(r.Groups))
	}

	// Defaults still applied
	if cfg.Polling.Idle != 30 {
		t.Errorf("Polling.Idle = %d, want 30", cfg.Polling.Idle)
	}
	if cfg.Agents.MaxConcurrent != 2 {
		t.Errorf("Agents.MaxConcurrent = %d, want 2", cfg.Agents.MaxConcurrent)
	}
}
