package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/goccy/go-yaml"

	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// InitCmd creates a .katazuke index file interactively.
type InitCmd struct {
	Dir string `arg:"" optional:"" help:"Directory to initialize (default: projects directory)."`
}

// dirInfo holds classification data for a directory entry.
type dirInfo struct {
	Name      string
	IsRepo    bool
	RepoCount int // child repo count (0 if IsRepo itself)
}

// Run executes the init command.
func (c *InitCmd) Run(globals *CLI) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dir := resolveInitDir(c.Dir, globals.ProjectsDir, cfg)

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("init: %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("init: %s is not a directory", dir)
	}

	dim := color.New(color.FgHiBlack)

	fmt.Printf("Scanning %s...\n\n", dir)

	dirs, err := classifyChildren(dir)
	if err != nil {
		return fmt.Errorf("scanning directory: %w", err)
	}

	printDirSummary(dirs, dim)

	groupCandidates := findGroupCandidates(dirs)
	if len(groupCandidates) == 0 {
		fmt.Println("All directories are repositories -- no index file needed.")
		return nil
	}

	existing, hasExisting, err := scanner.LoadIndex(dir)
	if err != nil {
		return fmt.Errorf("loading existing index: %w", err)
	}
	if hasExisting {
		fmt.Printf("%s\n\n", dim.Sprint("Existing .katazuke found -- using as defaults."))
	}

	selectedGroups, err := promptGroups(groupCandidates, existing)
	if err != nil {
		return err
	}

	selectedIgnores, err := promptIgnores(dirs, selectedGroups, existing)
	if err != nil {
		return err
	}

	if len(selectedGroups) == 0 && len(selectedIgnores) == 0 {
		fmt.Println("No groups or ignores selected -- no index file needed.")
		return nil
	}

	yamlBytes, err := generateIndex(selectedGroups, selectedIgnores)
	if err != nil {
		return fmt.Errorf("generating index: %w", err)
	}

	previewIndex(yamlBytes, dirs, selectedGroups, selectedIgnores)

	if globals.DryRun {
		bold := color.New(color.Bold)
		fmt.Println(bold.Sprint("Dry run -- no changes made."))
		return nil
	}

	return writeIndex(dir, yamlBytes)
}

// printDirSummary displays the classified directory listing.
func printDirSummary(dirs []dirInfo, dim *color.Color) {
	maxLen := 0
	for _, d := range dirs {
		name := d.Name
		if !d.IsRepo {
			name += "/"
		}
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	fmtStr := fmt.Sprintf("  %%-%ds %%s\n", maxLen+2)
	for _, d := range dirs {
		switch {
		case d.IsRepo:
			fmt.Printf(fmtStr, d.Name, dim.Sprint("git repo"))
		case d.RepoCount > 0:
			fmt.Printf(fmtStr, d.Name+"/", dim.Sprintf("%d %s inside", d.RepoCount, repoNoun(d.RepoCount)))
		default:
			fmt.Printf(fmtStr, d.Name+"/", dim.Sprint("no repos"))
		}
	}
	fmt.Println()
}

// findGroupCandidates returns non-repo dirs that contain at least one repo.
func findGroupCandidates(dirs []dirInfo) []dirInfo {
	var candidates []dirInfo
	for _, d := range dirs {
		if !d.IsRepo && d.RepoCount > 0 {
			candidates = append(candidates, d)
		}
	}
	return candidates
}

// promptGroups asks the user to select which directories are groups.
func promptGroups(candidates []dirInfo, existing scanner.IndexFile) ([]string, error) {
	existingSet := scanner.ToSet(existing.Groups)
	options := make([]huh.Option[string], len(candidates))
	for i, d := range candidates {
		label := fmt.Sprintf("%s (%d %s)", d.Name, d.RepoCount, repoNoun(d.RepoCount))
		options[i] = huh.NewOption(label, d.Name).Selected(existingSet[d.Name])
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which directories are groups (contain sub-projects)?").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}
	return selected, nil
}

// promptIgnores asks the user to select which directories should be ignored.
func promptIgnores(dirs []dirInfo, selectedGroups []string, existing scanner.IndexFile) ([]string, error) {
	groupSet := scanner.ToSet(selectedGroups)
	existingSet := scanner.ToSet(existing.Ignores)

	var candidates []dirInfo
	for _, d := range dirs {
		if !d.IsRepo && !groupSet[d.Name] {
			candidates = append(candidates, d)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	options := make([]huh.Option[string], len(candidates))
	for i, d := range candidates {
		label := d.Name
		if d.RepoCount > 0 {
			label += fmt.Sprintf(" (%d %s)", d.RepoCount, repoNoun(d.RepoCount))
		}
		options[i] = huh.NewOption(label, d.Name).Selected(existingSet[d.Name])
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which directories should be ignored?").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}
	return selected, nil
}

// previewIndex prints the generated YAML and discovery count.
func previewIndex(yamlBytes []byte, dirs []dirInfo, groups, ignores []string) {
	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	fmt.Printf("\n%s\n\n", bold.Sprint("Generated .katazuke:"))
	for _, line := range strings.Split(strings.TrimRight(string(yamlBytes), "\n"), "\n") {
		fmt.Printf("  %s\n", line)
	}

	repoCount := countDiscoveredRepos(dirs, groups, ignores)
	noun := "repositories"
	if repoCount == 1 {
		noun = "repository"
	}
	fmt.Printf("\n%s\n\n", dim.Sprintf("This configuration will discover %d %s.", repoCount, noun))
}

// writeIndex confirms with the user and writes the .katazuke file.
func writeIndex(dir string, yamlBytes []byte) error {
	indexPath := filepath.Join(dir, ".katazuke")

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Write to %s?", indexPath)).
				Value(&confirmed),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("prompt failed: %w", err)
	}
	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := os.WriteFile(indexPath, yamlBytes, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", indexPath, err)
	}

	repos, err := scanner.Scan(dir, scanner.Options{})
	if err != nil {
		fmt.Printf("Wrote .katazuke (could not verify: %v)\n", err)
		return nil
	}

	fmt.Printf("Wrote .katazuke (discovered %d repositories)\n", len(repos))
	return nil
}

// resolveInitDir determines the target directory for init. The positional
// arg takes priority, then falls through to the standard resolution.
func resolveInitDir(arg, cliProjectsDir string, cfg config.Config) string {
	if arg != "" {
		return config.ExpandHome(arg)
	}
	return resolveProjectsDir(cliProjectsDir, cfg)
}

// classifyChildren scans immediate children of dir and classifies each.
func classifyChildren(dir string) ([]dirInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var result []dirInfo
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}

		child := filepath.Join(dir, name)
		if git.IsRepo(child) {
			result = append(result, dirInfo{Name: name, IsRepo: true})
			continue
		}

		repoCount := countRepoChildren(child)
		result = append(result, dirInfo{Name: name, RepoCount: repoCount})
	}

	return result, nil
}

// countRepoChildren counts immediate children that are git repos.
func countRepoChildren(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
			continue
		}
		if git.IsRepo(filepath.Join(dir, entry.Name())) {
			count++
		}
	}
	return count
}

// generateIndex produces sorted YAML from groups and ignores.
func generateIndex(groups, ignores []string) ([]byte, error) {
	idx := scanner.IndexFile{}
	if len(groups) > 0 {
		sorted := make([]string, len(groups))
		copy(sorted, groups)
		sort.Strings(sorted)
		idx.Groups = sorted
	}
	if len(ignores) > 0 {
		sorted := make([]string, len(ignores))
		copy(sorted, ignores)
		sort.Strings(sorted)
		idx.Ignores = sorted
	}
	return yaml.Marshal(idx)
}

// countDiscoveredRepos estimates the number of repos that the scanner will
// discover with the given configuration.
func countDiscoveredRepos(dirs []dirInfo, groups, ignores []string) int {
	groupSet := scanner.ToSet(groups)
	ignoreSet := scanner.ToSet(ignores)
	count := 0
	for _, d := range dirs {
		if ignoreSet[d.Name] {
			continue
		}
		if d.IsRepo {
			// Top-level repos are always discovered (a repo can't also be a group).
			count++
			continue
		}
		if groupSet[d.Name] {
			// Groups are recursed into; count their child repos.
			count += d.RepoCount
		}
		// Non-group directories are not recursed into by the scanner,
		// so their child repos are not discovered.
	}
	return count
}

// repoNoun returns "repo" or "repos" based on count.
func repoNoun(count int) string {
	if count == 1 {
		return "repo"
	}
	return "repos"
}
