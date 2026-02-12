package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/github"
	"github.com/agrahamlincoln/katazuke/internal/repos"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
)

// ReposCmd handles repository checkout management.
type ReposCmd struct {
	Archived bool `help:"Show only archived repositories."`
	Backup   bool `help:"Create backup before deletion." default:"true"`
}

// Run executes the repos command.
func (c *ReposCmd) Run(globals *CLI) error {
	if !c.Archived {
		fmt.Println("Analyzing repositories...")
		fmt.Println("(Use --archived to find archived GitHub repositories)")
		return nil
	}

	return c.runArchived(globals)
}

func (c *ReposCmd) runArchived(globals *CLI) error {
	if globals.Verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	projectsDir := globals.ProjectsDir
	if projectsDir == "~/projects" {
		projectsDir = cfg.ProjectsDir
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

	slog.Debug("found repositories", "count", len(repoPaths))

	ghClient := github.NewClient(cfg.GithubToken)

	fmt.Printf("Checking archive status of %d repositories...\n", len(repoPaths))

	archived, err := repos.FindArchived(repoPaths, ghClient)
	if err != nil {
		return fmt.Errorf("checking archive status: %w", err)
	}

	if len(archived) == 0 {
		fmt.Println("No archived repositories found.")
		return nil
	}

	bold := color.New(color.Bold)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	green := color.New(color.FgGreen)

	fmt.Printf("\n%s\n\n", bold.Sprintf("Found %d archived repositories:", len(archived)))

	for _, r := range archived {
		fmt.Printf("  %s/%s\n", r.Owner, r.Repo)
		fmt.Printf("    Path: %s\n", r.Path)
		if r.IsClean {
			fmt.Printf("    %s\n", green.Sprint("Status: clean working tree"))
		} else {
			fmt.Printf("    %s\n", yellow.Sprint("Status: uncommitted changes (will not be removed)"))
		}
		fmt.Println()
	}

	if globals.DryRun {
		fmt.Println(bold.Sprint("Dry run -- no changes made."))
		return nil
	}

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

	// Build selection options.
	options := make([]huh.Option[string], len(removable))
	for i, r := range removable {
		label := fmt.Sprintf("%s/%s (%s)", r.Owner, r.Repo, r.Path)
		options[i] = huh.NewOption(label, r.Path)
	}

	var selected []string
	err = huh.NewForm(
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

	if len(selected) == 0 {
		fmt.Println("No repositories selected.")
		return nil
	}

	// Build a set of selected paths for quick lookup.
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
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
