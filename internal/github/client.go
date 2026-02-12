// Package github provides a client for querying the GitHub API,
// primarily to check repository metadata such as archive status.
package github

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Client wraps GitHub API access.
type Client struct {
	rest  *api.RESTClient
	token string
}

// NewClient creates a GitHub client. It attempts to use authentication from
// the gh CLI config, falling back to the provided token, falling back to
// unauthenticated access.
func NewClient(token string) *Client {
	c := &Client{token: token}

	// Try default gh CLI authentication first.
	rest, err := api.DefaultRESTClient()
	if err == nil {
		slog.Debug("using gh CLI authentication")
		c.rest = rest
		return c
	}
	slog.Debug("gh CLI auth not available", "error", err)

	// Fall back to explicit token.
	if token != "" {
		rest, err = api.NewRESTClient(api.ClientOptions{
			AuthToken: token,
		})
		if err == nil {
			slog.Debug("using explicit token authentication")
			c.rest = rest
			return c
		}
		slog.Debug("token auth failed", "error", err)
	}

	// Unauthenticated -- will hit rate limits quickly.
	slog.Debug("using unauthenticated access (rate limits apply)")
	rest, err = api.NewRESTClient(api.ClientOptions{})
	if err != nil {
		slog.Warn("could not create REST client", "error", err)
		return c
	}
	c.rest = rest
	return c
}

// repoResponse holds the fields we care about from GET /repos/{owner}/{repo}.
type repoResponse struct {
	Archived bool `json:"archived"`
}

// IsArchived checks if a repository is archived on GitHub.
func (c *Client) IsArchived(owner, repo string) (bool, error) {
	if c.rest == nil {
		return false, fmt.Errorf("no GitHub API client available")
	}

	var resp repoResponse
	err := c.rest.Get(fmt.Sprintf("repos/%s/%s", owner, repo), &resp)
	if err != nil {
		return false, fmt.Errorf("querying %s/%s: %w", owner, repo, err)
	}
	return resp.Archived, nil
}

// PRState represents the state of a GitHub pull request for a branch.
type PRState string

const (
	// PRStateNone means no PR was found for the branch.
	PRStateNone PRState = "none"
	// PRStateOpen means a PR is currently open.
	PRStateOpen PRState = "open"
	// PRStateMerged means the PR was merged.
	PRStateMerged PRState = "merged"
	// PRStateClosed means the PR was closed without merging.
	PRStateClosed PRState = "closed"
)

// prSearchResponse holds the response from the GitHub pulls API.
type prSearchResponse struct {
	State    string `json:"state"`
	MergedAt string `json:"merged_at"`
}

// BranchPRState returns the PR state for a branch. It checks the most recent
// PR associated with the given head branch. Returns PRStateNone if no PR
// exists for the branch.
func (c *Client) BranchPRState(owner, repo, branch string) (PRState, error) {
	if c.rest == nil {
		return PRStateNone, fmt.Errorf("no GitHub API client available")
	}

	var prs []prSearchResponse
	err := c.rest.Get(
		fmt.Sprintf("repos/%s/%s/pulls?head=%s:%s&state=all&per_page=1&sort=updated&direction=desc",
			owner, repo, owner, branch),
		&prs,
	)
	if err != nil {
		return PRStateNone, fmt.Errorf("querying PRs for %s/%s branch %s: %w", owner, repo, branch, err)
	}

	if len(prs) == 0 {
		return PRStateNone, nil
	}

	pr := prs[0]
	if pr.State == "open" {
		return PRStateOpen, nil
	}
	if pr.MergedAt != "" {
		return PRStateMerged, nil
	}
	return PRStateClosed, nil
}

// sshRemoteRe matches SSH-style GitHub remote URLs:
//
//	git@github.com:owner/repo.git
var sshRemoteRe = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)

// ParseGitHubRemote extracts owner and repo from a GitHub remote URL.
// Supports both SSH (git@github.com:owner/repo.git) and HTTPS
// (https://github.com/owner/repo.git) formats.
func ParseGitHubRemote(url string) (owner, repo string, ok bool) {
	// Try SSH format first.
	if m := sshRemoteRe.FindStringSubmatch(url); m != nil {
		return m[1], m[2], true
	}

	// Try HTTPS format.
	url = strings.TrimSuffix(url, ".git")
	for _, prefix := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(url, prefix) {
			rest := strings.TrimPrefix(url, prefix)
			parts := strings.SplitN(rest, "/", 3)
			if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
				return parts[0], parts[1], true
			}
		}
	}

	return "", "", false
}
