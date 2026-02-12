package git_test

import (
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
