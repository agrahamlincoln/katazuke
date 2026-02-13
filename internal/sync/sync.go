// Package sync implements repository synchronization logic for keeping
// local git repositories up to date with their remotes.
package sync

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/agrahamlincoln/katazuke/internal/parallel"
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
	// Switched indicates the repo was on a merged branch and switched to default.
	Switched
	// UpToDate indicates the repository was already current with the remote.
	UpToDate
)

// String returns the human-readable name of a Status value.
func (s Status) String() string {
	switch s {
	case Synced:
		return "Synced"
	case Skipped:
		return "Skipped"
	case Failed:
		return "Failed"
	case Switched:
		return "Switched"
	case UpToDate:
		return "UpToDate"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

// Result represents the outcome of syncing a single repository.
type Result struct {
	RepoPath      string
	RepoName      string
	Status        Status
	Message       string
	CommitsPulled int // number of commits pulled, populated when known
}

// Options controls sync behavior.
type Options struct {
	Strategy           string // "rebase", "merge", "ff-only"
	SkipDirty          bool
	AutoStash          bool
	DryRun             bool
	Verbose            bool
	SwitchMergedBranch bool
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
	IsMerged(repoPath, branch, base string) (bool, error)
	Checkout(repoPath, branch string) error
	MergeBase(repoPath string, ref1, ref2 string) (string, error)
	MergeTree(repoPath string, base, local, remote string) (string, bool, error)
	StashPush(repoPath string, message string) (bool, error)
	StashPop(repoPath string) error
	RebaseAbort(repoPath string) error
	MergeAbort(repoPath string) error
	RevListCount(repoPath, spec string) (int, error)
}

// ResultFunc is called sequentially as each repo finishes syncing.
// completed is the number of repos finished so far; total is the
// total number of repos being synced.
type ResultFunc func(completed, total int, result Result)

// All syncs all provided repository paths using the given number of
// workers and returns results. An optional callback is called
// sequentially as each repo completes.
func All(repos []string, opts Options, git GitOps, workers int, onResult ResultFunc) []Result {
	return parallel.Run(repos, workers, func(repoPath string) Result {
		return syncOne(repoPath, opts, git)
	}, func(completed, total int, result Result) {
		if onResult != nil {
			onResult(completed, total, result)
		}
	})
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

	if currentBranch == "" {
		return syncDetachedHEAD(repoPath, repoName, defaultBranch, opts, git)
	}

	if currentBranch != defaultBranch {
		return syncNonDefault(repoPath, repoName, currentBranch, defaultBranch, opts, git)
	}

	// Check working tree status.
	clean, err := git.IsClean(repoPath)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not check working tree: %v", err)
		return result
	}

	if clean {
		return syncClean(repoPath, repoName, defaultBranch, opts, git)
	}
	return syncDirty(repoPath, repoName, defaultBranch, opts, git)
}

func syncDetachedHEAD(repoPath, repoName, defaultBranch string, opts Options, git GitOps) Result {
	result := Result{
		RepoPath: repoPath,
		RepoName: repoName,
	}

	clean, err := git.IsClean(repoPath)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not check working tree: %v", err)
		return result
	}

	if !clean {
		result.Status = Skipped
		result.Message = "detached HEAD with dirty working tree"
		return result
	}

	if opts.DryRun {
		result.Status = Skipped
		result.Message = fmt.Sprintf("would switch from detached HEAD to %s and sync (dry run)", defaultBranch)
		return result
	}

	slog.Debug("switching from detached HEAD", "repo", repoName, "to", defaultBranch)
	if err := git.Checkout(repoPath, defaultBranch); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not switch to %s: %v", defaultBranch, err)
		return result
	}

	pullResult := syncClean(repoPath, repoName, defaultBranch, opts, git)
	if pullResult.Status == Failed {
		return pullResult
	}

	result.Status = Switched
	if pullResult.Status == UpToDate {
		result.Message = fmt.Sprintf("switched from detached HEAD to %s (up-to-date)", defaultBranch)
	} else {
		msg := fmt.Sprintf("switched from detached HEAD to %s and synced", defaultBranch)
		if pullResult.CommitsPulled > 0 {
			msg += fmt.Sprintf(" (%d %s)", pullResult.CommitsPulled, pluralCommit(pullResult.CommitsPulled))
		}
		result.Message = msg
	}
	return result
}

func syncNonDefault(repoPath, repoName, currentBranch, defaultBranch string, opts Options, git GitOps) Result {
	result := Result{
		RepoPath: repoPath,
		RepoName: repoName,
	}

	// Check if the current branch is merged into origin/<default>.
	remoteDefault := "origin/" + defaultBranch
	merged, err := git.IsMerged(repoPath, currentBranch, remoteDefault)
	if err != nil {
		// If we can't determine merge status, fall back to the original skip behavior.
		slog.Debug("could not check merge status", "repo", repoName, "error", err)
		result.Status = Skipped
		result.Message = fmt.Sprintf("on branch %q, not default branch %q", currentBranch, defaultBranch)
		return result
	}

	if !merged {
		result.Status = Skipped
		result.Message = fmt.Sprintf("on branch %q, not default branch %q", currentBranch, defaultBranch)
		return result
	}

	if !opts.SwitchMergedBranch {
		result.Status = Skipped
		result.Message = fmt.Sprintf("on branch %q (merged into %s, safe to switch)", currentBranch, defaultBranch)
		return result
	}

	// Only switch if working tree is clean.
	clean, err := git.IsClean(repoPath)
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not check working tree: %v", err)
		return result
	}

	if !clean {
		result.Status = Skipped
		result.Message = fmt.Sprintf("on branch %q (merged, but working tree is dirty)", currentBranch)
		return result
	}

	if opts.DryRun {
		result.Status = Skipped
		result.Message = fmt.Sprintf("would switch from merged branch %q to %s (dry run)", currentBranch, defaultBranch)
		return result
	}

	slog.Debug("switching from merged branch", "repo", repoName, "from", currentBranch, "to", defaultBranch)
	if err := git.Checkout(repoPath, defaultBranch); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("could not switch to %s: %v", defaultBranch, err)
		return result
	}

	// Now continue with normal sync (clean working tree on default branch).
	pullResult := syncClean(repoPath, repoName, defaultBranch, opts, git)
	if pullResult.Status == Failed {
		return pullResult
	}

	result.Status = Switched
	if pullResult.Status == UpToDate {
		result.Message = fmt.Sprintf("switched from merged branch %q to %s (up-to-date)", currentBranch, defaultBranch)
	} else {
		msg := fmt.Sprintf("switched from merged branch %q to %s and synced", currentBranch, defaultBranch)
		if pullResult.CommitsPulled > 0 {
			msg += fmt.Sprintf(" (%d %s)", pullResult.CommitsPulled, pluralCommit(pullResult.CommitsPulled))
		}
		result.Message = msg
	}
	return result
}

func syncClean(repoPath, repoName, defaultBranch string, opts Options, git GitOps) Result {
	result := Result{
		RepoPath: repoPath,
		RepoName: repoName,
	}

	// Check how many commits we're behind the remote. This uses the
	// already-fetched origin ref, so the count matches what pull will apply.
	remoteRef := "origin/" + defaultBranch
	behindCount, countErr := git.RevListCount(repoPath, "HEAD.."+remoteRef)
	if countErr == nil && behindCount == 0 {
		result.Status = UpToDate
		return result
	}

	if opts.DryRun {
		result.Status = Skipped
		if countErr == nil {
			result.Message = fmt.Sprintf("would pull, %d %s behind (dry run)", behindCount, pluralCommit(behindCount))
		} else {
			result.Message = "would pull (dry run)"
		}
		return result
	}

	slog.Debug("pulling", "repo", repoName, "strategy", opts.Strategy)
	if err := git.Pull(repoPath, opts.Strategy); err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("pull failed: %v", err)
		return result
	}

	result.Status = Synced
	if countErr == nil {
		result.CommitsPulled = behindCount
		result.Message = fmt.Sprintf("%d %s", behindCount, pluralCommit(behindCount))
	} else {
		result.Message = "pulled successfully"
	}
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

	// Check how many commits we're behind the remote. This uses the
	// already-fetched origin ref, so the count matches what pull will apply.
	behindCount, countErr := git.RevListCount(repoPath, "HEAD.."+remoteRef)
	if countErr == nil && behindCount == 0 {
		result.Status = UpToDate
		return result
	}

	if opts.DryRun {
		result.Status = Skipped
		if countErr == nil {
			result.Message = fmt.Sprintf("would stash, pull, and pop, %d %s behind (dry run)", behindCount, pluralCommit(behindCount))
		} else {
			result.Message = "would stash, pull, and pop (dry run)"
		}
		return result
	}

	// Stash, pull, pop.
	stashed, err := git.StashPush(repoPath, "katazuke: auto-stash before sync")
	if err != nil {
		result.Status = Failed
		result.Message = fmt.Sprintf("stash push failed: %v", err)
		return result
	}
	slog.Debug("stash push completed", "repo", repoName, "created", stashed)

	slog.Debug("pulling with stash", "repo", repoName, "strategy", opts.Strategy)
	if err := git.Pull(repoPath, opts.Strategy); err != nil {
		// Pull failed -- abort the partial pull to restore pre-pull state.
		slog.Debug("aborting partial pull", "repo", repoName, "strategy", opts.Strategy)
		abortPull(repoPath, opts.Strategy, git)
		result.Status = Failed
		result.Message = fmt.Sprintf("pull failed after stash (aborted, stash preserved): %v", err)
		return result
	}

	if stashed {
		slog.Debug("popping stash", "repo", repoName)
		if err := git.StashPop(repoPath); err != nil {
			result.Status = Failed
			result.Message = fmt.Sprintf("stash pop failed (stash preserved): %v", err)
			return result
		}
	}

	result.Status = Synced
	if countErr == nil {
		result.CommitsPulled = behindCount
		result.Message = fmt.Sprintf("%d %s, auto-stash", behindCount, pluralCommit(behindCount))
	} else {
		result.Message = "pulled with auto-stash"
	}
	return result
}

func pluralCommit(n int) string {
	if n == 1 {
		return "commit"
	}
	return "commits"
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
