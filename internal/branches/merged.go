// Package branches provides logic for finding and managing branches
// across multiple git repositories.
package branches

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

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
}

// FindMerged scans the given repositories and returns branches that have been
// merged into each repo's default branch. The current branch and the default
// branch itself are excluded from results. Work is parallelized across the
// given number of workers.
func FindMerged(repos []string, workers int, onProgress func(completed, total int)) ([]MergedBranch, error) {
	var resultCb func(int, int, []MergedBranch)
	if onProgress != nil {
		resultCb = func(completed, total int, _ []MergedBranch) {
			onProgress(completed, total)
		}
	}

	repoResults := parallel.Run(repos, workers, findMergedInRepo, resultCb)

	results := make([]MergedBranch, 0, len(repoResults))
	for _, rr := range repoResults {
		results = append(results, rr...)
	}
	return results, nil
}

func findMergedInRepo(repoPath string) []MergedBranch {
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

	merged, err := git.MergedBranches(repoPath, defaultBranch)
	if err != nil {
		slog.Warn("skipping repo: could not list merged branches",
			"repo", repoName, "error", err)
		return nil
	}

	var results []MergedBranch
	for _, branch := range merged {
		if branch == defaultBranch || branch == currentBranch {
			continue
		}

		commitDate, err := git.CommitDate(repoPath, branch)
		if err != nil {
			slog.Warn("could not get commit date, using zero time",
				"repo", repoName, "branch", branch, "error", err)
		}

		hasRemote, err := git.HasRemoteBranch(repoPath, "origin", branch)
		if err != nil {
			slog.Debug("could not check remote branch",
				"repo", repoName, "branch", branch, "error", err)
		}

		results = append(results, MergedBranch{
			RepoPath:   repoPath,
			RepoName:   repoName,
			Branch:     branch,
			LastCommit: commitDate,
			HasRemote:  hasRemote,
		})
	}

	return results
}

// Label returns a display string for the merged branch in the form "repo: branch".
// Branches with a remote counterpart are annotated with "(+ remote)".
func (m MergedBranch) Label() string {
	label := fmt.Sprintf("%s: %s", m.RepoName, m.Branch)
	if m.HasRemote {
		label += " (+ remote)"
	}
	return label
}
