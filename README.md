# katazuke (片付け)

**katazuke** (pronounced *kah-tah-zoo-keh*) is a Japanese term meaning "tidying up" or "putting things in order."

A developer workspace maintenance tool that helps you keep your `~/projects` directory clean and organized by managing stale branches, archived repositories, and out-of-date checkouts.

## Overview

As developers, our project directories accumulate clutter over time:
- Merged branches that were never cleaned up
- Archived repositories still taking up space
- Non-git directories that don't belong
- Stale local checkouts needing updates
- Abandoned branches languishing locally and remotely

`katazuke` automates the discovery and cleanup of these issues, helping you maintain a tidy development workspace with confidence.

## Who is this for?

**This tool is opinionated and designed for a specific PR-based workflow:**
- You work on feature branches (`graham/<short-name>` for private repos, `<short-name>` for OSS forks)
- You open PRs, merge them, and remote branches auto-delete
- You **forget to clean up local branches**, leaving dozens of stale branches cluttering your workspace

If this sounds familiar, `katazuke` will help. If your workflow differs significantly, this tool may not be the right fit. See [Workflow Context](#workflow-context) for details.

## Installation

### macOS (Homebrew)

```bash
brew tap agrahamlincoln/katazuke
brew install katazuke
```

### Arch Linux

Using an AUR helper like `paru`:

```bash
paru -S https://github.com/agrahamlincoln/aur-katazuke.git
```

Or manually:

```bash
git clone https://github.com/agrahamlincoln/aur-katazuke.git
cd aur-katazuke
makepkg -si
```

**Note**: This is a personal package, not published to the official AUR.

## Features

- 🌿 **Branch Cleanup**: Identify and remove merged branches
- 📦 **Archive Detection**: Find and remove archived/defunct repository checkouts
- 🔍 **Directory Validation**: Detect non-git directories in your projects folder
- 🔄 **Sync Automation**: Keep repositories up-to-date automatically
- 🗑️ **Stale Branch Detection**: Find abandoned branches (local and remote)
- ✅ **Safe Operations**: User prompts with justification before any deletion

## Usage

```bash
# Run full workspace audit
katazuke audit

# Clean up merged branches
katazuke branches --merged

# Remove archived repositories
katazuke repos --archived

# Update all repositories
katazuke sync

# Find stale branches
katazuke branches --stale
```

## Workflow Context

`katazuke` is designed around a specific contributor workflow. Understanding this context helps explain design decisions and feature priorities.

### Directory Structure

All repository checkouts live in a **flat directory structure** at `~/projects` by default:

```
~/projects/
├── repo1/
├── repo2/
├── some-library/
└── client-project/
```

While `katazuke` defaults to `~/projects`, it supports:
- **Configurable root paths** (e.g., `~/my-gitstuff`, `/var/cache/all/my/checkouts`)
- **Arbitrary grouped structures** using `.katazuke` index files as boundary markers

**How `.katazuke` works**:
- If `~/projects/.katazuke` exists → scan according to its `groups` and `ignores` lists
- If a group like `~/projects/work/.katazuke` exists → that group has its own nested organization
- If no `.katazuke` exists in a directory → assume its immediate children are repositories
- This allows unlimited depth while keeping scans efficient and predictable

**`.katazuke` format** (YAML or JSON):
```yaml
groups:
  - work
  - oss
ignores:
  - archive
  - tmp
```

Example grouped structure:
```
~/projects/
├── .katazuke             # Defines groups and ignores
├── work/
│   ├── .katazuke         # groups: [client-a, client-b]
│   ├── client-a/
│   │   ├── repo1/
│   │   └── repo2/
│   └── client-b/
│       └── repo3/
├── oss/
│   ├── project1/
│   └── project2/
└── archive/              # Ignored
```

### Private Repository Workflow

1. Clone the repository
2. Create a feature branch: `graham/<short-name>`
3. Commit changes and push to remote
4. Open a PR against `main`
5. Merge the PR (GitHub auto-deletes the remote branch via "Automatically delete head branches")
6. **Forget to delete the local branch** ⬅️ This is the problem

Over time, this accumulates dozens of local branches with short, context-free names that made sense during development but are meaningless weeks later.

### OSS/Fork Workflow

1. Fork the upstream repository
2. Clone your fork locally
3. Add the source repository as `upstream` remote
4. Create feature branches: `<short-name>` (not `graham/<name>`, to protect the default branch)
5. Push to your fork (`origin`)
6. Open a PR against `upstream`
7. After merge: **Forget to clean up local branch, fork's remote branch, and sync fork's `main` with upstream** ⬅️ More problems

This workflow can leave 50+ stale branches across local checkouts and forks, making it difficult to understand what work is in-progress versus abandoned.

### Design Philosophy

`katazuke` is **intentionally opinionated** and tailored to this workflow. It is not designed to accommodate all developer patterns or workflows. Contributions that add flags or options for alternative workflows may be rejected if they conflict with the core use case.

The tool is also **defensive by design**: it detects when your actual workflow has deviated from these patterns and avoids destructive actions that could disrupt your work.

## Development

See [PRD.md](PRD.md) for product requirements and design decisions.

## License

MIT
