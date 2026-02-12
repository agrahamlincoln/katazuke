package repos_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agrahamlincoln/katazuke/internal/repos"
)

// mockChecker implements repos.ArchiveChecker for testing.
type mockChecker struct {
	archived map[string]bool
	err      map[string]error
}

func (m *mockChecker) IsArchived(owner, repo string) (bool, error) {
	key := owner + "/" + repo
	if e, ok := m.err[key]; ok {
		return false, e
	}
	return m.archived[key], nil
}

// initRepoWithRemote creates a git repo at path with a GitHub remote.
func initRepoWithRemote(t *testing.T, path, remoteURL string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	gitInit(t, path)
	gitRun(t, path, "remote", "add", "origin", remoteURL)
	// Create an initial commit so status works.
	gitRun(t, path, "commit", "--allow-empty", "-m", "initial")
}

// initRepoNoRemote creates a git repo at path without a remote.
func initRepoNoRemote(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	gitInit(t, path)
	gitRun(t, path, "commit", "--allow-empty", "-m", "initial")
}

func gitInit(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", path, err, out)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func TestFindArchived(t *testing.T) {
	root := t.TempDir()

	// Archived repo with clean working tree.
	archivedClean := filepath.Join(root, "archived-clean")
	initRepoWithRemote(t, archivedClean, "git@github.com:owner/archived-clean.git")

	// Archived repo with dirty working tree.
	archivedDirty := filepath.Join(root, "archived-dirty")
	initRepoWithRemote(t, archivedDirty, "git@github.com:owner/archived-dirty.git")
	if err := os.WriteFile(filepath.Join(archivedDirty, "uncommitted.txt"), []byte("dirty"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Non-archived repo.
	active := filepath.Join(root, "active")
	initRepoWithRemote(t, active, "git@github.com:owner/active.git")

	// Repo without remote.
	noRemote := filepath.Join(root, "no-remote")
	initRepoNoRemote(t, noRemote)

	// Non-GitHub remote.
	gitlab := filepath.Join(root, "gitlab")
	initRepoWithRemote(t, gitlab, "git@gitlab.com:owner/gitlab.git")

	checker := &mockChecker{
		archived: map[string]bool{
			"owner/archived-clean": true,
			"owner/archived-dirty": true,
			"owner/active":         false,
		},
	}

	repoPaths := []string{archivedClean, archivedDirty, active, noRemote, gitlab}
	result, err := repos.FindArchived(repoPaths, checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 archived repos, got %d: %+v", len(result), result)
	}

	// Verify order matches input order.
	if result[0].Name != "archived-clean" {
		t.Errorf("expected first result to be archived-clean, got %s", result[0].Name)
	}
	if !result[0].IsClean {
		t.Error("expected archived-clean to be clean")
	}

	if result[1].Name != "archived-dirty" {
		t.Errorf("expected second result to be archived-dirty, got %s", result[1].Name)
	}
	if result[1].IsClean {
		t.Error("expected archived-dirty to be dirty")
	}
}

func TestFindArchivedAPIError(t *testing.T) {
	root := t.TempDir()

	errRepo := filepath.Join(root, "err-repo")
	initRepoWithRemote(t, errRepo, "git@github.com:owner/err-repo.git")

	checker := &mockChecker{
		archived: map[string]bool{},
		err: map[string]error{
			"owner/err-repo": fmt.Errorf("API rate limited"),
		},
	}

	result, err := repos.FindArchived([]string{errRepo}, checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// API errors should be skipped gracefully.
	if len(result) != 0 {
		t.Fatalf("expected 0 results when API errors, got %d", len(result))
	}
}

func TestFindArchivedEmpty(t *testing.T) {
	checker := &mockChecker{archived: map[string]bool{}}

	result, err := repos.FindArchived(nil, checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", len(result))
	}
}
