// Package repos provides operations for managing repository checkouts
// in the projects directory.
package repos

import (
	"log/slog"
	"path/filepath"

	"github.com/agrahamlincoln/katazuke/internal/github"
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
func FindArchived(repos []string, checker ArchiveChecker) ([]ArchivedRepo, error) {
	var archived []ArchivedRepo

	for _, repoPath := range repos {
		name := filepath.Base(repoPath)

		if !git.HasRemote(repoPath, "origin") {
			slog.Debug("skipping repo without origin remote", "repo", name)
			continue
		}

		remoteURL, err := git.RemoteURL(repoPath, "origin")
		if err != nil {
			slog.Debug("could not get remote URL", "repo", name, "error", err)
			continue
		}

		owner, repo, ok := github.ParseGitHubRemote(remoteURL)
		if !ok {
			slog.Debug("not a GitHub remote", "repo", name, "url", remoteURL)
			continue
		}

		isArchived, err := checker.IsArchived(owner, repo)
		if err != nil {
			slog.Warn("could not check archive status", "repo", name, "error", err)
			continue
		}

		if !isArchived {
			continue
		}

		clean, err := git.IsClean(repoPath)
		if err != nil {
			slog.Warn("could not check working tree status", "repo", name, "error", err)
			clean = false // assume dirty when in doubt
		}

		archived = append(archived, ArchivedRepo{
			Path:    repoPath,
			Name:    name,
			Owner:   owner,
			Repo:    repo,
			IsClean: clean,
		})
	}

	return archived, nil
}
