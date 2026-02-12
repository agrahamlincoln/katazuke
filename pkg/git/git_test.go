package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/agrahamlincoln/katazuke/pkg/git"
	"github.com/agrahamlincoln/katazuke/test/helpers"
)

func TestIsRepo(t *testing.T) {
	repo := helpers.NewTestRepo(t, "is-repo")
	if !git.IsRepo(repo.Path) {
		t.Error("expected path to be a git repo")
	}

	nonRepo := t.TempDir()
	if git.IsRepo(nonRepo) {
		t.Error("expected non-repo path to not be a git repo")
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "current-branch")
	branch, err := git.CurrentBranch(repo.Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected main, got %q", branch)
	}
}

func TestDefaultBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "default-branch")
	branch, err := git.DefaultBranch(repo.Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected main, got %q", branch)
	}
}

func TestListBranches(t *testing.T) {
	repo := helpers.NewTestRepo(t, "list-branches")
	repo.CreateBranch("feature/one")
	repo.Checkout("main")
	repo.CreateBranch("feature/two")
	repo.Checkout("main")

	branches, err := git.ListBranches(repo.Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{"main": true, "feature/one": true, "feature/two": true}
	if len(branches) != len(want) {
		t.Fatalf("expected %d branches, got %d: %v", len(want), len(branches), branches)
	}
	for _, b := range branches {
		if !want[b] {
			t.Errorf("unexpected branch %q", b)
		}
	}
}

func TestMergedBranches(t *testing.T) {
	repo := helpers.NewTestRepo(t, "merged-branches")

	// Create and merge a feature branch.
	repo.CreateBranch("feature/done")
	repo.WriteFile("done.txt", "done")
	repo.AddFile("done.txt")
	repo.Commit("Add done feature")
	repo.Checkout("main")
	repo.Merge("feature/done")

	// Create an unmerged branch.
	repo.CreateBranch("feature/wip")
	repo.WriteFile("wip.txt", "wip")
	repo.AddFile("wip.txt")
	repo.Commit("Add wip feature")
	repo.Checkout("main")

	merged, err := git.MergedBranches(repo.Path, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mergedSet := make(map[string]bool)
	for _, m := range merged {
		mergedSet[m] = true
	}
	if !mergedSet["feature/done"] {
		t.Error("expected feature/done to be merged")
	}
	if mergedSet["feature/wip"] {
		t.Error("expected feature/wip to NOT be merged")
	}
}

func TestIsMerged(t *testing.T) {
	repo := helpers.NewTestRepo(t, "is-merged")

	repo.CreateBranch("feature/merged")
	repo.WriteFile("m.txt", "merged")
	repo.AddFile("m.txt")
	repo.Commit("merged work")
	repo.Checkout("main")
	repo.Merge("feature/merged")

	ok, err := git.IsMerged(repo.Path, "feature/merged", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected branch to be merged")
	}

	ok, err = git.IsMerged(repo.Path, "nonexistent", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected nonexistent branch to not be merged")
	}
}

func TestCommitDate(t *testing.T) {
	repo := helpers.NewTestRepo(t, "commit-date")

	target := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	repo.CreateBranch("feature/dated")
	repo.WriteFile("dated.txt", "dated")
	repo.AddFile("dated.txt")
	repo.CommitWithDate("Dated commit", target)

	got, err := git.CommitDate(repo.Path, "feature/dated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compare to within a second (timezone normalization).
	diff := got.Sub(target)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expected commit date near %v, got %v", target, got)
	}
}

func TestIsClean(t *testing.T) {
	repo := helpers.NewTestRepo(t, "is-clean")

	clean, err := git.IsClean(repo.Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clean {
		t.Error("expected clean repo")
	}

	repo.WriteFile("dirty.txt", "uncommitted")
	clean, err = git.IsClean(repo.Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean {
		t.Error("expected dirty repo")
	}
}

func TestDeleteLocalBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "delete-branch")

	repo.CreateBranch("feature/to-delete")
	repo.WriteFile("del.txt", "delete me")
	repo.AddFile("del.txt")
	repo.Commit("to delete")
	repo.Checkout("main")
	repo.Merge("feature/to-delete")

	err := git.DeleteLocalBranch(repo.Path, "feature/to-delete", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	branches, _ := git.ListBranches(repo.Path)
	for _, b := range branches {
		if b == "feature/to-delete" {
			t.Error("branch should have been deleted")
		}
	}
}

// setupRemotePair creates a bare "remote" repo and a clone that uses it as origin.
// Returns the clone path and the bare remote path.
func setupRemotePair(t *testing.T, name string) (string, string) {
	t.Helper()

	// Create a normal repo first, then clone it bare, then clone that.
	origin := helpers.NewTestRepo(t, name+"-origin")

	tmpDir := t.TempDir()
	barePath := filepath.Join(tmpDir, name+"-bare.git")

	// Clone to bare repo.
	// #nosec G204 - git command with controlled inputs in test code
	cmd := exec.Command("git", "clone", "--bare", origin.Path, barePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create bare clone: %v\n%s", err, out)
	}

	// Clone the bare repo to get a working copy with a proper remote.
	clonePath := filepath.Join(tmpDir, name+"-clone")
	// #nosec G204 - git command with controlled inputs in test code
	cmd = exec.Command("git", "clone", barePath, clonePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone bare repo: %v\n%s", err, out)
	}

	// Set git identity in the clone.
	for _, kv := range [][2]string{{"user.name", "Test User"}, {"user.email", "test@example.com"}} {
		// #nosec G204 - git command with controlled inputs in test code
		cmd = exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = clonePath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to set git config: %v\n%s", err, out)
		}
	}

	return clonePath, barePath
}

// pushToRemote pushes a branch from a repo to a bare remote.
func pushToRemote(t *testing.T, repoPath, remote, branch string) {
	t.Helper()
	cmd := exec.Command("git", "push", remote, branch)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to push: %v\n%s", err, out)
	}
}

func TestPull(t *testing.T) {
	clonePath, barePath := setupRemotePair(t, "pull")

	// Push a new commit to the bare remote from a separate clone.
	tmpDir := t.TempDir()
	pusherPath := filepath.Join(tmpDir, "pusher")
	// #nosec G204 - git command with controlled inputs in test code
	cmd := exec.Command("git", "clone", barePath, pusherPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone for push: %v\n%s", err, out)
	}
	for _, kv := range [][2]string{{"user.name", "Test User"}, {"user.email", "test@example.com"}} {
		// #nosec G204 - git command with controlled inputs in test code
		cmd = exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = pusherPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to set git config: %v\n%s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(pusherPath, "new.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "add", "new.txt")
	cmd.Dir = pusherPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "upstream commit")
	cmd.Dir = pusherPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to commit: %v\n%s", err, out)
	}
	pushToRemote(t, pusherPath, "origin", "main")

	t.Run("rebase", func(t *testing.T) {
		err := git.Pull(clonePath, "rebase")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify the new file is present.
		if _, err := os.Stat(filepath.Join(clonePath, "new.txt")); err != nil {
			t.Error("expected new.txt to exist after pull")
		}
	})

	t.Run("invalid_strategy", func(t *testing.T) {
		err := git.Pull(clonePath, "invalid")
		if err == nil {
			t.Error("expected error for invalid strategy")
		}
	})
}

func TestStashPushPop(t *testing.T) {
	repo := helpers.NewTestRepo(t, "stash")

	// Create an uncommitted change.
	repo.WriteFile("wip.txt", "work in progress")
	repo.AddFile("wip.txt")

	// Stash the change.
	err := git.StashPush(repo.Path, "test stash")
	if err != nil {
		t.Fatalf("StashPush error: %v", err)
	}

	// Working tree should be clean after stash.
	clean, err := git.IsClean(repo.Path)
	if err != nil {
		t.Fatalf("IsClean error: %v", err)
	}
	if !clean {
		t.Error("expected clean working tree after stash push")
	}

	// Pop the stash.
	err = git.StashPop(repo.Path)
	if err != nil {
		t.Fatalf("StashPop error: %v", err)
	}

	// Working tree should be dirty again.
	clean, err = git.IsClean(repo.Path)
	if err != nil {
		t.Fatalf("IsClean error: %v", err)
	}
	if clean {
		t.Error("expected dirty working tree after stash pop")
	}
}

func TestMergeBase(t *testing.T) {
	repo := helpers.NewTestRepo(t, "merge-base")

	// Create a branch with a diverging commit.
	repo.CreateBranch("feature/diverge")
	repo.WriteFile("feature.txt", "feature work")
	repo.AddFile("feature.txt")
	repo.Commit("feature commit")
	repo.Checkout("main")

	base, err := git.MergeBase(repo.Path, "main", "feature/diverge")
	if err != nil {
		t.Fatalf("MergeBase error: %v", err)
	}
	if base == "" {
		t.Error("expected non-empty merge base")
	}

	// The merge base should be the tip of main (since feature branched from it).
	mainHead, err := run(repo.Path, "rev-parse", "main")
	if err != nil {
		t.Fatalf("rev-parse error: %v", err)
	}
	if base != mainHead {
		t.Errorf("expected merge base %q, got %q", mainHead, base)
	}
}

// run is a test helper that runs git in the given dir.
func run(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out[:len(out)-1]), nil // trim trailing newline
}

func TestCommitsAheadBehind(t *testing.T) {
	repo := helpers.NewTestRepo(t, "ahead-behind")

	// Create a branch with 2 commits ahead of main.
	repo.CreateBranch("feature/ahead")
	repo.WriteFile("f1.txt", "first")
	repo.AddFile("f1.txt")
	repo.Commit("first feature commit")
	repo.WriteFile("f2.txt", "second")
	repo.AddFile("f2.txt")
	repo.Commit("second feature commit")
	repo.Checkout("main")

	// Add 1 commit to main so the branch is behind.
	repo.WriteFile("main.txt", "main work")
	repo.AddFile("main.txt")
	repo.Commit("main commit")

	ahead, behind, err := git.CommitsAheadBehind(repo.Path, "feature/ahead", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ahead != 2 {
		t.Errorf("expected 2 ahead, got %d", ahead)
	}
	if behind != 1 {
		t.Errorf("expected 1 behind, got %d", behind)
	}
}

func TestCommitsAheadBehind_NoDivergence(t *testing.T) {
	repo := helpers.NewTestRepo(t, "no-diverge")

	repo.CreateBranch("feature/same")
	repo.Checkout("main")

	ahead, behind, err := git.CommitsAheadBehind(repo.Path, "feature/same", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ahead != 0 {
		t.Errorf("expected 0 ahead, got %d", ahead)
	}
	if behind != 0 {
		t.Errorf("expected 0 behind, got %d", behind)
	}
}

func TestHasRemoteBranch(t *testing.T) {
	clonePath, _ := setupRemotePair(t, "has-remote-branch")

	// "main" should exist on origin.
	has, err := git.HasRemoteBranch(clonePath, "origin", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected origin/main to exist")
	}

	// A non-existent branch should not exist.
	has, err = git.HasRemoteBranch(clonePath, "origin", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected origin/nonexistent to not exist")
	}
}

func TestCreateTag(t *testing.T) {
	repo := helpers.NewTestRepo(t, "create-tag")

	repo.CreateBranch("feature/to-archive")
	repo.WriteFile("archive.txt", "archive me")
	repo.AddFile("archive.txt")
	repo.Commit("archive commit")
	repo.Checkout("main")

	err := git.CreateTag(repo.Path, "archive/feature/to-archive", "feature/to-archive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the tag exists.
	out, err := run(repo.Path, "tag", "-l", "archive/feature/to-archive")
	if err != nil {
		t.Fatalf("unexpected error listing tags: %v", err)
	}
	if out != "archive/feature/to-archive" {
		t.Errorf("expected tag archive/feature/to-archive, got %q", out)
	}
}

func TestCommitSubject(t *testing.T) {
	repo := helpers.NewTestRepo(t, "commit-subject")

	repo.CreateBranch("feature/subject")
	repo.WriteFile("subject.txt", "test")
	repo.AddFile("subject.txt")
	repo.Commit("This is the subject line")

	subject, err := git.CommitSubject(repo.Path, "feature/subject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subject != "This is the subject line" {
		t.Errorf("expected %q, got %q", "This is the subject line", subject)
	}
}

func TestMergeTree(t *testing.T) {
	t.Run("no_conflict", func(t *testing.T) {
		repo := helpers.NewTestRepo(t, "merge-tree-clean")

		// Record the merge base (initial commit on main).
		baseRef, err := run(repo.Path, "rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("rev-parse error: %v", err)
		}

		// Create a branch that modifies a different file.
		repo.CreateBranch("feature/a")
		repo.WriteFile("a.txt", "aaa")
		repo.AddFile("a.txt")
		repo.Commit("add a")
		repo.Checkout("main")

		// Add a different change on main.
		repo.WriteFile("b.txt", "bbb")
		repo.AddFile("b.txt")
		repo.Commit("add b")

		_, hasConflicts, err := git.MergeTree(repo.Path, baseRef, "main", "feature/a")
		if err != nil {
			t.Fatalf("MergeTree error: %v", err)
		}
		if hasConflicts {
			t.Error("expected no conflicts for non-overlapping changes")
		}
	})

	t.Run("with_conflict", func(t *testing.T) {
		repo := helpers.NewTestRepo(t, "merge-tree-conflict")

		baseRef, err := run(repo.Path, "rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("rev-parse error: %v", err)
		}

		// Create a branch that modifies README.md.
		repo.CreateBranch("feature/conflict")
		repo.WriteFile("README.md", "feature version\n")
		repo.AddFile("README.md")
		repo.Commit("feature change to README")
		repo.Checkout("main")

		// Modify the same file on main.
		repo.WriteFile("README.md", "main version\n")
		repo.AddFile("README.md")
		repo.Commit("main change to README")

		_, hasConflicts, err := git.MergeTree(repo.Path, baseRef, "main", "feature/conflict")
		if err != nil {
			t.Fatalf("MergeTree error: %v", err)
		}
		if !hasConflicts {
			t.Error("expected conflicts for overlapping changes")
		}
	})
}

func TestConfigValue(t *testing.T) {
	repo := helpers.NewTestRepo(t, "config-value")

	email, err := git.ConfigValue(repo.Path, "user.email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "test@example.com" {
		t.Errorf("expected test@example.com, got %q", email)
	}
}

func TestCommitAuthors(t *testing.T) {
	repo := helpers.NewTestRepo(t, "commit-authors")

	repo.CreateBranch("feature/authored")
	repo.WriteFile("a.txt", "a")
	repo.AddFile("a.txt")
	repo.Commit("commit by test user")
	repo.WriteFile("b.txt", "b")
	repo.AddFile("b.txt")
	repo.Commit("another commit by test user")
	repo.Checkout("main")

	authors, err := git.CommitAuthors(repo.Path, "feature/authored", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authors) != 1 {
		t.Fatalf("expected 1 unique author, got %d: %v", len(authors), authors)
	}
	if authors[0] != "test@example.com" {
		t.Errorf("expected test@example.com, got %q", authors[0])
	}
}

func TestCommitAuthors_NoUniqueCommits(t *testing.T) {
	repo := helpers.NewTestRepo(t, "no-unique-commits")

	// Branch at same point as main -- no unique commits.
	repo.CreateBranch("feature/same")
	repo.Checkout("main")

	authors, err := git.CommitAuthors(repo.Path, "feature/same", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authors) != 0 {
		t.Errorf("expected no authors for branch with no unique commits, got %v", authors)
	}
}

func TestHasUpstream(t *testing.T) {
	repo := helpers.NewTestRepo(t, "has-upstream")

	// A local branch with no remote should not have an upstream.
	repo.CreateBranch("feature/local")
	repo.Checkout("main")

	if git.HasUpstream(repo.Path, "feature/local") {
		t.Error("expected feature/local to have no upstream")
	}
}

func TestHasUpstream_WithRemote(t *testing.T) {
	clonePath, _ := setupRemotePair(t, "upstream-remote")

	// "main" in a clone should have an upstream.
	if !git.HasUpstream(clonePath, "main") {
		t.Error("expected main to have an upstream in a clone")
	}
}
