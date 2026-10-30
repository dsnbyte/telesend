#!/usr/bin/env bash
set -euo pipefail

# Release automation script for Telesend
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh v0.2.1

VERSION_RAW="${1:-${VERSION:-}}"

if [ -z "$VERSION_RAW" ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 0.2.1"
  echo "         $0 v0.2.1"
  exit 1
fi

# Normalize version and tag
VERSION="${VERSION_RAW#v}"
TAG="v$VERSION"

# Validate semver format (e.g. 0.2.1 or 0.2.1-beta.1)
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
  echo "Error: Invalid version format '$VERSION_RAW'. Expected SemVer format (e.g., 0.2.1 or v0.2.1)." >&2
  exit 1
fi

# Ensure working directory is git repository root
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$REPO_ROOT" ]; then
  echo "Error: Must be run inside a git repository." >&2
  exit 1
fi
cd "$REPO_ROOT"

# Check working tree is clean
if [ -n "$(git status --porcelain)" ]; then
  echo "Error: Working tree is not clean. Commit or stash existing changes before releasing." >&2
  exit 1
fi

# Check branch is main
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$CURRENT_BRANCH" != "main" ]; then
  echo "Error: Release must be created on 'main' branch (currently on '$CURRENT_BRANCH')." >&2
  exit 1
fi

# Check tag doesn't already exist locally
if git rev-parse "$TAG" >/dev/null 2>&1; then
  echo "Error: Git tag '$TAG' already exists locally." >&2
  exit 1
fi

# Check tag doesn't already exist on remote
if git ls-remote --tags origin "$TAG" | grep -q "$TAG"; then
  echo "Error: Git tag '$TAG' already exists on remote origin." >&2
  exit 1
fi

echo "==> Preparing release $TAG (version: $VERSION)..."

# 1. Run tests before updating version
echo "==> Running test suite (make check)..."
make check

# 2. Update internal/version/version.go
VERSION_FILE="internal/version/version.go"
if [ ! -f "$VERSION_FILE" ]; then
  echo "Error: $VERSION_FILE not found." >&2
  exit 1
fi

echo "==> Updating $VERSION_FILE to \"$VERSION\"..."
sed -i.bak -E "s/var Version = \".*\"/var Version = \"$VERSION\"/" "$VERSION_FILE"
rm -f "${VERSION_FILE}.bak"

# 3. Rebuild binary and verify
echo "==> Building binary (make build)..."
make build

BUILT_VER="$("./dist/telesend" -V 2>/dev/null | tr -d '\r\n')"
if [ "$BUILT_VER" != "$VERSION" ]; then
  echo "Error: Built binary version ('$BUILT_VER') does not match expected version ('$VERSION')." >&2
  exit 1
fi
echo "==> Verified binary version: $BUILT_VER"

# 4. Stage and commit changes
echo "==> Committing changes and creating tag $TAG..."
git add "$VERSION_FILE"

# Include internal/mcp/tools.go or dist/telesend if tracked and modified
if git status --porcelain | grep -q "internal/mcp/tools.go"; then
  git add "internal/mcp/tools.go"
fi
if git ls-files --error-unmatch "dist/telesend" >/dev/null 2>&1; then
  git add "dist/telesend"
fi

git commit -m "chore(release): bump version to $TAG"
git tag -a "$TAG" -m "Release $TAG"

COMMIT_HASH="$(git rev-parse --short HEAD)"
echo ""
echo "============================================================"
echo "  Release $TAG created successfully!"
echo "============================================================"
echo "Commit: $COMMIT_HASH"
echo "Tag:    $TAG"
echo ""
echo "To publish the release and trigger GitHub Actions build:"
echo "  git push origin main --tags"
echo "============================================================"
