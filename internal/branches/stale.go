package branches

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// StaleBranch represents a branch that has not been committed to within
// the configured staleness threshold and has not been merged.
type StaleBranch struct {
	RepoPath          string
	RepoName          string
	Branch            string
	LastCommit        time.Time
	LastCommitMessage string
	CommitsAhead      int
	CommitsBehind     int
	HasRemote         bool
	// IsLocalOnly is true when the branch has no remote tracking branch.
	// These are candidates for cleanup but require extra caution since
	// commits may not exist anywhere else.
	IsLocalOnly bool
	// IsAutomation is true for branches matching known automation patterns
	// (e.g. dependabot/*, renovate/*, release-please--*).
	IsAutomation bool
	// IsOwnBranch is true when the user is the sole author of all commits
	// on this branch since it diverged from the default branch.
	IsOwnBranch bool
}

// Label returns a display string for the stale branch in the form "repo: branch".
func (s StaleBranch) Label() string {
	return fmt.Sprintf("%s: %s", s.RepoName, s.Branch)
}

// automationPrefixes lists branch name prefixes created by automation tools.
// These branches should be cleaned up locally when safe, but never deleted
// from remotes since the automation tool manages them.
var automationPrefixes = []string{
	"dependabot/",
	"renovate/",
	"release-please--",
}

// IsAutomationBranch returns true if the branch name matches a known
// automation pattern.
func IsAutomationBranch(branch string) bool {
	for _, prefix := range automationPrefixes {
		if strings.HasPrefix(branch, prefix) {
			return true
		}
	}
	return false
}

// FindStale scans the given repositories and returns branches whose last commit
// is older than the given threshold. Merged branches, the default branch, and
// the currently checked out branch are excluded. Work is parallelized across
// the given number of workers.
func FindStale(repos []string, threshold time.Duration, workers int, onProgress func(completed, total int)) ([]StaleBranch, error) {
	cutoff := time.Now().Add(-threshold)

	type staleArgs struct {
		repoPath string
		cutoff   time.Time
	}

	args := make([]staleArgs, len(repos))
	for i, r := range repos {
		args[i] = staleArgs{repoPath: r, cutoff: cutoff}
	}

	var resultCb func(int, int, []StaleBranch)
	if onProgress != nil {
		resultCb = func(completed, total int, _ []StaleBranch) {
			onProgress(completed, total)
		}
	}

	repoResults := parallel.Run(args, workers, func(a staleArgs) []StaleBranch {
		return findStaleInRepo(a.repoPath, a.cutoff)
	}, resultCb)

	results := make([]StaleBranch, 0, len(repoResults))
	for _, rr := range repoResults {
		results = append(results, rr...)
	}
	return results, nil
}

func findStaleInRepo(repoPath string, cutoff time.Time) []StaleBranch {
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

	allBranches, err := git.ListBranches(repoPath)
	if err != nil {
		slog.Warn("skipping repo: could not list branches",
			"repo", repoName, "error", err)
		return nil
	}

	mergedBranches, err := git.MergedBranches(repoPath, defaultBranch)
	if err != nil {
		slog.Warn("skipping repo: could not list merged branches",
			"repo", repoName, "error", err)
		return nil
	}
	mergedSet := make(map[string]bool, len(mergedBranches))
	for _, b := range mergedBranches {
		mergedSet[b] = true
	}

	// Get the user's identity for authorship checking.
	userEmail, _ := git.ConfigValue(repoPath, "user.email")

	var results []StaleBranch
	for _, branch := range allBranches {
		if branch == defaultBranch || branch == currentBranch {
			continue
		}
		if mergedSet[branch] {
			continue
		}

		commitDate, err := git.CommitDate(repoPath, branch)
		if err != nil {
			slog.Warn("could not get commit date, skipping branch",
				"repo", repoName, "branch", branch, "error", err)
			continue
		}

		if commitDate.After(cutoff) {
			continue
		}

		ahead, behind, err := git.CommitsAheadBehind(repoPath, branch, defaultBranch)
		if err != nil {
			slog.Warn("could not get ahead/behind counts",
				"repo", repoName, "branch", branch, "error", err)
		}

		hasRemote := false
		if git.HasRemote(repoPath, "origin") {
			hasRemote, err = git.HasRemoteBranch(repoPath, "origin", branch)
			if err != nil {
				slog.Debug("could not check remote branch",
					"repo", repoName, "branch", branch, "error", err)
			}
		}

		subject, err := git.CommitSubject(repoPath, branch)
		if err != nil {
			slog.Warn("could not get commit subject",
				"repo", repoName, "branch", branch, "error", err)
		}

		isOwn := checkAuthorship(repoPath, branch, defaultBranch, userEmail, repoName)
		isLocalOnly := !hasRemote && !git.HasUpstream(repoPath, branch)

		results = append(results, StaleBranch{
			RepoPath:          repoPath,
			RepoName:          repoName,
			Branch:            branch,
			LastCommit:        commitDate,
			LastCommitMessage: subject,
			CommitsAhead:      ahead,
			CommitsBehind:     behind,
			HasRemote:         hasRemote,
			IsLocalOnly:       isLocalOnly,
			IsAutomation:      IsAutomationBranch(branch),
			IsOwnBranch:       isOwn,
		})
	}

	return results
}

// checkAuthorship returns true if all commits on branch (since diverging from
// base) were authored by the given email. Returns true if the email is empty
// (can't determine identity) or if the branch has no unique commits (diverged
// at the same point).
func checkAuthorship(repoPath, branch, base, userEmail, repoName string) bool {
	if userEmail == "" {
		return true
	}
	authors, err := git.CommitAuthors(repoPath, branch, base)
	if err != nil {
		slog.Debug("could not check commit authors",
			"repo", repoName, "branch", branch, "error", err)
		return true
	}
	if len(authors) == 0 {
		return true
	}
	for _, a := range authors {
		if !strings.EqualFold(a, userEmail) {
			return false
		}
	}
	return true
}
