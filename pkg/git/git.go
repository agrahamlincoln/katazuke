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
