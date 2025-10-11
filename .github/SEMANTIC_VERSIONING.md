# Semantic Versioning

This project uses automated semantic versioning based on [Conventional Commits](https://www.conventionalcommits.org/).

## How It Works

When you push to the `main` branch, the release workflow automatically:
1. Analyzes commit messages since the last release
2. Determines the next version based on commit types
3. Generates a changelog
4. Creates a GitHub release with release notes

## Commit Message Format

Use this format for your commit messages:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Commit Types and Version Bumps

| Commit Type | Version Bump | Example |
|-------------|--------------|---------|
| `feat:` | **Minor** (0.x.0) | `feat: add Redis connection pooling` |
| `fix:` | **Patch** (0.0.x) | `fix: resolve race condition in lock acquisition` |
| `perf:` | **Patch** (0.0.x) | `perf: optimize query index lookup` |
| `refactor:` | **Patch** (0.0.x) | `refactor: simplify error handling` |
| `BREAKING CHANGE:` | **Major** (x.0.0) | `feat!: change Store API signature` |
| `docs:` | No release | Documentation only |
| `test:` | No release | Test updates only |
| `chore:` | No release | Maintenance tasks |
| `ci:` | No release | CI configuration |

### Breaking Changes

To trigger a major version bump, use `!` after the type or add `BREAKING CHANGE:` in the footer:

```bash
# Option 1: Using !
git commit -m "feat!: change Redis indexer API to support multiple indexes"

# Option 2: Using footer
git commit -m "feat: change Redis indexer API

BREAKING CHANGE: The RegisterIndex method now requires an entityType parameter"
```

## Examples

### Feature (Minor Version Bump)
```bash
git commit -m "feat: add distributed lock support with Redis

Implements automatic lock acquisition and release with configurable TTL.
Includes retry logic with exponential backoff."
```
**Result:** `v1.2.0` → `v1.3.0`

### Bug Fix (Patch Version Bump)
```bash
git commit -m "fix: prevent index drift in high-write scenarios

Adds transaction support to ensure index updates are atomic with
data writes."
```
**Result:** `v1.2.0` → `v1.2.1`

### Breaking Change (Major Version Bump)
```bash
git commit -m "feat!: redesign query API for better performance

BREAKING CHANGE: Query() now returns an iterator instead of []string.
Update code from:
  keys, _ := store.Query("users/")
To:
  results := store.Query("users/").All(ctx, &users)
"
```
**Result:** `v1.2.0` → `v2.0.0`

### Multiple Commits

When multiple commits are pushed together, the **highest** version bump applies:

- 3 `fix:` commits + 1 `feat:` commit = **Minor** version bump
- 2 `feat:` commits + 1 `feat!:` commit = **Major** version bump

## Best Practices

### DO ✅

- Use lowercase for types
- Keep descriptions concise (under 72 characters)
- Use present tense ("add" not "added")
- Reference issues: `fix: resolve deadlock (#123)`
- Group related changes in one commit when possible

### DON'T ❌

- Don't use past tense: ~~`feat: added caching`~~
- Don't be vague: ~~`fix: bug fix`~~
- Don't mix types: ~~`feat+fix: add feature and fix bug`~~
- Don't capitalize descriptions: ~~`Feat: Add support`~~

## Checking Before Push

Preview what version will be released:

```bash
# View commits since last tag
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# Check commit message format
git log -1 --pretty=%B
```

## Local Development

Commits to branches other than `main` don't trigger releases. This allows you to:

1. Work on feature branches with any commit style
2. Squash/rebase before merging to `main`
3. Write one well-formatted commit message for the merge

## Manual Release (Emergency)

If automated release fails, you can create a release manually:

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

Then create the GitHub release from the tag.

## Configuration

The semantic versioning configuration is in `.releaserc.json`. Modify it to:

- Change version bump rules
- Customize changelog sections
- Add release plugins
- Configure release branches

## CI/CD Integration

The release workflow (`.github/workflows/release.yml`):

1. ✅ Runs all tests
2. ✅ Verifies build compiles
3. ✅ Analyzes commits
4. ✅ Bumps version
5. ✅ Updates CHANGELOG.md
6. ✅ Creates GitHub release
7. ✅ Adds release notes

All steps must pass for a release to be created.
