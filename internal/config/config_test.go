package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.StaleThresholdDays != 30 {
		t.Errorf("expected stale threshold 30, got %d", cfg.StaleThresholdDays)
	}
	if cfg.ProjectsDir == "" {
		t.Error("expected non-empty projects dir")
	}
	if cfg.Sync.Strategy != "rebase" {
		t.Errorf("expected sync strategy rebase, got %q", cfg.Sync.Strategy)
	}
	if cfg.Sync.SkipDirty {
		t.Error("expected sync skip_dirty to be false by default")
	}
	if !cfg.Sync.AutoStash {
		t.Error("expected sync auto_stash to be true by default")
	}
	if !cfg.Sync.SwitchMergedBranch {
		t.Error("expected sync switch_merged_branch to be true by default")
	}
	expectedWorkers := min(4, runtime.NumCPU())
	if cfg.Sync.Workers != expectedWorkers {
		t.Errorf("expected sync workers %d, got %d", expectedWorkers, cfg.Sync.Workers)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	// When no config file exists, Load should return defaults without error.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StaleThresholdDays != 30 {
		t.Errorf("expected default stale threshold, got %d", cfg.StaleThresholdDays)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(
		"projects_dir: /custom/path\nstale_threshold_days: 60\nexclude_patterns:\n  - vendor\n  - node_modules\n",
	), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectsDir != "/custom/path" {
		t.Errorf("expected /custom/path, got %s", cfg.ProjectsDir)
	}
	if cfg.StaleThresholdDays != 60 {
		t.Errorf("expected 60, got %d", cfg.StaleThresholdDays)
	}
	if len(cfg.ExcludePatterns) != 2 {
		t.Errorf("expected 2 exclude patterns, got %d", len(cfg.ExcludePatterns))
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_PROJECTS_DIR", "/env/projects")
	t.Setenv("KATAZUKE_STALE_THRESHOLD_DAYS", "90")
	t.Setenv("KATAZUKE_GITHUB_TOKEN", "ghp_test123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectsDir != "/env/projects" {
		t.Errorf("expected /env/projects, got %s", cfg.ProjectsDir)
	}
	if cfg.StaleThresholdDays != 90 {
		t.Errorf("expected 90, got %d", cfg.StaleThresholdDays)
	}
	if cfg.GithubToken != "ghp_test123" {
		t.Errorf("expected ghp_test123, got %s", cfg.GithubToken)
	}
}

func TestGithubTokenFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "from_github")
	t.Setenv("GH_TOKEN", "from_gh")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// GITHUB_TOKEN should take precedence over GH_TOKEN when KATAZUKE_ is empty.
	if cfg.GithubToken != "from_github" {
		t.Errorf("expected from_github, got %s", cfg.GithubToken)
	}
}

func TestSyncConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(
		"sync:\n  strategy: ff-only\n  skip_dirty: true\n  auto_stash: false\n  switch_merged_branch: false\n  workers: 8\n",
	), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sync.Strategy != "ff-only" {
		t.Errorf("expected ff-only, got %q", cfg.Sync.Strategy)
	}
	if !cfg.Sync.SkipDirty {
		t.Error("expected skip_dirty to be true")
	}
	if cfg.Sync.AutoStash {
		t.Error("expected auto_stash to be false")
	}
	if cfg.Sync.SwitchMergedBranch {
		t.Error("expected switch_merged_branch to be false")
	}
	if cfg.Sync.Workers != 8 {
		t.Errorf("expected workers 8, got %d", cfg.Sync.Workers)
	}
}

func TestSyncEnvOverrides(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_SYNC_STRATEGY", "merge")
	t.Setenv("KATAZUKE_SYNC_SKIP_DIRTY", "true")
	t.Setenv("KATAZUKE_SYNC_AUTO_STASH", "false")
	t.Setenv("KATAZUKE_SYNC_SWITCH_MERGED_BRANCH", "false")
	t.Setenv("KATAZUKE_SYNC_WORKERS", "16")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sync.Strategy != "merge" {
		t.Errorf("expected merge, got %q", cfg.Sync.Strategy)
	}
	if !cfg.Sync.SkipDirty {
		t.Error("expected skip_dirty to be true")
	}
	if cfg.Sync.AutoStash {
		t.Error("expected auto_stash to be false")
	}
	if cfg.Sync.SwitchMergedBranch {
		t.Error("expected switch_merged_branch to be false")
	}
	if cfg.Sync.Workers != 16 {
		t.Errorf("expected workers 16, got %d", cfg.Sync.Workers)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandHome("~/projects")
	want := filepath.Join(home, "projects")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}

	// Non-tilde paths should be unchanged.
	got = expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expected /absolute/path, got %s", got)
	}
}
