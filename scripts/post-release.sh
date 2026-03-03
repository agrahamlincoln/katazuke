#!/usr/bin/env bash
set -euo pipefail

# post-release.sh -- Homebrew bolt-on for katazuke releases
#
# Called by tatara as a post-release hook. Receives version info via
# environment variables set by tatara:
#   TATARA_RELEASE_VERSION  -- version without "v" prefix (e.g., 1.2.3)
#   TATARA_RELEASE_TAG      -- full git tag (e.g., v1.2.3)
#
# Steps:
#   1. Cross-compile darwin-arm64 and linux-amd64 binaries
#   2. Create release tarballs
#   3. Upload tarballs to the GitHub release
#   4. Update the homebrew-katazuke formula
#   5. Push homebrew-katazuke

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

BINARY_NAME="katazuke"
PLATFORMS=("darwin-arm64" "linux-amd64")

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOMEBREW_REPO="$(cd "$REPO_ROOT/.." && pwd)/homebrew-katazuke"

VERSION="${TATARA_RELEASE_VERSION:?TATARA_RELEASE_VERSION not set}"
TAG="${TATARA_RELEASE_TAG:?TATARA_RELEASE_TAG not set}"

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
# Pre-flight checks
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
# Steps
# ---------------------------------------------------------------------------

step_build() {
    echo "1. Building release binaries..."
    rm -rf dist
    mkdir -p dist

    local release_commit release_date release_ldflags
    release_commit="$(git rev-parse --short HEAD)"
    release_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    release_ldflags="-X main.version=${VERSION} -X main.commit=${release_commit} -X main.date=${release_date}"

    for platform in "${PLATFORMS[@]}"; do
        IFS='-' read -r goos goarch <<< "$platform"
        GOOS="$goos" GOARCH="$goarch" go build -ldflags "$release_ldflags" \
            -o "dist/${BINARY_NAME}-${platform}" ./cmd/katazuke
    done
}

step_tarballs() {
    echo "2. Creating release tarballs..."
    mkdir -p dist/release

    for platform in "${PLATFORMS[@]}"; do
        tar -czf "dist/release/${BINARY_NAME}-${VERSION}-${platform}.tar.gz" \
            -C dist "${BINARY_NAME}-${platform}"
    done
}

step_upload() {
    echo "3. Uploading tarballs to GitHub release..."

    local tarballs=()
    for platform in "${PLATFORMS[@]}"; do
        tarballs+=("dist/release/${BINARY_NAME}-${VERSION}-${platform}.tar.gz")
    done
    gh release upload "$TAG" "${tarballs[@]}"
}

step_homebrew() {
    echo "4. Updating Homebrew formula..."

    local darwin_sha linux_sha
    darwin_sha="$(sha256_portable "dist/release/${BINARY_NAME}-${VERSION}-darwin-arm64.tar.gz")"
    linux_sha="$(sha256_portable "dist/release/${BINARY_NAME}-${VERSION}-linux-amd64.tar.gz")"
    echo "  darwin-arm64: $darwin_sha"
    echo "  linux-amd64:  $linux_sha"

    local formula="$HOMEBREW_REPO/katazuke.rb"
    sed -i.bak "s/version \".*\"/version \"${VERSION}\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "s|katazuke/releases/download/v[^/]*/|katazuke/releases/download/v${VERSION}/|g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-darwin-arm64/katazuke-${VERSION}-darwin-arm64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-linux-amd64/katazuke-${VERSION}-linux-amd64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "/darwin-arm64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$darwin_sha\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "/linux-amd64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$linux_sha\"/" "$formula" && rm "$formula.bak"
}

step_push_homebrew() {
    echo "5. Pushing homebrew-katazuke..."
    cd "$HOMEBREW_REPO"
    git add katazuke.rb
    git commit -m "chore: update to v${VERSION}"
    git push origin main
    cd "$REPO_ROOT"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    cd "$REPO_ROOT"

    echo "Homebrew bolt-on for ${TAG}"
    echo "========================="
    preflight_homebrew
    step_build
    step_tarballs
    step_upload
    step_homebrew
    step_push_homebrew

    echo ""
    echo "Homebrew release complete."
    echo "  - Binary tarballs uploaded to release"
    echo "  - Homebrew formula updated and pushed"
    echo ""
}

main
