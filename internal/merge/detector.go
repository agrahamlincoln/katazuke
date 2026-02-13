// Package merge provides hybrid merge detection that combines local git
// merge status with GitHub PR state to determine whether a branch has
// been merged. This catches squash-merges and other workflows that leave
// the local branch looking unmerged.
package merge

import (
	"log/slog"

	"github.com/agrahamlincoln/katazuke/internal/github"
)

// DetectionMethod indicates how a branch was determined to be merged.
type DetectionMethod int

const (
	// DetectedByGit means git recognized the branch as merged (fast-forward
	// or regular merge into the base branch).
	DetectedByGit DetectionMethod = iota
	// DetectedByGitHub means the GitHub API reported the branch's PR as
	// merged (e.g. squash-merge, which git does not recognize locally).
	DetectedByGitHub
)

// DetectedBranch pairs a branch name with the method used to detect it
// as merged. Callers use the method to decide whether force-deletion is
// needed (GitHub-detected branches require git branch -D).
type DetectedBranch struct {
	Name   string
	Method DetectionMethod
}

// GitChecker defines the git operations needed for merge detection.
// RemoteURL is included because the detector needs it to determine the
// GitHub owner/repo for API fallback on non-git-merged branches.
type GitChecker interface {
	IsMerged(repoPath, branch, base string) (bool, error)
	MergedBranches(repoPath, base string) ([]string, error)
	RemoteURL(repoPath, remote string) (string, error)
}

// PRChecker defines the GitHub API operations needed for merge detection.
type PRChecker interface {
	BranchPRInfo(owner, repo, branch string) (*github.PRInfo, error)
}

// Detector combines local git merge checks with GitHub PR state lookups
// to determine whether a branch has been merged. When no PRChecker is
// provided, it operates in git-only mode.
type Detector struct {
	git GitChecker
	pr  PRChecker
}

// NewDetector creates a Detector. If pr is nil, the detector uses only
// local git checks. In production, pass the GitHub client even without
// authentication -- API errors degrade gracefully to git-only results.
func NewDetector(git GitChecker, pr PRChecker) *Detector {
	return &Detector{git: git, pr: pr}
}

// GitOnlyDetector returns a Detector that only uses local git operations,
// without any GitHub API fallback. Intended for tests and environments
// without GitHub access.
func GitOnlyDetector() *Detector {
	return NewDetector(RealGitChecker{}, nil)
}

// IsMerged returns true if branch has been merged into base. It first
// checks the local git state (fast path), then falls back to querying
// the GitHub API for PR merge status. Callers that need to know the
// detection method (e.g. for force-deletion decisions) should use
// MergedBranches instead.
func (d *Detector) IsMerged(repoPath, branch, base string) (bool, error) {
	merged, err := d.git.IsMerged(repoPath, branch, base)
	if err != nil {
		return false, err
	}
	if merged {
		return true, nil
	}

	if d.pr == nil {
		return false, nil
	}

	return d.checkPR(repoPath, branch), nil
}

// MergedBranches returns branches that have been merged into base. It
// first collects the git-local merged set, then checks any remaining
// branches against the GitHub API. Each result includes the detection
// method so callers can decide whether force-deletion is needed.
func (d *Detector) MergedBranches(repoPath, base string, allBranches []string) ([]DetectedBranch, error) {
	gitMerged, err := d.git.MergedBranches(repoPath, base)
	if err != nil {
		return nil, err
	}

	gitMergedSet := make(map[string]bool, len(gitMerged))
	for _, b := range gitMerged {
		gitMergedSet[b] = true
	}

	result := make([]DetectedBranch, 0, len(gitMerged))
	for _, b := range gitMerged {
		result = append(result, DetectedBranch{Name: b, Method: DetectedByGit})
	}

	if d.pr == nil {
		return result, nil
	}

	owner, repo, ok := d.resolveGitHubRepo(repoPath)
	if !ok {
		return result, nil
	}

	// Check branches not in the git-merged set via GitHub API.
	for _, branch := range allBranches {
		if gitMergedSet[branch] {
			continue
		}
		if d.isPRMerged(owner, repo, branch) {
			result = append(result, DetectedBranch{Name: branch, Method: DetectedByGitHub})
		}
	}

	return result, nil
}

// resolveGitHubRepo resolves the remote URL for a repository and parses
// the GitHub owner/repo. Returns ok=false for non-GitHub remotes or
// when the remote URL cannot be determined.
func (d *Detector) resolveGitHubRepo(repoPath string) (owner, repo string, ok bool) {
	remoteURL, err := d.git.RemoteURL(repoPath, "origin")
	if err != nil {
		slog.Debug("could not get remote URL, skipping PR check",
			"repo", repoPath, "error", err)
		return "", "", false
	}
	owner, repo, ok = github.ParseGitHubRemote(remoteURL)
	if !ok {
		slog.Debug("non-GitHub remote, skipping PR check",
			"repo", repoPath, "url", remoteURL)
	}
	return owner, repo, ok
}

// isPRMerged queries the GitHub API for the PR state of a single branch.
// Returns true only if the PR was merged. Any error is logged and treated
// as "not merged" (graceful degradation).
func (d *Detector) isPRMerged(owner, repo, branch string) bool {
	info, err := d.pr.BranchPRInfo(owner, repo, branch)
	if err != nil {
		slog.Debug("PR check failed, assuming not merged",
			"repo", owner+"/"+repo, "branch", branch, "error", err)
		return false
	}
	return info.State == github.PRStateMerged
}

// checkPR queries the GitHub API for the PR state of a branch. Returns
// true only if the PR was merged. Used by IsMerged for single-branch checks
// where resolving the repo per call is acceptable.
func (d *Detector) checkPR(repoPath, branch string) bool {
	owner, repo, ok := d.resolveGitHubRepo(repoPath)
	if !ok {
		return false
	}
	return d.isPRMerged(owner, repo, branch)
}
