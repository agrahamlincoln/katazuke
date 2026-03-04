package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/agrahamlincoln/katazuke/pkg/git"
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

	repos, isLocal, err := resolveRepos(globals, cfg)
	if err != nil {
		return err
	}

	// projectsDir is only needed for workspace-wide operations.
	var projectsDir string
	if isLocal {
		fmt.Printf("Auditing 1 repository...\n")
	} else {
		projectsDir = resolveProjectsDir(globals.ProjectsDir, cfg)
		fmt.Printf("Auditing %s (%d repos)...\n", projectsDir, len(repos))
	}

	workers := cfg.Workers
	staleDays := cfg.StaleThresholdDays

	// Run analysis sections concurrently. Non-git dir scanning is skipped
	// in local mode because it is inherently workspace-scoped.
	var healthResults []audit.RepoHealth
	var branchResult audit.BranchSummary
	var nonGitDirs []audit.NonRepoDir
	var healthErr, branchErr, nonGitErr error

	var wg sync.WaitGroup

	wg.Go(func() {
		healthResults = audit.AnalyzeRepoHealth(repos, workers)
	})

	wg.Go(func() {
		branchResult, branchErr = analyzeBranches(repos, staleDays, workers)
	})

	if !isLocal {
		wg.Go(func() {
			nonGitDirs, nonGitErr = audit.FindNonRepoDirs(projectsDir, audit.Options{
				ExcludePatterns: cfg.ExcludePatterns,
			}, workers)
		})
	}

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
		ProjectsDir:   projectsDir,
		RepoCount:     len(repos),
		RepoHealth:    audit.SummarizeHealth(healthResults),
		HealthDetails: healthResults,
		Branches:      branchResult,
		NonGitDirs:    nonGitDirs,
		StaleDays:     staleDays,
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

	mergedByRepo := make(map[string]int)
	for _, m := range merged {
		mergedByRepo[m.RepoName]++
	}

	staleByRepo := make(map[string]int)
	for _, s := range stale {
		staleByRepo[s.RepoName]++
	}

	return audit.BranchSummary{
		MergedBranches: len(merged),
		MergedRepos:    len(mergedByRepo),
		StaleBranches:  len(stale),
		StaleRepos:     len(staleByRepo),
		MergedByRepo:   sortRepoBranchCounts(mergedByRepo),
		StaleByRepo:    sortRepoBranchCounts(staleByRepo),
	}, nil
}

func sortRepoBranchCounts(counts map[string]int) []audit.RepoBranchCount {
	result := make([]audit.RepoBranchCount, 0, len(counts))
	for name, count := range counts {
		result = append(result, audit.RepoBranchCount{RepoName: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].RepoName < result[j].RepoName
	})
	return result
}

const maxDetailLines = 5

func printDetailLines(lines []string) {
	dim := color.New(color.FgHiBlack)
	limit := min(len(lines), maxDetailLines)
	for _, line := range lines[:limit] {
		fmt.Printf("         %s\n", dim.Sprint(line))
	}
	if remaining := len(lines) - limit; remaining > 0 {
		fmt.Printf("         %s\n", dim.Sprintf("...and %d more", remaining))
	}
}

func printDashboard(r audit.DashboardResult) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)
	dim := color.New(color.FgHiBlack)

	h := r.RepoHealth
	buckets := audit.ReposByBucket(r.HealthDetails)
	actionable := 0

	// Repository Health section.
	fmt.Printf("\n%s\n", bold.Sprint("Repository Health:"))
	fmt.Printf("  %s %3d clean and up-to-date\n",
		green.Sprint("ok"), h.CleanUpToDate)
	if h.NeedsManualFix > 0 {
		fmt.Printf("  %s %3d needs manual fix\n",
			red.Sprint("!!"), h.NeedsManualFix)
		conflicted := make([]string, len(buckets.Conflicted))
		sort.Slice(buckets.Conflicted, func(i, j int) bool {
			return filepath.Base(buckets.Conflicted[i].Path) < filepath.Base(buckets.Conflicted[j].Path)
		})
		for i, cr := range buckets.Conflicted {
			conflicted[i] = fmt.Sprintf("%s (mid-%s)", filepath.Base(cr.Path), cr.ConflictState)
		}
		printDetailLines(conflicted)
		actionable++
	}
	if h.BehindRemote > 0 {
		fmt.Printf("  %s %3d behind remote         %s\n",
			yellow.Sprint("!!"), h.BehindRemote, dim.Sprint("(run: katazuke sync)"))
		behind := make([]string, len(buckets.Behind))
		sort.Slice(buckets.Behind, func(i, j int) bool {
			return buckets.Behind[i].BehindRemote > buckets.Behind[j].BehindRemote
		})
		for i, r := range buckets.Behind {
			name := filepath.Base(r.Path)
			noun := "commits"
			if r.BehindRemote == 1 {
				noun = "commit"
			}
			behind[i] = fmt.Sprintf("%s (%d %s behind)", name, r.BehindRemote, noun)
		}
		printDetailLines(behind)
		actionable++
	}
	if h.UncommittedChanges > 0 {
		fmt.Printf("  %s %3d uncommitted changes\n",
			yellow.Sprint("!!"), h.UncommittedChanges)
		dirty := make([]string, len(buckets.Dirty))
		sort.Slice(buckets.Dirty, func(i, j int) bool {
			return filepath.Base(buckets.Dirty[i].Path) < filepath.Base(buckets.Dirty[j].Path)
		})
		for i, r := range buckets.Dirty {
			name := filepath.Base(r.Path)
			if r.BehindRemote > 0 {
				noun := "commits"
				if r.BehindRemote == 1 {
					noun = "commit"
				}
				dirty[i] = fmt.Sprintf("%s (also %d %s behind)", name, r.BehindRemote, noun)
			} else {
				dirty[i] = name
			}
		}
		printDetailLines(dirty)
		actionable++
	}
	if h.OnNonDefaultBranch > 0 {
		fmt.Printf("  %s %3d on non-default branch\n",
			yellow.Sprint("!!"), h.OnNonDefaultBranch)
		nonDefault := make([]string, len(buckets.NonDefault))
		sort.Slice(buckets.NonDefault, func(i, j int) bool {
			return filepath.Base(buckets.NonDefault[i].Path) < filepath.Base(buckets.NonDefault[j].Path)
		})
		for i, r := range buckets.NonDefault {
			name := filepath.Base(r.Path)
			switch {
			case r.CurrentBranch == "":
				nonDefault[i] = fmt.Sprintf("%s (detached HEAD)", name)
			case r.IsMergedBranch:
				nonDefault[i] = fmt.Sprintf("%s (%s, merged)", name, r.CurrentBranch)
			default:
				nonDefault[i] = fmt.Sprintf("%s (%s)", name, r.CurrentBranch)
			}
		}
		printDetailLines(nonDefault)
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
			lines := make([]string, len(b.MergedByRepo))
			for i, rc := range b.MergedByRepo {
				noun := "branches"
				if rc.Count == 1 {
					noun = "branch"
				}
				lines[i] = fmt.Sprintf("%s (%d %s)", rc.RepoName, rc.Count, noun)
			}
			printDetailLines(lines)
			actionable++
		}
		if b.StaleBranches > 0 {
			fmt.Printf("  %s %3d stale branches across %d repos    %s\n",
				yellow.Sprint("!!"), b.StaleBranches, b.StaleRepos,
				dim.Sprintf("(run: katazuke branches --stale --stale-days=%d)", r.StaleDays))
			lines := make([]string, len(b.StaleByRepo))
			for i, rc := range b.StaleByRepo {
				noun := "branches"
				if rc.Count == 1 {
					noun = "branch"
				}
				lines[i] = fmt.Sprintf("%s (%d %s)", rc.RepoName, rc.Count, noun)
			}
			printDetailLines(lines)
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

	// --non-git is inherently workspace-scoped; it doesn't apply to a single repo.
	if !globals.Global {
		cwd, err := os.Getwd()
		if err == nil {
			if _, tlErr := git.TopLevel(cwd); tlErr == nil {
				fmt.Println("The --non-git flag requires workspace-wide mode. Use --global to scan the full projects directory.")
				return nil
			}
		}
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
