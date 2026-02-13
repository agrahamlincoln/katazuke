package merge

import "github.com/agrahamlincoln/katazuke/pkg/git"

// RealGitChecker implements GitChecker using the pkg/git package.
type RealGitChecker struct{}

// IsMerged returns true if branch has been merged into base.
func (RealGitChecker) IsMerged(repoPath, branch, base string) (bool, error) {
	return git.IsMerged(repoPath, branch, base)
}

// MergedBranches returns local branches merged into the given base branch.
func (RealGitChecker) MergedBranches(repoPath, base string) ([]string, error) {
	return git.MergedBranches(repoPath, base)
}

// RemoteURL returns the fetch URL of the given remote.
func (RealGitChecker) RemoteURL(repoPath, remote string) (string, error) {
	return git.RemoteURL(repoPath, remote)
}
