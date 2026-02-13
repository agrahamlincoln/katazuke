package repos_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agrahamlincoln/katazuke/internal/repos"
	"github.com/agrahamlincoln/katazuke/test/helpers"
)

func TestFindOnMergedBranch(t *testing.T) {
	// Repo on a merged branch with clean working tree.
	merged := helpers.NewTestRepo(t, "merged-repo")
	merged.CreateBranch("feature/done")
	merged.WriteFile("feature.txt", "feature work")
	merged.AddFile("feature.txt")
	merged.Commit("feature work")
	merged.Checkout("main")
	merged.Merge("feature/done")
	merged.Checkout("feature/done")

	// Repo on an unmerged branch.
	unmerged := helpers.NewTestRepo(t, "unmerged-repo")
	unmerged.CreateBranch("feature/wip")
	unmerged.WriteFile("wip.txt", "wip work")
	unmerged.AddFile("wip.txt")
	unmerged.Commit("wip work")

	// Repo on the default branch.
	onDefault := helpers.NewTestRepo(t, "on-default")

	repoPaths := []string{merged.Path, unmerged.Path, onDefault.Path}
	result := repos.FindOnMergedBranch(repoPaths, 1, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 merged branch repo, got %d", len(result))
	}

	if result[0].Name != "merged-repo" {
		t.Errorf("expected merged-repo, got %s", result[0].Name)
	}
	if result[0].CurrentBranch != "feature/done" {
		t.Errorf("expected current branch feature/done, got %s", result[0].CurrentBranch)
	}
	if result[0].DefaultBranch != "main" {
		t.Errorf("expected default branch main, got %s", result[0].DefaultBranch)
	}
	if !result[0].IsClean {
		t.Error("expected clean working tree")
	}
}

func TestFindOnMergedBranchDirty(t *testing.T) {
	// Repo on a merged branch with dirty working tree.
	dirtyMerged := helpers.NewTestRepo(t, "dirty-merged")
	dirtyMerged.CreateBranch("feature/done")
	dirtyMerged.WriteFile("feature.txt", "feature work")
	dirtyMerged.AddFile("feature.txt")
	dirtyMerged.Commit("feature work")
	dirtyMerged.Checkout("main")
	dirtyMerged.Merge("feature/done")
	dirtyMerged.Checkout("feature/done")
	if err := os.WriteFile(filepath.Join(dirtyMerged.Path, "uncommitted.txt"), []byte("dirty"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := repos.FindOnMergedBranch([]string{dirtyMerged.Path}, 1, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].IsClean {
		t.Error("expected dirty working tree")
	}
}

func TestFindOnMergedBranch_DetachedHEAD(t *testing.T) {
	repo := helpers.NewTestRepo(t, "detached-head")

	// Create and merge a feature branch.
	repo.CreateBranch("feature/done")
	repo.WriteFile("feature.txt", "feature work")
	repo.AddFile("feature.txt")
	repo.Commit("feature work")
	repo.Checkout("main")
	repo.Merge("feature/done")

	// Detach HEAD -- not "on a merged branch", should return no results.
	repo.DetachHead()

	result := repos.FindOnMergedBranch([]string{repo.Path}, 1, nil)

	if len(result) != 0 {
		t.Fatalf("expected 0 results for detached HEAD, got %d", len(result))
	}
}

func TestFindOnMergedBranchEmpty(t *testing.T) {
	result := repos.FindOnMergedBranch(nil, 1, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", len(result))
	}
}
