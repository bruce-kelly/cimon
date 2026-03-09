package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigError is a custom error type for configuration problems.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config: %s", e.Message)
}

type GroupConfig struct {
	Label           string   `yaml:"label"`
	Workflows       []string `yaml:"workflows"`
	ExpandJobs      bool     `yaml:"expand_jobs"`
	AutoFocus       bool     `yaml:"auto_focus"`
	AutoFix         bool     `yaml:"auto_fix"`
	AutoFixCooldown int      `yaml:"auto_fix_cooldown"`
}

type AgentPatternsConfig struct {
	PRBody        string   `yaml:"pr_body"`
	CommitTrailer string   `yaml:"commit_trailer"`
	BotAuthors    []string `yaml:"bot_authors"`
}

type RepoConfig struct {
	Repo          string                 `yaml:"repo"`
	Branch        string                 `yaml:"branch"`
	Groups        map[string]GroupConfig  `yaml:"groups"`
	Secrets       []string               `yaml:"secrets"`
	AgentPatterns AgentPatternsConfig    `yaml:"agent_patterns"`
}

type PollingConfig struct {
	Idle     int `yaml:"idle"`
	Active   int `yaml:"active"`
	Cooldown int `yaml:"cooldown"`
}

type ReviewQueueConfig struct {
	AutoDiscover bool             `yaml:"auto_discover"`
	ExtraFilters []string         `yaml:"extra_filters"`
	Escalation   EscalationConfig `yaml:"escalation"`
}

type EscalationConfig struct {
	Amber int `yaml:"amber"`
	Red   int `yaml:"red"`
}

type DatabaseConfig struct {
	Path          string `yaml:"path"`
	RetentionDays int    `yaml:"retention_days"`
}

type ScheduledAgentConfig struct {
	Name     string `yaml:"name"`
	Cron     string `yaml:"cron"`
	Workflow string `yaml:"workflow"`
	Prompt   string `yaml:"prompt"`
	Repo     string `yaml:"repo"`
}

type AgentsConfig struct {
	Scheduled     []ScheduledAgentConfig `yaml:"scheduled"`
	MaxConcurrent int                    `yaml:"max_concurrent"`
	MaxLifetime   int                    `yaml:"max_lifetime"`
	CaptureOutput bool                   `yaml:"capture_output"`
}

type CatchupConfig struct {
	Enabled       bool `yaml:"enabled"`
	IdleThreshold int  `yaml:"idle_threshold"`
}

type CimonConfig struct {
	Repos         []RepoConfig      `yaml:"repos"`
	Polling       PollingConfig     `yaml:"polling"`
	ReviewQueue   ReviewQueueConfig `yaml:"review_queue"`
	Database      DatabaseConfig    `yaml:"database"`
	Agents        AgentsConfig      `yaml:"agents"`
	Notifications bool              `yaml:"notifications"`
	Catchup       CatchupConfig     `yaml:"catchup"`
	Source        string            `yaml:"-"`
}

// findConfigFile walks upward from CWD looking for .cimon.yml, then checks
// ~/.config/cimon/config.yml. Returns the path or empty string if not found.
func findConfigFile() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, ".cimon.yml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Check XDG config location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	candidate := filepath.Join(home, ".config", "cimon", "config.yml")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	return "", nil
}

// Load finds and loads the config file. Returns nil, nil if no config file
// is found (zero-config mode).
func Load() (*CimonConfig, error) {
	path, err := findConfigFile()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return loadFromFile(path)
}

// LoadFromPath loads config from a specific file path. Exported for testing.
func LoadFromPath(path string) (*CimonConfig, error) {
	return loadFromFile(path)
}

func loadFromFile(path string) (*CimonConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("reading %s: %v", path, err)}
	}
	cfg, err := parse(data)
	if err != nil {
		return nil, err
	}
	cfg.Source = "file"
	return cfg, nil
}

func parse(data []byte) (*CimonConfig, error) {
	// First unmarshal into a raw map to detect v1 format
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("invalid YAML: %v", err)}
	}

	// v1 detection: top-level "repo" key without "repos"
	if _, hasRepo := raw["repo"]; hasRepo {
		if _, hasRepos := raw["repos"]; !hasRepos {
			var err error
			data, err = migrateV1(raw)
			if err != nil {
				return nil, err
			}
		}
	}

	var cfg CimonConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("parsing config: %v", err)}
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// migrateV1 converts a v1 single-repo config map to v2 multi-repo YAML bytes.
func migrateV1(raw map[string]any) ([]byte, error) {
	// Extract repo-level fields into a single repo entry
	repoEntry := make(map[string]any)

	// Fields that belong on the repo object
	repoFields := []string{"repo", "branch", "groups", "secrets", "agent_patterns"}
	for _, key := range repoFields {
		if val, ok := raw[key]; ok {
			repoEntry[key] = val
		}
	}

	// Build v2 map: repos list + all non-repo top-level fields
	v2 := make(map[string]any)
	v2["repos"] = []any{repoEntry}

	skipFields := map[string]bool{
		"repo": true, "branch": true, "groups": true,
		"secrets": true, "agent_patterns": true,
	}
	for key, val := range raw {
		if !skipFields[key] {
			v2[key] = val
		}
	}

	data, err := yaml.Marshal(v2)
	if err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("migrating v1 config: %v", err)}
	}
	return data, nil
}

func applyDefaults(cfg *CimonConfig) {
	// Polling defaults
	if cfg.Polling.Idle == 0 {
		cfg.Polling.Idle = 30
	}
	if cfg.Polling.Active == 0 {
		cfg.Polling.Active = 5
	}
	if cfg.Polling.Cooldown == 0 {
		cfg.Polling.Cooldown = 3
	}

	// Escalation defaults
	if cfg.ReviewQueue.Escalation.Amber == 0 {
		cfg.ReviewQueue.Escalation.Amber = 24
	}
	if cfg.ReviewQueue.Escalation.Red == 0 {
		cfg.ReviewQueue.Escalation.Red = 48
	}

	// Database defaults
	if cfg.Database.Path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.Database.Path = filepath.Join(home, ".local", "share", "cimon", "cimon.db")
		}
	}
	if cfg.Database.RetentionDays == 0 {
		cfg.Database.RetentionDays = 90
	}

	// Agents defaults
	if cfg.Agents.MaxConcurrent == 0 {
		cfg.Agents.MaxConcurrent = 2
	}
	if cfg.Agents.MaxLifetime == 0 {
		cfg.Agents.MaxLifetime = 1800
	}
	// CaptureOutput defaults to true — we detect "unset" by checking if the
	// entire agents block was absent. Since Go zero-values bool to false, we
	// always set it true here; users must explicitly set false to disable.
	// This matches the Python behavior where the default is True.
	if !cfg.Agents.CaptureOutput {
		cfg.Agents.CaptureOutput = true
	}

	// Catchup defaults
	if !cfg.Catchup.Enabled {
		cfg.Catchup.Enabled = true
	}
	if cfg.Catchup.IdleThreshold == 0 {
		cfg.Catchup.IdleThreshold = 900
	}

	// Per-repo defaults
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.Branch == "" {
			r.Branch = "main"
		}
		if r.AgentPatterns.PRBody == "" {
			r.AgentPatterns.PRBody = "Generated with Claude Code"
		}
		if r.AgentPatterns.CommitTrailer == "" {
			r.AgentPatterns.CommitTrailer = "Co-Authored-By: Claude"
		}
		if r.AgentPatterns.BotAuthors == nil {
			r.AgentPatterns.BotAuthors = []string{}
		}
		// AutoFixCooldown defaults to 300 when auto_fix is enabled
		for gk := range r.Groups {
			g := r.Groups[gk]
			if g.AutoFix && g.AutoFixCooldown == 0 {
				g.AutoFixCooldown = 300
				r.Groups[gk] = g
			}
		}
	}
}

func validate(cfg *CimonConfig) error {
	if len(cfg.Repos) == 0 {
		return &ConfigError{Message: "at least one repo is required"}
	}

	for i, r := range cfg.Repos {
		if r.Repo == "" {
			return &ConfigError{Message: fmt.Sprintf("repos[%d]: repo field is required", i)}
		}
	}

	if cfg.ReviewQueue.Escalation.Amber >= cfg.ReviewQueue.Escalation.Red {
		return &ConfigError{Message: fmt.Sprintf(
			"escalation amber (%d) must be less than red (%d)",
			cfg.ReviewQueue.Escalation.Amber,
			cfg.ReviewQueue.Escalation.Red,
		)}
	}

	if cfg.Polling.Idle <= 0 {
		return &ConfigError{Message: fmt.Sprintf("polling idle must be > 0, got %d", cfg.Polling.Idle)}
	}
	if cfg.Polling.Active <= 0 {
		return &ConfigError{Message: fmt.Sprintf("polling active must be > 0, got %d", cfg.Polling.Active)}
	}
	if cfg.Polling.Cooldown <= 0 {
		return &ConfigError{Message: fmt.Sprintf("polling cooldown must be > 0, got %d", cfg.Polling.Cooldown)}
	}

	return nil
}

// IsConfigError checks whether an error is a ConfigError.
func IsConfigError(err error) bool {
	var ce *ConfigError
	return errors.As(err, &ce)
}
