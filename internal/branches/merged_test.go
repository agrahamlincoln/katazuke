package branches_test

import (
	"testing"
	"time"

	"github.com/agrahamlincoln/katazuke/internal/branches"
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

	results, err := branches.FindMerged([]string{repo.Path}, 1)
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

	results, err := branches.FindMerged([]string{repo.Path}, 1)
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

	results, err := branches.FindMerged([]string{repo.Path}, 1)
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

	results, err := branches.FindMerged([]string{repo1.Path, repo2.Path}, 1)
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

	results, err := branches.FindMerged([]string{repo.Path}, 1)
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
	results, err := branches.FindMerged(nil, 1)
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
