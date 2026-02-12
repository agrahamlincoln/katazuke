// Package repos provides operations for managing repository checkouts
// in the projects directory.
package repos

import (
	"log/slog"
	"path/filepath"

	"github.com/agrahamlincoln/katazuke/internal/github"
	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// ArchiveChecker defines the interface for checking if a repository is archived.
type ArchiveChecker interface {
	IsArchived(owner, repo string) (bool, error)
}

// ArchivedRepo represents a local repository that is archived on GitHub.
type ArchivedRepo struct {
	Path    string
	Name    string
	Owner   string
	Repo    string
	IsClean bool
}

// FindArchived scans the given repository paths and checks their GitHub
// archive status. Repos without a GitHub remote are silently skipped.
// Work is parallelized across the given number of workers.
func FindArchived(repos []string, checker ArchiveChecker, workers int, onProgress func(completed, total int)) []ArchivedRepo {
	var resultCb func(int, int, *ArchivedRepo)
	if onProgress != nil {
		resultCb = func(completed, total int, _ *ArchivedRepo) {
			onProgress(completed, total)
		}
	}

	results := parallel.Run(repos, workers, func(repoPath string) *ArchivedRepo {
		return checkArchived(repoPath, checker)
	}, resultCb)

	var archived []ArchivedRepo
	for _, r := range results {
		if r != nil {
			archived = append(archived, *r)
		}
	}
	return archived
}

func checkArchived(repoPath string, checker ArchiveChecker) *ArchivedRepo {
	name := filepath.Base(repoPath)

	if !git.HasRemote(repoPath, "origin") {
		slog.Debug("skipping repo without origin remote", "repo", name)
		return nil
	}

	remoteURL, err := git.RemoteURL(repoPath, "origin")
	if err != nil {
		slog.Debug("could not get remote URL", "repo", name, "error", err)
		return nil
	}

	owner, repo, ok := github.ParseGitHubRemote(remoteURL)
	if !ok {
		slog.Debug("not a GitHub remote", "repo", name, "url", remoteURL)
		return nil
	}

	isArchived, err := checker.IsArchived(owner, repo)
	if err != nil {
		slog.Warn("could not check archive status", "repo", name, "error", err)
		return nil
	}

	if !isArchived {
		return nil
	}

	clean, err := git.IsClean(repoPath)
	if err != nil {
		slog.Warn("could not check working tree status", "repo", name, "error", err)
		clean = false // assume dirty when in doubt
	}

	return &ArchivedRepo{
		Path:    repoPath,
		Name:    name,
		Owner:   owner,
		Repo:    repo,
		IsClean: clean,
	}
}
