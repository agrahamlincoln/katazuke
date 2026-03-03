package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agrahamlincoln/katazuke/internal/config"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
)

// initRepo creates a bare-minimum git repo at the given path.
func initRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0750); err != nil {
		t.Fatal(err)
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
}

func TestClassifyChildren(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create repos.
	initRepo(t, filepath.Join(root, "dotfiles"))
	initRepo(t, filepath.Join(root, "katazuke"))

	// Create group dir with repos inside.
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0750); err != nil {
		t.Fatal(err)
	}
	initRepo(t, filepath.Join(workDir, "project-a"))
	initRepo(t, filepath.Join(workDir, "project-b"))

	// Create plain dir with no repos.
	if err := os.MkdirAll(filepath.Join(root, "downloads"), 0750); err != nil {
		t.Fatal(err)
	}

	dirs, err := classifyChildren(root)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]dirInfo)
	for _, d := range dirs {
		byName[d.Name] = d
	}

	if d, ok := byName["dotfiles"]; !ok || !d.IsRepo {
		t.Errorf("expected dotfiles to be a repo, got %+v", d)
	}
	if d, ok := byName["katazuke"]; !ok || !d.IsRepo {
		t.Errorf("expected katazuke to be a repo, got %+v", d)
	}
	if d, ok := byName["work"]; !ok || d.IsRepo || d.RepoCount != 2 {
		t.Errorf("expected work to be a group with 2 repos, got %+v", d)
	}
	if d, ok := byName["downloads"]; !ok || d.IsRepo || d.RepoCount != 0 {
		t.Errorf("expected downloads to be a plain dir, got %+v", d)
	}
}

func TestClassifyChildren_AllRepos(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initRepo(t, filepath.Join(root, "repo-a"))
	initRepo(t, filepath.Join(root, "repo-b"))

	dirs, err := classifyChildren(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range dirs {
		if !d.IsRepo {
			t.Errorf("expected %s to be a repo", d.Name)
		}
	}

	// No group candidates.
	var groupCandidates int
	for _, d := range dirs {
		if !d.IsRepo && d.RepoCount > 0 {
			groupCandidates++
		}
	}
	if groupCandidates != 0 {
		t.Errorf("expected 0 group candidates, got %d", groupCandidates)
	}
}

func TestClassifyChildren_SkipsHidden(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initRepo(t, filepath.Join(root, ".hidden-repo"))
	initRepo(t, filepath.Join(root, "visible-repo"))

	dirs, err := classifyChildren(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
	if dirs[0].Name != "visible-repo" {
		t.Errorf("expected visible-repo, got %s", dirs[0].Name)
	}
}

func TestGenerateIndex(t *testing.T) {
	t.Parallel()
	out, err := generateIndex([]string{"work", "oss"}, []string{"archive", "downloads"})
	if err != nil {
		t.Fatal(err)
	}

	s := string(out)
	if !strings.Contains(s, "groups:") {
		t.Error("expected groups key in output")
	}
	if !strings.Contains(s, "ignores:") {
		t.Error("expected ignores key in output")
	}

	// Verify sorted order.
	ossIdx := strings.Index(s, "oss")
	workIdx := strings.Index(s, "work")
	if ossIdx > workIdx {
		t.Error("expected groups to be sorted (oss before work)")
	}

	archiveIdx := strings.Index(s, "archive")
	downloadsIdx := strings.Index(s, "downloads")
	if archiveIdx > downloadsIdx {
		t.Error("expected ignores to be sorted (archive before downloads)")
	}
}

func TestGenerateIndex_GroupsOnly(t *testing.T) {
	t.Parallel()
	out, err := generateIndex([]string{"work"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	s := string(out)
	if !strings.Contains(s, "groups:") {
		t.Error("expected groups key")
	}
	if strings.Contains(s, "ignores:") {
		t.Error("expected no ignores key when empty")
	}
}

func TestGenerateIndex_IgnoresOnly(t *testing.T) {
	t.Parallel()
	out, err := generateIndex(nil, []string{"archive"})
	if err != nil {
		t.Fatal(err)
	}

	s := string(out)
	if strings.Contains(s, "groups:") {
		t.Error("expected no groups key when empty")
	}
	if !strings.Contains(s, "ignores:") {
		t.Error("expected ignores key")
	}
}

func TestGenerateIndex_RoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create structure: work/ has 2 repos, archive/ has 1 repo,
	// and katazuke is a top-level repo.
	initRepo(t, filepath.Join(root, "katazuke"))
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0750); err != nil {
		t.Fatal(err)
	}
	initRepo(t, filepath.Join(workDir, "proj-a"))
	initRepo(t, filepath.Join(workDir, "proj-b"))

	archiveDir := filepath.Join(root, "archive")
	if err := os.MkdirAll(archiveDir, 0750); err != nil {
		t.Fatal(err)
	}
	initRepo(t, filepath.Join(archiveDir, "old-proj"))

	// Generate and write index.
	yamlBytes, err := generateIndex([]string{"work"}, []string{"archive"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".katazuke"), yamlBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// Scan and verify.
	repos, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Should find: katazuke (top-level), proj-a, proj-b (in work group).
	// archive is ignored, so old-proj is excluded.
	if len(repos) != 3 {
		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = filepath.Base(r)
		}
		t.Errorf("expected 3 repos, got %d: %v", len(repos), names)
	}
}

func TestCountDiscoveredRepos(t *testing.T) {
	t.Parallel()
	dirs := []dirInfo{
		{Name: "katazuke", IsRepo: true},
		{Name: "dotfiles", IsRepo: true},
		{Name: "work", RepoCount: 4},
		{Name: "oss", RepoCount: 2},
		{Name: "archive", RepoCount: 3},
		{Name: "downloads", RepoCount: 0},
	}

	count := countDiscoveredRepos(dirs, []string{"work", "oss"}, []string{"archive"})
	// katazuke(1) + dotfiles(1) + work(4) + oss(2) = 8
	// archive ignored, downloads not a group so 0
	if count != 8 {
		t.Errorf("expected 8 repos, got %d", count)
	}
}

func TestResolveInitDir(t *testing.T) {
	t.Parallel()
	cfg := config.Config{ProjectsDir: "/home/user/projects"}

	tests := []struct {
		name           string
		arg            string
		cliProjectsDir string
		want           string
	}{
		{
			name: "positional arg takes priority",
			arg:  "/tmp/custom",
			want: "/tmp/custom",
		},
		{
			name:           "positional arg over CLI flag",
			arg:            "/tmp/arg",
			cliProjectsDir: "/tmp/flag",
			want:           "/tmp/arg",
		},
		{
			name:           "CLI flag when no arg",
			cliProjectsDir: "/tmp/flag",
			want:           "/tmp/flag",
		},
		{
			name: "config fallback when neither arg nor flag",
			want: "/home/user/projects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveInitDir(tt.arg, tt.cliProjectsDir, cfg)
			if got != tt.want {
				t.Errorf("resolveInitDir(%q, %q, cfg) = %q, want %q",
					tt.arg, tt.cliProjectsDir, got, tt.want)
			}
		})
	}
}
