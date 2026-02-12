package github

import "testing"

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "ssh with .git suffix",
			url:       "git@github.com:agrahamlincoln/katazuke.git",
			wantOwner: "agrahamlincoln",
			wantRepo:  "katazuke",
			wantOK:    true,
		},
		{
			name:      "ssh without .git suffix",
			url:       "git@github.com:agrahamlincoln/katazuke",
			wantOwner: "agrahamlincoln",
			wantRepo:  "katazuke",
			wantOK:    true,
		},
		{
			name:      "https with .git suffix",
			url:       "https://github.com/agrahamlincoln/katazuke.git",
			wantOwner: "agrahamlincoln",
			wantRepo:  "katazuke",
			wantOK:    true,
		},
		{
			name:      "https without .git suffix",
			url:       "https://github.com/agrahamlincoln/katazuke",
			wantOwner: "agrahamlincoln",
			wantRepo:  "katazuke",
			wantOK:    true,
		},
		{
			name:      "http url",
			url:       "http://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
		{
			name:   "non-github ssh",
			url:    "git@gitlab.com:owner/repo.git",
			wantOK: false,
		},
		{
			name:   "non-github https",
			url:    "https://gitlab.com/owner/repo.git",
			wantOK: false,
		},
		{
			name:   "empty string",
			url:    "",
			wantOK: false,
		},
		{
			name:   "random string",
			url:    "/some/local/path",
			wantOK: false,
		},
		{
			name:   "github url with no repo",
			url:    "https://github.com/owner",
			wantOK: false,
		},
		{
			name:      "github url with extra path segments",
			url:       "https://github.com/owner/repo/tree/main",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := ParseGitHubRemote(tt.url)
			if ok != tt.wantOK {
				t.Errorf("ParseGitHubRemote(%q) ok = %v, want %v", tt.url, ok, tt.wantOK)
				return
			}
			if !ok {
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("ParseGitHubRemote(%q) owner = %q, want %q", tt.url, owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("ParseGitHubRemote(%q) repo = %q, want %q", tt.url, repo, tt.wantRepo)
			}
		})
	}
}
