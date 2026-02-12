# Claude Code Context for katazuke

## Project Overview

**katazuke** (片付け - "tidying up") is a personal developer workspace maintenance tool for managing git repository clutter. It's designed specifically for a PR-based workflow with feature branches that accumulate over time.

## Core Philosophy

### Opinionated by Design

This tool is **intentionally tailored** to a specific workflow (see README.md "Who is this for?" section). Do not add features or options that deviate from this core workflow unless explicitly requested. The tool should remain focused and opinionated.

### Quality First

- No emojis in code, scripts, or automation output
- Fix code issues rather than suppressing linter warnings
- Maintain high code quality from the start

### Safety First

The tool is **defensive by design**: detect when actual workflow differs from expected patterns. Never take destructive actions if workflow doesn't match expectations. Never lose uncommitted work. Never leave repos in a conflicted state.

## Workflow Context

**Primary workflow**: PR-based development with feature branches (`graham/<name>` for private repos)

**The problem we solve**: Local branches that never get cleaned up after PRs are merged, leading to 50+ stale branches with meaningless short names. Also: archived repos taking up space, non-git clutter, and repos falling behind upstream.

**Not supported**: Other workflows, other version control systems, cloud sync, backup features, team/org features. See PRD.md for explicit non-goals.

## Repository & Release Structure

### Main Repository (`agrahamlincoln/katazuke`)
- Source code only (Go code, tests, justfile, documentation)

### Packaging Repositories (sibling directories)
- **Homebrew**: `agrahamlincoln/homebrew-katazuke` (expected at `~/projects/homebrew-katazuke`)
- **AUR**: `agrahamlincoln/aur-katazuke` (expected at `~/projects/aur-katazuke`, personal package)

Packaging lives in separate repos to follow ecosystem conventions, avoid checksum chicken-and-egg problems, and keep packaging history out of the main repo. The release script (`just release VERSION`) updates both automatically.

### Platform Support
- macOS: darwin-arm64 (Apple Silicon only)
- Linux: linux-amd64 (x86_64 only)
- No CI/CD: Manual releases via `just release VERSION` using `gh` CLI

## Development

### Build System
- **Primary**: `just` (justfile) -- run `just --list` for all commands
- **Removed**: Makefile (deleted, do not recreate)
- Key: `just build`, `just test`, `just lint`, `just test-e2e`, `just release VERSION`

### Testing
- **Unit tests**: Standard Go tests (`*_test.go`) alongside source
- **E2E tests**: Build tag `e2e`, located in `test/e2e/`
- **Test helpers**: `test/helpers/git.go` -- creates git repos with backdated commits for testing time-based thresholds without waiting
- **Key insight**: Use `CommitWithDate()` to test staleness thresholds

### Linting
- **Tool**: golangci-lint (config in `.golangci.yaml`)
- **Formatters**: gofmt, goimports (separate from linters in config)
- **Strategy**: Fix issues, don't suppress them (no nolint comments unless absolutely necessary with explanation)

### Dependencies
Dependencies are in `go.mod`. Key choices worth noting:
- **CLI parsing**: kong (not cobra)
- **Interactive prompts**: charmbracelet/huh
- **GitHub API**: cli/go-gh (leverages `gh` CLI auth, not go-github)
- **Git operations**: Shells out to git CLI via `pkg/git/` (not go-git)

## Project Layout

Standard Go project layout: `cmd/`, `internal/`, `pkg/`, `test/`. Each command (`branches`, `repos`, `audit`, `sync`) has a corresponding file in `cmd/katazuke/` and business logic in `internal/`.

Key conventions:
- `internal/scanner/` handles repository discovery using `.katazuke` index files
- `internal/config/` handles layered configuration: defaults -> config file -> env vars -> CLI flags
- `pkg/git/` is the shared git wrapper (shells out, not a library)
- Config file location: `$XDG_CONFIG_HOME/katazuke/config.yaml`
- Env var prefix: `KATAZUKE_*`
- GitHub token: `gh` CLI auth -> `KATAZUKE_GITHUB_TOKEN` -> `GITHUB_TOKEN` / `GH_TOKEN`

## Architecture Patterns

- **Interface-based testing**: Core operations (git, GitHub API) are behind interfaces so business logic can be tested with mocks. See `internal/sync/` for the most thorough example.
- **Config layering**: Defaults -> file -> env vars -> CLI flags. All in `internal/config/`.
- **Progress callbacks**: Commands pass a `ProgressFunc` to display real-time status.
- **Scanner with .katazuke index**: Repository discovery respects `.katazuke` YAML files that define `groups` (subdirs to scan) and `ignores` (subdirs to skip). Without an index file, immediate children are assumed to be repos.

## Code Standards

### Commit Messages
- Conventional commits format
- Subject: <72 characters, body lines: <80 characters
- Focus on why/what-changed, not how

### Comments
- Explain "why" (intent, business logic), not "what"
- **Exception**: Go doc comments MUST start with symbol name (Go convention)
- No redundant comments; keep comments updated with code changes

### Go Idioms
- Follow standard Go project layout
- Use `golangci-lint` -- comprehensive linter set enabled
- Prefer small, focused functions
- Avoid over-engineering

## What NOT to Do

- Add emojis anywhere (code, scripts, justfile, output)
- Add support for GitLab/Bitbucket/other VCS
- Add web dashboards or GUIs
- Add cloud sync or backup features
- Add team/organization features
- Add ML-based detection
- Make the tool work for workflows other than the documented one
- Suppress linter warnings without fixing the actual issue
- Create files unless explicitly necessary (prefer editing existing)
- Add features for hypothetical future requirements
- Recreate the Makefile

## Metrics Philosophy

Only track metrics that inform specific improvements to katazuke. Before adding a metric, ask: "If this metric shows an unexpected pattern, what specific change would we make to katazuke?" If the answer is unclear, don't track it.

Storage: Local only (`~/.local/share/katazuke/metrics/`), JSONL format, versioned schema. See PRD.md "Success Metrics" for full rationale.

## Design Checklist

When implementing features, consider:
1. Does this align with the opinionated workflow?
2. Can we track a metric to validate this feature works?
3. Will this be annoying (high decline rate)?
4. Is this the simplest possible implementation?
5. Have we tested this with backdated git commits?

## Key References

- **PRD.md**: Product requirements and design decisions (user journeys and metrics philosophy are authoritative; some technical details like dependencies were updated during implementation and may not match)
- **README.md**: User-facing documentation and workflow context
- **justfile**: `just --list` for all development commands
