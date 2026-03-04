package audit

import (
	"log/slog"
	"path/filepath"

	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// RepoHealth captures the health status of a single repository.
type RepoHealth struct {
	Path            string
	IsClean         bool
	OnDefaultBranch bool
	CurrentBranch   string
	BehindRemote    int // commits behind origin, -1 if unknown
	HasRemote       bool
	ConflictState   string // "rebase", "merge", "cherry-pick", or ""
	IsMergedBranch  bool   // non-default branch merged into origin/default
}

// RepoHealthSummary aggregates repo health into mutually exclusive buckets.
// Priority: conflicted > dirty > non-default branch > behind remote > clean.
type RepoHealthSummary struct {
	Total              int
	CleanUpToDate      int
	NeedsManualFix     int
	BehindRemote       int
	UncommittedChanges int
	OnNonDefaultBranch int
}

// RepoBranchCount holds a per-repo branch count for detail lines.
type RepoBranchCount struct {
	RepoName string
	Count    int
}

// BranchSummary holds counts of merged and stale branches.
type BranchSummary struct {
	MergedBranches int
	MergedRepos    int
	StaleBranches  int
	StaleRepos     int
	MergedByRepo   []RepoBranchCount // sorted by count desc
	StaleByRepo    []RepoBranchCount // sorted by count desc
}

// DashboardResult holds the complete audit dashboard data.
type DashboardResult struct {
	ProjectsDir   string
	RepoCount     int
	RepoHealth    RepoHealthSummary
	HealthDetails []RepoHealth
	Branches      BranchSummary
	NonGitDirs    []NonRepoDir
	StaleDays     int
}

// AnalyzeRepoHealth inspects repos in parallel and returns per-repo health data.
func AnalyzeRepoHealth(repos []string, workers int) []RepoHealth {
	return parallel.Run(repos, workers, inspectRepo, nil)
}

func inspectRepo(repoPath string) RepoHealth {
	repoName := filepath.Base(repoPath)

	h := RepoHealth{
		Path:         repoPath,
		BehindRemote: -1,
	}

	clean, err := git.IsClean(repoPath)
	if err != nil {
		slog.Debug("could not check clean status", "repo", repoName, "error", err)
	}
	h.IsClean = clean
	h.ConflictState = git.ConflictState(repoPath)

	currentBranch, err := git.CurrentBranch(repoPath)
	if err != nil {
		slog.Debug("could not get current branch", "repo", repoName, "error", err)
		return h
	}

	defaultBranch, err := git.DefaultBranch(repoPath)
	if err != nil {
		slog.Debug("could not get default branch", "repo", repoName, "error", err)
		return h
	}

	h.CurrentBranch = currentBranch
	h.OnDefaultBranch = currentBranch == defaultBranch
	h.HasRemote = git.HasRemote(repoPath, "origin")

	// Only check behind-remote when on default branch with a remote.
	if h.OnDefaultBranch && h.HasRemote {
		count, err := git.RevListCount(repoPath, "HEAD..origin/"+defaultBranch)
		if err != nil {
			slog.Debug("could not check behind remote", "repo", repoName, "error", err)
		} else {
			h.BehindRemote = count
		}
	}

	// Check if non-default branch has been merged into origin/default.
	if !h.OnDefaultBranch && h.HasRemote && currentBranch != "" {
		merged, err := git.IsMerged(repoPath, currentBranch, "origin/"+defaultBranch)
		if err != nil {
			slog.Debug("could not check merge status", "repo", repoName, "error", err)
		} else {
			h.IsMergedBranch = merged
		}
	}

	return h
}

// SummarizeHealth partitions repos into mutually exclusive health buckets.
func SummarizeHealth(repos []RepoHealth) RepoHealthSummary {
	s := RepoHealthSummary{Total: len(repos)}
	for _, r := range repos {
		switch {
		case r.ConflictState != "":
			s.NeedsManualFix++
		case !r.IsClean:
			s.UncommittedChanges++
		case !r.OnDefaultBranch:
			s.OnNonDefaultBranch++
		case r.BehindRemote > 0:
			s.BehindRemote++
		default:
			s.CleanUpToDate++
		}
	}
	return s
}

// HealthBuckets holds repos partitioned into mutually exclusive health categories.
type HealthBuckets struct {
	Conflicted []RepoHealth
	Dirty      []RepoHealth
	NonDefault []RepoHealth
	Behind     []RepoHealth
	Clean      []RepoHealth
}

// ReposByBucket partitions repos using the same priority as SummarizeHealth.
func ReposByBucket(repos []RepoHealth) HealthBuckets {
	var b HealthBuckets
	for _, r := range repos {
		switch {
		case r.ConflictState != "":
			b.Conflicted = append(b.Conflicted, r)
		case !r.IsClean:
			b.Dirty = append(b.Dirty, r)
		case !r.OnDefaultBranch:
			b.NonDefault = append(b.NonDefault, r)
		case r.BehindRemote > 0:
			b.Behind = append(b.Behind, r)
		default:
			b.Clean = append(b.Clean, r)
		}
	}
	return b
}
