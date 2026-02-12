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

## Installation

### macOS (Homebrew)

```bash
brew tap agrahamlincoln/katazuke
brew install katazuke
```

### Arch Linux (AUR)

```bash
git clone https://aur.archlinux.org/katazuke.git
cd katazuke
makepkg -si
```

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

## Development

See [PRD.md](PRD.md) for product requirements and design decisions.

## License

MIT
