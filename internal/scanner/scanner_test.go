package scanner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/agrahamlincoln/katazuke/internal/scanner"
)

// initRepo creates a bare-minimum git repo at the given path.
func initRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", path, err, out)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestScanFlat(t *testing.T) {
	root := t.TempDir()

	// Create three repos and one non-repo directory.
	initRepo(t, filepath.Join(root, "repo-a"))
	initRepo(t, filepath.Join(root, "repo-b"))
	initRepo(t, filepath.Join(root, "repo-c"))
	mkdirAll(t, filepath.Join(root, "not-a-repo"))

	repos, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(repos)
	want := []string{
		filepath.Join(root, "repo-a"),
		filepath.Join(root, "repo-b"),
		filepath.Join(root, "repo-c"),
	}
	if len(repos) != len(want) {
		t.Fatalf("expected %d repos, got %d: %v", len(want), len(repos), repos)
	}
	for i, r := range repos {
		if r != want[i] {
			t.Errorf("expected %s, got %s", want[i], r)
		}
	}
}

func TestScanWithIndex(t *testing.T) {
	root := t.TempDir()

	// Create .katazuke index with groups and ignores.
	writeFile(t, filepath.Join(root, ".katazuke"), []byte("groups:\n  - work\n  - oss\nignores:\n  - archive\n"))

	// Create group directories with repos inside.
	initRepo(t, filepath.Join(root, "work", "project-a"))
	initRepo(t, filepath.Join(root, "work", "project-b"))
	initRepo(t, filepath.Join(root, "oss", "lib"))

	// Create ignored directory (should not be scanned).
	initRepo(t, filepath.Join(root, "archive", "old-repo"))

	repos, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(repos)
	want := []string{
		filepath.Join(root, "oss", "lib"),
		filepath.Join(root, "work", "project-a"),
		filepath.Join(root, "work", "project-b"),
	}
	if len(repos) != len(want) {
		t.Fatalf("expected %d repos, got %d: %v", len(want), len(repos), repos)
	}
	for i, r := range repos {
		if r != want[i] {
			t.Errorf("expected %s, got %s", want[i], r)
		}
	}
}

func TestScanNestedIndex(t *testing.T) {
	root := t.TempDir()

	// Top-level index.
	writeFile(t, filepath.Join(root, ".katazuke"), []byte("groups:\n  - work\n"))

	// Nested index under work/.
	mkdirAll(t, filepath.Join(root, "work"))
	writeFile(t, filepath.Join(root, "work", ".katazuke"), []byte("groups:\n  - client-a\n"))

	// Repos under the nested group.
	initRepo(t, filepath.Join(root, "work", "client-a", "frontend"))
	initRepo(t, filepath.Join(root, "work", "client-a", "backend"))

	repos, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(repos), repos)
	}
}

func TestScanSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()

	initRepo(t, filepath.Join(root, ".hidden-repo"))
	initRepo(t, filepath.Join(root, "visible-repo"))

	repos, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
	if repos[0] != filepath.Join(root, "visible-repo") {
		t.Errorf("expected visible-repo, got %s", repos[0])
	}
}

func TestScanExcludePatterns(t *testing.T) {
	root := t.TempDir()

	initRepo(t, filepath.Join(root, "keep"))
	initRepo(t, filepath.Join(root, "vendor"))

	repos, err := scanner.Scan(root, scanner.Options{ExcludePatterns: []string{"vendor"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
}

func TestScanRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".katazuke"), []byte("groups:\n  - foo\nunknown: bar\n"))

	_, err := scanner.Scan(root, scanner.Options{})
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestScanEmptyIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".katazuke"), []byte(""))
	initRepo(t, filepath.Join(root, "repo"))

	// Empty index means "has index but empty groups" -- non-group children are scanned.
	repos, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
}
