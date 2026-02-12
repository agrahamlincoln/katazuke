// Package helpers provides test utilities for creating git repositories and scenarios.
package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRepo represents a test git repository
type TestRepo struct {
	Path string
	t    *testing.T
}

// NewTestRepo creates a new test repository in a temporary directory
func NewTestRepo(t *testing.T, name string) *TestRepo {
	t.Helper()

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, name)

	if err := os.MkdirAll(repoPath, 0750); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	repo := &TestRepo{
		Path: repoPath,
		t:    t,
	}

	// Initialize git repo
	repo.run("git", "init")
	repo.run("git", "config", "user.name", "Test User")
	repo.run("git", "config", "user.email", "test@example.com")

	// Create initial commit
	repo.WriteFile("README.md", "# Test Repository\n")
	repo.run("git", "add", "README.md")
	repo.CommitWithDate("Initial commit", time.Now())

	return repo
}

// WriteFile writes a file to the repository
func (r *TestRepo) WriteFile(filename, content string) {
	r.t.Helper()
	path := filepath.Join(r.Path, filename)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		r.t.Fatalf("Failed to write file %s: %v", filename, err)
	}
}

// AddFile stages a file for commit
func (r *TestRepo) AddFile(filename string) {
	r.t.Helper()
	r.run("git", "add", filename)
}

// Commit creates a commit with the current timestamp
func (r *TestRepo) Commit(message string) {
	r.t.Helper()
	r.CommitWithDate(message, time.Now())
}

// CommitWithDate creates a commit with a specific timestamp
// This is crucial for testing stale branch detection without waiting 30 days!
func (r *TestRepo) CommitWithDate(message string, date time.Time) {
	r.t.Helper()
	dateStr := date.Format(time.RFC3339)
	// #nosec G204 - git command with controlled inputs in test code
	cmd := exec.Command("git", "commit", "-m", message, "--date", dateStr)
	cmd.Dir = r.Path
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GIT_AUTHOR_DATE=%s", dateStr),
		fmt.Sprintf("GIT_COMMITTER_DATE=%s", dateStr),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("Failed to commit: %v\n%s", err, output)
	}
}

// CreateBranch creates a new branch
func (r *TestRepo) CreateBranch(name string) {
	r.t.Helper()
	r.run("git", "checkout", "-b", name)
}

// Checkout switches to a branch
func (r *TestRepo) Checkout(branch string) {
	r.t.Helper()
	r.run("git", "checkout", branch)
}

// Merge merges a branch into the current branch
func (r *TestRepo) Merge(branch string) {
	r.t.Helper()
	r.run("git", "merge", "--no-ff", branch, "-m", fmt.Sprintf("Merge branch '%s'", branch))
}

// AddRemote adds a remote to the repository
func (r *TestRepo) AddRemote(name, url string) {
	r.t.Helper()
	r.run("git", "remote", "add", name, url)
}

// Push pushes to a remote
func (r *TestRepo) Push(remote, branch string) {
	r.t.Helper()
	r.run("git", "push", remote, branch)
}

// CurrentBranch returns the current branch name
func (r *TestRepo) CurrentBranch() string {
	r.t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("Failed to get current branch: %v", err)
	}
	return string(output)
}

// Branches returns a list of all branch names
func (r *TestRepo) Branches() []string {
	r.t.Helper()
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("Failed to list branches: %v", err)
	}

	var branches []string
	for _, line := range splitLines(string(output)) {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// run executes a git command in the repository
func (r *TestRepo) run(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("Git command failed: git %v\n%s", args, output)
	}
}

// splitLines splits a string by newlines
func splitLines(s string) []string {
	var lines []string
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] != '\n' {
			j++
		}
		lines = append(lines, s[i:j])
		i = j + 1
	}
	return lines
}
