package branches_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/agrahamlincoln/katazuke/internal/branches"
	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/test/helpers"
)

func TestFindMerged_NoMergedBranches(t *testing.T) {
	repo := helpers.NewTestRepo(t, "no-merged")

	// Create an unmerged branch.
	repo.CreateBranch("feature/wip")
	repo.WriteFile("wip.txt", "work in progress")
	repo.AddFile("wip.txt")
	repo.Commit("wip commit")
	repo.Checkout("main")

	results, err := branches.FindMerged([]string{repo.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no merged branches, got %d: %v", len(results), results)
	}
}

func TestFindMerged_OneMergedBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "one-merged")

	// Create and merge a feature branch.
	repo.CreateBranch("feature/done")
	repo.WriteFile("done.txt", "completed work")
	repo.AddFile("done.txt")
	repo.Commit("done commit")
	repo.Checkout("main")
	repo.Merge("feature/done")

	results, err := branches.FindMerged([]string{repo.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 merged branch, got %d", len(results))
	}
	if results[0].Branch != "feature/done" {
		t.Errorf("expected branch feature/done, got %q", results[0].Branch)
	}
	if results[0].RepoName != "one-merged" {
		t.Errorf("expected repo name one-merged, got %q", results[0].RepoName)
	}
	if results[0].ForceDelete {
		t.Error("expected ForceDelete=false for git-detected merged branch")
	}
}

func TestFindMerged_ExcludesDefaultAndCurrentBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "exclude-special")

	// Merge a feature branch so "main" appears in merged list.
	repo.CreateBranch("feature/merged")
	repo.WriteFile("m.txt", "merged")
	repo.AddFile("m.txt")
	repo.Commit("merged commit")
	repo.Checkout("main")
	repo.Merge("feature/merged")

	results, err := branches.FindMerged([]string{repo.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range results {
		if r.Branch == "main" {
			t.Error("default branch 'main' should be excluded from results")
		}
	}
	// Only feature/merged should appear.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Branch != "feature/merged" {
		t.Errorf("expected feature/merged, got %q", results[0].Branch)
	}
}

func TestFindMerged_MultipleRepos(t *testing.T) {
	repo1 := helpers.NewTestRepo(t, "repo-one")
	repo2 := helpers.NewTestRepo(t, "repo-two")

	// Merge a branch in repo1.
	repo1.CreateBranch("feature/a")
	repo1.WriteFile("a.txt", "a")
	repo1.AddFile("a.txt")
	repo1.Commit("commit a")
	repo1.Checkout("main")
	repo1.Merge("feature/a")

	// Merge two branches in repo2.
	repo2.CreateBranch("feature/b")
	repo2.WriteFile("b.txt", "b")
	repo2.AddFile("b.txt")
	repo2.Commit("commit b")
	repo2.Checkout("main")
	repo2.Merge("feature/b")

	repo2.CreateBranch("feature/c")
	repo2.WriteFile("c.txt", "c")
	repo2.AddFile("c.txt")
	repo2.Commit("commit c")
	repo2.Checkout("main")
	repo2.Merge("feature/c")

	results, err := branches.FindMerged([]string{repo1.Path, repo2.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 merged branches, got %d", len(results))
	}

	branchNames := make(map[string]bool)
	for _, r := range results {
		branchNames[r.Branch] = true
	}
	for _, want := range []string{"feature/a", "feature/b", "feature/c"} {
		if !branchNames[want] {
			t.Errorf("expected branch %q in results", want)
		}
	}
}

func TestFindMerged_CommitDateIsPopulated(t *testing.T) {
	repo := helpers.NewTestRepo(t, "dated-merge")

	target := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	repo.CreateBranch("feature/dated")
	repo.WriteFile("dated.txt", "dated")
	repo.AddFile("dated.txt")
	repo.CommitWithDate("dated commit", target)
	repo.Checkout("main")
	repo.Merge("feature/dated")

	results, err := branches.FindMerged([]string{repo.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	diff := results[0].LastCommit.Sub(target)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expected commit date near %v, got %v", target, results[0].LastCommit)
	}
}

func TestFindMerged_EmptyRepoList(t *testing.T) {
	results, err := branches.FindMerged(nil, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty repo list, got %d", len(results))
	}
}

func TestMergedBranch_Label(t *testing.T) {
	mb := branches.MergedBranch{
		RepoName: "my-repo",
		Branch:   "feature/test",
	}
	want := "my-repo: feature/test"
	if got := mb.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestMergedBranch_LabelWithRemote(t *testing.T) {
	mb := branches.MergedBranch{
		RepoName:  "my-repo",
		Branch:    "feature/test",
		HasRemote: true,
	}
	want := "my-repo: feature/test (+ remote)"
	if got := mb.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestFindMerged_HasRemoteField(t *testing.T) {
	// Create a bare remote and a clone with a proper origin.
	origin := helpers.NewTestRepo(t, "remote-merged-origin")

	tmpDir := t.TempDir()
	barePath := filepath.Join(tmpDir, "remote-merged-bare.git")

	// #nosec G204 - git command with controlled inputs in test code
	cmd := exec.Command("git", "clone", "--bare", origin.Path, barePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create bare clone: %v\n%s", err, out)
	}

	clonePath := filepath.Join(tmpDir, "remote-merged-clone")
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

	// Create a branch, push it to origin, then merge it locally.
	gitRun(t, clonePath, "checkout", "-b", "feature/pushed")
	writeFile(t, clonePath, "pushed.txt", "pushed content")
	gitRun(t, clonePath, "add", "pushed.txt")
	gitRun(t, clonePath, "commit", "-m", "pushed commit")
	gitRun(t, clonePath, "push", "origin", "feature/pushed")
	gitRun(t, clonePath, "checkout", "main")
	gitRun(t, clonePath, "merge", "--no-ff", "feature/pushed", "-m", "Merge feature/pushed")

	// Create another branch, merge it, but do NOT push to origin.
	gitRun(t, clonePath, "checkout", "-b", "feature/local-only")
	writeFile(t, clonePath, "local.txt", "local content")
	gitRun(t, clonePath, "add", "local.txt")
	gitRun(t, clonePath, "commit", "-m", "local commit")
	gitRun(t, clonePath, "checkout", "main")
	gitRun(t, clonePath, "merge", "--no-ff", "feature/local-only", "-m", "Merge feature/local-only")

	results, err := branches.FindMerged([]string{clonePath}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 merged branches, got %d", len(results))
	}

	byBranch := make(map[string]branches.MergedBranch)
	for _, r := range results {
		byBranch[r.Branch] = r
	}

	pushed, ok := byBranch["feature/pushed"]
	if !ok {
		t.Fatal("expected feature/pushed in results")
	}
	if !pushed.HasRemote {
		t.Error("expected feature/pushed to have HasRemote=true")
	}

	localOnly, ok := byBranch["feature/local-only"]
	if !ok {
		t.Fatal("expected feature/local-only in results")
	}
	if localOnly.HasRemote {
		t.Error("expected feature/local-only to have HasRemote=false")
	}
}

func TestFindMerged_HasRemoteFalseWithoutOrigin(t *testing.T) {
	// A repo with no remotes should always have HasRemote=false.
	repo := helpers.NewTestRepo(t, "no-remote")

	repo.CreateBranch("feature/done")
	repo.WriteFile("done.txt", "done")
	repo.AddFile("done.txt")
	repo.Commit("done commit")
	repo.Checkout("main")
	repo.Merge("feature/done")

	results, err := branches.FindMerged([]string{repo.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].HasRemote {
		t.Error("expected HasRemote=false for repo without origin")
	}
}

func TestFindMerged_DetachedHEAD(t *testing.T) {
	repo := helpers.NewTestRepo(t, "detached-head")

	// Create and merge a feature branch.
	repo.CreateBranch("feature/done")
	repo.WriteFile("done.txt", "done")
	repo.AddFile("done.txt")
	repo.Commit("done commit")
	repo.Checkout("main")
	repo.Merge("feature/done")

	// Detach HEAD.
	repo.DetachHead()

	results, err := branches.FindMerged([]string{repo.Path}, merge.GitOnlyDetector(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// feature/done should still appear as merged; detached HEAD should not
	// cause an error or exclude any valid branch.
	if len(results) != 1 {
		t.Fatalf("expected 1 merged branch, got %d", len(results))
	}
	if results[0].Branch != "feature/done" {
		t.Errorf("expected feature/done, got %q", results[0].Branch)
	}
}

// gitRun is a test helper that runs a git command in the given directory.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	// #nosec G204 - git command with controlled inputs in test code
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// writeFile is a test helper that writes content to a file in the given directory.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}
