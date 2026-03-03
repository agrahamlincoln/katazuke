package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/audit"
	"github.com/agrahamlincoln/katazuke/internal/branches"
	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/internal/metrics"
	"github.com/agrahamlincoln/katazuke/internal/oplog"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
)

// AuditCmd handles workspace auditing.
type AuditCmd struct {
	NonGit bool `name:"non-git" help:"Show only non-git directories."`
}

// Run executes the audit command.
func (c *AuditCmd) Run(globals *CLI) error {
	if c.NonGit {
		return c.runNonGit(globals)
	}

	return c.runDashboard(globals)
}

func (c *AuditCmd) runDashboard(globals *CLI) error {
	if globals.Verbose {
		enableVerboseLogging()
	}

	ml := metrics.NewOrNil()
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	_ = ml.LogCommand("audit", flags)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	projectsDir := resolveProjectsDir(globals.ProjectsDir, cfg)

	slog.Debug("scanning for repositories", "dir", projectsDir)

	repos, err := scanner.Scan(projectsDir, scanner.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	})
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	fmt.Printf("Auditing %s (%d repos)...\n", projectsDir, len(repos))

	workers := cfg.Workers
	staleDays := cfg.StaleThresholdDays

	// Run three analysis sections concurrently.
	var healthResults []audit.RepoHealth
	var branchResult audit.BranchSummary
	var nonGitDirs []audit.NonRepoDir
	var healthErr, branchErr, nonGitErr error

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		healthResults = audit.AnalyzeRepoHealth(repos, workers)
	}()

	go func() {
		defer wg.Done()
		branchResult, branchErr = analyzeBranches(repos, staleDays, workers)
	}()

	go func() {
		defer wg.Done()
		nonGitDirs, nonGitErr = audit.FindNonRepoDirs(projectsDir, audit.Options{
			ExcludePatterns: cfg.ExcludePatterns,
		}, workers)
	}()

	wg.Wait()

	if healthErr != nil {
		return fmt.Errorf("analyzing repo health: %w", healthErr)
	}
	if branchErr != nil {
		return fmt.Errorf("analyzing branches: %w", branchErr)
	}
	if nonGitErr != nil {
		return fmt.Errorf("scanning non-git dirs: %w", nonGitErr)
	}

	result := audit.DashboardResult{
		ProjectsDir: projectsDir,
		RepoCount:   len(repos),
		RepoHealth:  audit.SummarizeHealth(healthResults),
		Branches:    branchResult,
		NonGitDirs:  nonGitDirs,
		StaleDays:   staleDays,
	}

	printDashboard(result)
	return nil
}

func analyzeBranches(repos []string, staleDays, workers int) (audit.BranchSummary, error) {
	detector := merge.GitOnlyDetector()

	merged, err := branches.FindMerged(repos, detector, workers, nil)
	if err != nil {
		return audit.BranchSummary{}, fmt.Errorf("finding merged branches: %w", err)
	}

	threshold := time.Duration(staleDays) * 24 * time.Hour
	stale, err := branches.FindStale(repos, threshold, detector, workers, nil)
	if err != nil {
		return audit.BranchSummary{}, fmt.Errorf("finding stale branches: %w", err)
	}

	mergedRepos := make(map[string]bool)
	for _, m := range merged {
		mergedRepos[m.RepoPath] = true
	}

	staleRepos := make(map[string]bool)
	for _, s := range stale {
		staleRepos[s.RepoPath] = true
	}

	return audit.BranchSummary{
		MergedBranches: len(merged),
		MergedRepos:    len(mergedRepos),
		StaleBranches:  len(stale),
		StaleRepos:     len(staleRepos),
	}, nil
}

func printDashboard(r audit.DashboardResult) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	dim := color.New(color.FgHiBlack)

	h := r.RepoHealth
	actionable := 0

	// Repository Health section.
	fmt.Printf("\n%s\n", bold.Sprint("Repository Health:"))
	fmt.Printf("  %s %3d clean and up-to-date\n",
		green.Sprint("ok"), h.CleanUpToDate)
	if h.BehindRemote > 0 {
		fmt.Printf("  %s %3d behind remote         %s\n",
			yellow.Sprint("!!"), h.BehindRemote, dim.Sprint("(run: katazuke sync)"))
		actionable++
	}
	if h.UncommittedChanges > 0 {
		fmt.Printf("  %s %3d uncommitted changes\n",
			yellow.Sprint("!!"), h.UncommittedChanges)
		actionable++
	}
	if h.OnNonDefaultBranch > 0 {
		fmt.Printf("  %s %3d on non-default branch\n",
			yellow.Sprint("!!"), h.OnNonDefaultBranch)
		actionable++
	}

	// Branch Cleanup section.
	b := r.Branches
	if b.MergedBranches > 0 || b.StaleBranches > 0 {
		fmt.Printf("\n%s\n", bold.Sprint("Branch Cleanup:"))
		if b.MergedBranches > 0 {
			fmt.Printf("  %s %3d merged branches across %d repos   %s\n",
				yellow.Sprint("!!"), b.MergedBranches, b.MergedRepos,
				dim.Sprint("(run: katazuke branches --merged)"))
			actionable++
		}
		if b.StaleBranches > 0 {
			fmt.Printf("  %s %3d stale branches across %d repos    %s\n",
				yellow.Sprint("!!"), b.StaleBranches, b.StaleRepos,
				dim.Sprintf("(run: katazuke branches --stale --stale-days=%d)", r.StaleDays))
			actionable++
		}
	}

	// Non-Git Directories section.
	if len(r.NonGitDirs) > 0 {
		fmt.Printf("\n%s\n", bold.Sprintf("Non-Git Directories (%d found):", len(r.NonGitDirs)))
		for _, d := range r.NonGitDirs {
			fmt.Printf("  %-20s %8s  %d files\n",
				d.Name, formatSize(d.Size), d.FileCount)
		}
		fmt.Printf("  %s\n", dim.Sprint("(run: katazuke audit --non-git for details)"))
		actionable++
	}

	// Summary line.
	fmt.Println()
	if actionable > 0 {
		noun := "items"
		if actionable == 1 {
			noun = "item"
		}
		fmt.Printf("%s\n", bold.Sprintf("%d actionable %s found.", actionable, noun))
	} else {
		fmt.Printf("%s\n", green.Sprint("Workspace is clean."))
	}
}

func (c *AuditCmd) runNonGit(globals *CLI) error {
	if globals.Verbose {
		enableVerboseLogging()
	}

	ml := metrics.NewOrNil()
	defer func() { _ = ml.Close() }()
	ol := oplog.NewOrNil()
	defer func() { _ = ol.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	_ = ml.LogCommand("audit --non-git", flags)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	projectsDir := resolveProjectsDir(globals.ProjectsDir, cfg)

	fmt.Printf("Scanning %s for non-repository directories...\n", projectsDir)

	scanStart := time.Now()
	dirs, err := audit.FindNonRepoDirs(projectsDir, audit.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	}, cfg.Workers)
	if err != nil {
		return fmt.Errorf("scanning for non-repo directories: %w", err)
	}
	_ = ml.LogPerf(0, int(time.Since(scanStart).Milliseconds()))

	if len(dirs) == 0 {
		fmt.Println("No non-repository directories found.")
		return nil
	}

	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	fmt.Printf("\n%s\n\n", bold.Sprintf("Found %d non-repository directory(ies):", len(dirs)))

	for _, d := range dirs {
		fmt.Printf("  %s\n", bold.Sprint(d.Name))
		fmt.Printf("    Path:     %s\n", d.Path)
		fmt.Printf("    Size:     %s\n", formatSize(d.Size))
		fmt.Printf("    Modified: %s\n", dim.Sprint(formatAge(d.LastModified)))
		fmt.Printf("    Files:    %d (%s)\n", d.FileCount, d.Summary)
		fmt.Println()
	}

	if globals.DryRun {
		fmt.Println(bold.Sprint("Dry run -- no changes made."))
		return nil
	}

	return promptNonGitActions(dirs, ml, ol)
}

const (
	actionKeep   = "keep"
	actionRemove = "remove"
	actionMove   = "move"
)

func promptNonGitActions(dirs []audit.NonRepoDir, ml *metrics.Logger, ol *oplog.Logger) error {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)

	quarantineDir, err := defaultQuarantinePath()
	if err != nil {
		return fmt.Errorf("resolving quarantine path: %w", err)
	}

	type dirAction struct {
		dir    audit.NonRepoDir
		action string
	}

	var actions []dirAction

	for _, d := range dirs {
		var action string
		label := fmt.Sprintf("%s (%s, %d files)", d.Name, formatSize(d.Size), d.FileCount)

		err := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title(label).
					Options(
						huh.NewOption("Keep (do nothing)", actionKeep),
						huh.NewOption("Remove (delete permanently)", actionRemove),
						huh.NewOption("Move to quarantine", actionMove),
					).
					Value(&action),
			),
		).Run()
		if err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}

		actions = append(actions, dirAction{dir: d, action: action})

		accepted := action == actionRemove || action == actionMove
		fp := metrics.Fingerprint(d.Path)
		_ = ml.LogSuggestion("remove_non_git_dir", fp, accepted, 0)
	}

	// Execute actions.
	var removed, moved, kept int
	for _, a := range actions {
		switch a.action {
		case actionKeep:
			kept++
		case actionRemove:
			fmt.Printf("Removing %s...\n", a.dir.Path)
			if err := os.RemoveAll(a.dir.Path); err != nil {
				fmt.Printf("  %s\n", red.Sprintf("Failed to remove %s: %v", a.dir.Path, err))
				continue
			}
			_ = ol.Log(oplog.Operation{
				Type:      oplog.OpDeleteDir,
				Path:      a.dir.Path,
				SizeBytes: a.dir.Size,
			})
			fmt.Printf("  %s\n", green.Sprintf("Removed %s", a.dir.Path))
			removed++
		case actionMove:
			dest := filepath.Join(quarantineDir, a.dir.Name)
			fmt.Printf("Moving %s to %s...\n", a.dir.Path, dest)
			if err := moveToQuarantine(a.dir.Path, dest); err != nil {
				fmt.Printf("  %s\n", red.Sprintf("Failed to move %s: %v", a.dir.Path, err))
				continue
			}
			_ = ol.Log(oplog.Operation{
				Type:        oplog.OpMoveDir,
				Path:        a.dir.Path,
				Destination: dest,
				SizeBytes:   a.dir.Size,
			})
			fmt.Printf("  %s\n", yellow.Sprintf("Moved to %s", dest))
			moved++
		}
	}

	fmt.Println()
	if removed > 0 {
		fmt.Println(bold.Sprintf("Removed %d directory(ies).", removed))
	}
	if moved > 0 {
		fmt.Println(bold.Sprintf("Moved %d directory(ies) to %s.", moved, quarantineDir))
	}
	if kept > 0 {
		fmt.Println(bold.Sprintf("Kept %d directory(ies).", kept))
	}

	return nil
}

func moveToQuarantine(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return fmt.Errorf("creating quarantine directory: %w", err)
	}
	return os.Rename(src, dest)
}

func defaultQuarantinePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "katazuke-quarantine"), nil
}

// formatSize formats bytes into a human-readable string.
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
