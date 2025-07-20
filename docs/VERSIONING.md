# Versioning and Release Process

This document explains how GoFlow handles versioning and releases.

## Overview

GoFlow uses **automated semantic versioning** based on [Conventional Commits](https://www.conventionalcommits.org/). The system automatically:

1. Analyzes commits since the last release
2. Determines the appropriate version bump
3. Updates version files
4. Creates git tags and GitHub releases
5. Publishes to the Go module proxy

## Version Bump Rules

| Commit Type | Version Bump | Example |
|-------------|--------------|---------|
| `feat:` | Minor (0.X.0) | `feat: add new task type` |
| `fix:` | Patch (0.0.X) | `fix: resolve race condition` |
| `chore:` | Patch (0.0.X) | `chore: update dependencies` |
| `BREAKING CHANGE:` | Major (X.0.0) | `feat!: change API interface` |
| `docs:`, `style:`, `refactor:`, `perf:`, `test:` | None | `docs: update README` |

## Commit Message Format

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Examples

```bash
# New feature (minor version bump)
git commit -m "feat: add retry mechanism for failed tasks"

# Bug fix (patch version bump)
git commit -m "fix: resolve race condition in worker pool"

# Breaking change (major version bump)
git commit -m "feat!: change task interface to use context"

# Documentation (no version bump)
git commit -m "docs: update API documentation"

# Maintenance (patch version bump)
git commit -m "chore: update dependencies"

# Breaking change in commit body
git commit -m "feat: change task interface

BREAKING CHANGE: Task interface now requires context parameter"
```

## Version Files

The versioning system manages several files:

- **VERSION**: Contains the current version (e.g., `0.1.0`)
- **go.mod**: Updated with version comment
- **CHANGELOG.md**: Automatically generated changelog

## Manual Version Management

### Check Current Version

```bash
make version
# or
./scripts/version.sh version
```

### Check Next Version

```bash
make version-next
# or
./scripts/version.sh next
```

### Bump Version (Files Only)

```bash
make version-bump
# or
./scripts/version.sh bump
```

### Create Release

```bash
make version-release
# or
./scripts/version.sh release
```

## Automated Releases

### GitHub Actions Workflow

The `.github/workflows/release.yml` workflow automatically:

1. **Triggers on**: Push to `main` branch or tag push
2. **Analyzes commits**: Since last tag
3. **Determines version**: Based on conventional commits
4. **Runs tests**: Ensures code quality
5. **Creates release**: Updates files, creates tag, pushes to GitHub
6. **Publishes**: To Go module proxy

### Release Process

1. **Push to main**: With conventional commits
2. **Workflow triggers**: GitHub Actions runs
3. **Version calculation**: Based on commit types
4. **Quality checks**: Tests, linting, vetting
5. **File updates**: VERSION, go.mod, CHANGELOG.md
6. **Git operations**: Commit, tag, push
7. **GitHub release**: Created with changelog
8. **Module publication**: Available on proxy.golang.org

## Best Practices

### For Contributors

1. **Use conventional commits**: Always follow the format
2. **Be specific**: Clear, descriptive commit messages
3. **Scope when helpful**: `feat(api): add new endpoint`
4. **Breaking changes**: Use `!` or `BREAKING CHANGE:`

### For Maintainers

1. **Review commits**: Ensure conventional format
2. **Monitor releases**: Check automated releases
3. **Manual releases**: Use when needed for control
4. **Version files**: Keep in sync

## Troubleshooting

### Common Issues

1. **No version bump**: Check commit message format
2. **Wrong version**: Verify conventional commit types
3. **Release fails**: Check GitHub Actions logs
4. **Tag conflicts**: Ensure no duplicate tags

### Debugging

```bash
# Check commit history
git log --oneline

# Verify conventional commits
git log --pretty=format:"%s" | grep -E "^(feat|fix|docs|style|refactor|perf|test|chore)"

# Check version script
./scripts/version.sh help
```

## Configuration

### Environment Variables

- `GITHUB_TOKEN`: Required for GitHub releases
- `GIT_REMOTE`: Git remote name (default: `origin`)
- `MAIN_BRANCH`: Main branch name (default: `main`)

### Customization

The versioning system can be customized by modifying:

- `scripts/version.sh`: Version calculation logic
- `.github/workflows/release.yml`: Release workflow
- `.releaserc`: Release configuration

## Integration with Go Modules

When a release is created:

1. **Tag is pushed**: `v1.2.3`
2. **Go module proxy**: Automatically picks up the tag
3. **Module available**: `go get github.com/ignatij/goflow@v1.2.3`

## Security

- **Signed commits**: Recommended for security
- **Protected branches**: Use branch protection rules
- **Required reviews**: Enforce PR reviews
- **Automated checks**: CI/CD validation

## Support

For issues with the versioning system:

1. Check the [GitHub Actions logs](https://github.com/ignatij/goflow/actions)
2. Review the [Conventional Commits specification](https://www.conventionalcommits.org/)
3. Open an issue with details about the problem 