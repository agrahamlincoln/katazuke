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
		{IsClean: false, ConflictState: "rebase"},                 // conflicted wins over dirty
	}

	s := SummarizeHealth(repos)

	if s.Total != 7 {
		t.Errorf("Total = %d, want 7", s.Total)
	}
	if s.NeedsManualFix != 1 {
		t.Errorf("NeedsManualFix = %d, want 1", s.NeedsManualFix)
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
		{IsClean: false, OnDefaultBranch: true, ConflictState: "merge"},
	}

	s := SummarizeHealth(repos)

	sum := s.CleanUpToDate + s.NeedsManualFix + s.BehindRemote + s.UncommittedChanges + s.OnNonDefaultBranch
	if sum != s.Total {
		t.Errorf("bucket sum %d != Total %d (clean=%d conflict=%d behind=%d dirty=%d nondefault=%d)",
			sum, s.Total, s.CleanUpToDate, s.NeedsManualFix, s.BehindRemote, s.UncommittedChanges, s.OnNonDefaultBranch)
	}
}

func TestReposByBucket(t *testing.T) {
	repos := []RepoHealth{
		{Path: "/a/clean1", IsClean: true, OnDefaultBranch: true, BehindRemote: 0},
		{Path: "/a/behind1", IsClean: true, OnDefaultBranch: true, BehindRemote: 3},
		{Path: "/a/dirty1", IsClean: false, OnDefaultBranch: true, BehindRemote: 0},
		{Path: "/a/nondefault1", IsClean: true, OnDefaultBranch: false, CurrentBranch: "feature/x", BehindRemote: -1},
		{Path: "/a/clean2", IsClean: true, OnDefaultBranch: true, BehindRemote: -1},
		{Path: "/a/dirty2", IsClean: false, OnDefaultBranch: false, BehindRemote: 5},
		{Path: "/a/conflicted1", IsClean: false, OnDefaultBranch: true, ConflictState: "rebase"},
	}

	b := ReposByBucket(repos)

	if len(b.Conflicted) != 1 {
		t.Errorf("Conflicted = %d, want 1", len(b.Conflicted))
	}
	if len(b.Dirty) != 2 {
		t.Errorf("Dirty = %d, want 2", len(b.Dirty))
	}
	if len(b.NonDefault) != 1 {
		t.Errorf("NonDefault = %d, want 1", len(b.NonDefault))
	}
	if len(b.Behind) != 1 {
		t.Errorf("Behind = %d, want 1", len(b.Behind))
	}
	if len(b.Clean) != 2 {
		t.Errorf("Clean = %d, want 2", len(b.Clean))
	}

	// Verify totals match.
	total := len(b.Conflicted) + len(b.Dirty) + len(b.NonDefault) + len(b.Behind) + len(b.Clean)
	if total != len(repos) {
		t.Errorf("bucket total %d != input count %d", total, len(repos))
	}
}

func TestReposByBucketEmpty(t *testing.T) {
	b := ReposByBucket(nil)
	if len(b.Conflicted) != 0 || len(b.Dirty) != 0 || len(b.NonDefault) != 0 || len(b.Behind) != 0 || len(b.Clean) != 0 {
		t.Errorf("expected all empty, got conflicted=%d dirty=%d nondefault=%d behind=%d clean=%d",
			len(b.Conflicted), len(b.Dirty), len(b.NonDefault), len(b.Behind), len(b.Clean))
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

func TestAnalyzeRepoHealth_Conflicted(t *testing.T) {
	root := t.TempDir()

	repo := filepath.Join(root, "conflicted-repo")
	initGitRepo(t, repo)

	// Place a rebase-merge sentinel directory to simulate mid-rebase.
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0750); err != nil {
		t.Fatalf("create rebase-merge dir: %v", err)
	}

	results := AnalyzeRepoHealth([]string{repo}, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.ConflictState != "rebase" {
		t.Errorf("expected ConflictState=%q, got %q", "rebase", r.ConflictState)
	}
}
