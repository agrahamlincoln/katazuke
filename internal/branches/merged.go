// Package branches provides logic for finding and managing branches
// across multiple git repositories.
package branches

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// MergedBranch represents a branch that has been merged into the default branch.
type MergedBranch struct {
	RepoPath   string
	RepoName   string
	Branch     string
	LastCommit time.Time
}

// FindMerged scans the given repositories and returns branches that have been
// merged into each repo's default branch. The current branch and the default
// branch itself are excluded from results.
func FindMerged(repos []string) ([]MergedBranch, error) {
	var results []MergedBranch

	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)

		defaultBranch, err := git.DefaultBranch(repoPath)
		if err != nil {
			slog.Warn("skipping repo: could not determine default branch",
				"repo", repoName, "error", err)
			continue
		}

		currentBranch, err := git.CurrentBranch(repoPath)
		if err != nil {
			slog.Warn("skipping repo: could not determine current branch",
				"repo", repoName, "error", err)
			continue
		}

		merged, err := git.MergedBranches(repoPath, defaultBranch)
		if err != nil {
			slog.Warn("skipping repo: could not list merged branches",
				"repo", repoName, "error", err)
			continue
		}

		for _, branch := range merged {
			if branch == defaultBranch || branch == currentBranch {
				continue
			}

			commitDate, err := git.CommitDate(repoPath, branch)
			if err != nil {
				slog.Warn("could not get commit date, using zero time",
					"repo", repoName, "branch", branch, "error", err)
			}

			results = append(results, MergedBranch{
				RepoPath:   repoPath,
				RepoName:   repoName,
				Branch:     branch,
				LastCommit: commitDate,
			})
		}
	}

	return results, nil
}

// Label returns a display string for the merged branch in the form "repo: branch".
func (m MergedBranch) Label() string {
	return fmt.Sprintf("%s: %s", m.RepoName, m.Branch)
}
