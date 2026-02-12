// Package git provides functions for interacting with git repositories
// by shelling out to the git CLI.
package git

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// run executes a git command in the given directory and returns its output.
func run(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
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
	return splitNonEmpty(out), nil
}

// MergedBranches returns local branches that have been merged into the given base branch.
func MergedBranches(repoPath, base string) ([]string, error) {
	out, err := run(repoPath, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(out), nil
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
func StashPush(repoPath string, message string) error {
	_, err := run(repoPath, "stash", "push", "-m", message)
	return err
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
