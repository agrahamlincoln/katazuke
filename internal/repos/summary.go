package repos

import (
	"log/slog"
	"path/filepath"

	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// RepoStatus holds basic health info for a single repository.
type RepoStatus struct {
	Path    string
	Name    string
	IsClean bool
	Branch  string
}

// Summary holds aggregate health statistics for a set of repositories.
type Summary struct {
	Total int
	Clean int
	Dirty int
}

// Summarize collects basic health info for all given repositories.
func Summarize(repos []string, workers int, onProgress func(completed, total int)) Summary {
	var resultCb func(int, int, RepoStatus)
	if onProgress != nil {
		resultCb = func(completed, total int, _ RepoStatus) {
			onProgress(completed, total)
		}
	}

	results := parallel.Run(repos, workers, func(repoPath string) RepoStatus {
		name := filepath.Base(repoPath)
		clean, err := git.IsClean(repoPath)
		if err != nil {
			slog.Debug("could not check working tree status", "repo", name, "error", err)
		}
		branch, err := git.CurrentBranch(repoPath)
		if err != nil {
			slog.Debug("could not get current branch", "repo", name, "error", err)
		}
		return RepoStatus{
			Path:    repoPath,
			Name:    name,
			IsClean: clean,
			Branch:  branch,
		}
	}, resultCb)

	s := Summary{Total: len(results)}
	for _, r := range results {
		if r.IsClean {
			s.Clean++
		} else {
			s.Dirty++
		}
	}
	return s
}
