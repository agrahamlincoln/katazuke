package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// initGitRepo creates a minimal git repository at the given path.
func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	gitRun(t, path, "init")
	gitRun(t, path, "config", "user.name", "Test User")
	gitRun(t, path, "config", "user.email", "test@example.com")
	gitRun(t, path, "commit", "--allow-empty", "-m", "initial")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// createDir creates a directory with some files for testing.
func createDir(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	for name, content := range files {
		filePath := filepath.Join(path, name)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", filePath, err)
		}
	}
}

func TestFindNonRepoDirs(t *testing.T) {
	root := t.TempDir()

	// Create a git repo -- should be skipped.
	initGitRepo(t, filepath.Join(root, "my-repo"))

	// Create a non-repo directory with files.
	createDir(t, filepath.Join(root, "random-dir"), map[string]string{
		"file.txt": "hello",
		"notes.md": "some notes",
	})

	// Create another non-repo directory.
	createDir(t, filepath.Join(root, "scripts"), map[string]string{
		"build.sh": "#!/bin/bash",
		"test.sh":  "#!/bin/bash",
		"run.sh":   "#!/bin/bash",
	})

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 non-repo dirs, got %d: %+v", len(result), result)
	}

	// Results should be in directory listing order.
	names := make(map[string]bool)
	for _, d := range result {
		names[d.Name] = true
	}

	if !names["random-dir"] {
		t.Error("expected random-dir in results")
	}
	if !names["scripts"] {
		t.Error("expected scripts in results")
	}
}

func TestFindNonRepoDirsEmpty(t *testing.T) {
	root := t.TempDir()

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 results for empty dir, got %d", len(result))
	}
}

func TestFindNonRepoDirsAllRepos(t *testing.T) {
	root := t.TempDir()

	initGitRepo(t, filepath.Join(root, "repo1"))
	initGitRepo(t, filepath.Join(root, "repo2"))

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 non-repo dirs when all are repos, got %d", len(result))
	}
}

func TestFindNonRepoDirsSkipsHidden(t *testing.T) {
	root := t.TempDir()

	// Hidden directory should be skipped.
	createDir(t, filepath.Join(root, ".hidden"), map[string]string{
		"secret.txt": "hidden",
	})

	// Visible non-repo directory.
	createDir(t, filepath.Join(root, "visible"), map[string]string{
		"file.txt": "visible",
	})

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(result), result)
	}

	if result[0].Name != "visible" {
		t.Errorf("expected visible, got %s", result[0].Name)
	}
}

func TestFindNonRepoDirsExcludePatterns(t *testing.T) {
	root := t.TempDir()

	createDir(t, filepath.Join(root, "vendor"), map[string]string{
		"lib.go": "package vendor",
	})
	createDir(t, filepath.Join(root, "keepers"), map[string]string{
		"main.go": "package main",
	})

	result, err := FindNonRepoDirs(root, Options{
		ExcludePatterns: []string{"vendor"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(result), result)
	}

	if result[0].Name != "keepers" {
		t.Errorf("expected keepers, got %s", result[0].Name)
	}
}

func TestFindNonRepoDirsWithKatazukeIndex(t *testing.T) {
	root := t.TempDir()

	// Write .katazuke index.
	indexContent := "groups:\n  - work\nignores:\n  - archive\n"
	if err := os.WriteFile(filepath.Join(root, ".katazuke"), []byte(indexContent), 0600); err != nil {
		t.Fatalf("write .katazuke: %v", err)
	}

	// Create group directory -- should be skipped (it's a group, not a candidate).
	if err := os.MkdirAll(filepath.Join(root, "work"), 0750); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	// Create ignored directory -- should be skipped.
	createDir(t, filepath.Join(root, "archive"), map[string]string{
		"old.txt": "archived",
	})

	// Create a normal non-repo directory -- should be found.
	createDir(t, filepath.Join(root, "random-stuff"), map[string]string{
		"notes.txt": "some notes",
	})

	// Create a git repo -- should not appear.
	initGitRepo(t, filepath.Join(root, "my-repo"))

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(result), result)
	}

	if result[0].Name != "random-stuff" {
		t.Errorf("expected random-stuff, got %s", result[0].Name)
	}
}

func TestNonRepoDirInfo(t *testing.T) {
	root := t.TempDir()

	// Create a directory with known files.
	dir := filepath.Join(root, "project")
	createDir(t, dir, map[string]string{
		"main.go":          "package main\nfunc main() {}",
		"main_test.go":     "package main",
		"utils.go":         "package main",
		"config.yaml":      "key: value",
		"sub/nested.go":    "package sub",
		"sub/nested_2.go":  "package sub",
		"README.md":        "# README",
		"data/input.json":  `{"key": "value"}`,
		"data/output.json": `{"result": true}`,
	})

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	d := result[0]

	if d.Name != "project" {
		t.Errorf("expected name 'project', got %q", d.Name)
	}

	if d.Path != dir {
		t.Errorf("expected path %q, got %q", dir, d.Path)
	}

	if d.FileCount != 9 {
		t.Errorf("expected 9 files, got %d", d.FileCount)
	}

	if d.Size <= 0 {
		t.Errorf("expected positive size, got %d", d.Size)
	}

	if d.LastModified.IsZero() {
		t.Error("expected non-zero last modified time")
	}

	// LastModified should be recent (within last minute).
	if time.Since(d.LastModified) > time.Minute {
		t.Errorf("expected recent last modified time, got %v", d.LastModified)
	}

	// Summary should mention .go files (most common).
	if d.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestBuildSummary(t *testing.T) {
	tests := []struct {
		name      string
		extCounts map[string]int
		want      string
	}{
		{
			name:      "empty",
			extCounts: map[string]int{},
			want:      "empty",
		},
		{
			name:      "single extension",
			extCounts: map[string]int{".go": 5},
			want:      "5 .go",
		},
		{
			name: "three extensions",
			extCounts: map[string]int{
				".go":   10,
				".yaml": 3,
				".md":   2,
			},
			want: "10 .go, 3 .yaml, 2 .md",
		},
		{
			name: "more than three extensions",
			extCounts: map[string]int{
				".go":   10,
				".yaml": 5,
				".md":   3,
				".json": 2,
				".txt":  1,
			},
			want: "10 .go, 5 .yaml, 3 .md, 3 others",
		},
		{
			name: "tie broken alphabetically",
			extCounts: map[string]int{
				".b": 5,
				".a": 5,
				".c": 5,
			},
			want: "5 .a, 5 .b, 5 .c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSummary(tt.extCounts)
			if got != tt.want {
				t.Errorf("buildSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindNonRepoDirsSkipsFiles(t *testing.T) {
	root := t.TempDir()

	// Create a regular file at root level -- should be skipped (not a directory).
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Create a non-repo directory.
	createDir(t, filepath.Join(root, "stuff"), map[string]string{
		"file.txt": "content",
	})

	result, err := FindNonRepoDirs(root, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].Name != "stuff" {
		t.Errorf("expected stuff, got %s", result[0].Name)
	}
}

func TestInspectDirEmptyDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "empty")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info, err := inspectDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.FileCount != 0 {
		t.Errorf("expected 0 files, got %d", info.FileCount)
	}

	if info.Size != 0 {
		t.Errorf("expected 0 size, got %d", info.Size)
	}

	if info.Summary != "empty" {
		t.Errorf("expected 'empty' summary, got %q", info.Summary)
	}
}
