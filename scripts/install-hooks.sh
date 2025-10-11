#!/bin/bash
# Install git hooks for commit message validation and pre-commit checks

set -e

HOOKS_DIR=".git/hooks"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

echo "Installing git hooks..."

# Create commit-msg hook
cat > "$HOOKS_DIR/commit-msg" << 'EOF'
#!/bin/sh
# Validates commit message format for semantic versioning

commit_msg_file=$1
commit_msg=$(cat "$commit_msg_file")

# Regex for conventional commits
conventional_commit_regex='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\(.+\))?(!)?: .{1,}'

# Check if commit message matches conventional format
if ! echo "$commit_msg" | grep -qE "$conventional_commit_regex"; then
    echo ""
    echo "❌ Invalid commit message format"
    echo ""
    echo "Commit message must follow Conventional Commits format:"
    echo ""
    echo "  <type>(<scope>): <description>"
    echo ""
    echo "Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert"
    echo ""
    echo "Examples:"
    echo "  feat: add Redis connection pooling"
    echo "  fix: resolve race condition in locks"
    echo "  feat!: redesign Store API (breaking change)"
    echo ""
    echo "See .github/SEMANTIC_VERSIONING.md for details"
    echo ""
    exit 1
fi

# Check description length (should be concise)
description=$(echo "$commit_msg" | head -n1 | sed -E 's/^[a-z]+(\(.+\))?(!)?: //')
if [ ${#description} -gt 72 ]; then
    echo ""
    echo "⚠️  Warning: Commit description is ${#description} characters (max 72 recommended)"
    echo ""
fi

exit 0
EOF

chmod +x "$HOOKS_DIR/commit-msg"
echo "✅ Installed commit-msg hook"

# Create pre-commit hook
cat > "$HOOKS_DIR/pre-commit" << 'EOF'
#!/bin/sh
# Pre-commit hook to verify code quality

echo "🔍 Running pre-commit checks..."

# Check if go is available
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed"
    exit 1
fi

# Set GOROOT and PATH for Homebrew installations
# Check if GOROOT is invalid or not set
if [ ! -d "$GOROOT" ] && [ -d "/opt/homebrew/Cellar/go" ]; then
    GO_VERSION=$(ls -1 /opt/homebrew/Cellar/go | tail -1)
    export GOROOT="/opt/homebrew/Cellar/go/$GO_VERSION/libexec"
    export PATH="/opt/homebrew/bin:$PATH"
fi

# Verify go.mod is tidy
echo "  → Checking go.mod..."
GOROOT=$GOROOT go mod tidy
if ! git diff --exit-code go.mod go.sum &> /dev/null; then
    echo "❌ go.mod is not tidy. Run 'go mod tidy' and commit the changes."
    git checkout go.mod go.sum
    exit 1
fi

# Run build check
echo "  → Building..."
if ! GOROOT=$GOROOT go build -v ./... &> /dev/null; then
    echo "❌ Build failed"
    exit 1
fi

# Run short tests (fast)
echo "  → Running tests..."
if ! GOROOT=$GOROOT go test -short ./... &> /dev/null; then
    echo "❌ Tests failed"
    exit 1
fi

# Run go fmt check
echo "  → Checking formatting..."
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
    echo "❌ Some files need formatting:"
    echo "$unformatted"
    echo ""
    echo "Run: go fmt ./..."
    exit 1
fi

echo "✅ All pre-commit checks passed"
exit 0
EOF

chmod +x "$HOOKS_DIR/pre-commit"
echo "✅ Installed pre-commit hook"

echo ""
echo "Git hooks installed successfully!"
echo ""
echo "To bypass hooks (not recommended):"
echo "  git commit --no-verify"
echo ""
