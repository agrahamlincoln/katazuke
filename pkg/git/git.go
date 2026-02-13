// Package git provides functions for interacting with git repositories
// by shelling out to the git CLI.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// run wraps git command execution with consistent error formatting and output trimming.
func run(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, exitErr.Stderr)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRepo returns true if the given path is inside a git repository.
func IsRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// CurrentBranch returns the name of the currently checked-out branch.
func CurrentBranch(repoPath string) (string, error) {
	return run(repoPath, "branch", "--show-current")
}

// DefaultBranch returns the default branch name (main or master) by checking
// what the origin HEAD points to, falling back to a local heuristic.
func DefaultBranch(repoPath string) (string, error) {
	// Try the remote HEAD symref first.
	out, err := run(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	if err == nil {
		// Output is like "origin/main" -- strip the remote prefix.
		parts := strings.SplitN(out, "/", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
		return out, nil
	}

	// Fallback: check if "main" or "master" exists locally.
	branches, err := ListBranches(repoPath)
	if err != nil {
		return "", err
	}
	for _, b := range branches {
		if b == "main" {
			return "main", nil
		}
	}
	for _, b := range branches {
		if b == "master" {
			return "master", nil
		}
	}
	return "", fmt.Errorf("could not determine default branch for %s", repoPath)
}

// ListBranches returns all local branch names.
func ListBranches(repoPath string) ([]string, error) {
	out, err := run(repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	return filterBranches(splitNonEmpty(out)), nil
}

// MergedBranches returns local branches that have been merged into the given base branch.
func MergedBranches(repoPath, base string) ([]string, error) {
	out, err := run(repoPath, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	return filterBranches(splitNonEmpty(out)), nil
}

// filterBranches removes pseudo-branch entries from git branch output.
// When HEAD is detached, git includes a "(HEAD detached at ...)" entry in
// branch listings even with --format=%(refname:short). These are not real
// branch names and must be excluded.
func filterBranches(branches []string) []string {
	result := make([]string, 0, len(branches))
	for _, b := range branches {
		if !strings.HasPrefix(b, "(") {
			result = append(result, b)
		}
	}
	return result
}

// IsMerged returns true if the given branch has been merged into base.
func IsMerged(repoPath, branch, base string) (bool, error) {
	merged, err := MergedBranches(repoPath, base)
	if err != nil {
		return false, err
	}
	for _, m := range merged {
		if m == branch {
			return true, nil
		}
	}
	return false, nil
}

// RemoteURL returns the fetch URL of the given remote (usually "origin").
func RemoteURL(repoPath, remote string) (string, error) {
	return run(repoPath, "remote", "get-url", remote)
}

// Fetch fetches from the given remote.
func Fetch(repoPath, remote string) error {
	_, err := run(repoPath, "fetch", remote)
	return err
}

// DeleteLocalBranch deletes a local branch. If force is true, uses -D instead of -d.
func DeleteLocalBranch(repoPath, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(repoPath, "branch", flag, branch)
	return err
}

// DeleteRemoteBranch deletes a branch on the given remote.
func DeleteRemoteBranch(repoPath, remote, branch string) error {
	_, err := run(repoPath, "push", remote, "--delete", branch)
	return err
}

// CommitDate returns the author date of the latest commit on the given branch.
func CommitDate(repoPath, branch string) (time.Time, error) {
	out, err := run(repoPath, "log", "-1", "--format=%aI", branch)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, out)
}

// IsClean returns true if the working tree has no uncommitted changes.
func IsClean(repoPath string) (bool, error) {
	out, err := run(repoPath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// HasRemote returns true if the given remote exists.
func HasRemote(repoPath, remote string) bool {
	_, err := run(repoPath, "remote", "get-url", remote)
	return err == nil
}

// Pull pulls from the default remote using the given strategy.
// Valid strategies: "rebase", "merge", "ff-only".
func Pull(repoPath string, strategy string) error {
	args := []string{"pull"}
	switch strategy {
	case "rebase":
		args = append(args, "--rebase")
	case "ff-only":
		args = append(args, "--ff-only")
	case "merge", "":
		// default git pull behavior
	default:
		return fmt.Errorf("unknown pull strategy: %q", strategy)
	}
	_, err := run(repoPath, args...)
	return err
}

// StashPush stashes the current working tree changes with the given message.
// It returns true if a stash entry was actually created, false if there was
// nothing to stash (git stash push exits 0 either way).
func StashPush(repoPath string, message string) (bool, error) {
	// Capture the stash ref before pushing so we can detect whether a new
	// entry was created. This avoids parsing porcelain output which varies
	// by locale.
	beforeRef, _ := run(repoPath, "rev-parse", "--quiet", "--verify", "refs/stash")

	_, err := run(repoPath, "stash", "push", "-m", message)
	if err != nil {
		return false, err
	}

	afterRef, _ := run(repoPath, "rev-parse", "--quiet", "--verify", "refs/stash")
	return afterRef != beforeRef, nil
}

// StashPop applies and removes the most recent stash entry.
func StashPop(repoPath string) error {
	_, err := run(repoPath, "stash", "pop")
	return err
}

// RebaseAbort aborts an in-progress rebase, restoring the branch to its pre-rebase state.
func RebaseAbort(repoPath string) error {
	_, err := run(repoPath, "rebase", "--abort")
	return err
}

// MergeAbort aborts an in-progress merge, restoring the branch to its pre-merge state.
func MergeAbort(repoPath string) error {
	_, err := run(repoPath, "merge", "--abort")
	return err
}

// MergeBase returns the best common ancestor commit between two refs.
func MergeBase(repoPath string, ref1, ref2 string) (string, error) {
	return run(repoPath, "merge-base", ref1, ref2)
}

// MergeTree performs a three-way merge-tree between base, local, and remote tree-ish
// references. It returns the merge output, whether conflicts were detected, and any error.
// This is a read-only operation that does not modify the working tree.
func MergeTree(repoPath string, base, local, remote string) (string, bool, error) {
	cmd := exec.Command("git", "merge-tree", base, local, remote)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		// merge-tree exits non-zero on conflicts in some versions
		return output, true, nil
	}
	// Old-style merge-tree outputs conflict markers when there are conflicts.
	hasConflicts := strings.Contains(output, "<<<<<<")
	return output, hasConflicts, nil
}

// Checkout switches to the given branch.
func Checkout(repoPath, branch string) error {
	_, err := run(repoPath, "checkout", branch)
	return err
}

// CreateTag creates a lightweight tag at the given ref.
func CreateTag(repoPath, tagName, ref string) error {
	_, err := run(repoPath, "tag", tagName, ref)
	return err
}

// CommitsAheadBehind returns the number of commits that branch is ahead of and
// behind base. This uses rev-list to count commits reachable from one ref but
// not the other.
func CommitsAheadBehind(repoPath, branch, base string) (ahead int, behind int, err error) {
	out, err := run(repoPath, "rev-list", "--left-right", "--count", base+"..."+branch)
	if err != nil {
		return 0, 0, err
	}
	_, err = fmt.Sscanf(out, "%d\t%d", &behind, &ahead)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing rev-list output %q: %w", out, err)
	}
	return ahead, behind, nil
}

// HasRemoteBranch returns true if the given branch exists on the specified remote.
func HasRemoteBranch(repoPath, remote, branch string) (bool, error) {
	out, err := run(repoPath, "branch", "-r", "--list", remote+"/"+branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// CommitSubject returns the subject line of the latest commit on the given ref.
func CommitSubject(repoPath, ref string) (string, error) {
	return run(repoPath, "log", "-1", "--format=%s", ref)
}

// ConfigValue returns the value of a git config key in the given repo.
func ConfigValue(repoPath, key string) (string, error) {
	return run(repoPath, "config", key)
}

// CommitAuthors returns the set of unique author emails for all commits on
// branch that are not reachable from base. This identifies who contributed
// to the branch since it diverged.
func CommitAuthors(repoPath, branch, base string) ([]string, error) {
	out, err := run(repoPath, "log", "--format=%ae", base+".."+branch)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	seen := make(map[string]bool)
	var authors []string
	for _, email := range splitNonEmpty(out) {
		if !seen[email] {
			seen[email] = true
			authors = append(authors, email)
		}
	}
	return authors, nil
}

// HasUpstream returns true if the given branch has a remote tracking branch configured.
func HasUpstream(repoPath, branch string) bool {
	_, err := run(repoPath, "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	return err == nil
}

// splitNonEmpty splits a newline-separated string and returns non-empty lines.
func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
