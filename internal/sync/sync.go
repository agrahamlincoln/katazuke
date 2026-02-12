// Package sync implements repository synchronization logic for keeping
// local git repositories up to date with their remotes.
package sync

import (
	"fmt"
	"log/slog"
	"path/filepath"
)

// Status represents the outcome of syncing a single repository.
type Status int

const (
	// Synced indicates the repository was successfully updated.
	Synced Status = iota
	// Skipped indicates the repository was intentionally skipped.
	Skipped
	// Failed indicates an error occurred while syncing.
	Failed
)

// Result represents the outcome of syncing a single repository.
type Result struct {
	RepoPath string
	RepoName string
	Status   Status
	Message  string
}

// Options controls sync behavior.
type Options struct {
	Strategy  string // "rebase", "merge", "ff-only"
	SkipDirty bool
	AutoStash bool
	DryRun    bool
	Verbose   bool
}

// GitOps defines the git operations needed by the sync logic.
// This interface enables testing with mocks.
type GitOps interface {
	Fetch(repoPath, remote string) error
	IsClean(repoPath string) (bool, error)
	CurrentBranch(repoPath string) (string, error)
	DefaultBranch(repoPath string) (string, error)
	HasRemote(repoPath, remote string) bool
	Pull(repoPath string, strategy string) error
	MergeBase(repoPath string, ref1, ref2 string) (string, error)
	MergeTree(repoPath string, base, local, remote string) (string, bool, error)
	StashPush(repoPath string, message string) error
	StashPop(repoPath string) error
	RebaseAbort(repoPath string) error
	MergeAbort(repoPath string) error
}

// All syncs all provided repository paths and returns results.
func All(repos []string, opts Options, git GitOps) []Result {
	results := make([]Result, 0, len(repos))
	for _, repoPath := range repos {
		result := syncOne(repoPath, opts, git)
		results = append(results, result)
	}
	return results
}

func syncOne(repoPath string, opts Options, git GitOps) Result {
	repoName := filepath.Base(repoPath)
	result := Result{
		RepoPath: repoPath,
		RepoName: repoName,
	}

	// Check for origin remote.
	if !git.HasRemote(repoPath, "origin") {
		result.Status = Skipped
		result.Message = "no origin remote"
		return result
	}

	// Always fetch first (safe operation).
	slog.Debug("fetching", "repo", repoName)
	if err := git.Fetch(repoPath, "origin"); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("fetch failed: %v", err)
		return result
	}

	// Determine the default branch.
	defaultBranch, err := git.DefaultBranch(repoPath)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not determine default branch: %v", err)
		return result
	}

	// Check if we're on the default branch.
	currentBranch, err := git.CurrentBranch(repoPath)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not determine current branch: %v", err)
		return result
	}

	if currentBranch != defaultBranch {
		result.Status = Skipped
		result.Message = fmt.Sprintf("on branch %q, not default branch %q", currentBranch, defaultBranch)
		return result
	}

	// Check working tree status.
	clean, err := git.IsClean(repoPath)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not check working tree: %v", err)
		return result
	}

	if clean {
		return syncClean(repoPath, repoName, opts, git)
	}
	return syncDirty(repoPath, repoName, defaultBranch, opts, git)
}

func syncClean(repoPath, repoName string, opts Options, git GitOps) Result {
	result := Result{
		RepoPath: repoPath,
		RepoName: repoName,
	}

	if opts.DryRun {
		result.Status = Skipped
		result.Message = "would pull (dry run)"
		return result
	}

	slog.Debug("pulling", "repo", repoName, "strategy", opts.Strategy)
	if err := git.Pull(repoPath, opts.Strategy); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("pull failed: %v", err)
		return result
	}

	result.Status = Synced
	result.Message = "pulled successfully"
	return result
}

func syncDirty(repoPath, repoName, defaultBranch string, opts Options, git GitOps) Result {
	result := Result{
		RepoPath: repoPath,
		RepoName: repoName,
	}

	if opts.SkipDirty {
		result.Status = Skipped
		result.Message = "dirty working tree (skip_dirty enabled)"
		return result
	}

	if !opts.AutoStash {
		result.Status = Skipped
		result.Message = "dirty working tree (auto_stash disabled)"
		return result
	}

	// Simulate the merge with merge-tree to check for conflicts.
	remoteRef := "origin/" + defaultBranch
	base, err := git.MergeBase(repoPath, "HEAD", remoteRef)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("merge-base failed: %v", err)
		return result
	}

	_, hasConflicts, err := git.MergeTree(repoPath, base, "HEAD", remoteRef)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("merge-tree simulation failed: %v", err)
		return result
	}

	if hasConflicts {
		result.Status = Skipped
		result.Message = "dirty working tree with potential merge conflicts"
		return result
	}

	if opts.DryRun {
		result.Status = Skipped
		result.Message = "would stash, pull, and pop (dry run)"
		return result
	}

	// Stash, pull, pop.
	slog.Debug("stashing changes", "repo", repoName)
	if err := git.StashPush(repoPath, "katazuke: auto-stash before sync"); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("stash push failed: %v", err)
		return result
	}

	slog.Debug("pulling with stash", "repo", repoName, "strategy", opts.Strategy)
	if err := git.Pull(repoPath, opts.Strategy); err != nil {
		// Pull failed -- abort the partial pull to restore pre-pull state.
		slog.Debug("aborting partial pull", "repo", repoName, "strategy", opts.Strategy)
		abortPull(repoPath, opts.Strategy, git)
		result.Status = Failed
		result.Message = fmt.Sprintf("pull failed after stash (aborted, stash preserved): %v", err)
		return result
	}

	slog.Debug("popping stash", "repo", repoName)
	if err := git.StashPop(repoPath); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("stash pop failed (stash preserved): %v", err)
		return result
	}

	result.Status = Synced
	result.Message = "pulled with auto-stash"
	return result
}

// abortPull attempts to abort a partial pull by running the appropriate
// abort command based on the strategy used. Errors are logged but not
// returned since this is a best-effort cleanup.
func abortPull(repoPath, strategy string, git GitOps) {
	switch strategy {
	case "rebase":
		if err := git.RebaseAbort(repoPath); err != nil {
			slog.Debug("rebase --abort failed (may not be in rebase state)", "error", err)
		}
	default:
		if err := git.MergeAbort(repoPath); err != nil {
			slog.Debug("merge --abort failed (may not be in merge state)", "error", err)
		}
	}
}
