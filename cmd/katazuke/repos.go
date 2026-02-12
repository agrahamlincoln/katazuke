package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/github"
	"github.com/agrahamlincoln/katazuke/internal/metrics"
	"github.com/agrahamlincoln/katazuke/internal/repos"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// ReposCmd handles repository checkout management.
type ReposCmd struct {
	Archived bool `help:"Show only archived repositories." xor:"mode"`
	Merged   bool `help:"Show only repos on merged branches." xor:"mode"`
}

// Run executes the repos command.
func (c *ReposCmd) Run(globals *CLI) error {
	if globals.Verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	if c.Archived {
		return c.runArchived(globals)
	}
	if c.Merged {
		return c.runMerged(globals)
	}

	// No flags: show summary + all issue types.
	return c.runAll(globals)
}

func (c *ReposCmd) loadRepos(globals *CLI) ([]string, *config.Config, *metrics.Logger, error) {
	ml := metrics.NewOrNil()

	cfg, err := config.Load()
	if err != nil {
		_ = ml.Close()
		return nil, nil, nil, fmt.Errorf("loading config: %w", err)
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
		_ = ml.Close()
		return nil, nil, nil, fmt.Errorf("scanning repositories: %w", err)
	}

	if len(repoPaths) == 0 {
		fmt.Println("No repositories found.")
		_ = ml.Close()
		return nil, nil, nil, nil
	}

	slog.Debug("found repositories", "count", len(repoPaths))
	return repoPaths, &cfg, ml, nil
}

func (c *ReposCmd) runAll(globals *CLI) error {
	repoPaths, cfg, ml, err := c.loadRepos(globals)
	if err != nil {
		return err
	}
	if repoPaths == nil {
		return nil
	}
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	_ = ml.LogCommand("repos", flags)

	bold := color.New(color.Bold)
	workers := cfg.Sync.Workers
	slog.Debug("using worker pool", "workers", workers)

	scanStart := time.Now()

	// Repository summary.
	fmt.Printf("Summarizing %d repositories...\n", len(repoPaths))
	summary := repos.Summarize(repoPaths, workers, progressPrinter())
	fmt.Printf("\n%s\n", bold.Sprint("Repository Summary"))
	fmt.Printf("  Total: %d\n", summary.Total)
	fmt.Printf("  Clean: %d\n", summary.Clean)
	fmt.Printf("  Dirty: %d\n", summary.Dirty)
	fmt.Println()

	// Find merged branch repos.
	fmt.Printf("Checking for repos on merged branches...\n")
	mergedRepos := repos.FindOnMergedBranch(repoPaths, workers, progressPrinter())

	// Find archived repos.
	ghClient := github.NewClient(cfg.GithubToken)
	fmt.Printf("Checking archive status...\n")
	archived := repos.FindArchived(repoPaths, ghClient, workers, progressPrinter())

	_ = ml.LogPerf(len(repoPaths), int(time.Since(scanStart).Milliseconds()))

	hasIssues := false

	if len(mergedRepos) > 0 {
		hasIssues = true
		printMergedRepos(mergedRepos)
		if !globals.DryRun {
			if err := promptMergedRepoActions(mergedRepos, ml); err != nil {
				return err
			}
		}
	}

	if len(archived) > 0 {
		hasIssues = true
		printArchivedRepos(archived)
		if !globals.DryRun {
			if err := promptArchivedRepoActions(archived, ml); err != nil {
				return err
			}
		}
	}

	if !hasIssues {
		fmt.Println("No issues found. All repositories look good.")
	}

	if globals.DryRun && hasIssues {
		fmt.Println(bold.Sprint("Dry run -- no changes made."))
	}

	return nil
}

func (c *ReposCmd) runMerged(globals *CLI) error {
	repoPaths, cfg, ml, err := c.loadRepos(globals)
	if err != nil {
		return err
	}
	if repoPaths == nil {
		return nil
	}
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	_ = ml.LogCommand("repos --merged", flags)

	workers := cfg.Sync.Workers
	slog.Debug("using worker pool", "workers", workers)
	fmt.Printf("Checking %d repositories for merged branches...\n", len(repoPaths))

	scanStart := time.Now()
	mergedRepos := repos.FindOnMergedBranch(repoPaths, workers, progressPrinter())
	_ = ml.LogPerf(len(repoPaths), int(time.Since(scanStart).Milliseconds()))

	if len(mergedRepos) == 0 {
		fmt.Println("No repositories are on merged branches.")
		return nil
	}

	printMergedRepos(mergedRepos)

	if globals.DryRun {
		bold := color.New(color.Bold)
		fmt.Println(bold.Sprint("Dry run -- no changes made."))
		return nil
	}

	return promptMergedRepoActions(mergedRepos, ml)
}

func (c *ReposCmd) runArchived(globals *CLI) error {
	repoPaths, cfg, ml, err := c.loadRepos(globals)
	if err != nil {
		return err
	}
	if repoPaths == nil {
		return nil
	}
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	_ = ml.LogCommand("repos --archived", flags)

	workers := cfg.Sync.Workers
	slog.Debug("using worker pool", "workers", workers)

	scanStart := time.Now()
	ghClient := github.NewClient(cfg.GithubToken)

	fmt.Printf("Checking archive status of %d repositories...\n", len(repoPaths))

	archived := repos.FindArchived(repoPaths, ghClient, workers, progressPrinter())
	_ = ml.LogPerf(len(repoPaths), int(time.Since(scanStart).Milliseconds()))

	if len(archived) == 0 {
		fmt.Println("No archived repositories found.")
		return nil
	}

	printArchivedRepos(archived)

	if globals.DryRun {
		bold := color.New(color.Bold)
		fmt.Println(bold.Sprint("Dry run -- no changes made."))
		return nil
	}

	return promptArchivedRepoActions(archived, ml)
}

func printMergedRepos(mergedRepos []repos.MergedBranchRepo) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	fmt.Printf("%s\n\n", bold.Sprintf("Found %d repo(s) on merged branches:", len(mergedRepos)))

	for _, r := range mergedRepos {
		fmt.Printf("  %s\n", bold.Sprint(r.Name))
		fmt.Printf("    Branch: %s (merged into %s)\n", r.CurrentBranch, r.DefaultBranch)
		if r.IsClean {
			fmt.Printf("    %s\n", green.Sprint("Status: clean (safe to switch)"))
		} else {
			fmt.Printf("    %s\n", yellow.Sprint("Status: dirty working tree"))
		}
	}
	fmt.Println()
}

func promptMergedRepoActions(mergedRepos []repos.MergedBranchRepo, ml *metrics.Logger) error {
	// Filter to only switchable repos (clean working tree).
	var switchable []repos.MergedBranchRepo
	for _, r := range mergedRepos {
		if r.IsClean {
			switchable = append(switchable, r)
		}
	}

	if len(switchable) == 0 {
		return nil
	}

	options := make([]huh.Option[string], len(switchable))
	for i, r := range switchable {
		label := fmt.Sprintf("%s: %s -> %s", r.Name, r.CurrentBranch, r.DefaultBranch)
		options[i] = huh.NewOption(label, r.Path)
	}

	var selected []string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select repos to switch to default branch").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return fmt.Errorf("selection prompt: %w", err)
	}

	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
	}

	// Log suggestions.
	for _, r := range switchable {
		accepted := selectedSet[r.Path]
		fp := repoFingerprint(r.Path)
		_ = ml.LogSuggestion("switch_merged_branch_repo", fp, accepted, 0)
	}

	if len(selected) == 0 {
		fmt.Println("No repositories selected.")
		return nil
	}

	// Ask whether to also delete the old branch.
	var deleteBranch bool
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Also delete the merged branch after switching?").
				Value(&deleteBranch),
		),
	).Run()
	if err != nil {
		return fmt.Errorf("prompt failed: %w", err)
	}

	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	switched := 0

	for _, r := range switchable {
		if !selectedSet[r.Path] {
			continue
		}

		slog.Debug("switching to default branch", "repo", r.Name, "from", r.CurrentBranch, "to", r.DefaultBranch)
		if err := git.Checkout(r.Path, r.DefaultBranch); err != nil {
			fmt.Printf("  %s\n", red.Sprintf("Failed to switch %s: %v", r.Name, err))
			continue
		}
		fmt.Printf("  %s\n", green.Sprintf("Switched %s to %s", r.Name, r.DefaultBranch))
		switched++

		if deleteBranch {
			if err := git.DeleteLocalBranch(r.Path, r.CurrentBranch, false); err != nil {
				fmt.Printf("  %s\n", red.Sprintf("Failed to delete branch %s in %s: %v", r.CurrentBranch, r.Name, err))
			} else {
				fmt.Printf("  %s\n", green.Sprintf("Deleted branch %s in %s", r.CurrentBranch, r.Name))
			}
		}
	}

	fmt.Printf("\n%s\n", bold.Sprintf("Switched %d repo(s) to default branch.", switched))
	return nil
}

func printArchivedRepos(archived []repos.ArchivedRepo) {
	bold := color.New(color.Bold)
	yellow := color.New(color.FgYellow)
	green := color.New(color.FgGreen)

	fmt.Printf("%s\n\n", bold.Sprintf("Found %d archived repo(s):", len(archived)))

	for _, r := range archived {
		fmt.Printf("  %s/%s\n", r.Owner, r.Repo)
		fmt.Printf("    Path: %s\n", r.Path)
		if r.IsClean {
			fmt.Printf("    %s\n", green.Sprint("Status: clean working tree"))
		} else {
			fmt.Printf("    %s\n", yellow.Sprint("Status: uncommitted changes (will not be removed)"))
		}
	}
	fmt.Println()
}

func promptArchivedRepoActions(archived []repos.ArchivedRepo, ml *metrics.Logger) error {
	red := color.New(color.FgRed)
	green := color.New(color.FgGreen)
	bold := color.New(color.Bold)

	// Filter to only removable repos (clean working tree).
	var removable []repos.ArchivedRepo
	for _, r := range archived {
		if r.IsClean {
			removable = append(removable, r)
		}
	}

	if len(removable) == 0 {
		fmt.Println(red.Sprint("No archived repositories can be removed (all have uncommitted changes)."))
		return nil
	}

	options := make([]huh.Option[string], len(removable))
	for i, r := range removable {
		label := fmt.Sprintf("%s/%s (%s)", r.Owner, r.Repo, r.Path)
		options[i] = huh.NewOption(label, r.Path)
	}

	var selected []string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select archived repositories to remove").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return fmt.Errorf("selection prompt: %w", err)
	}

	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
	}
	for _, r := range removable {
		accepted := selectedSet[r.Path]
		fp := repoFingerprint(r.Path)
		_ = ml.LogSuggestion("delete_archived_repo", fp, accepted, 0)
	}

	if len(selected) == 0 {
		fmt.Println("No repositories selected.")
		return nil
	}

	removed := 0
	for _, r := range removable {
		if !selectedSet[r.Path] {
			continue
		}

		fmt.Printf("Removing %s/%s at %s...\n", r.Owner, r.Repo, r.Path)
		if err := os.RemoveAll(r.Path); err != nil {
			fmt.Printf("  %s\n", red.Sprintf("Failed to remove %s: %v", r.Path, err))
			continue
		}
		fmt.Printf("  %s\n", green.Sprintf("Removed %s", r.Path))
		removed++
	}

	fmt.Printf("\n%s\n", bold.Sprintf("Removed %d archived repositories.", removed))
	return nil
}

// repoFingerprint returns a stable fingerprint for a repository using
// its remote URL when available, falling back to the repo path.
func repoFingerprint(repoPath string) string {
	remote, err := git.RemoteURL(repoPath, "origin")
	if err != nil || remote == "" {
		remote = repoPath
	}
	return metrics.Fingerprint(remote)
}
