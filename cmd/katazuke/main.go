// Package main provides the katazuke CLI tool for workspace maintenance.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/huh"
	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/branches"
	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/metrics"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// CLI defines the top-level command structure for katazuke.
type CLI struct {
	DryRun      bool   `name:"dry-run" short:"n" help:"Show what would be done without making changes."`
	Verbose     bool   `name:"verbose" short:"v" help:"Verbose output."`
	ProjectsDir string `name:"projects-dir" short:"p" help:"Projects directory." default:"~/projects" env:"KATAZUKE_PROJECTS_DIR"`

	Branches BranchesCmd `cmd:"" help:"Manage branches across repositories."`
	Repos    ReposCmd    `cmd:"" help:"Manage repository checkouts."`
	Audit    AuditCmd    `cmd:"" help:"Run full workspace audit."`
	Sync     SyncCmd     `cmd:"" help:"Sync all repositories."`
	Version  VersionCmd  `cmd:"" help:"Show version information."`
}

// BranchesCmd handles branch management across repositories.
type BranchesCmd struct {
	Merged    bool `help:"Show only merged branches." xor:"mode"`
	Stale     bool `help:"Show only stale branches." xor:"mode"`
	StaleDays int  `name:"stale-days" help:"Days before a branch is considered stale." default:"30"`
}

// Run executes the branches command.
func (c *BranchesCmd) Run(globals *CLI) error {
	if !c.Merged && !c.Stale {
		return fmt.Errorf("specify --merged or --stale")
	}

	if c.Merged {
		return c.runMerged(globals)
	}

	return c.runStale(globals)
}

func (c *BranchesCmd) runMerged(globals *CLI) error {
	if globals.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Metrics are best-effort local telemetry for improving katazuke.
	// Logging errors are intentionally discarded because metrics must never
	// interrupt the user's workflow or cause a command to fail.
	ml := metrics.NewOrNil()
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	_ = ml.LogCommand("branches --merged", flags)

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

	slog.Debug("scanning for repositories", "dir", projectsDir)

	scanStart := time.Now()
	repos, err := scanner.Scan(projectsDir, scanner.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	})
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	slog.Debug("found repositories", "count", len(repos))

	merged, err := branches.FindMerged(repos, cfg.Sync.Workers)
	if err != nil {
		return fmt.Errorf("finding merged branches: %w", err)
	}
	_ = ml.LogPerf(len(repos), int(time.Since(scanStart).Milliseconds()))

	if len(merged) == 0 {
		fmt.Println("No merged branches found.")
		return nil
	}

	printMergedSummary(merged)

	if globals.DryRun {
		return nil
	}

	selected, err := promptForDeletion(merged)
	if err != nil {
		return err
	}

	// Log suggestion events for each merged branch.
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s.RepoPath+":"+s.Branch] = true
	}
	for _, m := range merged {
		accepted := selectedSet[m.RepoPath+":"+m.Branch]
		fp := branchFingerprint(m.RepoPath, m.Branch)
		ageDays := int(time.Since(m.LastCommit).Hours() / 24)
		_ = ml.LogSuggestion("delete_merged_branch", fp, accepted, ageDays)
	}

	if len(selected) == 0 {
		fmt.Println("No branches selected for deletion.")
		return nil
	}

	deleteRemote, err := promptForRemoteDeletion(selected)
	if err != nil {
		return err
	}

	return deleteSelectedBranches(selected, deleteRemote)
}

func printMergedSummary(merged []branches.MergedBranch) {
	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	fmt.Printf("\n%s\n\n", bold.Sprintf("Found %d merged branch(es):", len(merged)))

	currentRepo := ""
	for _, m := range merged {
		if m.RepoName != currentRepo {
			currentRepo = m.RepoName
			fmt.Printf("  %s\n", bold.Sprint(m.RepoName))
		}
		age := formatAge(m.LastCommit)
		fmt.Printf("    %s  %s\n", m.Branch, dim.Sprintf("(%s)", age))
	}
	fmt.Println()
}

func promptForDeletion(merged []branches.MergedBranch) ([]branches.MergedBranch, error) {
	options := make([]huh.Option[int], len(merged))
	for i, m := range merged {
		options[i] = huh.NewOption(m.Label(), i)
	}

	var selectedIndices []int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select branches to delete").
				Options(options...).
				Value(&selectedIndices),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	selected := make([]branches.MergedBranch, len(selectedIndices))
	for i, idx := range selectedIndices {
		selected[i] = merged[idx]
	}
	return selected, nil
}

// promptForRemoteDeletion asks the user whether to also delete remote branches,
// but only if any of the selected branches have a remote counterpart.
func promptForRemoteDeletion(selected []branches.MergedBranch) (bool, error) {
	hasAnyRemote := false
	for _, m := range selected {
		if m.HasRemote {
			hasAnyRemote = true
			break
		}
	}
	if !hasAnyRemote {
		return false, nil
	}

	var deleteRemote bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Also delete remote branches on origin?").
				Value(&deleteRemote),
		),
	)
	if err := form.Run(); err != nil {
		return false, fmt.Errorf("prompt failed: %w", err)
	}
	return deleteRemote, nil
}

func deleteSelectedBranches(selected []branches.MergedBranch, deleteRemote bool) error {
	var failed []string
	var remoteFailed []string

	for _, m := range selected {
		slog.Debug("deleting branch", "repo", m.RepoName, "branch", m.Branch)
		if err := git.DeleteLocalBranch(m.RepoPath, m.Branch, false); err != nil {
			fmt.Printf("  failed to delete %s in %s: %v\n", m.Branch, m.RepoName, err)
			failed = append(failed, m.Label())
			continue
		}
		fmt.Printf("  deleted %s in %s\n", m.Branch, m.RepoName)

		if deleteRemote && m.HasRemote {
			if err := git.DeleteRemoteBranch(m.RepoPath, "origin", m.Branch); err != nil {
				fmt.Printf("  failed to delete remote %s in %s: %v\n", m.Branch, m.RepoName, err)
				remoteFailed = append(remoteFailed, m.Label())
				continue
			}
			fmt.Printf("  deleted remote %s in %s\n", m.Branch, m.RepoName)
		}
	}

	fmt.Println()
	bold := color.New(color.Bold)
	deleted := len(selected) - len(failed)
	if deleted > 0 {
		fmt.Println(bold.Sprintf("Deleted %d local branch(es).", deleted))
	}
	if deleteRemote {
		remoteCount := 0
		for _, m := range selected {
			if m.HasRemote {
				remoteCount++
			}
		}
		remoteDeleted := remoteCount - len(remoteFailed)
		if remoteDeleted > 0 {
			fmt.Println(bold.Sprintf("Deleted %d remote branch(es).", remoteDeleted))
		}
	}

	failed = append(failed, remoteFailed...)
	if len(failed) > 0 {
		return fmt.Errorf("failed to delete %d branch(es): %s",
			len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func (c *BranchesCmd) runStale(globals *CLI) error {
	if globals.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Metrics logging errors are discarded; see comment in runMerged.
	ml := metrics.NewOrNil()
	defer func() { _ = ml.Close() }()

	var flags []string
	if globals.DryRun {
		flags = append(flags, "--dry-run")
	}
	if globals.Verbose {
		flags = append(flags, "--verbose")
	}
	flags = append(flags, fmt.Sprintf("--stale-days=%d", c.StaleDays))
	_ = ml.LogCommand("branches --stale", flags)

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

	staleDays := c.StaleDays
	if staleDays <= 0 {
		staleDays = cfg.StaleThresholdDays
	}

	slog.Debug("scanning for repositories", "dir", projectsDir)

	scanStart := time.Now()
	repos, err := scanner.Scan(projectsDir, scanner.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	})
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	slog.Debug("found repositories", "count", len(repos))

	threshold := time.Duration(staleDays) * 24 * time.Hour
	stale, err := branches.FindStale(repos, threshold, cfg.Sync.Workers)
	if err != nil {
		return fmt.Errorf("finding stale branches: %w", err)
	}
	_ = ml.LogPerf(len(repos), int(time.Since(scanStart).Milliseconds()))

	if len(stale) == 0 {
		fmt.Println("No stale branches found.")
		return nil
	}

	printStaleSummary(stale)

	if globals.DryRun {
		return nil
	}

	return promptAndExecuteStaleActions(stale, ml)
}

func printStaleSummary(stale []branches.StaleBranch) {
	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	fmt.Printf("\n%s\n\n", bold.Sprintf("Found %d stale branch(es):", len(stale)))

	currentRepo := ""
	for _, s := range stale {
		if s.RepoName != currentRepo {
			currentRepo = s.RepoName
			fmt.Printf("  %s\n", bold.Sprint(s.RepoName))
		}
		age := formatAge(s.LastCommit)
		subject := truncate(s.LastCommitMessage, maxCommitSummaryLen)
		remote := ""
		if s.HasRemote {
			remote = " [remote]"
		}
		fmt.Printf("    %s  %s  %s  +%d/-%d%s\n",
			s.Branch,
			dim.Sprintf("(%s)", age),
			dim.Sprint(subject),
			s.CommitsAhead, s.CommitsBehind,
			dim.Sprint(remote),
		)
	}
	fmt.Println()
}

// Maximum characters for commit message display in different contexts.
const (
	maxCommitSummaryLen     = 50
	maxCommitDescriptionLen = 40
)

// staleAction represents a user-selected action for a stale branch.
type staleAction string

const (
	staleActionDelete  staleAction = "delete"
	staleActionKeep    staleAction = "keep"
	staleActionArchive staleAction = "archive"
)

func promptAndExecuteStaleActions(stale []branches.StaleBranch, ml *metrics.Logger) error {
	actions := make(map[int]staleAction, len(stale))

	for i, s := range stale {
		var action staleAction
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[staleAction]().
					Title(s.Label()).
					Description(fmt.Sprintf("%s (%s, +%d/-%d)",
						truncate(s.LastCommitMessage, maxCommitDescriptionLen),
						formatAge(s.LastCommit),
						s.CommitsAhead, s.CommitsBehind)).
					Options(
						huh.NewOption("Delete", staleActionDelete),
						huh.NewOption("Keep", staleActionKeep),
						huh.NewOption("Archive (tag + delete)", staleActionArchive),
					).
					Value(&action),
			),
		)
		if err := form.Run(); err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}
		actions[i] = action

		// Log the suggestion: accepted means delete or archive (actionable).
		fp := branchFingerprint(s.RepoPath, s.Branch)
		ageDays := int(time.Since(s.LastCommit).Hours() / 24)
		accepted := action == staleActionDelete || action == staleActionArchive
		_ = ml.LogSuggestion("delete_stale_branch", fp, accepted, ageDays)
	}

	return executeStaleActions(stale, actions)
}

func executeStaleActions(stale []branches.StaleBranch, actions map[int]staleAction) error {
	bold := color.New(color.Bold)
	var failures []string
	deleted, archived, kept := 0, 0, 0

	for i, s := range stale {
		action := actions[i]
		switch action {
		case staleActionKeep:
			kept++
			continue

		case staleActionArchive:
			tagName := "archive/" + s.Branch
			if err := git.CreateTag(s.RepoPath, tagName, s.Branch); err != nil {
				fmt.Printf("  failed to create tag %s in %s: %v\n", tagName, s.RepoName, err)
				failures = append(failures, s.Label())
				continue
			}
			fmt.Printf("  created tag %s in %s\n", tagName, s.RepoName)

			if err := git.DeleteLocalBranch(s.RepoPath, s.Branch, true); err != nil {
				fmt.Printf("  failed to delete local %s in %s: %v\n", s.Branch, s.RepoName, err)
				failures = append(failures, s.Label())
				continue
			}
			fmt.Printf("  deleted local %s in %s\n", s.Branch, s.RepoName)

			if s.HasRemote {
				if err := git.DeleteRemoteBranch(s.RepoPath, "origin", s.Branch); err != nil {
					fmt.Printf("  failed to delete remote %s in %s: %v\n", s.Branch, s.RepoName, err)
					failures = append(failures, s.Label())
					continue
				}
				fmt.Printf("  deleted remote %s in %s\n", s.Branch, s.RepoName)
			}
			archived++

		case staleActionDelete:
			if err := git.DeleteLocalBranch(s.RepoPath, s.Branch, true); err != nil {
				fmt.Printf("  failed to delete %s in %s: %v\n", s.Branch, s.RepoName, err)
				failures = append(failures, s.Label())
				continue
			}
			fmt.Printf("  deleted %s in %s\n", s.Branch, s.RepoName)

			if s.HasRemote {
				if err := git.DeleteRemoteBranch(s.RepoPath, "origin", s.Branch); err != nil {
					fmt.Printf("  failed to delete remote %s in %s: %v\n", s.Branch, s.RepoName, err)
					failures = append(failures, s.Label())
					continue
				}
				fmt.Printf("  deleted remote %s in %s\n", s.Branch, s.RepoName)
			}
			deleted++
		}
	}

	fmt.Println()
	if deleted > 0 {
		fmt.Println(bold.Sprintf("Deleted %d branch(es).", deleted))
	}
	if archived > 0 {
		fmt.Println(bold.Sprintf("Archived %d branch(es).", archived))
	}
	if kept > 0 {
		fmt.Println(bold.Sprintf("Kept %d branch(es).", kept))
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed to process %d branch(es): %s",
			len(failures), strings.Join(failures, ", "))
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "unknown date"
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "1 day ago"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	case days < 365:
		months := days / 30
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := days / 365
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// branchFingerprint returns a stable fingerprint for a branch using the
// repo's remote URL when available, falling back to the repo path.
func branchFingerprint(repoPath, branch string) string {
	remote, err := git.RemoteURL(repoPath, "origin")
	if err != nil || remote == "" {
		remote = repoPath
	}
	return metrics.Fingerprint(remote, branch)
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

// VersionCmd shows version information.
type VersionCmd struct{}

// Run executes the version command.
func (c *VersionCmd) Run() error {
	fmt.Printf("katazuke %s (commit: %s, built: %s)\n", version, commit, date)
	return nil
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("katazuke"),
		kong.Description(`katazuke (片付け) - "tidying up"

A developer workspace maintenance tool that helps you keep your ~/projects
directory clean and organized by managing stale branches, archived repositories,
and out-of-date checkouts.`),
		kong.UsageOnError(),
		kong.Vars{"version": fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)},
	)
	err := ctx.Run(&cli)
	ctx.FatalIfErrorf(err)
	// Explicitly exit with 0 on success so tests can verify exit behavior.
	os.Exit(0)
}
