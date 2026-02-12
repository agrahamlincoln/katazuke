package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/audit"
	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/metrics"
)

// AuditCmd handles workspace auditing.
type AuditCmd struct {
	NonGit bool `name:"non-git" help:"Show only non-git directories."`
}

// Run executes the audit command.
func (c *AuditCmd) Run(globals *CLI) error {
	if !c.NonGit {
		fmt.Println("Auditing workspace...")
		fmt.Println("(Use --non-git to find non-repository directories)")
		return nil
	}

	return c.runNonGit(globals)
}

func (c *AuditCmd) runNonGit(globals *CLI) error {
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
	_ = ml.LogCommand("audit --non-git", flags)

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

	fmt.Printf("Scanning %s for non-repository directories...\n", projectsDir)

	scanStart := time.Now()
	dirs, err := audit.FindNonRepoDirs(projectsDir, audit.Options{
		ExcludePatterns: cfg.ExcludePatterns,
	}, cfg.Sync.Workers)
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

	return promptNonGitActions(dirs, ml)
}

const (
	actionKeep   = "keep"
	actionRemove = "remove"
	actionMove   = "move"
)

func promptNonGitActions(dirs []audit.NonRepoDir, ml *metrics.Logger) error {
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
			fmt.Printf("  %s\n", green.Sprintf("Removed %s", a.dir.Path))
			removed++
		case actionMove:
			dest := filepath.Join(quarantineDir, a.dir.Name)
			fmt.Printf("Moving %s to %s...\n", a.dir.Path, dest)
			if err := moveToQuarantine(a.dir.Path, dest); err != nil {
				fmt.Printf("  %s\n", red.Sprintf("Failed to move %s: %v", a.dir.Path, err))
				continue
			}
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
