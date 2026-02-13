// Package main provides the katazuke CLI tool for workspace maintenance.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/huh"
	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/branches"
	"github.com/agrahamlincoln/katazuke/internal/config"
	ghclient "github.com/agrahamlincoln/katazuke/internal/github"
	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/internal/metrics"
	"github.com/agrahamlincoln/katazuke/internal/parallel"
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
	ProjectsDir string `name:"projects-dir" short:"p" help:"Projects directory (default: from config file, or ~/projects)." default:"" env:"KATAZUKE_PROJECTS_DIR"`

	Branches BranchesCmd `cmd:"" help:"Manage branches across repositories."`
	Repos    ReposCmd    `cmd:"" help:"Manage repository checkouts."`
	Audit    AuditCmd    `cmd:"" help:"Run full workspace audit."`
	Sync     SyncCmd     `cmd:"" help:"Sync all repositories."`
	Version  VersionCmd  `cmd:"" help:"Show version information."`
}

// BranchesCmd handles branch management across repositories.
type BranchesCmd struct {
	Merged    bool `help:"Filter to only merged branches."`
	Stale     bool `help:"Filter to only stale branches."`
	StaleDays int  `name:"stale-days" help:"Days before a branch is considered stale (only applies to stale filtering)." default:"30"`
}

// Run executes the branches command.
// When neither --merged nor --stale is specified, both are shown.
func (c *BranchesCmd) Run(globals *CLI) error {
	showBoth := !c.Merged && !c.Stale

	if c.Merged || showBoth {
		if err := c.runMerged(globals); err != nil {
			return err
		}
	}

	if c.Stale || showBoth {
		if err := c.runStale(globals); err != nil {
			return err
		}
	}

	return nil
}

func (c *BranchesCmd) runMerged(globals *CLI) error {
	if globals.Verbose {
		enableVerboseLogging()
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

	projectsDir := resolveProjectsDir(globals.ProjectsDir, cfg)

	slog.Debug("scanning for repositories", "dir", projectsDir)

	scanStart := time.Now()
	repos, err := scanner.Scan(projectsDir, scanner.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	})
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	slog.Debug("found repositories", "count", len(repos))

	workers := cfg.Workers
	slog.Debug("using worker pool", "workers", workers)
	fmt.Printf("Scanning %d repositories for merged branches...\n", len(repos))

	gh := ghclient.NewClient(cfg.GithubToken)
	detector := merge.NewDetector(merge.RealGitChecker{}, gh)
	merged, err := branches.FindMerged(repos, detector, workers, progressPrinter())
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

// mergedSummaryThreshold is the number of branches above which the
// merged summary shows per-repo counts instead of individual branches.
const mergedSummaryThreshold = 25

func printMergedSummary(merged []branches.MergedBranch) {
	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	fmt.Printf("\n%s\n\n", bold.Sprintf("Found %d merged branch(es):", len(merged)))

	if len(merged) > mergedSummaryThreshold {
		counts := make(map[string]int)
		var order []string
		for _, m := range merged {
			if counts[m.RepoName] == 0 {
				order = append(order, m.RepoName)
			}
			counts[m.RepoName]++
		}
		for _, repo := range order {
			noun := "branches"
			if counts[repo] == 1 {
				noun = "branch"
			}
			fmt.Printf("  %s  %s\n", bold.Sprint(repo), dim.Sprintf("(%d %s)", counts[repo], noun))
		}
	} else {
		currentRepo := ""
		for _, m := range merged {
			if m.RepoName != currentRepo {
				currentRepo = m.RepoName
				fmt.Printf("  %s\n", bold.Sprint(m.RepoName))
			}
			age := formatAge(m.LastCommit)
			fmt.Printf("    %s  %s\n", m.Branch, dim.Sprintf("(%s)", age))
		}
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
				Height(15).
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

// branchToDelete holds the common fields needed to delete any branch,
// regardless of whether it came from the merged or stale workflow.
type branchToDelete struct {
	repoPath  string
	repoName  string
	branch    string
	hasRemote bool
	// canDeleteRemote is false for automation branches and branches
	// with other contributors, preventing remote deletion even when
	// the user opts in.
	canDeleteRemote bool
	// forceLocal controls whether git branch -D (force) is used instead
	// of -d. Required for squash-merged branches that git does not
	// recognize as merged, and for stale branches.
	forceLocal bool
}

// deleteBranches deletes branches locally and optionally their remote
// counterparts. Each branch's forceLocal field controls whether
// git branch -D (force) is used for that specific branch.
func deleteBranches(toDelete []branchToDelete, deleteRemote bool) error {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)
	dim := color.New(color.FgHiBlack)

	var localFailed []string
	var remoteFailed []string
	total := len(toDelete)

	for i, b := range toDelete {
		completed := i + 1
		remaining := total - completed
		label := fmt.Sprintf("%s: %s", b.repoName, b.branch)

		fmt.Print(clearLine)

		slog.Debug("deleting branch", "repo", b.repoName, "branch", b.branch)
		if err := git.DeleteLocalBranch(b.repoPath, b.branch, b.forceLocal); err != nil {
			fmt.Printf("  %s %s: %s (%v)\n", red.Sprint("[fail]"), b.repoName, b.branch, err)
			localFailed = append(localFailed, label)
			if remaining > 0 {
				fmt.Printf("%s  %s %d remaining...", clearLine, dim.Sprintf("[%d/%d]", completed, total), remaining)
			}
			continue
		}
		fmt.Printf("  %s %s: %s\n", green.Sprint("[deleted]"), b.repoName, b.branch)

		if deleteRemote && b.hasRemote && b.canDeleteRemote {
			if err := git.DeleteRemoteBranch(b.repoPath, "origin", b.branch); err != nil {
				if isRemoteRefNotFound(err) {
					fmt.Printf("  %s %s: %s (remote already deleted)\n", yellow.Sprint("[skip]"), b.repoName, b.branch)
				} else {
					fmt.Printf("  %s %s: %s remote (%v)\n", red.Sprint("[fail]"), b.repoName, b.branch, err)
					remoteFailed = append(remoteFailed, label)
				}
			} else {
				fmt.Printf("  %s %s: %s (remote)\n", green.Sprint("[deleted]"), b.repoName, b.branch)
			}
		}

		if remaining > 0 {
			fmt.Printf("%s  %s %d remaining...", clearLine, dim.Sprintf("[%d/%d]", completed, total), remaining)
		}
	}

	fmt.Print(clearLine)

	fmt.Println()
	deleted := len(toDelete) - len(localFailed)
	if deleted > 0 {
		fmt.Println(bold.Sprintf("Deleted %d branch(es).", deleted))
	}
	if deleteRemote {
		remoteCount := 0
		for _, b := range toDelete {
			if b.hasRemote && b.canDeleteRemote {
				remoteCount++
			}
		}
		remoteDeleted := remoteCount - len(remoteFailed)
		if remoteDeleted > 0 {
			fmt.Println(bold.Sprintf("Deleted %d remote branch(es).", remoteDeleted))
		}
	}

	var errParts []string
	if len(localFailed) > 0 {
		errParts = append(errParts, fmt.Sprintf("failed to delete %d local branch(es): %s",
			len(localFailed), strings.Join(localFailed, ", ")))
	}
	if len(remoteFailed) > 0 {
		errParts = append(errParts, fmt.Sprintf("failed to delete %d remote branch(es): %s",
			len(remoteFailed), strings.Join(remoteFailed, ", ")))
	}
	if len(errParts) > 0 {
		return fmt.Errorf("%s", strings.Join(errParts, "; "))
	}
	return nil
}

func deleteSelectedBranches(selected []branches.MergedBranch, deleteRemote bool) error {
	toDelete := make([]branchToDelete, len(selected))
	for i, m := range selected {
		toDelete[i] = branchToDelete{
			repoPath:        m.RepoPath,
			repoName:        m.RepoName,
			branch:          m.Branch,
			hasRemote:       m.HasRemote,
			canDeleteRemote: true,
			forceLocal:      m.ForceDelete,
		}
	}
	return deleteBranches(toDelete, deleteRemote)
}

func (c *BranchesCmd) runStale(globals *CLI) error {
	if globals.Verbose {
		enableVerboseLogging()
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

	projectsDir := resolveProjectsDir(globals.ProjectsDir, cfg)

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

	workers := cfg.Workers
	slog.Debug("using worker pool", "workers", workers)
	fmt.Printf("Scanning %d repositories for stale branches...\n", len(repos))

	gh := ghclient.NewClient(cfg.GithubToken)
	detector := merge.NewDetector(merge.RealGitChecker{}, gh)

	threshold := time.Duration(staleDays) * 24 * time.Hour
	stale, err := branches.FindStale(repos, threshold, detector, workers, progressPrinter())
	if err != nil {
		return fmt.Errorf("finding stale branches: %w", err)
	}
	_ = ml.LogPerf(len(repos), int(time.Since(scanStart).Milliseconds()))

	// Filter out branches with open PRs using GitHub API.
	stale = filterByPRStatus(stale, gh, workers)

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

// prCheckResult pairs a stale branch with the outcome of its PR status check.
type prCheckResult struct {
	branch  branches.StaleBranch
	exclude bool
}

// filterByPRStatus uses the GitHub API to exclude branches with open PRs
// from the stale list. Branches whose PRs were merged are kept as cleanup
// candidates. API failures are logged but do not prevent the branch from
// appearing in results (fail-open).
func filterByPRStatus(stale []branches.StaleBranch, gh *ghclient.Client, workers int) []branches.StaleBranch {
	slog.Debug("checking PR status for stale branches", "count", len(stale))

	dim := color.New(color.FgHiBlack)
	fmt.Printf("Checking PR status for %d branches...\n", len(stale))

	results := parallel.Run(stale, workers, func(s branches.StaleBranch) prCheckResult {
		if !s.HasRemote {
			return prCheckResult{branch: s}
		}

		remote, err := git.RemoteURL(s.RepoPath, "origin")
		if err != nil {
			return prCheckResult{branch: s}
		}

		owner, repo, ok := ghclient.ParseGitHubRemote(remote)
		if !ok {
			return prCheckResult{branch: s}
		}

		info, err := gh.BranchPRInfo(owner, repo, s.Branch)
		if err != nil {
			slog.Debug("could not check PR status, keeping branch in results",
				"repo", s.RepoName, "branch", s.Branch, "error", err)
			return prCheckResult{branch: s}
		}

		if info.State == ghclient.PRStateOpen {
			slog.Debug("excluding branch with open PR",
				"repo", s.RepoName, "branch", s.Branch)
			return prCheckResult{branch: s, exclude: true}
		}

		if info.State == ghclient.PRStateMerged {
			// Verify local branch tip matches the PR's head SHA
			// to prevent false positives from reused branch names.
			localSHA, shaErr := git.RevParse(s.RepoPath, s.Branch)
			if shaErr == nil && localSHA == info.HeadSHA {
				s.PRNumber = info.Number
				s.PRMergedAt = info.MergedAt
			}
		}

		return prCheckResult{branch: s}
	}, func(completed, total int, _ prCheckResult) {
		remaining := total - completed
		if remaining > 0 {
			fmt.Printf("%s  %s", clearLine, dim.Sprintf("[%d/%d]", completed, total))
		} else {
			fmt.Print(clearLine)
		}
	})

	filtered := make([]branches.StaleBranch, 0, len(stale))
	for _, r := range results {
		if !r.exclude {
			filtered = append(filtered, r.branch)
		}
	}
	return filtered
}

func printStaleSummary(stale []branches.StaleBranch) {
	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)
	yellow := color.New(color.FgYellow)

	fmt.Printf("\n%s\n\n", bold.Sprintf("Found %d stale branch(es):", len(stale)))

	currentRepo := ""
	for _, s := range stale {
		if s.RepoName != currentRepo {
			currentRepo = s.RepoName
			fmt.Printf("  %s\n", bold.Sprint(s.RepoName))
		}

		scope := "local only"
		if s.HasRemote {
			scope = "local + remote"
		}

		age := formatAge(s.LastCommit)
		subject := truncate(s.LastCommitMessage, maxCommitSummaryLen)

		// Highlight local-only branches with commits ahead to warn about data loss.
		aheadStr := fmt.Sprintf("+%d", s.CommitsAhead)
		if s.IsLocalOnly && s.CommitsAhead > 0 {
			aheadStr = yellow.Sprintf("+%d", s.CommitsAhead)
		}

		fmt.Printf("    %s (%s)  %s  %s  %s/-%d\n",
			s.Branch,
			scope,
			dim.Sprintf("last commit %s", age),
			dim.Sprint(subject),
			aheadStr, s.CommitsBehind,
		)
	}
	fmt.Println()
}

// maxCommitSummaryLen is the maximum characters for commit messages in the
// stale branch summary view.
const maxCommitSummaryLen = 50

// clearLine is the ANSI escape sequence to move the cursor to the start
// of the line and erase its contents.
const clearLine = "\r\033[2K"

// progressPrinter returns a callback that displays an inline progress
// counter. The line is cleared when all items complete.
func progressPrinter() func(completed, total int) {
	dim := color.New(color.FgHiBlack)
	return func(completed, total int) {
		remaining := total - completed
		if remaining > 0 {
			fmt.Printf("%s  %s %d remaining...",
				clearLine, dim.Sprintf("[%d/%d]", completed, total), remaining)
		} else {
			fmt.Print(clearLine)
		}
	}
}

// promptAndExecuteStaleActions categorizes stale branches into safety tiers,
// presents a multi-select per tier, and deletes the selected branches.
func promptAndExecuteStaleActions(stale []branches.StaleBranch, ml *metrics.Logger) error {
	safe, automation, review := categorizeStaleBranches(stale)

	tiers := []struct {
		title       string
		description string
		branches    []branches.StaleBranch
		preselect   bool
	}{
		{
			"Safe to delete",
			"Your branches that also exist on the remote. No work will be lost.",
			safe, true,
		},
		{
			"Automation branches",
			"Created by tools like Dependabot or Renovate. The remote tool manages these.",
			automation, true,
		},
		{
			"Needs review",
			"Local-only or other-author branches. Check before deleting -- work may not exist elsewhere.",
			review, false,
		},
	}

	var selected []branches.StaleBranch
	for _, tier := range tiers {
		if len(tier.branches) == 0 {
			continue
		}
		tierSelected, err := promptTierSelection(tier.title, tier.description, tier.branches, tier.preselect)
		if err != nil {
			return err
		}
		selected = append(selected, tierSelected...)
	}

	// Log metrics for all branches.
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s.RepoPath+":"+s.Branch] = true
	}
	for _, s := range stale {
		fp := branchFingerprint(s.RepoPath, s.Branch)
		ageDays := int(time.Since(s.LastCommit).Hours() / 24)
		_ = ml.LogSuggestion("delete_stale_branch", fp, selectedSet[s.RepoPath+":"+s.Branch], ageDays)
	}

	if len(selected) == 0 {
		fmt.Println("No branches selected for deletion.")
		return nil
	}

	deleteRemote, err := promptForStaleRemoteDeletion(selected)
	if err != nil {
		return err
	}

	return executeStaleDeletes(selected, deleteRemote)
}

// categorizeStaleBranches groups branches into safety tiers for the
// multi-select UI. Automation branches are always in their own tier
// regardless of other properties. Own branches with remotes are "safe"
// because the work exists elsewhere. Everything else (local-only,
// other-author) needs manual review.
func categorizeStaleBranches(stale []branches.StaleBranch) (safe, automation, review []branches.StaleBranch) {
	for _, s := range stale {
		switch {
		case s.IsAutomation:
			automation = append(automation, s)
		case s.HasRemote && s.IsOwnBranch:
			safe = append(safe, s)
		default:
			review = append(review, s)
		}
	}
	return
}

// promptTierSelection presents a multi-select for a single tier of stale
// branches. Returns the branches the user selected for deletion.
func promptTierSelection(title, description string, tier []branches.StaleBranch, preselect bool) ([]branches.StaleBranch, error) {
	options := make([]huh.Option[int], len(tier))
	for i, s := range tier {
		options[i] = huh.NewOption(staleBranchLabel(s), i).Selected(preselect)
	}

	var selectedIndices []int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title(title).
				Description(description).
				Options(options...).
				Height(15).
				Value(&selectedIndices),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	result := make([]branches.StaleBranch, len(selectedIndices))
	for i, idx := range selectedIndices {
		result[i] = tier[idx]
	}
	return result, nil
}

// staleBranchLabel builds a display label for a stale branch option including
// scope, age, commit subject, commit delta, and PR merge info.
func staleBranchLabel(s branches.StaleBranch) string {
	scope := "local only"
	if s.HasRemote {
		scope = "local + remote"
	}

	age := formatAge(s.LastCommit)
	subject := truncate(s.LastCommitMessage, maxCommitSummaryLen)

	label := fmt.Sprintf("%s: %s (%s) - last commit %s", s.RepoName, s.Branch, scope, age)
	if subject != "" {
		label += fmt.Sprintf(" - \"%s\"", subject)
	}
	label += fmt.Sprintf(" +%d/-%d", s.CommitsAhead, s.CommitsBehind)

	if s.PRNumber > 0 {
		if !s.PRMergedAt.IsZero() {
			label += fmt.Sprintf(" [merged PR #%d on %s]", s.PRNumber, s.PRMergedAt.Format("Jan 2, 2006"))
		} else {
			label += fmt.Sprintf(" [merged PR #%d]", s.PRNumber)
		}
	}

	return label
}

// promptForStaleRemoteDeletion asks whether to also delete remote branches
// when any of the selected stale branches have a remote that is safe to delete.
func promptForStaleRemoteDeletion(selected []branches.StaleBranch) (bool, error) {
	hasRemote := false
	for _, s := range selected {
		if s.HasRemote && safeToDeleteRemote(s) {
			hasRemote = true
			break
		}
	}
	if !hasRemote {
		return false, nil
	}

	var deleteRemote bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Also delete remote branches on origin?").
				Description("Only your own branches will be deleted remotely. Automation and other-author branches are skipped.").
				Value(&deleteRemote),
		),
	)
	if err := form.Run(); err != nil {
		return false, fmt.Errorf("prompt failed: %w", err)
	}
	return deleteRemote, nil
}

// isRemoteRefNotFound returns true if the error indicates the remote
// branch has already been deleted. This matches against git's error
// message text, which could vary across locales or git versions.
func isRemoteRefNotFound(err error) bool {
	return strings.Contains(err.Error(), "remote ref does not exist")
}

// safeToDeleteRemote returns true if the branch can safely have its remote
// deleted. Automation branches and branches with other contributors should
// never have their remotes deleted by this tool.
func safeToDeleteRemote(s branches.StaleBranch) bool {
	return !s.IsAutomation && s.IsOwnBranch
}

// executeStaleDeletes deletes the selected stale branches locally, and
// optionally their remote counterparts where safe.
func executeStaleDeletes(selected []branches.StaleBranch, deleteRemote bool) error {
	toDelete := make([]branchToDelete, len(selected))
	for i, s := range selected {
		toDelete[i] = branchToDelete{
			repoPath:        s.RepoPath,
			repoName:        s.RepoName,
			branch:          s.Branch,
			hasRemote:       s.HasRemote,
			canDeleteRemote: safeToDeleteRemote(s),
			forceLocal:      true,
		}
	}
	return deleteBranches(toDelete, deleteRemote)
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

// enableVerboseLogging configures the default slog logger to emit debug-level
// messages to stderr.
func enableVerboseLogging() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

// resolveProjectsDir returns the projects directory from the CLI flag if
// provided, otherwise from the loaded config (which has defaults applied).
func resolveProjectsDir(cliValue string, cfg config.Config) string {
	if cliValue != "" {
		return config.ExpandHome(cliValue)
	}
	return cfg.ProjectsDir
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
