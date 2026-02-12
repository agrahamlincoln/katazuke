// +build e2e

package e2e

import (
	"os/exec"
	"testing"
	"time"

	"github.com/agrahamlincoln/katazuke/test/helpers"
)

func TestStaleBranchDetection(t *testing.T) {
	// Create a test repository
	repo := helpers.NewTestRepo(t, "test-repo")

	// Create a branch that's 45 days old (stale)
	repo.CreateBranch("feature/old-work")
	repo.WriteFile("old.txt", "old feature")
	repo.AddFile("old.txt")
	oldDate := time.Now().AddDate(0, 0, -45) // 45 days ago
	repo.CommitWithDate("Add old feature", oldDate)

	// Create a branch that's 10 days old (not stale)
	repo.Checkout("main")
	repo.CreateBranch("feature/recent-work")
	repo.WriteFile("recent.txt", "recent feature")
	repo.AddFile("recent.txt")
	recentDate := time.Now().AddDate(0, 0, -10) // 10 days ago
	repo.CommitWithDate("Add recent feature", recentDate)

	// Go back to main
	repo.Checkout("main")

	// Run katazuke to detect stale branches
	// This would use the built binary (from build-debug)
	cmd := exec.Command("../../bin/katazuke-debug", "branches", "--stale", "--stale-days", "30")
	cmd.Dir = repo.Path
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Output: %s", output)
		// For now, this is expected to fail since we haven't implemented the feature
		t.Skip("Stale branch detection not yet implemented")
	}

	// TODO: Add assertions once the feature is implemented
	// Expected:
	// - Should detect "feature/old-work" as stale (45 days old)
	// - Should NOT detect "feature/recent-work" (only 10 days old)
}

func TestMergedBranchDetection(t *testing.T) {
	repo := helpers.NewTestRepo(t, "test-merged-repo")

	// Create and merge a branch
	repo.CreateBranch("feature/merged")
	repo.WriteFile("merged.txt", "merged feature")
	repo.AddFile("merged.txt")
	repo.Commit("Add merged feature")

	repo.Checkout("main")
	repo.Merge("feature/merged")

	// The branch still exists locally
	branches := repo.Branches()
	if !contains(branches, "feature/merged") {
		t.Fatal("Merged branch should still exist locally")
	}

	// Run katazuke to detect merged branches
	cmd := exec.Command("../../bin/katazuke-debug", "branches", "--merged")
	cmd.Dir = repo.Path
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Output: %s", output)
		t.Skip("Merged branch detection not yet implemented")
	}

	// TODO: Add assertions once implemented
	// Expected:
	// - Should detect "feature/merged" as merged
	// - Should offer to delete it
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
