package repos

import (
	"log/slog"
	"path/filepath"

	"github.com/agrahamlincoln/katazuke/internal/merge"
	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// MergedBranchRepo represents a repository that is currently on a branch
// that has been merged into the default branch.
type MergedBranchRepo struct {
	Path          string
	Name          string
	CurrentBranch string
	DefaultBranch string
	IsClean       bool
}

// FindOnMergedBranch scans the given repository paths and identifies repos
// that are checked out on a branch that has been merged into the default
// branch. Work is parallelized across the given number of workers. The
// detector combines local git checks with GitHub API lookups to catch
// squash-merges.
//
// Note: local git checks operate on locally cached remote refs without
// fetching first, so results reflect the last fetch rather than current
// remote state.
func FindOnMergedBranch(repos []string, detector *merge.Detector, workers int, onProgress func(completed, total int)) []MergedBranchRepo {
	var resultCb func(int, int, *MergedBranchRepo)
	if onProgress != nil {
		resultCb = func(completed, total int, _ *MergedBranchRepo) {
			onProgress(completed, total)
		}
	}

	results := parallel.Run(repos, workers, func(repoPath string) *MergedBranchRepo {
		return checkMergedBranch(repoPath, detector)
	}, resultCb)

	var merged []MergedBranchRepo
	for _, r := range results {
		if r != nil {
			merged = append(merged, *r)
		}
	}
	return merged
}

func checkMergedBranch(repoPath string, detector *merge.Detector) *MergedBranchRepo {
	name := filepath.Base(repoPath)

	currentBranch, err := git.CurrentBranch(repoPath)
	if err != nil {
		slog.Debug("could not get current branch", "repo", name, "error", err)
		return nil
	}

	if currentBranch == "" {
		return nil
	}

	defaultBranch, err := git.DefaultBranch(repoPath)
	if err != nil {
		slog.Debug("could not get default branch", "repo", name, "error", err)
		return nil
	}

	if currentBranch == defaultBranch {
		return nil
	}

	// Determine merge base: use remote default branch if available.
	base := defaultBranch
	if git.HasRemote(repoPath, "origin") {
		base = "origin/" + defaultBranch
	}

	merged, err := detector.IsMerged(repoPath, currentBranch, base)
	if err != nil || !merged {
		return nil
	}

	clean, err := git.IsClean(repoPath)
	if err != nil {
		slog.Debug("could not check working tree status", "repo", name, "error", err)
		clean = false
	}

	return &MergedBranchRepo{
		Path:          repoPath,
		Name:          name,
		CurrentBranch: currentBranch,
		DefaultBranch: defaultBranch,
		IsClean:       clean,
	}
}
