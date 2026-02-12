package sync

import "github.com/agrahamlincoln/katazuke/pkg/git"

// RealGitOps implements GitOps using the pkg/git package.
type RealGitOps struct{}

// Fetch fetches from the given remote.
func (RealGitOps) Fetch(repoPath, remote string) error {
	return git.Fetch(repoPath, remote)
}

// IsClean returns true if the working tree has no uncommitted changes.
func (RealGitOps) IsClean(repoPath string) (bool, error) {
	return git.IsClean(repoPath)
}

// CurrentBranch returns the name of the currently checked-out branch.
func (RealGitOps) CurrentBranch(repoPath string) (string, error) {
	return git.CurrentBranch(repoPath)
}

// DefaultBranch returns the default branch name.
func (RealGitOps) DefaultBranch(repoPath string) (string, error) {
	return git.DefaultBranch(repoPath)
}

// HasRemote returns true if the given remote exists.
func (RealGitOps) HasRemote(repoPath, remote string) bool {
	return git.HasRemote(repoPath, remote)
}

// Pull pulls from the default remote using the given strategy.
func (RealGitOps) Pull(repoPath string, strategy string) error {
	return git.Pull(repoPath, strategy)
}

// MergeBase returns the best common ancestor commit between two refs.
func (RealGitOps) MergeBase(repoPath string, ref1, ref2 string) (string, error) {
	return git.MergeBase(repoPath, ref1, ref2)
}

// MergeTree performs a three-way merge-tree simulation.
func (RealGitOps) MergeTree(repoPath string, base, local, remote string) (string, bool, error) {
	return git.MergeTree(repoPath, base, local, remote)
}

// StashPush stashes working tree changes with the given message.
func (RealGitOps) StashPush(repoPath string, message string) error {
	return git.StashPush(repoPath, message)
}

// StashPop applies and removes the most recent stash entry.
func (RealGitOps) StashPop(repoPath string) error {
	return git.StashPop(repoPath)
}

// RebaseAbort aborts an in-progress rebase.
func (RealGitOps) RebaseAbort(repoPath string) error {
	return git.RebaseAbort(repoPath)
}

// MergeAbort aborts an in-progress merge.
func (RealGitOps) MergeAbort(repoPath string) error {
	return git.MergeAbort(repoPath)
}
