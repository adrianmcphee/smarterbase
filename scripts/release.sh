#!/bin/bash
# Simple release tagging script for SmarterBase
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh v1.0.0

set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Error: Version number required"
    echo "Usage: ./scripts/release.sh <version>"
    echo "Example: ./scripts/release.sh v1.0.0"
    exit 1
fi

# Validate version format (v1.0.0)
if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Version must be in format v1.0.0"
    exit 1
fi

# Check if tag already exists
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "Error: Tag $VERSION already exists"
    exit 1
fi

echo "Creating release $VERSION..."

# Ensure we're on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "Warning: Not on main branch (currently on $CURRENT_BRANCH)"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Ensure working directory is clean
if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Working directory is not clean. Commit or stash changes first."
    git status --short
    exit 1
fi

# Run tests
echo "Running tests..."
if command -v go &> /dev/null; then
    go test ./... -short
    if [ $? -ne 0 ]; then
        echo "Error: Tests failed"
        exit 1
    fi
else
    echo "Warning: go command not found in PATH, skipping tests"
    echo "Make sure tests pass before pushing the release!"
fi

# Create annotated tag
echo "Creating git tag $VERSION..."
git tag -a "$VERSION" -m "Release $VERSION"

echo ""
echo "✅ Release $VERSION created successfully!"
echo ""
echo "Next steps:"
echo "  1. Review the tag: git show $VERSION"
echo "  2. Push the tag: git push origin $VERSION"
echo "  3. Create GitHub release at: https://github.com/adrianmcphee/smarterbase/releases/new?tag=$VERSION"
echo ""
