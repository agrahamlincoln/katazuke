// Package config handles loading and validating katazuke configuration
// from files, environment variables, and CLI flag overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// SyncConfig holds configuration for the sync command.
type SyncConfig struct {
	Strategy           string `yaml:"strategy"`             // "rebase", "merge", or "ff-only"
	SkipDirty          bool   `yaml:"skip_dirty"`           // skip dirty repos without merge-tree check
	AutoStash          bool   `yaml:"auto_stash"`           // attempt stash/pop for dirty repos
	SwitchMergedBranch bool   `yaml:"switch_merged_branch"` // auto-switch repos on merged branches to default
	Workers            int    `yaml:"workers"`              // number of parallel workers
}

// Config holds all katazuke configuration.
type Config struct {
	ProjectsDir        string     `yaml:"projects_dir"`
	StaleThresholdDays int        `yaml:"stale_threshold_days"`
	GithubToken        string     `yaml:"github_token"`
	ExcludePatterns    []string   `yaml:"exclude_patterns"`
	Sync               SyncConfig `yaml:"sync"`
}

// Defaults returns a Config with default values.
func Defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		ProjectsDir:        filepath.Join(home, "projects"),
		StaleThresholdDays: 30,
		ExcludePatterns:    []string{".archive", "vendor"},
		Sync: SyncConfig{
			Strategy:           "rebase",
			SkipDirty:          false,
			AutoStash:          true,
			SwitchMergedBranch: true,
			Workers:            min(4, runtime.NumCPU()),
		},
	}
}

// Load reads configuration from the config file and environment variables.
// Values are layered: defaults < config file < environment variables.
func Load() (Config, error) {
	cfg := Defaults()

	if err := loadFile(&cfg); err != nil {
		return cfg, err
	}

	applyEnv(&cfg)
	return cfg, nil
}

// configPath returns the path to the config file.
func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "katazuke", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "katazuke", "config.yaml")
}

func loadFile(cfg *Config) error {
	path := filepath.Clean(configPath())
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil // no config file is fine
	}
	if err != nil {
		return fmt.Errorf("reading config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Expand ~ in projects_dir.
	cfg.ProjectsDir = expandHome(cfg.ProjectsDir)
	return nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("KATAZUKE_PROJECTS_DIR"); v != "" {
		cfg.ProjectsDir = expandHome(v)
	}
	if v := os.Getenv("KATAZUKE_STALE_THRESHOLD_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.StaleThresholdDays = days
		}
	}
	if v := os.Getenv("KATAZUKE_GITHUB_TOKEN"); v != "" {
		cfg.GithubToken = v
	}
	if v := os.Getenv("GITHUB_TOKEN"); v != "" && cfg.GithubToken == "" {
		cfg.GithubToken = v
	}
	if v := os.Getenv("GH_TOKEN"); v != "" && cfg.GithubToken == "" {
		cfg.GithubToken = v
	}
	if v := os.Getenv("KATAZUKE_SYNC_STRATEGY"); v != "" {
		cfg.Sync.Strategy = v
	}
	if v := os.Getenv("KATAZUKE_SYNC_SKIP_DIRTY"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Sync.SkipDirty = b
		}
	}
	if v := os.Getenv("KATAZUKE_SYNC_AUTO_STASH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Sync.AutoStash = b
		}
	}
	if v := os.Getenv("KATAZUKE_SYNC_SWITCH_MERGED_BRANCH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Sync.SwitchMergedBranch = b
		}
	}
	if v := os.Getenv("KATAZUKE_SYNC_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Sync.Workers = n
		}
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
