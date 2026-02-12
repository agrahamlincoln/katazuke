# Justfile for katazuke development
# Run `just --list` to see all available commands

# Default recipe (runs when you just type `just`)
default:
    @just --list

# Setup development environment
setup:
    #!/usr/bin/env bash
    set -euo pipefail

    echo "Setting up katazuke development environment..."
    echo ""

    # Check for required tools
    echo "Checking for required tools..."

    if ! command -v go &> /dev/null; then
        echo "ERROR: Go not found. Please install Go first."
        exit 1
    fi
    echo "Go found: $(go version)"

    if ! command -v just &> /dev/null; then
        echo "ERROR: just not found. Install with: brew install just"
        exit 1
    fi
    echo "just found"

    if ! command -v golangci-lint &> /dev/null; then
        echo "ERROR: golangci-lint not found. Install with: brew install golangci-lint"
        exit 1
    fi
    echo "golangci-lint found"

    echo ""
    echo "Installing Go dependencies..."
    go mod download
    echo "Dependencies installed"

    echo ""
    echo "Setup complete!"
    echo ""
    echo "Next steps:"
    echo "  1. Build the app:  just build"
    echo "  2. Run tests:      just test"
    echo "  3. Run linter:     just lint"
    echo ""

# Build variables
binary_name := "katazuke"
build_dir := "bin"
version := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
commit := `git rev-parse --short HEAD 2>/dev/null || echo "none"`
date := `date -u +"%Y-%m-%dT%H:%M:%SZ"`
ldflags := "-X main.version=" + version + " -X main.commit=" + commit + " -X main.date=" + date

# Build the binary for local platform
build:
    @echo "Building {{binary_name}} {{version}}..."
    @mkdir -p {{build_dir}}
    go build -ldflags "{{ldflags}}" -o {{build_dir}}/{{binary_name}} ./cmd/katazuke
    @echo "Built {{build_dir}}/{{binary_name}}"

# Build with debug symbols (for testing/debugging)
build-debug:
    @echo "Building {{binary_name}} (debug) {{version}}..."
    @mkdir -p {{build_dir}}
    go build -gcflags="all=-N -l" -ldflags "{{ldflags}}" -o {{build_dir}}/{{binary_name}}-debug ./cmd/katazuke
    @echo "Built {{build_dir}}/{{binary_name}}-debug"

# Install the binary to /usr/local/bin
install: build
    @echo "Installing {{binary_name}} to /usr/local/bin..."
    @install -m 755 {{build_dir}}/{{binary_name}} /usr/local/bin/{{binary_name}}
    @echo "Installed {{binary_name}}"

# Clean build artifacts
clean:
    @echo "Cleaning build artifacts..."
    @rm -rf {{build_dir}} dist/ test-repos/
    @go clean
    @echo "Cleaned"

# Run unit tests
test:
    @echo "Running unit tests..."
    go test -v -race -cover ./...

# Run unit tests with coverage report
test-coverage:
    @echo "Running tests with coverage..."
    go test -v -race -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run end-to-end tests
test-e2e: build-debug
    @echo "Running E2E tests..."
    go test -v -race -tags=e2e ./test/e2e/...

# Run all tests (unit + e2e)
test-all: test test-e2e

# Run linter
lint:
    @echo "Running golangci-lint..."
    @if command -v golangci-lint >/dev/null 2>&1; then \
        golangci-lint run; \
    else \
        echo "ERROR: golangci-lint not installed"; \
        echo "Install with: brew install golangci-lint"; \
        exit 1; \
    fi

# Run linter and fix issues where possible
lint-fix:
    @echo "Running golangci-lint with auto-fix..."
    golangci-lint run --fix

# Download and tidy dependencies
deps:
    @echo "Downloading dependencies..."
    go mod download
    go mod tidy
    @echo "Dependencies updated"

# Build for all platforms (cross-compile)
build-all:
    @echo "Building for all platforms..."
    @mkdir -p dist
    GOOS=darwin GOARCH=arm64 go build -ldflags "{{ldflags}}" -o dist/{{binary_name}}-darwin-arm64 ./cmd/katazuke
    GOOS=linux GOARCH=amd64 go build -ldflags "{{ldflags}}" -o dist/{{binary_name}}-linux-amd64 ./cmd/katazuke
    @echo "Built all platform binaries in dist/"

# Build Homebrew package (for local testing)
package-homebrew: build-all
    @echo "Building Homebrew package..."
    @echo "Binaries built in dist/"
    @echo "Update homebrew/katazuke.rb with new version and SHA256"

# Build AUR package (for local testing)
package-aur:
    @echo "Building AUR package..."
    cd aur && makepkg -f
    @echo "Built AUR package"
    @echo "Install with: cd aur && makepkg -si"

# Install AUR package locally
install-aur: package-aur
    @echo "Installing AUR package..."
    cd aur && makepkg -si

# Create a new release (fully automated)
release VERSION:
    #!/usr/bin/env bash
    set -euo pipefail

    echo "Creating release v{{VERSION}}..."
    echo ""

    # 1. Build release artifacts
    echo "1. Building release artifacts..."
    just build-all

    # 2. Create tarballs
    echo "2. Creating release tarballs..."
    mkdir -p dist/release
    cd dist && tar -czf release/{{binary_name}}-{{VERSION}}-darwin-arm64.tar.gz {{binary_name}}-darwin-arm64
    cd dist && tar -czf release/{{binary_name}}-{{VERSION}}-linux-amd64.tar.gz {{binary_name}}-linux-amd64

    # 3. Calculate SHA256 checksums
    echo "3. Calculating SHA256 checksums..."
    darwin_sha=$(shasum -a 256 dist/release/{{binary_name}}-{{VERSION}}-darwin-arm64.tar.gz | cut -d' ' -f1)
    linux_sha=$(shasum -a 256 dist/release/{{binary_name}}-{{VERSION}}-linux-amd64.tar.gz | cut -d' ' -f1)
    echo "  darwin-arm64: $darwin_sha"
    echo "  linux-amd64:  $linux_sha"

    # 4. Update Homebrew formula
    echo "4. Updating Homebrew formula..."
    sed -i '' "s/version \".*\"/version \"{{VERSION}}\"/" homebrew/katazuke.rb
    sed -i '' "s|katazuke/releases/download/v[^/]*/|katazuke/releases/download/v{{VERSION}}/|g" homebrew/katazuke.rb
    sed -i '' "s/katazuke-[0-9.]*-darwin-arm64/katazuke-{{VERSION}}-darwin-arm64/g" homebrew/katazuke.rb
    sed -i '' "s/katazuke-[0-9.]*-linux-amd64/katazuke-{{VERSION}}-linux-amd64/g" homebrew/katazuke.rb
    # Update SHA256s
    sed -i '' "/darwin-arm64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$darwin_sha\"/" homebrew/katazuke.rb
    sed -i '' "/linux-amd64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$linux_sha\"/" homebrew/katazuke.rb

    # 5. Update AUR PKGBUILD
    echo "5. Updating AUR PKGBUILD..."
    sed -i '' "s/^pkgver=.*/pkgver={{VERSION}}/" aur/PKGBUILD

    # 6. Copy PKGBUILD to release dir
    echo "6. Copying PKGBUILD..."
    cp aur/PKGBUILD dist/release/PKGBUILD

    # 7. Commit formula updates
    echo "7. Committing formula updates..."
    git add homebrew/katazuke.rb aur/PKGBUILD
    git commit -m "chore: release v{{VERSION}}"

    # 8. Create git tag
    echo "8. Creating git tag..."
    git tag -a "v{{VERSION}}" -m "Release v{{VERSION}}"

    # 9. Push commits and tag
    echo "9. Pushing to origin..."
    git push origin main
    git push origin "v{{VERSION}}"

    # 10. Create GitHub release and upload assets
    echo "10. Creating GitHub release..."
    if ! command -v gh &> /dev/null; then
        echo "ERROR: gh CLI not found. Install with: brew install gh"
        echo "Skipping GitHub release creation."
        echo "Manually create release and upload files from dist/release/"
        exit 1
    fi

    gh release create "v{{VERSION}}" \
        --title "v{{VERSION}}" \
        --generate-notes \
        dist/release/{{binary_name}}-{{VERSION}}-darwin-arm64.tar.gz \
        dist/release/{{binary_name}}-{{VERSION}}-linux-amd64.tar.gz \
        dist/release/PKGBUILD

    echo ""
    echo "Release v{{VERSION}} complete!"
    echo "  - Homebrew formula updated with SHA256s"
    echo "  - PKGBUILD version updated"
    echo "  - GitHub release created with assets"
    echo ""

# Run the binary (build first if needed)
run *ARGS: build
    {{build_dir}}/{{binary_name}} {{ARGS}}

# Show version information
version:
    @echo "Version: {{version}}"
    @echo "Commit:  {{commit}}"
    @echo "Date:    {{date}}"

# Check code formatting
fmt-check:
    @echo "Checking code formatting..."
    @test -z "$(gofmt -l .)" || (echo "ERROR: Code not formatted. Run 'just fmt' to fix." && exit 1)
    @echo "Code is formatted"

# Format code
fmt:
    @echo "Formatting code..."
    gofmt -w .
    @echo "Code formatted"

# Run pre-commit checks (lint + test + fmt-check)
pre-commit: fmt-check lint test
    @echo "All pre-commit checks passed"
