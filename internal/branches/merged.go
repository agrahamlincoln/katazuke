// Package branches provides logic for finding and managing branches
// across multiple git repositories.
package branches

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	ghclient "github.com/agrahamlincoln/katazuke/internal/github"
	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// MergeMethodResolver determines how a PR was merged by inspecting the
// merge commit. Implemented by *github.Client.
type MergeMethodResolver interface {
	PRMergeMethod(owner, repo, mergeCommitSHA string) (string, error)
}

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
	// PRNumber is the GitHub PR number (0 if not available or git-detected).
	PRNumber int
	// PRMergedAt is the timestamp when the PR was merged on GitHub.
	PRMergedAt time.Time
	// MergeCommitSHA is the merge commit SHA from the GitHub API, used for
	// determining the merge method without an extra API call.
	MergeCommitSHA string
	// MergeMethod is "merge", "squash", or "" if unknown.
	MergeMethod string
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

// EnrichMergeMethod fetches the merge method (merge vs squash) for branches
// that were detected via the GitHub API. Uses the MergeCommitSHA already
// captured during detection to avoid extra API calls per branch.
// The returned slice is the same as the input; items are modified in place.
func EnrichMergeMethod(merged []MergedBranch, resolver MergeMethodResolver, workers int) []MergedBranch {
	if resolver == nil {
		return merged
	}
	type enrichJob struct {
		index          int
		owner, repo    string
		mergeCommitSHA string
	}

	// Cache remote URL per repo to avoid redundant git subprocess calls.
	remoteCache := make(map[string]string)

	var jobs []enrichJob
	for i, m := range merged {
		if m.MergeCommitSHA == "" {
			continue
		}
		remote, cached := remoteCache[m.RepoPath]
		if !cached {
			var err error
			remote, err = git.RemoteURL(m.RepoPath, "origin")
			if err != nil {
				remoteCache[m.RepoPath] = ""
				continue
			}
			remoteCache[m.RepoPath] = remote
		}
		if remote == "" {
			continue
		}
		owner, repo, ok := ghclient.ParseGitHubRemote(remote)
		if !ok {
			continue
		}
		jobs = append(jobs, enrichJob{
			index:          i,
			owner:          owner,
			repo:           repo,
			mergeCommitSHA: m.MergeCommitSHA,
		})
	}

	if len(jobs) == 0 {
		return merged
	}

	type enrichResult struct {
		index       int
		mergeMethod string
	}

	results := parallel.Run(jobs, workers, func(j enrichJob) enrichResult {
		method, err := resolver.PRMergeMethod(j.owner, j.repo, j.mergeCommitSHA)
		if err != nil {
			slog.Debug("could not determine merge method",
				"repo", j.owner+"/"+j.repo, "error", err)
			return enrichResult{index: j.index}
		}
		return enrichResult{index: j.index, mergeMethod: method}
	}, nil)

	for _, r := range results {
		if r.mergeMethod != "" {
			merged[r.index].MergeMethod = r.mergeMethod
		}
	}

	return merged
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
			RepoPath:       repoPath,
			RepoName:       repoName,
			Branch:         d.Name,
			LastCommit:     commitDate,
			HasRemote:      hasRemote,
			ForceDelete:    d.Method == merge.DetectedByGitHub,
			PRNumber:       d.PRNumber,
			PRMergedAt:     d.PRMergedAt,
			MergeCommitSHA: d.MergeCommitSHA,
		})
	}

	return results
}

// Label returns a display string for the merged branch in the form "repo: branch".
// Branches with a remote counterpart are annotated with "(backed up remotely)".
// PR info is appended when available.
func (m MergedBranch) Label() string {
	label := fmt.Sprintf("%s: %s", m.RepoName, m.Branch)
	if m.HasRemote {
		label += " (backed up remotely)"
	}
	if m.PRNumber > 0 {
		if m.MergeMethod != "" {
			label += fmt.Sprintf(" [%s-merged PR #%d]", m.MergeMethod, m.PRNumber)
		} else {
			label += fmt.Sprintf(" [merged PR #%d]", m.PRNumber)
		}
	} else if m.ForceDelete {
		label += " [merged]"
	}
	return label
}
