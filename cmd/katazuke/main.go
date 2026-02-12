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

	// --stale not yet implemented
	fmt.Println("Stale branch detection not yet implemented.")
	return nil
}

func (c *BranchesCmd) runMerged(globals *CLI) error {
	if globals.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

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

	if len(selected) == 0 {
		fmt.Println("No branches selected for deletion.")
		return nil
	}

	return deleteSelectedBranches(selected)
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

func deleteSelectedBranches(selected []branches.MergedBranch) error {
	var failed []string

	for _, m := range selected {
		slog.Debug("deleting branch", "repo", m.RepoName, "branch", m.Branch)
		if err := git.DeleteLocalBranch(m.RepoPath, m.Branch, false); err != nil {
			fmt.Printf("  failed to delete %s in %s: %v\n", m.Branch, m.RepoName, err)
			failed = append(failed, m.Label())
			continue
		}
		fmt.Printf("  deleted %s in %s\n", m.Branch, m.RepoName)
	}

	fmt.Println()
	deleted := len(selected) - len(failed)
	if deleted > 0 {
		fmt.Println(color.New(color.Bold).Sprintf("Deleted %d branch(es).", deleted))
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed to delete %d branch(es): %s",
			len(failed), strings.Join(failed, ", "))
	}
	return nil
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
