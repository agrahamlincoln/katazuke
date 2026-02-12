package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/metrics"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
	"github.com/agrahamlincoln/katazuke/internal/sync"
)

// SyncCmd handles repository synchronization.
type SyncCmd struct {
	Pattern string `name:"pattern" short:"f" help:"Filter repositories by name pattern (glob)." default:""`
}

// Run executes the sync command.
func (c *SyncCmd) Run(globals *CLI) error {
	if globals.Verbose {
		enableVerboseLogging()
	}

	ml := metrics.NewOrNil()
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	if c.Pattern != "" {
		flags = append(flags, fmt.Sprintf("--pattern=%s", c.Pattern))
	}
	_ = ml.LogCommand("sync", flags)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	projectsDir := globals.ProjectsDir
	if projectsDir == "" || projectsDir == "~/projects" {
		projectsDir = cfg.ProjectsDir
	} else {
		projectsDir = expandHome(projectsDir)
	}

	fmt.Printf("Scanning %s for repositories...\n", projectsDir)

	repoPaths, err := scanner.Scan(projectsDir, scanner.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	})
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	if len(repoPaths) == 0 {
		fmt.Println("No repositories found.")
		return nil
	}

	if c.Pattern != "" {
		repoPaths = filterByPattern(repoPaths, c.Pattern)
		if len(repoPaths) == 0 {
			fmt.Printf("No repositories matching %q found.\n", c.Pattern)
			return nil
		}
	}

	slog.Debug("found repositories", "count", len(repoPaths))

	opts := sync.Options{
		Strategy:           cfg.Sync.Strategy,
		SkipDirty:          cfg.Sync.SkipDirty,
		AutoStash:          cfg.Sync.AutoStash,
		SwitchMergedBranch: cfg.Sync.SwitchMergedBranch,
		DryRun:             globals.DryRun,
		Verbose:            globals.Verbose,
	}

	workers := cfg.Sync.Workers
	fmt.Printf("Syncing %d repositories (%d workers)...\n\n", len(repoPaths), workers)

	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)
	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	var synced, skipped, failed, switched int
	syncStart := time.Now()

	sync.All(repoPaths, opts, sync.RealGitOps{}, workers, func(completed, total int, r sync.Result) {
		remaining := total - completed

		// Clear the status line, print result, redraw status.
		fmt.Print("\r\033[2K")
		switch r.Status {
		case sync.Synced:
			synced++
			fmt.Printf("  %s %s\n", green.Sprint("[synced]"), r.RepoName)
		case sync.Switched:
			switched++
			fmt.Printf("  %s %s: %s\n", green.Sprint("[switched]"), r.RepoName, r.Message)
		case sync.Skipped:
			skipped++
			fmt.Printf("  %s %s: %s\n", yellow.Sprint("[skip]"), r.RepoName, r.Message)
		case sync.Failed:
			failed++
			fmt.Printf("  %s %s: %s\n", red.Sprint("[fail]"), r.RepoName, r.Message)
		}

		if remaining > 0 {
			fmt.Printf("\r\033[2K  %s %d remaining...",
				dim.Sprintf("[%d/%d]", completed, total),
				remaining)
		}
	})

	_ = ml.LogPerf(len(repoPaths), int(time.Since(syncStart).Milliseconds()))

	// Clear final status line.
	fmt.Print("\r\033[2K")
	fmt.Println()
	summary := fmt.Sprintf("Synced %d, switched %d, skipped %d, failed %d", synced, switched, skipped, failed)
	if globals.DryRun {
		summary += " (dry run)"
	}
	fmt.Println(bold.Sprint(summary))
	return nil
}

// filterByPattern filters repository paths by matching the base name against
// a glob pattern.
func filterByPattern(repos []string, pattern string) []string {
	var filtered []string
	for _, repoPath := range repos {
		name := filepath.Base(repoPath)
		if matched, _ := filepath.Match(pattern, name); matched {
			filtered = append(filtered, repoPath)
		}
	}
	return filtered
}
