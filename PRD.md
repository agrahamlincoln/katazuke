# Product Requirements Document: katazuke

## Vision

**katazuke** (片付け - "tidying up") is a developer workspace maintenance tool that helps engineers maintain clean, organized project directories by automating the discovery and cleanup of stale branches, archived repositories, and out-of-date checkouts.

## Goals

1. **Reduce cognitive overhead** of managing multiple repository checkouts
2. **Prevent accidental data loss** through safe, user-confirmed operations
3. **Save time** by automating repetitive git maintenance tasks
4. **Improve workspace hygiene** by making cleanup a regular, low-friction activity
5. **Cross-platform support** for macOS (Homebrew) and Linux (AUR/Arch)

## Non-Goals

- Managing non-git version control systems
- Replacing git commands or becoming a git wrapper
- Cloud synchronization or backup features
- IDE or editor integration (CLI-first approach)
- Managing dependencies or build artifacts

## Core User Journeys

### 1. Cleaning Up Merged Branches

**User Story**: As a developer, I want to remove local branches that have been merged and no longer need to exist, so my branch list stays manageable.

**Workflow**:
1. User runs `katazuke branches --merged`
2. Tool scans all repositories in `~/projects`
3. For each repo, identifies branches that:
   - Exist locally
   - Have been pushed to remote
   - Have been merged into main/master
   - Are not currently checked out
4. Presents list with details (last commit date, merge date, PR link if available)
5. User confirms deletion (batch or individual)
6. Tool deletes local branches and optionally remote branches

**Success Criteria**:
- Correctly identifies merged branches
- Never suggests deleting current branch
- Shows clear justification for each deletion
- Supports dry-run mode

### 2. Removing Archived Repository Checkouts

**User Story**: As a developer, I want to identify and remove checkouts of repositories that have been archived on GitHub, so I don't waste disk space on defunct projects.

**Workflow**:
1. User runs `katazuke repos --archived`
2. Tool scans all git repositories in `~/projects`
3. For each repo with a GitHub remote:
   - Queries GitHub API to check archive status
   - Checks for recent activity (commits, issues, PRs)
4. Presents list of archived/defunct repos with:
   - Archive date
   - Last commit date
   - Repository size
   - Path on disk
5. User confirms removal
6. Tool removes directory (with option to backup first)

**Success Criteria**:
- Accurately detects archived status via GitHub API
- Handles repositories without remotes gracefully
- Provides option to create backup archive before deletion
- Never deletes repos with uncommitted changes

### 3. Identifying Non-Git Directories

**User Story**: As a developer, I want to find directories in `~/projects` that aren't git repositories, so I can decide if they belong there or should be moved/removed.

**Workflow**:
1. User runs `katazuke audit --non-git`
2. Tool scans `~/projects` for directories without `.git`
3. Presents list with:
   - Directory name
   - Size
   - Last modified date
   - Contents summary (file types, notable files)
4. User decides action: keep, move, or remove
5. Tool executes user's choice

**Success Criteria**:
- Identifies all non-git directories
- Excludes common expected directories (.DS_Store files, hidden dirs)
- Provides helpful context about directory contents
- Supports moving to a "quarantine" location

### 4. Keeping Repositories Up-to-Date

**User Story**: As a developer, I want to automatically update all my repositories before starting work, so I don't have to manually run `git checkout main && git pull` in each one.

**Workflow**:
1. User runs `katazuke sync`
2. Tool scans all repositories in `~/projects`
3. For each repository:
   - Checks if on main/master branch
   - Checks for uncommitted changes
   - If clean: `git checkout main && git pull`
   - If dirty: offers to stash, pull, and pop stash
   - If conflicts: skips and reports
4. Provides summary of:
   - Successfully updated repos
   - Skipped repos (with reasons)
   - Repos with conflicts needing attention

**Success Criteria**:
- Never loses uncommitted work
- Handles stash operations correctly
- Detects and reports merge conflicts
- Runs efficiently (parallel operations where possible)
- Supports selective sync (by repo pattern)

### 5. Finding Stale/Abandoned Branches

**User Story**: As a developer, I want to find branches I started but never finished, so I can clean them up or decide to complete the work.

**Workflow**:
1. User runs `katazuke branches --stale`
2. Tool scans all repositories for branches that:
   - Haven't been committed to in X days (configurable, default 60)
   - Exist both locally and remotely
   - Are not merged
   - Are not main/master/develop
3. Presents list with:
   - Branch name
   - Last commit date and message
   - Commits ahead/behind main
   - Remote branch status
4. User chooses action per branch:
   - Delete (local and remote)
   - Keep working (checkout)
   - Archive (tag and delete)
   - Ignore (add to config)
5. Tool executes actions with confirmation

**Success Criteria**:
- Configurable staleness threshold
- Never deletes branches with unpushed commits
- Provides easy way to resume work
- Supports archiving branches as tags

## Technical Requirements

### Language & Platform

- **Language**: Go
  - Single binary deployment
  - Excellent cross-platform support
  - Strong standard library for file operations
  - Good CLI framework ecosystem (cobra)
  - Fast execution

### Architecture

```
katazuke/
├── cmd/
│   └── katazuke/
│       └── main.go           # CLI entry point
├── internal/
│   ├── audit/                # Workspace auditing logic
│   ├── branches/             # Branch management
│   ├── repos/                # Repository operations
│   ├── sync/                 # Sync/update operations
│   ├── github/               # GitHub API client
│   └── config/               # Configuration management
├── pkg/
│   └── git/                  # Reusable git operations
├── homebrew/                 # Homebrew formula
├── aur/                      # AUR package files
└── docs/
```

### Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/go-git/go-git/v5` - Git operations (or shell out to git CLI)
- `github.com/google/go-github/v58` - GitHub API client
- `github.com/fatih/color` - Colored terminal output
- `github.com/AlecAivazis/survey/v2` - Interactive prompts

### Configuration

Support configuration via:
1. `~/.config/katazuke/config.yaml` - User preferences
2. `~/.katazukerc` - Legacy support
3. Environment variables (`KATAZUKE_*`)
4. CLI flags (highest priority)

**Configuration Options**:
```yaml
projects_dir: ~/projects
stale_threshold_days: 60
github_token: ghp_xxx  # Optional, for higher API limits
exclude_patterns:
  - ".archive"
  - "vendor"
prompts:
  batch_operations: true
  confirm_deletions: true
backup:
  enabled: true
  location: ~/katazuke-backups
```

### GitHub API Integration

- Support both personal access tokens and GitHub CLI auth
- Graceful degradation if GitHub unavailable
- Respect rate limits
- Cache API responses appropriately

### Safety Features

1. **Dry Run Mode**: `--dry-run` flag for all operations
2. **Backups**: Optional automatic backups before deletions
3. **Confirmation Prompts**: Required for destructive operations
4. **Detailed Justifications**: Explain why each item is flagged
5. **Undo Support**: Keep operation logs for reversal
6. **Uncommitted Changes Protection**: Never touch repos with uncommitted work

## Installation & Distribution

### macOS - Homebrew Tap

1. Create `homebrew-katazuke` repository
2. Formula downloads pre-built binary from GitHub releases
3. Installation: `brew install agrahamlincoln/katazuke/katazuke`

### Arch Linux - AUR

1. Create PKGBUILD in `aur/` directory
2. PKGBUILD downloads source and compiles
3. Installation: `makepkg -si`

### Build Process

- GitHub Actions for CI/CD
- Build binaries for multiple platforms:
  - `linux/amd64`
  - `linux/arm64`
  - `darwin/amd64` (Intel Mac)
  - `darwin/arm64` (Apple Silicon)
- Attach binaries to GitHub releases
- Automated Homebrew formula updates

## Success Metrics

- **Time Saved**: Average time saved per week on manual git maintenance
- **Disk Space Recovered**: Total space freed by cleanup operations
- **Error Prevention**: Incidents avoided (accidental work on stale branches, etc.)
- **Adoption**: Active users, repeat usage rate
- **Safety**: Zero reported cases of accidental data loss

## Future Enhancements (Post-MVP)

- GitLab/Bitbucket support
- Web dashboard for workspace statistics
- Integration with git hooks for automatic cleanup
- Team/organization shared configurations
- Machine learning for smarter staleness detection
- Plugin system for custom cleanup rules
- Integration with backup tools (Time Machine, restic)

## Open Questions

1. Should we support nested git repositories (submodules, mono repos)?
2. How should we handle private repositories vs public?
3. Should sync operation support rebasing instead of pulling?
4. What's the right default for stale threshold (30, 60, 90 days)?
5. Should we integrate with GitHub CLI (`gh`) or use API directly?

## Timeline (Proposed)

- **Phase 1** (MVP): Branch cleanup + archived repo detection
- **Phase 2**: Sync automation + non-git directory detection
- **Phase 3**: Stale branch detection + configuration system
- **Phase 4**: Polish, testing, packaging (Homebrew + AUR)
