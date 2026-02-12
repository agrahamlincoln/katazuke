# Contributing to katazuke

Thanks for your interest in contributing to katazuke! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.21 or later
- Git
- Make
- golangci-lint (optional, for linting)

### Getting Started

```bash
# Clone the repository
git clone https://github.com/agrahamlincoln/katazuke.git
cd katazuke

# Install dependencies
make deps

# Build the binary
make build

# Run tests
make test

# Run linter (requires golangci-lint)
make lint
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
# Build for all platforms
make build-all

# Binaries will be in dist/
ls -lh dist/
```

## Release Process

1. Update version in relevant files
2. Update CHANGELOG.md
3. Create and push a git tag: `git tag -a v0.1.0 -m "Release v0.1.0"`
4. Push tag: `git push origin v0.1.0`
5. GitHub Actions will build and create release
6. Update Homebrew formula
7. Update AUR package

## Questions?

Feel free to open an issue for any questions or concerns!
