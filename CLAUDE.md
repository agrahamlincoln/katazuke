# Claude Code Context for katazuke

## Project Overview

**katazuke** (片付け - "tidying up") is a personal developer workspace maintenance tool for managing git repository clutter. It's designed specifically for a PR-based workflow with feature branches that accumulate over time.

## Core Philosophy

### Opinionated by Design

This tool is **intentionally tailored** to a specific workflow (see README.md "Who is this for?" section). Do not add features or options that deviate from this core workflow unless explicitly requested. The tool should remain focused and opinionated.

### Quality First

- No emojis in code, scripts, or automation
- Fix code issues rather than suppressing linter warnings
- Maintain high code quality from the start - don't compromise

## Workflow Context

**Primary workflow**: PR-based development with feature branches (`graham/<name>` for private repos)

**The problem we solve**: Local branches that never get cleaned up after PRs are merged, leading to 50+ stale branches with meaningless short names.

**Not supported**: Other workflows, other version control systems, cloud sync, backup features. See PRD.md for explicit non-goals.

## Repository Structure

### Main Repository
- **Purpose**: Source code only
- **Location**: `agrahamlincoln/katazuke`
- **Contents**: Go code, tests, justfile, documentation

### Packaging Repositories
- **Homebrew**: `agrahamlincoln/homebrew-katazuke` (follows Homebrew tap convention)
- **AUR**: `agrahamlincoln/aur-katazuke` (personal package, not on official AUR)

**Why separate?**: Clean separation of concerns, follows ecosystem conventions, avoids chicken-and-egg checksum problems, keeps packaging history out of main repo.

**Local clones**: Expected at `~/projects/homebrew-katazuke` and `~/projects/aur-katazuke` (sibling directories). The release process requires these.

## Platform Support

**Supported**:
- macOS: darwin-arm64 (Apple Silicon only - developer's machine)
- Linux: linux-amd64 (x86_64 only)

**Not supported**: Intel Macs, ARM Linux - developer doesn't have these devices.

## Development Tools

### Build System
- **Primary**: `just` (justfile) - all development tasks
- **Removed**: Makefile (deleted, do not recreate)
- **Key commands**: `just setup`, `just build`, `just test`, `just lint`, `just release VERSION`

### Testing
- **Unit tests**: Standard Go tests (`*_test.go`)
- **E2E tests**: Build tag `e2e`, located in `test/e2e/`
- **Test helpers**: `test/helpers/git.go` - creates git repos with backdated commits for testing stale detection
- **Key insight**: Use `CommitWithDate()` to test 30-day thresholds without waiting 30 days

### Linting
- **Tool**: golangci-lint (comprehensive config in `.golangci.yaml`)
- **Formatters**: gofmt, goimports (separate from linters in config)
- **Strategy**: Fix issues, don't suppress them (no nolint comments unless absolutely necessary with explanation)

### Release Process

**Fully automated** via `just release VERSION`:
1. Builds for both platforms
2. Creates tarballs, calculates SHA256s
3. Updates Homebrew formula in `../homebrew-katazuke`
4. Commits, tags, pushes main repo
5. Creates GitHub release
6. Downloads source tarball, calculates SHA256
7. Updates PKGBUILD in `../aur-katazuke`
8. Commits and pushes both packaging repos

**Dependencies**: `gh` CLI for creating releases

**No CI/CD**: Manual releases only, no GitHub Actions. This is intentional for a personal tool.

## Metrics Philosophy

**Core principle**: Only track metrics that inform specific improvements to katazuke.

**The actionability test**: Before adding a metric, ask: "If this metric shows an unexpected pattern, what specific change would we make to katazuke?" If the answer is unclear, don't track it.

**What we track**:
1. Acceptance rate by action type → identifies broken features
2. Repeat offenders → identifies false positives
3. Command usage → guides development priorities
4. Performance metrics → keeps tool fast
5. Age distribution → tunes thresholds
6. Impact counters → motivational only

**Storage**: Local only (`~/.local/share/katazuke/metrics/`), JSONL format, versioned schema, ~730KB/year

See PRD.md "Success Metrics" for full rationale.

## Code Standards

### Commit Messages
- Use conventional commits format
- Subject: <72 characters
- Body lines: <80 characters
- Focus on why/what-changed, not how (implementation details in code)
- Examine git diff to understand actual changes

### Comments
- Explain "why" (intent, business logic), not "what" (code is self-documenting)
- **Exception**: Go doc comments MUST start with symbol name (Go convention)
- No redundant comments (don't restate function signatures)
- Keep comments updated with code changes

### Go Idioms
- Follow standard Go project layout (`cmd/`, `internal/`, `pkg/`)
- Use `golangci-lint` - comprehensive linter set enabled
- Prefer small, focused functions
- Avoid over-engineering (no premature abstractions)

## Directory Structure

```
katazuke/
├── cmd/katazuke/          # CLI entry point (main.go)
├── internal/              # Private packages (will have: audit/, branches/, repos/, sync/, config/)
├── pkg/git/              # Reusable git operations (importable)
├── test/
│   ├── e2e/              # E2E tests (build tag: e2e)
│   └── helpers/          # Test utilities (git repo creation, etc.)
├── justfile              # Build automation (replaces Makefile)
├── PRD.md               # Product requirements (comprehensive design doc)
├── README.md            # User-facing documentation
└── CLAUDE.md            # This file
```

## Common Patterns

### `.katazuke` Index Files

**Purpose**: Marks directories that organize repos/groups (not repos themselves)

**Format**: YAML with strict schema (only `groups` and `ignores` fields allowed)

**Behavior**:
- If present → scan subdirectories listed in `groups`, ignore listed in `ignores`
- If absent → assume immediate children are repositories (stop recursion)
- Allows unlimited nesting depth without arbitrary limits

**Example**:
```yaml
groups:
  - work
  - oss
ignores:
  - archive
  - tmp
```

### Workflow Detection

The tool should be **defensive** and detect when actual workflow differs from expected patterns. Never take destructive actions if workflow doesn't match expectations.

## What NOT to Do

- ❌ Add emojis anywhere (code, scripts, justfile, output)
- ❌ Add support for GitLab/Bitbucket/other VCS
- ❌ Add web dashboards or GUIs
- ❌ Add cloud sync or backup features
- ❌ Add team/organization features
- ❌ Add ML-based detection
- ❌ Make the tool work for workflows other than the documented one
- ❌ Suppress linter warnings without fixing the actual issue
- ❌ Create files unless explicitly necessary (prefer editing existing)
- ❌ Add features for hypothetical future requirements

## Key Files

- **PRD.md**: Source of truth for all design decisions, comprehensive
- **README.md**: User-facing docs, workflow context
- **justfile**: All development automation (build, test, lint, release)
- **test/helpers/git.go**: Critical for E2E testing (date manipulation)
- **.golangci.yaml**: Comprehensive linting config (formatters separate from linters)

## Current State

**Phase 0**: Developer environment foundation
- ✅ Basic CLI with stub commands
- ✅ Testing infrastructure (unit + e2e framework)
- ✅ Justfile with full automation
- ✅ Linting configuration
- ✅ Packaging repos created and populated (`homebrew-katazuke`, `aur-katazuke`)
- ✅ Release automation updated to work with sibling packaging repos
- ⏳ No core features implemented yet

**Next steps**:
1. Begin implementing Phase 1 features (branch cleanup, archived repo detection)

## Questions to Ask

When implementing features, consider:
1. Does this align with the opinionated workflow?
2. Can we track a metric to validate this feature works?
3. Will this be annoying (high decline rate)?
4. Is this the simplest possible implementation?
5. Have we tested this with backdated git commits?

## Resources

- PRD.md: Complete product requirements and design decisions
- README.md: User documentation and workflow context
- justfile: `just --list` shows all available commands
- Reference repos: `~/projects/squirrel-bot` and `~/projects/freshfire` (for Go patterns, justfile structure)
