package sync

import (
	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// RealGitOps implements GitOps using the pkg/git package and the hybrid
// merge detector for IsMerged checks.
type RealGitOps struct {
	detector *merge.Detector
}

// NewRealGitOps creates a RealGitOps that delegates IsMerged calls to
// the given merge detector (which combines git + GitHub API checks).
func NewRealGitOps(detector *merge.Detector) *RealGitOps {
	return &RealGitOps{detector: detector}
}

// Fetch fetches from the given remote.
func (r *RealGitOps) Fetch(repoPath, remote string) error {
	return git.Fetch(repoPath, remote)
}

// IsClean returns true if the working tree has no uncommitted changes.
func (r *RealGitOps) IsClean(repoPath string) (bool, error) {
	return git.IsClean(repoPath)
}

// CurrentBranch returns the name of the currently checked-out branch.
func (r *RealGitOps) CurrentBranch(repoPath string) (string, error) {
	return git.CurrentBranch(repoPath)
}

// DefaultBranch returns the default branch name.
func (r *RealGitOps) DefaultBranch(repoPath string) (string, error) {
	return git.DefaultBranch(repoPath)
}

// HasRemote returns true if the given remote exists.
func (r *RealGitOps) HasRemote(repoPath, remote string) bool {
	return git.HasRemote(repoPath, remote)
}

// Pull pulls from the default remote using the given strategy.
func (r *RealGitOps) Pull(repoPath string, strategy string) error {
	return git.Pull(repoPath, strategy)
}

// IsMerged returns true if the given branch has been merged into base.
// It delegates to the hybrid merge detector which checks both local git
// state and GitHub PR status.
func (r *RealGitOps) IsMerged(repoPath, branch, base string) (bool, error) {
	return r.detector.IsMerged(repoPath, branch, base)
}

// Checkout switches to the given branch.
func (r *RealGitOps) Checkout(repoPath, branch string) error {
	return git.Checkout(repoPath, branch)
}

// MergeBase returns the best common ancestor commit between two refs.
func (r *RealGitOps) MergeBase(repoPath string, ref1, ref2 string) (string, error) {
	return git.MergeBase(repoPath, ref1, ref2)
}

// MergeTree performs a three-way merge-tree simulation.
func (r *RealGitOps) MergeTree(repoPath string, base, local, remote string) (string, bool, error) {
	return git.MergeTree(repoPath, base, local, remote)
}

// StashPush stashes working tree changes with the given message.
// It returns true if a stash entry was actually created.
func (r *RealGitOps) StashPush(repoPath string, message string) (bool, error) {
	return git.StashPush(repoPath, message)
}

// StashPop applies and removes the most recent stash entry.
func (r *RealGitOps) StashPop(repoPath string) error {
	return git.StashPop(repoPath)
}

// RebaseAbort aborts an in-progress rebase.
func (r *RealGitOps) RebaseAbort(repoPath string) error {
	return git.RebaseAbort(repoPath)
}

// MergeAbort aborts an in-progress merge.
func (r *RealGitOps) MergeAbort(repoPath string) error {
	return git.MergeAbort(repoPath)
}
