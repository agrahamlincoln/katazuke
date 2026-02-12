//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agrahamlincoln/katazuke/test/helpers"
)

// binaryPath returns the absolute path to the katazuke-debug binary,
// resolving from the project root rather than using relative paths
// that break when cmd.Dir is set to a temp directory.
func binaryPath(t *testing.T) string {
	t.Helper()
	// The test runs from the test/e2e/ directory, so go up two levels.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return filepath.Join(wd, "..", "..", "bin", "katazuke-debug")
}

func TestStaleBranchDetection(t *testing.T) {
	repo := helpers.NewTestRepo(t, "test-repo")
	projectsDir := filepath.Dir(repo.Path)

	// Create a branch that's 45 days old (stale).
	repo.CreateBranch("feature/old-work")
	repo.WriteFile("old.txt", "old feature")
	repo.AddFile("old.txt")
	repo.CommitWithDate("Add old feature", time.Now().AddDate(0, 0, -45))
	repo.Checkout("main")

	// Create a branch that's 10 days old (not stale).
	repo.CreateBranch("feature/recent-work")
	repo.WriteFile("recent.txt", "recent feature")
	repo.AddFile("recent.txt")
	repo.CommitWithDate("Add recent feature", time.Now().AddDate(0, 0, -10))
	repo.Checkout("main")

	// Run katazuke with --dry-run to avoid interactive prompts.
	cmd := exec.Command(binaryPath(t),
		"branches", "--stale", "--stale-days", "30",
		"--dry-run", "--projects-dir", projectsDir)
	cmd.Dir = repo.Path
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		t.Fatalf("katazuke exited with error: %v\nOutput: %s", err, outputStr)
	}

	// Should detect feature/old-work as stale (45 days old).
	if !strings.Contains(outputStr, "feature/old-work") {
		t.Errorf("expected output to contain stale branch 'feature/old-work'\nOutput: %s", outputStr)
	}

	// Should NOT detect feature/recent-work (only 10 days old).
	if strings.Contains(outputStr, "feature/recent-work") {
		t.Errorf("expected output to NOT contain recent branch 'feature/recent-work'\nOutput: %s", outputStr)
	}

	// Should show the summary header.
	if !strings.Contains(outputStr, "stale branch") {
		t.Errorf("expected output to contain 'stale branch' summary\nOutput: %s", outputStr)
	}
}

func TestMergedBranchDetection(t *testing.T) {
	repo := helpers.NewTestRepo(t, "test-merged-repo")
	projectsDir := filepath.Dir(repo.Path)

	// Create and merge a branch.
	repo.CreateBranch("feature/merged")
	repo.WriteFile("merged.txt", "merged feature")
	repo.AddFile("merged.txt")
	repo.Commit("Add merged feature")
	repo.Checkout("main")
	repo.Merge("feature/merged")

	// The branch still exists locally.
	branches := repo.Branches()
	if !contains(branches, "feature/merged") {
		t.Fatal("Merged branch should still exist locally")
	}

	// Run katazuke with --dry-run.
	cmd := exec.Command(binaryPath(t),
		"branches", "--merged",
		"--dry-run", "--projects-dir", projectsDir)
	cmd.Dir = repo.Path
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		t.Fatalf("katazuke exited with error: %v\nOutput: %s", err, outputStr)
	}

	if !strings.Contains(outputStr, "feature/merged") {
		t.Errorf("expected output to contain merged branch 'feature/merged'\nOutput: %s", outputStr)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
