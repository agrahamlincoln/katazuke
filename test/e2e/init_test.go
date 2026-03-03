//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_AllRepos_NoIndexNeeded(t *testing.T) {
	// When all children are repos, init should exit without prompts.
	projectsDir := t.TempDir()
	initGitRepo(t, filepath.Join(projectsDir, "repo-a"))
	initGitRepo(t, filepath.Join(projectsDir, "repo-b"))

	cmd := exec.Command(binaryPath(t), "init", projectsDir)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		t.Fatalf("expected exit 0, got error: %v\nOutput: %s", err, outputStr)
	}

	if !strings.Contains(outputStr, "no index file needed") {
		t.Errorf("expected 'no index file needed' message\nOutput: %s", outputStr)
	}

	// Should not have created a .katazuke file.
	if _, err := os.Stat(filepath.Join(projectsDir, ".katazuke")); !os.IsNotExist(err) {
		t.Error("expected no .katazuke file to be created")
	}
}

func TestInit_ClassifiesDirectories(t *testing.T) {
	root := t.TempDir()

	// Top-level repo.
	initGitRepo(t, filepath.Join(root, "dotfiles"))

	// Group directory with repos.
	initGitRepo(t, filepath.Join(root, "work", "project-a"))
	initGitRepo(t, filepath.Join(root, "work", "project-b"))

	// Plain directory.
	if err := os.MkdirAll(filepath.Join(root, "downloads"), 0750); err != nil {
		t.Fatal(err)
	}

	// init exits non-zero because the TTY prompt fails in CI, but the
	// classification output before the prompt is what we're testing.
	cmd := exec.Command(binaryPath(t), "init", root)
	output, _ := cmd.CombinedOutput() //nolint:errcheck // expected non-zero exit
	outputStr := string(output)

	// Verify classification output.
	if !strings.Contains(outputStr, "dotfiles") || !strings.Contains(outputStr, "git repo") {
		t.Errorf("expected dotfiles classified as git repo\nOutput: %s", outputStr)
	}
	if !strings.Contains(outputStr, "work/") || !strings.Contains(outputStr, "2 repos inside") {
		t.Errorf("expected work/ classified with 2 repos inside\nOutput: %s", outputStr)
	}
	if !strings.Contains(outputStr, "downloads/") || !strings.Contains(outputStr, "no repos") {
		t.Errorf("expected downloads/ classified as no repos\nOutput: %s", outputStr)
	}
}

func TestInit_SkipsHiddenDirectories(t *testing.T) {
	root := t.TempDir()

	// Hidden repo (should be skipped).
	initGitRepo(t, filepath.Join(root, ".hidden"))

	// Visible repo.
	initGitRepo(t, filepath.Join(root, "visible"))

	// Expected non-zero exit (no TTY for prompts); testing output only.
	cmd := exec.Command(binaryPath(t), "init", root)
	output, _ := cmd.CombinedOutput() //nolint:errcheck // expected non-zero exit
	outputStr := string(output)

	if strings.Contains(outputStr, ".hidden") {
		t.Errorf("hidden directory should not appear in output\nOutput: %s", outputStr)
	}
	if !strings.Contains(outputStr, "visible") {
		t.Errorf("visible repo should appear in output\nOutput: %s", outputStr)
	}
}

func TestInit_ExistingFileDetected(t *testing.T) {
	root := t.TempDir()

	// Create structure with a group.
	initGitRepo(t, filepath.Join(root, "myrepo"))
	initGitRepo(t, filepath.Join(root, "work", "proj"))

	// Write an existing .katazuke file.
	if err := os.WriteFile(filepath.Join(root, ".katazuke"), []byte("groups:\n- work\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Expected non-zero exit (no TTY for prompts); testing output only.
	cmd := exec.Command(binaryPath(t), "init", root)
	output, _ := cmd.CombinedOutput() //nolint:errcheck // expected non-zero exit
	outputStr := string(output)

	if !strings.Contains(outputStr, "Existing .katazuke found") {
		t.Errorf("expected existing file detection message\nOutput: %s", outputStr)
	}
}

func TestInit_GeneratedIndexWorksWithScanner(t *testing.T) {
	// This tests the round-trip: generate an index file and verify
	// that the scanner discovers the correct repositories.
	root := t.TempDir()

	// Top-level repo.
	initGitRepo(t, filepath.Join(root, "katazuke"))

	// Group with repos.
	initGitRepo(t, filepath.Join(root, "work", "proj-a"))
	initGitRepo(t, filepath.Join(root, "work", "proj-b"))
	initGitRepo(t, filepath.Join(root, "work", "proj-c"))

	// Ignored group.
	initGitRepo(t, filepath.Join(root, "archive", "old-proj"))

	// Write index: work is a group, archive is ignored.
	indexContent := "groups:\n- work\nignores:\n- archive\n"
	if err := os.WriteFile(filepath.Join(root, ".katazuke"), []byte(indexContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Run branches --merged --dry-run to verify scanner picks up correct repos.
	cmd := exec.Command(binaryPath(t),
		"branches", "--merged", "--dry-run",
		"--projects-dir", root)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		t.Fatalf("katazuke exited with error: %v\nOutput: %s", err, outputStr)
	}

	// Should scan 4 repos: katazuke + proj-a + proj-b + proj-c.
	// archive/old-proj should be ignored.
	if !strings.Contains(outputStr, "Scanning 4 repositories") {
		t.Errorf("expected 4 repositories discovered, got:\n%s", outputStr)
	}
}

// initGitRepo creates a minimal git repository at the given path.
func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = path
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", path, err, out)
	}
	// Create initial commit so the repo is fully initialized.
	readme := filepath.Join(path, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	add := exec.Command("git", "add", ".")
	add.Dir = path
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	commit := exec.Command("git", "commit", "-m", "init")
	commit.Dir = path
	commit.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}
