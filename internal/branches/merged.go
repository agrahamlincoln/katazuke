// Package branches provides logic for finding and managing branches
// across multiple git repositories.
package branches

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// MergedBranch represents a branch that has been merged into the default branch.
type MergedBranch struct {
	RepoPath   string
	RepoName   string
	Branch     string
	LastCommit time.Time
	HasRemote  bool
	// ForceDelete is true when the branch was detected as merged via the
	// GitHub API (e.g. squash-merge) rather than by git. These branches
	// require git branch -D because git does not recognize them as merged.
	ForceDelete bool
}

// FindMerged scans the given repositories and returns branches that have been
// merged into each repo's default branch. The current branch and the default
// branch itself are excluded from results. Work is parallelized across the
// given number of workers. The detector combines local git checks with
// GitHub API lookups to catch squash-merges.
func FindMerged(repos []string, detector *merge.Detector, workers int, onProgress func(completed, total int)) ([]MergedBranch, error) {
	var resultCb func(int, int, []MergedBranch)
	if onProgress != nil {
		resultCb = func(completed, total int, _ []MergedBranch) {
			onProgress(completed, total)
		}
	}

	repoResults := parallel.Run(repos, workers, func(repoPath string) []MergedBranch {
		return findMergedInRepo(repoPath, detector)
	}, resultCb)

	results := make([]MergedBranch, 0, len(repoResults))
	for _, rr := range repoResults {
		results = append(results, rr...)
	}
	return results, nil
}

func findMergedInRepo(repoPath string, detector *merge.Detector) []MergedBranch {
	repoName := filepath.Base(repoPath)

	defaultBranch, err := git.DefaultBranch(repoPath)
	if err != nil {
		slog.Warn("skipping repo: could not determine default branch",
			"repo", repoName, "error", err)
		return nil
	}

	currentBranch, err := git.CurrentBranch(repoPath)
	if err != nil {
		slog.Warn("skipping repo: could not determine current branch",
			"repo", repoName, "error", err)
		return nil
	}

	if currentBranch == "" {
		slog.Debug("repo has detached HEAD, no branch to exclude", "repo", repoName)
	}

	allBranches, err := git.ListBranches(repoPath)
	if err != nil {
		slog.Warn("skipping repo: could not list branches",
			"repo", repoName, "error", err)
		return nil
	}

	// Filter out default and current branches before passing to the detector
	// to avoid unnecessary API calls for branches we'd discard anyway.
	candidates := make([]string, 0, len(allBranches))
	for _, b := range allBranches {
		if b != defaultBranch && b != currentBranch {
			candidates = append(candidates, b)
		}
	}

	detected, err := detector.MergedBranches(repoPath, defaultBranch, candidates)
	if err != nil {
		slog.Warn("skipping repo: could not list merged branches",
			"repo", repoName, "error", err)
		return nil
	}

	// The detector's git-merged set can include default/current
	// branches since git branch --merged is not filtered by the
	// candidates list. Exclude them here as a safety net.
	var results []MergedBranch
	for _, d := range detected {
		if d.Name == defaultBranch || d.Name == currentBranch {
			continue
		}

		commitDate, err := git.CommitDate(repoPath, d.Name)
		if err != nil {
			slog.Warn("could not get commit date, using zero time",
				"repo", repoName, "branch", d.Name, "error", err)
		}

		hasRemote, err := git.HasRemoteBranch(repoPath, "origin", d.Name)
		if err != nil {
			slog.Debug("could not check remote branch",
				"repo", repoName, "branch", d.Name, "error", err)
		}

		results = append(results, MergedBranch{
			RepoPath:    repoPath,
			RepoName:    repoName,
			Branch:      d.Name,
			LastCommit:  commitDate,
			HasRemote:   hasRemote,
			ForceDelete: d.Method == merge.DetectedByGitHub,
		})
	}

	return results
}

// Label returns a display string for the merged branch in the form "repo: branch".
// Branches with a remote counterpart are annotated with "(backed up remotely)".
func (m MergedBranch) Label() string {
	label := fmt.Sprintf("%s: %s", m.RepoName, m.Branch)
	if m.HasRemote {
		label += " (backed up remotely)"
	}
	return label
}
