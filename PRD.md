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
   - Runs `git fetch` (always safe)
   - Checks for uncommitted changes (`git status --porcelain`)
   - **If clean**: `git pull --rebase` (or configured strategy)
   - **If dirty**:
     - Use `git merge-tree` to simulate merge and detect conflicts
     - If simulation shows conflicts → skip and report
     - If simulation is clean → `git stash && git pull --rebase && git stash pop`
     - If stash pop fails → `git stash pop --abort` and restore original state, skip and report
4. Provides summary of:
   - Successfully updated repos
   - Skipped repos (with reasons)
   - Repos needing manual attention

**Success Criteria**:
- Never loses uncommitted work
- Never leaves repos in conflicted state
- Uses `git merge-tree` to detect conflicts before attempting stash/pop
- Runs efficiently (parallel operations where possible)
- Supports selective sync (by repo pattern)
- Configurable sync strategy (rebase, merge, ff-only)

### 5. Finding Stale/Abandoned Branches

**User Story**: As a developer, I want to find branches I started but never finished, so I can clean them up or decide to complete the work.

**Workflow**:
1. User runs `katazuke branches --stale`
2. Tool scans all repositories for branches that:
   - Haven't been committed to in 30 days (configurable via `stale_threshold_days`)
   - "Touched" means last commit date on the branch (local or remote)
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
- Configurable staleness threshold (default: 30 days)
- Never deletes branches with unpushed commits
- Provides easy way to resume work
- Supports archiving branches as tags

## Technical Requirements

### Language & Platform

- **Language**: Go
  - Single binary deployment
  - Excellent cross-platform support
  - Strong standard library for file operations
  - Good CLI framework ecosystem
  - Fast execution

### Architecture

```
katazuke/
├── cmd/katazuke/              # CLI entry point and command definitions
│   ├── main.go                # CLI structure (kong), version, top-level flags
│   ├── branches.go            # branches --merged / --stale commands
│   ├── repos.go               # repos --archived command
│   ├── audit.go               # audit --non-git command
│   └── sync.go                # sync command
├── internal/
│   ├── audit/                 # Non-repo directory detection
│   ├── branches/              # Merged branch detection
│   ├── config/                # Configuration management
│   ├── github/                # GitHub API client
│   ├── repos/                 # Archived repo detection
│   ├── scanner/               # Repository discovery (.katazuke index)
│   └── sync/                  # Sync with conflict detection
├── pkg/
│   └── git/                   # Reusable git operations (shells out to git CLI)
└── test/
    ├── e2e/                   # E2E tests (build tag: e2e)
    └── helpers/               # Test utilities (git repo creation)
```

Packaging lives in separate repositories (see Packaging section below).

### Dependencies

- `github.com/alecthomas/kong` - CLI framework
- `github.com/charmbracelet/huh` - Interactive prompts
- `github.com/cli/go-gh/v2` - GitHub API client (leverages gh CLI auth)
- `github.com/fatih/color` - Colored terminal output
- `github.com/goccy/go-yaml` - YAML parsing

### Configuration

Support configuration via (highest priority last):
1. Built-in defaults
2. `$XDG_CONFIG_HOME/katazuke/config.yaml` (or `~/.config/katazuke/config.yaml`)
3. Environment variables (`KATAZUKE_*`)
4. CLI flags (highest priority)

**Configuration Options**:
```yaml
projects_dir: ~/projects
stale_threshold_days: 30  # Branch considered stale if no commits in N days
github_token: ghp_xxx     # Also: KATAZUKE_GITHUB_TOKEN, GITHUB_TOKEN, GH_TOKEN
exclude_patterns:
  - ".archive"
  - "vendor"
sync:
  strategy: rebase    # 'rebase', 'merge', or 'ff-only'
  skip_dirty: false   # If true, skip dirty repos without merge-tree check
  auto_stash: true    # If true, attempt stash/pop for dirty repos
```

**Environment variable overrides**: `KATAZUKE_PROJECTS_DIR`, `KATAZUKE_STALE_THRESHOLD_DAYS`, `KATAZUKE_SYNC_STRATEGY`, `KATAZUKE_SYNC_SKIP_DIRTY`, `KATAZUKE_SYNC_AUTO_STASH`

### Directory Structure Support

**Default Behavior**: Scan `~/projects` as a flat directory structure
- If no `.katazuke` file exists, assume each immediate child is a git repository
- Fast, single-level scan with no recursion
- This handles the common case without any configuration

**Configurable Root Path**:
- Support alternate paths via config: `projects_dir: /path/to/repos`
- Expand `~` to user home directory
- Validate path exists and is readable

**`.katazuke` Index File as Boundary Marker**:
- **Purpose**: Marks a directory as organizing repositories/groups (not a repository itself)
- **Location**: Can exist at any level in the hierarchy
- **Self-documenting structure**: Presence indicates "scan my subdirectories," absence means "my children are repos"

**Index File Format**:

Strict schema supporting only two fields. Written in YAML (JSON also supported as YAML superset):

```yaml
groups:
  - work
  - oss
  - experiments
ignores:
  - archive
  - tmp
  - old-projects
```

**Fields**:
- `groups`: List of subdirectories that contain repositories or nested `.katazuke` files (optional, defaults to empty list)
- `ignores`: List of subdirectories to skip during scanning (optional, defaults to empty list)

**Schema Validation**:
- Only `groups` and `ignores` fields are allowed
- Both fields must be lists of strings
- Unknown fields are rejected with error
- Empty file or missing fields treated as empty lists

**Example Hierarchies**:

*Flat structure (no `.katazuke` needed)*:
```
~/projects/
├── repo1/
├── repo2/
└── repo3/
```

*Single-level grouping with ignores*:
```
~/projects/
├── .katazuke          # groups: [work, oss, personal]
│                      # ignores: [archive, tmp]
├── work/
│   ├── repo1/
│   └── repo2/
├── oss/
│   └── project1/
├── personal/
│   └── experiment/
├── archive/           # Ignored
└── tmp/               # Ignored
```

*Nested grouping (unlimited depth)*:
```
~/projects/
├── .katazuke          # groups: [work, oss]
├── work/
│   ├── .katazuke      # groups: [client-a, client-b]
│   │                  # ignores: [deprecated]
│   ├── client-a/
│   │   ├── .katazuke  # groups: [frontend, backend]
│   │   ├── frontend/
│   │   │   ├── repo1/
│   │   │   └── repo2/
│   │   └── backend/
│   │       └── api/
│   ├── client-b/
│   │   └── project/
│   └── deprecated/    # Ignored
└── oss/
    ├── lib1/
    └── lib2/
```

**Scan Algorithm**:
1. Start at `projects_dir` (e.g., `~/projects`)
2. Look for `.katazuke` index file in current directory
3. **If `.katazuke` exists**:
   - Parse YAML/JSON and validate schema (only `groups` and `ignores` allowed)
   - Read `groups` list (default: empty)
   - Read `ignores` list (default: empty)
   - For each group directory:
     - Skip if in `ignores` list
     - Recursively descend and repeat from step 2
   - Scan non-group, non-ignored directories at this level as repositories
4. **If `.katazuke` does NOT exist**:
   - Assume all immediate child directories are repositories (respecting global `exclude_patterns`)
   - Stop recursion (don't scan deeper)
5. For each discovered repository, perform requested operations

**Initial Setup / Discovery**:
- If user runs `katazuke` in a directory without `.katazuke`:
  - Assume flat structure (children are repos)
- If user wants grouping:
  - Run `katazuke init` to create `.katazuke` interactively
  - Tool scans subdirectories and asks which are "groups" vs "repos" vs "ignored"
  - Generates `.katazuke` file(s) accordingly in YAML format
- User can manually create/edit `.katazuke` files in YAML or JSON

**Defensive Behavior**:
- If `.katazuke` contains unknown fields → reject with error message
- If `.katazuke` lists a group that doesn't exist → warn and skip
- If directory listed in both `groups` and `ignores` → ignore takes precedence, warn user
- If directory is in global `exclude_patterns` → skip entirely
- If directory is hidden (starts with `.`) → skip (except `.git`)
- Detect cycles (e.g., symlinks) and warn/skip

**Safety**:
- No arbitrary depth limits needed (`.katazuke` provides natural boundaries)
- Skip directories matching global `exclude_patterns`
- Skip directories in local `ignores` lists
- Ignore hidden directories (except `.git`)
- Track visited paths to prevent infinite loops from symlinks

### Git Authentication

- **No custom authentication handling**: Shell out to git commands and rely on user's existing git configuration
- User must have git properly configured (SSH keys, credential helpers, etc.)
- Private and public repositories are handled identically—git handles auth transparently
- If user needs to enter credentials manually, the tool will work but be tedious (user's problem to fix)

### Sync Strategy (Safe Conflict Detection)

**Goal**: Update repositories without leaving them in a conflicted or broken state.

**Algorithm**:
1. **Always fetch first**: `git fetch` is safe and never modifies working tree
2. **Check working tree status**: `git status --porcelain`
3. **For clean repos** (no uncommitted changes):
   - Execute configured strategy: `git pull --rebase` (or `--ff-only`, or merge)
   - If pull fails, report error and leave repo unchanged
4. **For dirty repos** (uncommitted changes):
   - **Simulate merge** using `git merge-tree`:
     ```bash
     git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main
     ```
   - If merge-tree output shows conflicts → skip repo and report
   - If merge-tree shows clean merge → safe to proceed:
     ```bash
     git stash push -m "katazuke auto-stash" &&
     git pull --rebase &&
     git stash pop
     ```
   - If `git stash pop` fails:
     ```bash
     git stash pop --abort  # or reset stash
     git pull --abort       # undo the pull
     ```
     Report failure and skip repo (original state restored)

**Configuration**:
```yaml
sync:
  strategy: rebase          # 'rebase', 'merge', or 'ff-only'
  skip_dirty: false         # If true, skip dirty repos without merge-tree check
  auto_stash: true          # If true, attempt stash/pop for dirty repos
```

**Safety guarantees**:
- Never lose uncommitted work
- Never leave repo in conflicted state
- Always report what happened (success, skipped, or failed)
- Provide undo/rollback if operation fails midway

### GitHub API Integration

**Client**: Uses `github.com/cli/go-gh/v2` which leverages `gh` CLI authentication
- Piggybacks on user's existing `gh` CLI config for auth
- Falls back to explicit token or unauthenticated access

**Authentication** (in order of precedence):
1. `gh` CLI config (if `gh` is installed and authenticated)
2. Personal access token from config file or env (`KATAZUKE_GITHUB_TOKEN`, `GITHUB_TOKEN`, `GH_TOKEN`)
3. Unauthenticated (lower rate limits, public repos only)

**Features**:
- Graceful degradation if GitHub unavailable or unauthenticated
- Respect rate limits (check remaining, pause if needed)
- Cache API responses appropriately (e.g., archive status TTL: 1 hour)
- GitHub token only needed for API calls (checking archived status, etc.), not git operations

**API Operations**:
- Check if repository is archived
- Get repository metadata (last push date, default branch)
- Optionally: check PR status for merged branches

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

- **No CI/CD**: Manual releases only via `just release VERSION` (intentional for a personal tool)
- Build binaries for supported platforms only:
  - `darwin/arm64` (Apple Silicon)
  - `linux/amd64` (x86_64)
- Attach binaries to GitHub releases via `gh` CLI
- Automated Homebrew formula and AUR PKGBUILD updates

## Success Metrics

### Philosophy: Actionable Metrics Only

**Core Principle**: We only track metrics that directly inform improvements to katazuke.

**The Actionability Test**: Before adding a metric, ask:
> "If this metric shows an unexpected pattern, what specific change would we make to katazuke?"

If the answer is unclear or "it's just interesting," don't track it.

**Why This Matters**:
- Prevents data hoarding (storing GB of unused JSON)
- Keeps implementation focused and simple
- Every metric has a purpose and drives decisions

### Metrics We Track (and Why)

#### 1. **Acceptance Rate by Action Type**
**Tracks**: Suggestions shown vs accepted, per action type (delete_merged_branch, delete_stale_branch, etc.)

**Action**: If acceptance <50% → detection logic is broken OR need "never suggest this" option

**Value**: Identifies which features work and which are just noise

#### 2. **Repeat Offenders**
**Tracks**: Items suggested multiple times without being accepted

**Action**: After 3+ suggestions → auto-add to ignore list OR improve detection logic

**Value**: Automatically identifies false positives

#### 3. **Command Usage Frequency**
**Tracks**: Which commands and flags are used

**Action**: Remove unused commands, prioritize development on popular features

**Value**: Guides where to invest development time

#### 4. **Performance Metrics**
**Tracks**: Scan duration, repos scanned, slow operations

**Action**: Optimize slow operations, warn users if performance degrades

**Value**: Keeps tool fast as repos/branches grow

#### 5. **Age Distribution**
**Tracks**: Age of items when accepted vs declined (for stale detection)

**Action**: If avg_accepted >> threshold → threshold is too conservative, auto-tune defaults

**Value**: Tune thresholds based on real usage patterns

#### 6. **Impact Counters** (motivational only)
**Tracks**: Branches deleted, disk freed, repos removed

**Action**: None - purely motivational ("freed 5GB this year!")

**Value**: Shows tangible value, encourages continued use

### Metrics We DON'T Track (and Why)

- **Dry-run vs Real-run Ratio**: Hard to interpret - low trust or just cautious users? No clear action.
- **Session Interruptions**: Don't know why (found nothing vs too annoying). Ambiguous signal.
- **Decision Time**: Interesting but unclear action (shorter prompts? more detail? context-dependent).
- **Workspace Health Score**: Cool gamification but doesn't tell us what to fix.
- **Time Saved**: Cannot quantify - how long would manual cleanup take? Unknowable.
- **Safety**: Users won't report tool errors; absence of complaints ≠ safety.

### Implementation

**Storage Location**: `~/.local/share/katazuke/metrics/`
- Event log files: `events-YYYY-MM.jsonl` (one file per month, JSONL format)
- Schema version tracked in each event for future compatibility
- **Storage cost**: ~200 bytes/event, ~730KB/year (10 suggestions/day) - negligible

**Minimal Event Schema** (v1):
```json
{
  "schema_version": 1,
  "timestamp": "2026-02-12T01:00:00Z",
  "session_id": "uuid",

  "suggestion": {
    "action_type": "delete_merged_branch",
    "item_fingerprint": "sha256-hash",
    "accepted": true
  },

  "command": {
    "name": "branches",
    "flags": ["--merged", "--dry-run"]
  },

  "perf": {
    "repos_scanned": 50,
    "scan_duration_ms": 4200
  },

  "age_days": 45,

  "impact": {
    "disk_freed_bytes": 102400
  }
}
```

**Privacy**: All metrics stay local, never transmitted anywhere.

### Future Analytics

With versioned event logs, we can later build:
- `katazuke stats` - Show personal cleanup statistics
- `katazuke tune` - Auto-adjust thresholds based on acceptance patterns
- Charts/graphs of cleanup activity over time

All analytics remain **local-only** and **optional**.

## Future Enhancements (Post-MVP)

**Worth Exploring**:
- **Plugin system for custom cleanup rules**: Consider whether base cleanup rules should be implemented as plugins or declarative configuration. This might benefit the core functionality itself and allow extensibility without bloat.

**Out of Scope**:
- GitLab/Bitbucket support (not aligned with core workflow)
- Web dashboard (unnecessary complexity)
- Git hooks integration (wrong level of automation)
- Team/organization shared configurations (personal tool)
- ML-based staleness detection (over-engineering)
- Backup tool integration (separate concern)

## Open Questions

(None currently - all design decisions have been made!)

## Developer Environment

**Philosophy**: Establish excellent DX before building core features.

### Testing Infrastructure

**E2E Tests**:
- Automated git repository scenario creation
- Tests should simulate real user workflows (create branches, merge PRs, etc.)
- Use built binary (debug mode) to perform cleanup operations
- Validate expected outcomes
- **Date simulation**: Use git commit timestamp manipulation to test stale detection without waiting 30 days
  - `GIT_COMMITTER_DATE` and `GIT_AUTHOR_DATE` environment variables
  - Or `git commit --date` flag to backdate commits

**Unit Tests**:
- Standard Go testing patterns
- Table-driven tests for complex logic
- Mocks for GitHub API interactions
- Coverage reporting

### Build Tooling

**Justfile** (common developer tasks):
- `just lint` - Run golangci-lint
- `just build` - Build binary for local platform
- `just test` - Run unit tests
- `just test-e2e` - Run end-to-end tests
- `just package-homebrew` - Build Homebrew package
- `just package-aur` - Build AUR package
- `just release VERSION` - Bump version, tag, build release artifacts
- `just install` - Install binary locally for testing

### Packaging

**Repository Structure**:
- **Main repo** (`agrahamlincoln/katazuke`): Source code only
- **Homebrew tap** (`agrahamlincoln/homebrew-katazuke`): Homebrew formula (follows Homebrew tap conventions)
- **AUR package** (`agrahamlincoln/aur-katazuke`): PKGBUILD for Arch Linux

**Homebrew**:
- Separate `homebrew-katazuke` repository (Homebrew convention)
- Install: `brew tap agrahamlincoln/katazuke && brew install katazuke`
- Formula automatically updated by release script

**AUR**:
- Separate `aur-katazuke` repository
- **Not published to aur.archlinux.org** (personal use only)
- Install: `paru -S https://github.com/agrahamlincoln/aur-katazuke.git`
- Or: `git clone https://github.com/agrahamlincoln/aur-katazuke.git && cd aur-katazuke && makepkg -si`
- PKGBUILD automatically updated by release script

**Benefits of Separate Repos**:
- Clean separation: main repo = code, packaging repos = distribution
- Follows ecosystem conventions (Homebrew tap pattern, AUR pattern)
- No chicken-and-egg problems with checksums
- Packaging updates don't clutter main repo history

### CI/CD

**No automated CI initially**:
- No GitHub Actions workflows
- Manual release process via justfile commands
- Scripts that mimic CI tasks (build, test, package)
- Can be triggered manually: `just release 0.1.0`

### Minimal Viable Binary (COMPLETE)

1. Basic CLI structure (kong)
2. Help text (`katazuke --help`)
3. Version command (`katazuke version`)
4. Packaging repositories (`homebrew-katazuke`, `aur-katazuke`)
5. Successful packaging for Homebrew and AUR
6. Installation and execution on both macOS and Linux

### Release Automation

**Fully automated release process** via `just release VERSION`:
1. Build binaries for all platforms (darwin-arm64, linux-amd64)
2. Create release tarballs
3. Calculate SHA256 checksums
4. Update Homebrew formula in `../homebrew-katazuke` repo
5. Commit, tag, and push main repo
6. Create GitHub release with binary tarballs
7. Download GitHub-generated source tarball
8. Calculate source tarball SHA256
9. Update PKGBUILD in `../aur-katazuke` repo
10. Commit and push both packaging repos

**No manual steps required** - developer only runs `just release 0.2.0`

## Timeline

- **Phase 0** (Foundation): Developer environment, testing infrastructure, packaging -- COMPLETE
- **Phase 1** (MVP): Branch cleanup (merged) + archived repo detection -- COMPLETE
- **Phase 2**: Sync automation + non-git directory audit + configuration system -- COMPLETE
- **Phase 3**: Stale branch detection + remote branch cleanup + metrics -- NOT STARTED
- **Phase 4**: Polish, documentation
