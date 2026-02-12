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

# Create a new release (fully automated)
# Requires: gh CLI, sibling repos ../homebrew-katazuke and ../aur-katazuke
release VERSION:
    #!/usr/bin/env bash
    set -euo pipefail

    echo "Creating release v{{VERSION}}..."
    echo ""

    # Portable SHA256: sha256sum (Linux) or shasum (macOS)
    sha256() {
        if command -v sha256sum &> /dev/null; then
            sha256sum "$1" | cut -d' ' -f1
        else
            shasum -a 256 "$1" | cut -d' ' -f1
        fi
    }

    # Validate prerequisites
    if ! command -v gh &> /dev/null; then
        echo "ERROR: gh CLI not found. Install with: brew install gh"
        exit 1
    fi

    homebrew_repo="$(cd .. && pwd)/homebrew-katazuke"
    aur_repo="$(cd .. && pwd)/aur-katazuke"

    if [ ! -d "$homebrew_repo/.git" ]; then
        echo "ERROR: Homebrew repo not found at $homebrew_repo"
        echo "Clone it: gh repo clone agrahamlincoln/homebrew-katazuke $homebrew_repo"
        exit 1
    fi
    if [ ! -d "$aur_repo/.git" ]; then
        echo "ERROR: AUR repo not found at $aur_repo"
        echo "Clone it: gh repo clone agrahamlincoln/aur-katazuke $aur_repo"
        exit 1
    fi

    # 1. Build release artifacts with explicit version
    echo "1. Building release artifacts..."
    rm -rf dist
    mkdir -p dist
    release_ldflags="-X main.version={{VERSION}} -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=darwin GOARCH=arm64 go build -ldflags "$release_ldflags" -o dist/{{binary_name}}-darwin-arm64 ./cmd/katazuke
    GOOS=linux GOARCH=amd64 go build -ldflags "$release_ldflags" -o dist/{{binary_name}}-linux-amd64 ./cmd/katazuke

    # 2. Create tarballs
    echo "2. Creating release tarballs..."
    mkdir -p dist/release
    tar -czf dist/release/{{binary_name}}-{{VERSION}}-darwin-arm64.tar.gz -C dist {{binary_name}}-darwin-arm64
    tar -czf dist/release/{{binary_name}}-{{VERSION}}-linux-amd64.tar.gz -C dist {{binary_name}}-linux-amd64

    # 3. Calculate SHA256 checksums for binary tarballs
    echo "3. Calculating SHA256 checksums..."
    darwin_sha=$(sha256 dist/release/{{binary_name}}-{{VERSION}}-darwin-arm64.tar.gz)
    linux_sha=$(sha256 dist/release/{{binary_name}}-{{VERSION}}-linux-amd64.tar.gz)
    echo "  darwin-arm64: $darwin_sha"
    echo "  linux-amd64:  $linux_sha"

    # 4. Update Homebrew formula in sibling repo
    echo "4. Updating Homebrew formula..."
    formula="$homebrew_repo/katazuke.rb"
    sed -i.bak "s/version \".*\"/version \"{{VERSION}}\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "s|katazuke/releases/download/v[^/]*/|katazuke/releases/download/v{{VERSION}}/|g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-darwin-arm64/katazuke-{{VERSION}}-darwin-arm64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-linux-amd64/katazuke-{{VERSION}}-linux-amd64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "/darwin-arm64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$darwin_sha\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "/linux-amd64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$linux_sha\"/" "$formula" && rm "$formula.bak"

    # 5. Tag main repo (no commit needed - version is set via ldflags)
    echo "5. Tagging release..."
    git tag -a "v{{VERSION}}" -m "Release v{{VERSION}}"

    # 6. Push main repo
    echo "6. Pushing to origin..."
    git push origin main
    git push origin "v{{VERSION}}"

    # 7. Create GitHub release with binary tarballs
    echo "7. Creating GitHub release..."
    gh release create "v{{VERSION}}" \
        --title "v{{VERSION}}" \
        --generate-notes \
        dist/release/{{binary_name}}-{{VERSION}}-darwin-arm64.tar.gz \
        dist/release/{{binary_name}}-{{VERSION}}-linux-amd64.tar.gz

    # 8. Download source tarball and calculate SHA256 for AUR
    echo "8. Downloading source tarball for AUR checksum..."
    gh release download "v{{VERSION}}" \
        --repo agrahamlincoln/katazuke \
        --archive tar.gz \
        --output dist/release/source.tar.gz \
        --clobber
    source_sha=$(sha256 dist/release/source.tar.gz)
    echo "  source: $source_sha"

    # 9. Update AUR PKGBUILD in sibling repo
    echo "9. Updating AUR PKGBUILD..."
    pkgbuild="$aur_repo/PKGBUILD"
    sed -i.bak "s/^pkgver=.*/pkgver={{VERSION}}/" "$pkgbuild" && rm "$pkgbuild.bak"
    sed -i.bak "s/^sha256sums=.*/sha256sums=('$source_sha')/" "$pkgbuild" && rm "$pkgbuild.bak"

    # 10. Commit and push homebrew-katazuke
    echo "10. Pushing homebrew-katazuke..."
    cd "$homebrew_repo"
    git add katazuke.rb
    git commit -m "chore: update to v{{VERSION}}"
    git push origin main

    # 11. Commit and push aur-katazuke
    echo "11. Pushing aur-katazuke..."
    cd "$aur_repo"
    git add PKGBUILD
    git commit -m "chore: update to v{{VERSION}}"
    git push origin main

    echo ""
    echo "Release v{{VERSION}} complete!"
    echo "  - GitHub release created with binary tarballs"
    echo "  - Homebrew formula updated with binary SHA256s"
    echo "  - PKGBUILD updated with source SHA256"
    echo "  - Both packaging repos pushed"
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
