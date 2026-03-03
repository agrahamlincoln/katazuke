package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSummarizeHealth(t *testing.T) {
	repos := []RepoHealth{
		{IsClean: true, OnDefaultBranch: true, BehindRemote: 0},   // clean
		{IsClean: true, OnDefaultBranch: true, BehindRemote: 3},   // behind
		{IsClean: false, OnDefaultBranch: true, BehindRemote: 0},  // dirty
		{IsClean: true, OnDefaultBranch: false, BehindRemote: -1}, // non-default
		{IsClean: true, OnDefaultBranch: true, BehindRemote: -1},  // clean (unknown remote)
		{IsClean: false, OnDefaultBranch: false, BehindRemote: 5}, // dirty wins over non-default
	}

	s := SummarizeHealth(repos)

	if s.Total != 6 {
		t.Errorf("Total = %d, want 6", s.Total)
	}
	if s.CleanUpToDate != 2 {
		t.Errorf("CleanUpToDate = %d, want 2", s.CleanUpToDate)
	}
	if s.BehindRemote != 1 {
		t.Errorf("BehindRemote = %d, want 1", s.BehindRemote)
	}
	if s.UncommittedChanges != 2 {
		t.Errorf("UncommittedChanges = %d, want 2", s.UncommittedChanges)
	}
	if s.OnNonDefaultBranch != 1 {
		t.Errorf("OnNonDefaultBranch = %d, want 1", s.OnNonDefaultBranch)
	}
}

func TestSummarizeHealthPartitioning(t *testing.T) {
	repos := []RepoHealth{
		{IsClean: true, OnDefaultBranch: true, BehindRemote: 0},
		{IsClean: true, OnDefaultBranch: true, BehindRemote: 5},
		{IsClean: false, OnDefaultBranch: true, BehindRemote: 0},
		{IsClean: true, OnDefaultBranch: false, BehindRemote: -1},
		{IsClean: false, OnDefaultBranch: false, BehindRemote: 10},
		{IsClean: true, OnDefaultBranch: true, BehindRemote: -1},
		{IsClean: true, OnDefaultBranch: true, BehindRemote: 1},
	}

	s := SummarizeHealth(repos)

	sum := s.CleanUpToDate + s.BehindRemote + s.UncommittedChanges + s.OnNonDefaultBranch
	if sum != s.Total {
		t.Errorf("bucket sum %d != Total %d (clean=%d behind=%d dirty=%d nondefault=%d)",
			sum, s.Total, s.CleanUpToDate, s.BehindRemote, s.UncommittedChanges, s.OnNonDefaultBranch)
	}
}

func TestSummarizeHealthEmpty(t *testing.T) {
	s := SummarizeHealth(nil)

	if s.Total != 0 {
		t.Errorf("Total = %d, want 0", s.Total)
	}
	if s.CleanUpToDate != 0 || s.BehindRemote != 0 || s.UncommittedChanges != 0 || s.OnNonDefaultBranch != 0 {
		t.Errorf("expected all zeros, got %+v", s)
	}
}

func TestAnalyzeRepoHealth(t *testing.T) {
	root := t.TempDir()

	// Clean repo on default branch.
	cleanRepo := filepath.Join(root, "clean-repo")
	initGitRepo(t, cleanRepo)

	// Dirty repo (uncommitted changes).
	dirtyRepo := filepath.Join(root, "dirty-repo")
	initGitRepo(t, dirtyRepo)
	if err := os.WriteFile(filepath.Join(dirtyRepo, "dirty.txt"), []byte("uncommitted"), 0600); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	// Repo on feature branch.
	featureRepo := filepath.Join(root, "feature-repo")
	initGitRepo(t, featureRepo)
	gitRun(t, featureRepo, "checkout", "-b", "feature/test")

	repos := []string{cleanRepo, dirtyRepo, featureRepo}
	results := AnalyzeRepoHealth(repos, 1)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Build a map for easy lookup since parallel.Run may reorder.
	byPath := make(map[string]RepoHealth)
	for _, r := range results {
		byPath[r.Path] = r
	}

	// Clean repo should be clean and on default branch.
	clean := byPath[cleanRepo]
	if !clean.IsClean {
		t.Error("clean repo: expected IsClean=true")
	}
	if !clean.OnDefaultBranch {
		t.Error("clean repo: expected OnDefaultBranch=true")
	}

	// Dirty repo should be dirty.
	dirty := byPath[dirtyRepo]
	if dirty.IsClean {
		t.Error("dirty repo: expected IsClean=false")
	}

	// Feature branch repo should not be on default branch.
	feature := byPath[featureRepo]
	if feature.OnDefaultBranch {
		t.Error("feature repo: expected OnDefaultBranch=false")
	}
	if !feature.IsClean {
		t.Error("feature repo: expected IsClean=true")
	}
}

func TestAnalyzeRepoHealthNoRemote(t *testing.T) {
	root := t.TempDir()

	repo := filepath.Join(root, "no-remote")
	initGitRepo(t, repo)

	results := AnalyzeRepoHealth([]string{repo}, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.HasRemote {
		t.Error("expected HasRemote=false for repo without remote")
	}
	if r.BehindRemote != -1 {
		t.Errorf("expected BehindRemote=-1, got %d", r.BehindRemote)
	}
}
