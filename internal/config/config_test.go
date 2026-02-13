package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if cfg.Workers != expectedWorkers {
		t.Errorf("expected workers %d, got %d", expectedWorkers, cfg.Workers)
	}
	if cfg.Sync.Workers != 0 {
		t.Errorf("expected deprecated sync workers 0, got %d", cfg.Sync.Workers)
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
	if cfg.Workers != 8 {
		t.Errorf("expected workers 8 (promoted from sync.workers), got %d", cfg.Workers)
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
	if cfg.Workers != 16 {
		t.Errorf("expected workers 16 (promoted from KATAZUKE_SYNC_WORKERS), got %d", cfg.Workers)
	}
}

func TestInvalidSyncStrategy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(
		"sync:\n  strategy: yolo\n",
	), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid strategy, got nil")
	}
	if !strings.Contains(err.Error(), "invalid sync strategy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInvalidSyncStrategyFromEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_SYNC_STRATEGY", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid strategy from env, got nil")
	}
	if !strings.Contains(err.Error(), "invalid sync strategy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTopLevelWorkersFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(
		"workers: 12\n",
	), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 12 {
		t.Errorf("expected workers 12, got %d", cfg.Workers)
	}
}

func TestWorkersBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Old-style config: only sync.workers set.
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(
		"sync:\n  workers: 10\n",
	), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 10 {
		t.Errorf("expected workers 10 (promoted from sync.workers), got %d", cfg.Workers)
	}
}

func TestWorkersPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Both set: top-level should win.
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(
		"workers: 6\nsync:\n  workers: 10\n",
	), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 6 {
		t.Errorf("expected workers 6 (top-level wins), got %d", cfg.Workers)
	}
}

func TestWorkersPrecedenceEdgeCase(t *testing.T) {
	// Known limitation: when top-level workers is explicitly set to the
	// default value, migration from sync.workers still fires because we
	// cannot distinguish "absent" from "set to default" after YAML parsing.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	defaultWorkers := Defaults().Workers

	configDir := filepath.Join(dir, "katazuke")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := fmt.Sprintf("workers: %d\nsync:\n  workers: 10\n", defaultWorkers)
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// sync.workers wins because we can't detect that top-level was explicitly
	// set when its value matches the default.
	if cfg.Workers != 10 {
		t.Errorf("expected workers 10 (sync.workers promoted due to default-matching edge case), got %d", cfg.Workers)
	}
}

func TestWorkersEnvVar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_WORKERS", "20")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 20 {
		t.Errorf("expected workers 20, got %d", cfg.Workers)
	}
}

func TestWorkersEnvVarPrecedence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_SYNC_WORKERS", "8")
	t.Setenv("KATAZUKE_WORKERS", "12")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 12 {
		t.Errorf("expected workers 12 (KATAZUKE_WORKERS wins), got %d", cfg.Workers)
	}
}

func TestSyncWorkersEnvFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KATAZUKE_SYNC_WORKERS", "8")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 8 {
		t.Errorf("expected workers 8 (from KATAZUKE_SYNC_WORKERS fallback), got %d", cfg.Workers)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := ExpandHome("~/projects")
	want := filepath.Join(home, "projects")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}

	// Non-tilde paths should be unchanged.
	got = ExpandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expected /absolute/path, got %s", got)
	}
}
