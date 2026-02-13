package branches_test

import (
	"testing"
	"time"

	"github.com/agrahamlincoln/katazuke/internal/branches"
	"github.com/agrahamlincoln/katazuke/test/helpers"
)

func TestFindStale_NoStaleBranches(t *testing.T) {
	repo := helpers.NewTestRepo(t, "no-stale")

	// Create a branch with a recent commit (now).
	repo.CreateBranch("feature/active")
	repo.WriteFile("active.txt", "active work")
	repo.AddFile("active.txt")
	repo.Commit("active commit")
	repo.Checkout("main")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no stale branches, got %d: %v", len(results), results)
	}
}

func TestFindStale_OneStaleBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "one-stale")

	// Create a branch with an old commit (60 days ago).
	staleDate := time.Now().Add(-60 * 24 * time.Hour)
	repo.CreateBranch("feature/old")
	repo.WriteFile("old.txt", "old work")
	repo.AddFile("old.txt")
	repo.CommitWithDate("old commit", staleDate)
	repo.Checkout("main")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch, got %d", len(results))
	}
	if results[0].Branch != "feature/old" {
		t.Errorf("expected branch feature/old, got %q", results[0].Branch)
	}
	if results[0].RepoName != "one-stale" {
		t.Errorf("expected repo name one-stale, got %q", results[0].RepoName)
	}
	if results[0].LastCommitMessage != "old commit" {
		t.Errorf("expected commit message %q, got %q", "old commit", results[0].LastCommitMessage)
	}
}

func TestFindStale_ExcludesMergedBranches(t *testing.T) {
	repo := helpers.NewTestRepo(t, "exclude-merged")

	// Create a branch with an old commit, then merge it.
	staleDate := time.Now().Add(-60 * 24 * time.Hour)
	repo.CreateBranch("feature/merged-old")
	repo.WriteFile("merged.txt", "merged")
	repo.AddFile("merged.txt")
	repo.CommitWithDate("merged commit", staleDate)
	repo.Checkout("main")
	repo.Merge("feature/merged-old")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no stale branches (merged should be excluded), got %d", len(results))
	}
}

func TestFindStale_ExcludesDefaultAndCurrentBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "exclude-special")

	// Create a stale branch, then switch back to main.
	staleDate := time.Now().Add(-60 * 24 * time.Hour)
	repo.CreateBranch("feature/stale")
	repo.WriteFile("stale.txt", "stale")
	repo.AddFile("stale.txt")
	repo.CommitWithDate("stale commit", staleDate)
	repo.Checkout("main")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.Branch == "main" {
			t.Error("default branch 'main' should be excluded from results")
		}
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch, got %d", len(results))
	}
	if results[0].Branch != "feature/stale" {
		t.Errorf("expected feature/stale, got %q", results[0].Branch)
	}
}

func TestFindStale_RespectsThreshold(t *testing.T) {
	repo := helpers.NewTestRepo(t, "threshold")

	// Create a branch that is 10 days old.
	tenDaysAgo := time.Now().Add(-10 * 24 * time.Hour)
	repo.CreateBranch("feature/ten-days")
	repo.WriteFile("ten.txt", "ten days")
	repo.AddFile("ten.txt")
	repo.CommitWithDate("ten day commit", tenDaysAgo)
	repo.Checkout("main")

	// With a 30-day threshold, this should not be stale.
	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no stale branches with 30-day threshold, got %d", len(results))
	}

	// With a 7-day threshold, this should be stale.
	results, err = branches.FindStale([]string{repo.Path}, 7*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch with 7-day threshold, got %d", len(results))
	}
}

func TestFindStale_MultipleRepos(t *testing.T) {
	repo1 := helpers.NewTestRepo(t, "repo-one")
	repo2 := helpers.NewTestRepo(t, "repo-two")

	staleDate := time.Now().Add(-60 * 24 * time.Hour)

	// One stale branch in repo1.
	repo1.CreateBranch("feature/old-a")
	repo1.WriteFile("a.txt", "a")
	repo1.AddFile("a.txt")
	repo1.CommitWithDate("old a", staleDate)
	repo1.Checkout("main")

	// Two stale branches in repo2.
	repo2.CreateBranch("feature/old-b")
	repo2.WriteFile("b.txt", "b")
	repo2.AddFile("b.txt")
	repo2.CommitWithDate("old b", staleDate)
	repo2.Checkout("main")

	repo2.CreateBranch("feature/old-c")
	repo2.WriteFile("c.txt", "c")
	repo2.AddFile("c.txt")
	repo2.CommitWithDate("old c", staleDate)
	repo2.Checkout("main")

	results, err := branches.FindStale([]string{repo1.Path, repo2.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 stale branches, got %d", len(results))
	}

	branchNames := make(map[string]bool)
	for _, r := range results {
		branchNames[r.Branch] = true
	}
	for _, want := range []string{"feature/old-a", "feature/old-b", "feature/old-c"} {
		if !branchNames[want] {
			t.Errorf("expected branch %q in results", want)
		}
	}
}

func TestFindStale_DetachedHEAD(t *testing.T) {
	repo := helpers.NewTestRepo(t, "detached-head")

	staleDate := time.Now().Add(-60 * 24 * time.Hour)
	repo.CreateBranch("feature/old")
	repo.WriteFile("old.txt", "old work")
	repo.AddFile("old.txt")
	repo.CommitWithDate("old commit", staleDate)
	repo.Checkout("main")

	// Detach HEAD.
	repo.DetachHead()

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// feature/old should still appear as stale; detached HEAD should not
	// cause an error or exclude any valid branch.
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch, got %d", len(results))
	}
	if results[0].Branch != "feature/old" {
		t.Errorf("expected feature/old, got %q", results[0].Branch)
	}
}

func TestFindStale_CommitsAheadBehind(t *testing.T) {
	repo := helpers.NewTestRepo(t, "ahead-behind")

	staleDate := time.Now().Add(-60 * 24 * time.Hour)

	// Create a stale branch with 2 commits.
	repo.CreateBranch("feature/diverged")
	repo.WriteFile("f1.txt", "first")
	repo.AddFile("f1.txt")
	repo.CommitWithDate("first feature commit", staleDate)
	repo.WriteFile("f2.txt", "second")
	repo.AddFile("f2.txt")
	repo.CommitWithDate("second feature commit", staleDate)
	repo.Checkout("main")

	// Add a commit to main so the branch is behind.
	repo.WriteFile("main-update.txt", "main update")
	repo.AddFile("main-update.txt")
	repo.Commit("main update")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch, got %d", len(results))
	}
	if results[0].CommitsAhead != 2 {
		t.Errorf("expected 2 commits ahead, got %d", results[0].CommitsAhead)
	}
	if results[0].CommitsBehind != 1 {
		t.Errorf("expected 1 commit behind, got %d", results[0].CommitsBehind)
	}
}

func TestFindStale_EmptyRepoList(t *testing.T) {
	results, err := branches.FindStale(nil, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty repo list, got %d", len(results))
	}
}

func TestStaleBranch_Label(t *testing.T) {
	sb := branches.StaleBranch{
		RepoName: "my-repo",
		Branch:   "feature/test",
	}
	want := "my-repo: feature/test"
	if got := sb.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestIsAutomationBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   bool
	}{
		{"dependabot/npm_and_yarn/lodash-4.17.21", true},
		{"dependabot/go_modules/golang.org/x/text-0.14.0", true},
		{"renovate/all-minor", true},
		{"renovate/configure", true},
		{"release-please--branches--main", true},
		{"release-please--branches--main--components--katazuke", true},
		{"feature/add-login", false},
		{"graham/fix-bug", false},
		{"main", false},
		{"dependabot", false},
		{"my-dependabot/thing", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			if got := branches.IsAutomationBranch(tt.branch); got != tt.want {
				t.Errorf("IsAutomationBranch(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestFindStale_IsAutomationField(t *testing.T) {
	repo := helpers.NewTestRepo(t, "automation-stale")

	staleDate := time.Now().Add(-60 * 24 * time.Hour)

	// Create an automation branch.
	repo.CreateBranch("dependabot/go_modules/something")
	repo.WriteFile("dep.txt", "dep update")
	repo.AddFile("dep.txt")
	repo.CommitWithDate("dep commit", staleDate)
	repo.Checkout("main")

	// Create a normal branch.
	repo.CreateBranch("feature/normal")
	repo.WriteFile("normal.txt", "normal work")
	repo.AddFile("normal.txt")
	repo.CommitWithDate("normal commit", staleDate)
	repo.Checkout("main")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 stale branches, got %d", len(results))
	}

	byBranch := make(map[string]branches.StaleBranch)
	for _, r := range results {
		byBranch[r.Branch] = r
	}

	dep := byBranch["dependabot/go_modules/something"]
	if !dep.IsAutomation {
		t.Error("expected dependabot branch to be marked as automation")
	}

	normal := byBranch["feature/normal"]
	if normal.IsAutomation {
		t.Error("expected feature branch to NOT be marked as automation")
	}
}

func TestFindStale_IsOwnBranch(t *testing.T) {
	repo := helpers.NewTestRepo(t, "own-branch")

	staleDate := time.Now().Add(-60 * 24 * time.Hour)

	// Create a branch by the repo's configured user (test@example.com).
	repo.CreateBranch("feature/own")
	repo.WriteFile("own.txt", "own work")
	repo.AddFile("own.txt")
	repo.CommitWithDate("own commit", staleDate)
	repo.Checkout("main")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch, got %d", len(results))
	}
	if !results[0].IsOwnBranch {
		t.Error("expected branch to be marked as own (same author)")
	}
}

func TestFindStale_IsLocalOnly(t *testing.T) {
	repo := helpers.NewTestRepo(t, "local-only")

	staleDate := time.Now().Add(-60 * 24 * time.Hour)

	// In a repo with no remote, all branches are local-only.
	repo.CreateBranch("feature/local")
	repo.WriteFile("local.txt", "local work")
	repo.AddFile("local.txt")
	repo.CommitWithDate("local commit", staleDate)
	repo.Checkout("main")

	results, err := branches.FindStale([]string{repo.Path}, 30*24*time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale branch, got %d", len(results))
	}
	if !results[0].IsLocalOnly {
		t.Error("expected branch to be marked as local-only")
	}
}
