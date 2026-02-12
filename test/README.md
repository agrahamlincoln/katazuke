# Testing Infrastructure

This directory contains test infrastructure for katazuke.

## Directory Structure

```
test/
├── helpers/       # Test helper utilities (git repo creation, etc.)
├── e2e/          # End-to-end tests
└── README.md     # This file
```

## Running Tests

### Unit Tests
```bash
just test
# or
go test ./...
```

### E2E Tests
```bash
just test-e2e
# or
go test -tags=e2e ./test/e2e/...
```

### All Tests
```bash
just test-all
```

## E2E Test Strategy

E2E tests create real git repositories with realistic scenarios and run the built katazuke binary against them.

### Key Features

1. **Real Git Repositories**: Tests use actual git commands to create realistic scenarios
2. **Date Manipulation**: Use `CommitWithDate()` to create commits with past timestamps
3. **Temporary Directories**: Each test gets its own isolated temp directory (cleaned up automatically)

### Example: Testing Stale Branch Detection

```go
func TestStaleBranch(t *testing.T) {
    repo := helpers.NewTestRepo(t, "my-repo")

    // Create a branch with a 45-day-old commit
    repo.CreateBranch("feature/old")
    repo.WriteFile("old.txt", "content")
    repo.AddFile("old.txt")
    oldDate := time.Now().AddDate(0, 0, -45)
    repo.CommitWithDate("Old work", oldDate)

    // Run katazuke
    cmd := exec.Command("../../bin/katazuke-debug", "branches", "--stale")
    cmd.Dir = repo.Path
    output, err := cmd.CombinedOutput()

    // Assert behavior
    // ...
}
```

### Why Date Manipulation Works

Git commits contain timestamp metadata. We can set these timestamps to past dates using:
- `--date` flag on `git commit`
- `GIT_AUTHOR_DATE` and `GIT_COMMITTER_DATE` environment variables

This allows testing "30-day-old branches" without waiting 30 actual days!

## Test Helpers

### `helpers.NewTestRepo(t, name)`
Creates a fresh git repository in a temporary directory with:
- Git initialized
- User configured
- Initial commit on `main` branch

### `TestRepo.CommitWithDate(message, date)`
Creates a commit with a specific timestamp. **This is critical for testing stale detection.**

### `TestRepo.CreateBranch(name)`
Creates and checks out a new branch.

### `TestRepo.Merge(branch)`
Merges a branch (with `--no-ff` for merge commits).

## Build Tags

E2E tests use `// +build e2e` build tag. This means:
- `go test ./...` **does NOT** run E2E tests
- `go test -tags=e2e ./test/e2e/...` **does** run E2E tests

This separation is intentional:
- Unit tests run fast and often
- E2E tests are slower and run before commits/releases
