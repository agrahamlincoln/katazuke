#!/usr/bin/env bash
set -euo pipefail

# release.sh -- build and publish a katazuke release
#
# Usage:
#   ./scripts/release.sh                  # auto-detect version, release
#   ./scripts/release.sh 0.4.0            # manual version, release
#   ./scripts/release.sh --dry-run        # auto-detect version, dry-run
#   ./scripts/release.sh 0.4.0 --dry-run  # manual version, dry-run

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

REPO_OWNER="agrahamlincoln"
REPO_NAME="katazuke"
BINARY_NAME="katazuke"
GITHUB_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}"
PLATFORMS=("darwin-arm64" "linux-amd64")

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOMEBREW_REPO="$(cd "$REPO_ROOT/.." && pwd)/homebrew-katazuke"
AUR_REPO="$(cd "$REPO_ROOT/.." && pwd)/aur-katazuke"

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

VERSION=""
DRY_RUN=false

for arg in "$@"; do
    case "$arg" in
        --dry-run)
            DRY_RUN=true
            ;;
        --*)
            echo "ERROR: Unknown flag: $arg"
            echo "Usage: $0 [VERSION] [--dry-run]"
            exit 1
            ;;
        *)
            if [ -n "$VERSION" ]; then
                echo "ERROR: Unexpected argument: $arg (version already set to $VERSION)"
                echo "Usage: $0 [VERSION] [--dry-run]"
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

parse_semver() {
    local ver="$1"
    ver="${ver#v}"  # strip leading v
    IFS='.' read -r SEMVER_MAJOR SEMVER_MINOR SEMVER_PATCH <<< "$ver"
}

bump_version() {
    local kind="$1" major="$2" minor="$3" patch="$4"
    case "$kind" in
        major) echo "$(( major + 1 )).0.0" ;;
        minor) echo "${major}.$(( minor + 1 )).0" ;;
        patch) echo "${major}.${minor}.$(( patch + 1 ))" ;;
    esac
}

# ---------------------------------------------------------------------------
# Auto-version detection
# ---------------------------------------------------------------------------

detect_version() {
    local last_tag
    last_tag="$(git describe --tags --abbrev=0 2>/dev/null)" || {
        echo "ERROR: No git tags found. Cannot auto-detect version."
        echo "Create an initial tag first, e.g.: git tag -a v0.1.0 -m 'Initial release'"
        exit 1
    }

    local subjects
    subjects="$(git log "${last_tag}..HEAD" --pretty=format:'%s')"

    if [ -z "$subjects" ]; then
        echo "ERROR: No commits since ${last_tag}. Nothing to release."
        exit 1
    fi

    local bump="patch"
    while IFS= read -r subject; do
        if [[ "$subject" =~ ^[a-z]+(\(.+\))?\!: ]]; then
            bump="major"
            break
        elif [[ "$subject" =~ ^feat(\(.+\))?: ]]; then
            bump="minor"
            # don't break -- a later commit might be major
        fi
    done <<< "$subjects"

    parse_semver "$last_tag"
    local new_version
    new_version="$(bump_version "$bump" "$SEMVER_MAJOR" "$SEMVER_MINOR" "$SEMVER_PATCH")"

    if git tag -l "v${new_version}" | grep -q .; then
        echo "ERROR: Tag v${new_version} already exists."
        exit 1
    fi

    VERSION="$new_version"
    VERSION_SOURCE="auto-detected: ${bump} bump"
}

# ---------------------------------------------------------------------------
# Release notes generation
# ---------------------------------------------------------------------------

generate_release_notes() {
    local last_tag
    last_tag="$(git describe --tags --abbrev=0)"
    git log "${last_tag}..HEAD" --pretty=format:"- %s ([%h](${GITHUB_URL}/commit/%H))"
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------

preflight_ok=true

check_result() {
    local name="$1" ok="$2"
    if $ok; then
        echo "  [OK]   $name"
    else
        echo "  [FAIL] $name"
        preflight_ok=false
    fi
}

check_gh_cli() {
    command -v gh &> /dev/null
}

check_main_branch() {
    [ "$(git rev-parse --abbrev-ref HEAD)" = "main" ]
}

check_clean_tree() {
    [ -z "$(git status --porcelain --untracked-files=no)" ]
}

check_gh_auth() {
    gh auth status &> /dev/null
}

check_remote_sync() {
    git fetch origin &> /dev/null
    [ "$(git rev-parse main)" = "$(git rev-parse origin/main)" ]
}

ensure_packaging_repo() {
    local repo_path="$1" repo_name="$2"
    if [ ! -d "$repo_path/.git" ]; then
        echo "  ...    Cloning ${repo_name}..."
        gh repo clone "${REPO_OWNER}/${repo_name}" "$repo_path" -- --quiet
    fi
    git -C "$repo_path" fetch origin --quiet
    git -C "$repo_path" reset --hard origin/main --quiet
    git -C "$repo_path" clean -fd --quiet
}

check_packaging_repo() {
    local repo_path="$1" repo_name="$2"
    ensure_packaging_repo "$repo_path" "$repo_name" 2>/dev/null
    [ -d "$repo_path/.git" ]
}

run_preflight_checks() {
    echo "Pre-flight checks:"
    preflight_ok=true

    check_result "gh CLI installed" "$(check_gh_cli && echo true || echo false)"
    check_result "On main branch" "$(check_main_branch && echo true || echo false)"
    check_result "Working tree clean" "$(check_clean_tree && echo true || echo false)"
    check_result "gh auth status" "$(check_gh_auth && echo true || echo false)"
    check_result "Local main matches origin/main" "$(check_remote_sync && echo true || echo false)"
    check_result "homebrew-katazuke repo ready" "$(check_packaging_repo "$HOMEBREW_REPO" "homebrew-katazuke" && echo true || echo false)"
    check_result "aur-katazuke repo ready" "$(check_packaging_repo "$AUR_REPO" "aur-katazuke" && echo true || echo false)"

    echo ""
    if ! $preflight_ok; then
        echo "Pre-flight checks failed. Fix the issues above and try again."
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Release steps
# ---------------------------------------------------------------------------

step_build() {
    echo "1. Building release artifacts..."
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

step_checksums() {
    echo "3. Calculating SHA256 checksums..."
    DARWIN_SHA="$(sha256_portable "dist/release/${BINARY_NAME}-${VERSION}-darwin-arm64.tar.gz")"
    LINUX_SHA="$(sha256_portable "dist/release/${BINARY_NAME}-${VERSION}-linux-amd64.tar.gz")"
    echo "  darwin-arm64: $DARWIN_SHA"
    echo "  linux-amd64:  $LINUX_SHA"
}

step_homebrew() {
    echo "4. Updating Homebrew formula..."
    local formula="$HOMEBREW_REPO/katazuke.rb"
    sed -i.bak "s/version \".*\"/version \"${VERSION}\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "s|katazuke/releases/download/v[^/]*/|katazuke/releases/download/v${VERSION}/|g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-darwin-arm64/katazuke-${VERSION}-darwin-arm64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "s/katazuke-[0-9.]*-linux-amd64/katazuke-${VERSION}-linux-amd64/g" "$formula" && rm "$formula.bak"
    sed -i.bak "/darwin-arm64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$DARWIN_SHA\"/" "$formula" && rm "$formula.bak"
    sed -i.bak "/linux-amd64.tar.gz/,/sha256/ s/sha256 \".*\"/sha256 \"$LINUX_SHA\"/" "$formula" && rm "$formula.bak"
}

step_tag() {
    echo "5. Tagging release..."
    git tag -a "v${VERSION}" -m "Release v${VERSION}"
}

step_push_main() {
    echo "6. Pushing to origin..."
    git push origin main
    git push origin "v${VERSION}"
}

step_github_release() {
    echo "7. Creating GitHub release..."
    local tarballs=()
    for platform in "${PLATFORMS[@]}"; do
        tarballs+=("dist/release/${BINARY_NAME}-${VERSION}-${platform}.tar.gz")
    done
    gh release create "v${VERSION}" \
        --title "v${VERSION}" \
        --notes-file <(generate_release_notes) \
        "${tarballs[@]}"
}

step_aur() {
    echo "8. Downloading source tarball for AUR checksum..."
    gh release download "v${VERSION}" \
        --repo "${REPO_OWNER}/${REPO_NAME}" \
        --archive tar.gz \
        --output dist/release/source.tar.gz \
        --clobber
    SOURCE_SHA="$(sha256_portable dist/release/source.tar.gz)"
    echo "  source: $SOURCE_SHA"

    echo "9. Updating AUR PKGBUILD..."
    local pkgbuild="$AUR_REPO/PKGBUILD"
    local release_commit
    release_commit="$(git rev-parse --short HEAD)"
    sed -i.bak "s/^pkgver=.*/pkgver=${VERSION}/" "$pkgbuild" && rm "$pkgbuild.bak"
    sed -i.bak "s/^_commit=.*/_commit=${release_commit}/" "$pkgbuild" && rm "$pkgbuild.bak"
    sed -i.bak "s/^sha256sums=.*/sha256sums=('${SOURCE_SHA}')/" "$pkgbuild" && rm "$pkgbuild.bak"
}

step_push_packaging() {
    echo "10. Pushing homebrew-katazuke..."
    cd "$HOMEBREW_REPO"
    git add katazuke.rb
    git commit -m "chore: update to v${VERSION}"
    git push origin main

    echo "11. Pushing aur-katazuke..."
    cd "$AUR_REPO"
    git add PKGBUILD
    git commit -m "chore: update to v${VERSION}"
    git push origin main

    cd "$REPO_ROOT"
}

step_summary() {
    echo ""
    echo "Release v${VERSION} complete!"
    echo "  - GitHub release created with binary tarballs"
    echo "  - Homebrew formula updated with binary SHA256s"
    echo "  - PKGBUILD updated with source SHA256"
    echo "  - Both packaging repos pushed"
    echo ""
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    cd "$REPO_ROOT"

    # Auto-detect version if not provided
    VERSION_SOURCE="manual"
    if [ -z "$VERSION" ]; then
        detect_version
    else
        # Validate manual version tag doesn't exist
        if git tag -l "v${VERSION}" | grep -q .; then
            echo "ERROR: Tag v${VERSION} already exists."
            exit 1
        fi
    fi

    # Gather info for display
    local last_tag commit_count
    last_tag="$(git describe --tags --abbrev=0)"
    commit_count="$(git rev-list "${last_tag}..HEAD" --count)"

    if $DRY_RUN; then
        echo "Release Plan"
        echo "============"
        echo "  Version:  ${VERSION} (${VERSION_SOURCE})"
        echo "  Tag:      v${VERSION}"
        echo "  Commits:  ${commit_count} since ${last_tag}"
        echo ""
        echo "Commits to include:"
        git log "${last_tag}..HEAD" --pretty=format:"  - %s (%h)"
        echo ""
        echo ""

        run_preflight_checks

        echo "Dry run complete. No changes made."
        exit 0
    fi

    echo "Creating release v${VERSION}..."
    echo ""

    run_preflight_checks

    step_build
    step_tarballs
    step_checksums
    step_homebrew
    step_tag
    step_push_main
    step_github_release
    step_aur
    step_push_packaging
    step_summary
}

main
