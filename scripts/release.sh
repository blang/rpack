#!/usr/bin/env bash
# Release script for rpack
# Builds binaries, creates tar.gz archives, and publishes a GitHub release.
#
# Usage: just release
# Prerequisites:
#   1. Create and push a git tag:  git tag v0.2.0 && git push origin v0.2.0
#   2. Then run:                   just release
#
# The script auto-detects the version from the latest git tag.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$PROJECT_ROOT/dist"

# ---------------------------------------------------------------------------
# Prerequisites
# ---------------------------------------------------------------------------
check_deps() {
    local err=0
    for cmd in go git gh just; do
        if ! command -v "$cmd" &>/dev/null; then
            echo "ERROR: '$cmd' is required but not found in PATH" >&2
            err=1
        fi
    done

    if ! gh auth status &>/dev/null; then
        echo "ERROR: 'gh' is not authenticated. Run 'gh auth login' first." >&2
        err=1
    fi

    if [ "$err" -ne 0 ]; then
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Version detection
# ---------------------------------------------------------------------------
detect_version() {
    local version
    version="$(git -C "$PROJECT_ROOT" describe --tags --abbrev=0 2>/dev/null)" || {
        echo "ERROR: No git tags found. Create a tag first:" >&2
        echo "  git tag v0.2.0 && git push origin v0.2.0" >&2
        exit 1
    }

    # Require semver-style tag (vX.Y.Z)
    if ! echo "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+'; then
        echo "ERROR: latest tag '$version' does not match vX.Y.Z format" >&2
        exit 1
    fi

    # Confirm the tag is pushed
    if ! git -C "$PROJECT_ROOT" ls-remote --tags origin "$version" 2>/dev/null | grep -q "$version"; then
        echo "ERROR: tag '$version' exists locally but is not pushed to origin." >&2
        echo "  Run: git push origin $version" >&2
        exit 1
    fi

    echo "$version"
}

# ---------------------------------------------------------------------------
# Build — delegates to Justfile build-all
# ---------------------------------------------------------------------------
build_binaries() {
    echo "==> Building rpack binaries ..."
    cd "$PROJECT_ROOT"
    rm -f "$DIST_DIR"/*.tar.gz "$DIST_DIR"/checksums.txt
    just build-all
    echo "==> Binaries built in $DIST_DIR"
}

# ---------------------------------------------------------------------------
# Checksums
# ---------------------------------------------------------------------------
generate_checksums() {
    echo "==> Generating SHA256 checksums ..."
    cd "$DIST_DIR"
    sha256sum ./*.tar.gz > checksums.txt
    cd "$PROJECT_ROOT"
    cat "$DIST_DIR/checksums.txt"
}

# ---------------------------------------------------------------------------
# Changelog
# ---------------------------------------------------------------------------
generate_release_notes() {
    local version="$1"

    # Find the previous tag
    local prev_tag
    prev_tag="$(git -C "$PROJECT_ROOT" tag --sort=-creatordate | grep -A1 "^${version}$" | tail -1)" || true

    local notes=""

    if [ -n "$prev_tag" ]; then
        local commits
        commits="$(git -C "$PROJECT_ROOT" log --no-merges --pretty=format:'- %s (%h)' "${prev_tag}..${version}")"

        notes+="$commits"$'\n\n'
        notes+="**Full Changelog**: https://github.com/blang/rpack/compare/${prev_tag}...${version}"
    else
        notes+="Initial release of rpack"$'\n\n'
        notes+="**Full Changelog**: https://github.com/blang/rpack/commits/${version}"
    fi

    echo "$notes"
}

# ---------------------------------------------------------------------------
# GitHub Release
# ---------------------------------------------------------------------------
create_release() {
    local version="$1"
    local notes="$2"

    echo "==> Creating GitHub release $version ..."
    gh release create "$version" \
        --repo blang/rpack \
        --title "$version" \
        --notes "$notes" \
        "$DIST_DIR"/*.tar.gz \
        "$DIST_DIR/checksums.txt"

    echo "==> Release created: https://github.com/blang/rpack/releases/tag/$version"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    check_deps

    local version
    version="$(detect_version)"

    build_binaries
    generate_checksums

    local notes
    notes="$(generate_release_notes "$version")"

    create_release "$version" "$notes"
}

main
