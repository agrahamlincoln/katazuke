# Contributing to katazuke

Thanks for your interest in contributing to katazuke! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.25 or later
- Git
- [just](https://github.com/casey/just) - Task runner
- golangci-lint

### Getting Started

```bash
# Clone the repository
git clone https://github.com/agrahamlincoln/katazuke.git
cd katazuke

# Setup development environment (checks tools, installs dependencies)
just setup

# Build the binary
just build

# Run tests
just test

# Run linter
just lint
```

## Project Structure

```
katazuke/
├── cmd/katazuke/          # CLI entry point
├── internal/              # Internal packages (not for import)
│   ├── audit/            # Workspace auditing logic
│   ├── branches/         # Branch management
│   ├── repos/            # Repository operations
│   ├── sync/             # Sync/update operations
│   ├── github/           # GitHub API client
│   └── config/           # Configuration management
├── pkg/git/              # Reusable git operations (importable)
├── homebrew/             # Homebrew formula
├── aur/                  # AUR package files
└── docs/                 # Documentation
```

## Code Style

- Follow standard Go conventions
- Run `gofmt` and `goimports` on all code
- Use meaningful variable and function names
- Write comments for exported functions and complex logic
- Comments should explain "why", not "what" (except for Go where godoc requires it)

## Git Workflow

1. Create a feature branch from `main`
2. Make your changes with clear, atomic commits
3. Use conventional commit messages:
   - `feat: add new feature`
   - `fix: resolve bug`
   - `docs: update documentation`
   - `refactor: improve code structure`
   - `test: add tests`
4. Run tests and linter before committing
5. Push your branch and create a pull request

## Testing

- Write tests for new functionality
- Maintain or improve code coverage
- Test edge cases and error conditions
- Use table-driven tests where appropriate

```bash
# Run tests
make test

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race ./...
```

## Pull Request Guidelines

- Provide a clear description of the changes
- Reference any related issues
- Ensure all tests pass
- Update documentation as needed
- Keep PRs focused and reasonably sized
- Respond to review feedback promptly

## Building for Distribution

```bash
# Build for all platforms (darwin-arm64, linux-amd64)
just build-all

# Binaries will be in dist/
ls -lh dist/
```

## Release Process

Releases are fully automated using the justfile:

```bash
# Create and publish a release
just release 0.1.0
```

This will automatically:
1. Build binaries for all platforms
2. Create release tarballs
3. Calculate SHA256 checksums
4. Update `homebrew/katazuke.rb` with new version and SHA256s
5. Update `aur/PKGBUILD` with new version
6. Commit the formula updates
7. Create git tag `v0.1.0`
8. Push commits and tag to origin
9. Create GitHub release with all assets (requires `gh` CLI)
