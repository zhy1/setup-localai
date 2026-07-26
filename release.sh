#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

usage() {
  cat <<'EOF'
Usage: ./release.sh VERSION

Example:
  ./release.sh 1.0.0

This script creates an annotated git tag like v1.0.0, pushes the current branch
and tag to origin, and triggers the GitHub Actions release workflow.
EOF
  exit 1
}

if [[ $# -ne 1 ]]; then
  usage
fi

VERSION="$1"
if [[ "$VERSION" != v* ]]; then
  TAG="v$VERSION"
else
  TAG="$VERSION"
fi

if ! command -v git >/dev/null 2>&1; then
  echo "git is required" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree is dirty. Commit or stash changes before releasing." >&2
  git status --short
  exit 1
fi

CURRENT_BRANCH="$(git symbolic-ref --short HEAD)"
if [[ "$CURRENT_BRANCH" != "main" ]]; then
  echo "Warning: you are releasing from branch '$CURRENT_BRANCH'." >&2
  echo "It is recommended to release from 'main'." >&2
fi

echo "Fetching latest origin/main..."
git fetch origin main

LOCAL_HEAD="$(git rev-parse HEAD)"
REMOTE_HEAD="$(git rev-parse origin/main)"
if [[ "$LOCAL_HEAD" != "$REMOTE_HEAD" ]]; then
  echo "Local branch is not synchronized with origin/main." >&2
  echo "Please pull or rebase before releasing." >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1; then
  echo "Tag '$TAG' already exists." >&2
  exit 1
fi

echo "Creating annotated tag '$TAG'..."
git tag -a "$TAG" -m "Release $TAG"

echo "Pushing branch and tag to origin..."
git push origin HEAD

git push origin "$TAG"

echo "Release tag '$TAG' pushed. GitHub Actions will publish binaries to GitHub Release."