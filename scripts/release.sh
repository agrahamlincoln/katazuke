#!/usr/bin/env bash
set -euo pipefail

# release.sh -- orchestrate a katazuke release via tatara + Homebrew bolt-on
#
# Phase 1: tatara handles version detection, PKGBUILD update, release commit,
#           tag, push, GitHub release, and optional remote pacman build.
# Phase 2: cross-compile binaries, upload to the GitHub release, and update
#           the homebrew-katazuke formula.
#
# Usage:
#   ./scripts/release.sh                       # auto-detect version, full release
#   ./scripts/release.sh 0.9.0                 # manual version, full release
#   ./scripts/release.sh --dry-run             # preview what would happen
#   ./scripts/release.sh --skip-homebrew       # pacman-only release (tatara only)
#   ./scripts/release.sh 0.9.0 --dry-run      # manual version, dry-run

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

REPO_OWNER="agrahamlincoln"
REPO_NAME="katazuke"
BINARY_NAME="katazuke"
PLATFORMS=("darwin-arm64" "linux-amd64")

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOMEBREW_REPO="$(cd "$REPO_ROOT/.." && pwd)/homebrew-katazuke"

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

VERSION=""
DRY_RUN=false
SKIP_HOMEBREW=false

for arg in "$@"; do
    case "$arg" in
        --dry-run)
            DRY_RUN=true
            ;;
        --skip-homebrew)
            SKIP_HOMEBREW=true
            ;;
        --*)
            echo "ERROR: Unknown flag: $arg"
            echo "Usage: $0 [VERSION] [--dry-run] [--skip-homebrew]"
            exit 1
            ;;
        *)
            if [ -n "$VERSION" ]; then
                echo "ERROR: Unexpected argument: $arg (version already set to $VERSION)"
                echo "Usage: $0 [VERSION] [--dry-run] [--skip-homebrew]"
                exit 1
            fi
            VERSION="$arg"
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Utility functions
# ---------------------------------------------------------------------------

sha256_portable() {
    if command -v sha256sum &> /dev/null; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

# ---------------------------------------------------------------------------
# Pre-flight checks (only for things tatara doesn't cover)
# ---------------------------------------------------------------------------

preflight_homebrew() {
    echo "Homebrew pre-flight checks:"
    local ok=true

    if [ ! -d "$HOMEBREW_REPO/.git" ]; then
        echo "  [FAIL] homebrew-katazuke repo not found at $HOMEBREW_REPO"
        ok=false
    else
        echo "  [OK]   homebrew-katazuke repo found"
        git -C "$HOMEBREW_REPO" fetch origin --quiet
        if [ "$(git -C "$HOMEBREW_REPO" rev-parse main)" != "$(git -C "$HOMEBREW_REPO" rev-parse origin/main)" ]; then
            echo "  [FAIL] homebrew-katazuke not in sync with origin"
            ok=false
        else
            echo "  [OK]   homebrew-katazuke in sync with origin"
        fi
    fi

    if ! $ok; then
        echo "Homebrew pre-flight checks failed."
        exit 1
    fi
    echo ""
}

# ---------------------------------------------------------------------------
# Phase 1: tatara release
# ---------------------------------------------------------------------------

run_tatara() {
    if ! command -v tatara &> /dev/null; then
        echo "ERROR: tatara not found on PATH."
        echo "Install tatara first: https://github.com/agrahamlincoln/tatara"
        exit 1
    fi

    local tatara_args=("release" "$REPO_ROOT")
    if [ -n "$VERSION" ]; then
        tatara_args+=("--version" "$VERSION")
    fi
    if $DRY_RUN; then
        tatara_args+=("--dry-run")
    fi

    echo "Phase 1: tatara release"
    echo "======================="
    tatara "${tatara_args[@]}"
    echo ""
}

# ---------------------------------------------------------------------------
# Phase 2: Homebrew bolt-on
# ---------------------------------------------------------------------------

step_build() {
    echo "1. Building release binaries..."
    rm -rf dist
    mkdir -p dist

    # Read version from the tag tatara just created
    local release_version release_commit release_date release_ldflags
    release_version="$(git describe --tags --abbrev=0)"
    release_version="${release_version#v}"
    release_commit="$(git rev-parse --short HEAD)"
    release_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    release_ldflags="-X main.version=${release_version} -X main.commit=${release_commit} -X main.date=${release_date}"

    for platform in "${PLATFORMS[@]}"; do
        IFS='-' read -r goos goarch <<< "$platform"
        GOOS="$goos" GOARCH="$goarch" go build -ldflags "$release_ldflags" \
            -o "dist/${BINARY_NAME}-${platform}" ./cmd/katazuke
    done
}

step_tarballs() {
    echo "2. Creating release tarballs..."
    mkdir -p dist/release
    local release_version
    release_version="$(git describe --tags --abbrev=0)"
    release_version="${release_version#v}"

    for platform in "${PLATFORMS[@]}"; do
        tar -czf "dist/release/${BINARY_NAME}-${release_version}-${platform}.tar.gz" \
            -C dist "${BINARY_NAME}-${platform}"
    done
}

step_upload() {
    echo "3. Uploading tarballs to GitHub release..."
    local tag
    tag="$(git describe --tags --abbrev=0)"
    local release_version="${tag#v}"

    local tarballs=()
    for platform in "${PLATFORMS[@]}"; do
        tarballs+=("dist/release/${BINARY_NAME}-${release_version}-${platform}.tar.gz")
    done
    gh release upload "$tag" "${tarballs[@]}"
}

step_homebrew() {
    echo "4. Updating Homebrew formula..."
    local release_version
    release_version="$(git describe --tags --abbrev=0)"
    release_version="${release_version#v}"

    local darwin_sha linux_sha
    darwin_sha="$(sha256_portable "dist/release/${BINARY_NAME}-${release_version}-darwin-arm64.tar.gz")"
    linux_sha="$(sha256_portable "dist/release/${BINARY_NAME}-${release_version}-linux-amd64.tar.gz")"
    echo "  darwin-arm64: $darwin_sha"
    echo "  linux-amd64:  $linux_sha"

    local formula="$HOMEBREW_REPO/katazuke.rb"
    sed -i.bak "s/version \".*\"/version \"${release_version}\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "s|katazuke/releases/download/v[^/]*/|katazuke/releases/download/v${release_version}/|g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-darwin-arm64/katazuke-${release_version}-darwin-arm64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-linux-amd64/katazuke-${release_version}-linux-amd64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "/darwin-arm64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$darwin_sha\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "/linux-amd64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$linux_sha\"/" "$formula" && rm "$formula.bak"
}

step_push_homebrew() {
    echo "5. Pushing homebrew-katazuke..."
    local release_version
    release_version="$(git describe --tags --abbrev=0)"
    release_version="${release_version#v}"

    cd "$HOMEBREW_REPO"
    git add katazuke.rb
    git commit -m "chore: update to v${release_version}"
    git push origin main
    cd "$REPO_ROOT"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    cd "$REPO_ROOT"

    # Phase 1: tatara handles version detection, tagging, push, GH release
    run_tatara

    if $DRY_RUN; then
        if ! $SKIP_HOMEBREW; then
            echo "Homebrew bolt-on would also run (use --skip-homebrew to skip)."
        fi
        echo "Dry run complete. No changes made."
        exit 0
    fi

    if $SKIP_HOMEBREW; then
        echo "Skipping Homebrew (--skip-homebrew)."
        echo ""
        echo "Release complete (pacman only)."
        exit 0
    fi

    # Phase 2: Homebrew bolt-on
    echo "Phase 2: Homebrew bolt-on"
    echo "========================="
    preflight_homebrew
    step_build
    step_tarballs
    step_upload
    step_homebrew
    step_push_homebrew

    echo ""
    echo "Release complete!"
    echo "  - GitHub release created by tatara"
    echo "  - Binary tarballs uploaded to release"
    echo "  - Homebrew formula updated and pushed"
    echo ""
}

main
