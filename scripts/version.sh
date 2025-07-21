#!/bin/bash

# Version management script for GoFlow
# This script handles semantic versioning based on conventional commits

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
VERSION_FILE="VERSION"
CHANGELOG_FILE="CHANGELOG.md"
GIT_REMOTE="origin"
MAIN_BRANCH="main"

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to get current version
get_current_version() {
    if [ -f "$VERSION_FILE" ]; then
        cat "$VERSION_FILE"
    else
        echo "0.1.0"
    fi
}

# Function to get the next version based on commit types
get_next_version() {
    local current_version=$1
    local major=$(echo "$current_version" | cut -d. -f1)
    local minor=$(echo "$current_version" | cut -d. -f2)
    local patch=$(echo "$current_version" | cut -d. -f3)
    
    # Get commits since last tag
    local last_tag
    if git describe --tags --abbrev=0 2>/dev/null; then
        last_tag=$(git describe --tags --abbrev=0)
    else
        # If no tags exist, get all commits
        last_tag=""
    fi
    
    local commits
    if [ -n "$last_tag" ]; then
        commits=$(git log --pretty=format:"%s" "$last_tag"..HEAD)
    else
        commits=$(git log --pretty=format:"%s")
    fi
    
    local has_breaking=false
    local has_feature=false
    local has_fix=false
    local has_chore=false
    
    while IFS= read -r commit; do
        if [[ "$commit" =~ ^(feat|fix|docs|style|refactor|perf|test|chore)(\(.+\))?:.* ]]; then
            local commit_type=$(echo "$commit" | sed -E 's/^(feat|fix|docs|style|refactor|perf|test|chore)(\(.+\))?:.*/\1/')
            
            case $commit_type in
                "feat")
                    has_feature=true
                    ;;
                "fix")
                    has_fix=true
                    ;;
                "chore")
                    has_chore=true
                    ;;
            esac
            
            # Check for breaking changes
            if [[ "$commit" =~ .*BREAKING\ CHANGE.* ]]; then
                has_breaking=true
            fi
        fi
    done <<< "$commits"
    
    # Determine version bump
    if [ "$has_breaking" = true ]; then
        echo "$((major + 1)).0.0"
    elif [ "$has_feature" = true ]; then
        echo "$major.$((minor + 1)).0"
    elif [ "$has_fix" = true ] || [ "$has_chore" = true ]; then
        echo "$major.$minor.$((patch + 1))"
    else
        echo "$current_version"
    fi
}

# Function to update version file
update_version_file() {
    local version=$1
    echo "$version" > "$VERSION_FILE"
    print_success "Updated version to $version"
}

# Function to update go.mod version
update_go_mod_version() {
    local version=$1
    # Remove 'v' prefix if present
    local clean_version=${version#v}
    
    # Update go.mod if it contains a version directive
    if grep -q "^// Version:" go.mod; then
        # Use a different delimiter to avoid issues with forward slashes
        sed -i.bak "s|^// Version:.*|// Version: $clean_version|" go.mod
        rm -f go.mod.bak
    else
        # Add version directive if it doesn't exist
        echo "// Version: $clean_version" >> go.mod
    fi
    print_success "Updated go.mod version to $clean_version"
}

# Function to generate changelog
generate_changelog() {
    local version=$1
    local last_tag
    if git describe --tags --abbrev=0 2>/dev/null; then
        last_tag=$(git describe --tags --abbrev=0)
    else
        last_tag=""
    fi
    
    # Create changelog entry
    local changelog_entry="## [${version#v}] - $(date +%Y-%m-%d)\n\n"
    
    # Get commits since last tag
    local commits
    if [ -n "$last_tag" ]; then
        commits=$(git log --pretty=format:"%s" "$last_tag"..HEAD)
    else
        commits=$(git log --pretty=format:"%s")
    fi
    
    local features=""
    local fixes=""
    local docs=""
    local chores=""
    local other=""
    
    while IFS= read -r commit; do
        if [[ "$commit" =~ ^(feat|fix|docs|style|refactor|perf|test|chore)(\(.+\))?:.* ]]; then
            local commit_type=$(echo "$commit" | sed -E 's/^(feat|fix|docs|style|refactor|perf|test|chore)(\(.+\))?:.*/\1/')
            local commit_message=$(echo "$commit" | sed -E 's/^(feat|fix|docs|style|refactor|perf|test|chore)(\(.+\))?: //')
            
            case $commit_type in
                "feat")
                    features="${features}- ${commit_message}\n"
                    ;;
                "fix")
                    fixes="${fixes}- ${commit_message}\n"
                    ;;
                "docs")
                    docs="${docs}- ${commit_message}\n"
                    ;;
                "chore")
                    chores="${chores}- ${commit_message}\n"
                    ;;
                *)
                    other="${other}- ${commit_message}\n"
                    ;;
            esac
        fi
    done <<< "$commits"
    
    # Build changelog entry
    if [ -n "$features" ]; then
        changelog_entry="${changelog_entry}### Features\n${features}\n"
    fi
    
    if [ -n "$fixes" ]; then
        changelog_entry="${changelog_entry}### Bug Fixes\n${fixes}\n"
    fi
    
    if [ -n "$docs" ]; then
        changelog_entry="${changelog_entry}### Documentation\n${docs}\n"
    fi
    
    if [ -n "$chores" ]; then
        changelog_entry="${changelog_entry}### Maintenance\n${chores}\n"
    fi
    
    if [ -n "$other" ]; then
        changelog_entry="${changelog_entry}### Other Changes\n${other}\n"
    fi
    
    # Prepend to changelog file
    if [ -f "$CHANGELOG_FILE" ]; then
        echo -e "$changelog_entry\n$(cat "$CHANGELOG_FILE")" > "$CHANGELOG_FILE"
    else
        echo -e "$changelog_entry" > "$CHANGELOG_FILE"
    fi
    
    print_success "Generated changelog for version $version"
}

# Function to create git tag
create_git_tag() {
    local version=$1
    local tag="v$version"
    
    git tag -a "$tag" -m "Release $tag"
    print_success "Created git tag $tag"
}

# Function to push changes
push_changes() {
    local version=$1
    local tag="v$version"
    
    git add "$VERSION_FILE" "$CHANGELOG_FILE" go.mod
    git commit -m "chore: release version $version"
    git push "$GIT_REMOTE" "$MAIN_BRANCH"
    git push "$GIT_REMOTE" "$tag"
    
    print_success "Pushed changes and tag $tag"
}

# Function to show current version
show_version() {
    local current_version=$(get_current_version)
    print_info "Current version: $current_version"
}

# Function to show next version
show_next_version() {
    local current_version=$(get_current_version)
    local next_version=$(get_next_version "$current_version")
    
    # Debug output
    echo "DEBUG: current_version='$current_version'" >&2
    echo "DEBUG: next_version='$next_version'" >&2
    
    if [ "$current_version" = "$next_version" ]; then
        print_info "No version bump needed. Current version: $current_version"
    else
        print_info "Current version: $current_version"
        print_info "Next version: $next_version"
    fi
}

# Function to bump version
bump_version() {
    local current_version=$(get_current_version)
    local next_version=$(get_next_version "$current_version")
    
    if [ "$current_version" = "$next_version" ]; then
        print_warning "No version bump needed. Current version: $current_version"
        return 0
    fi
    
    print_info "Bumping version from $current_version to $next_version"
    
    update_version_file "$next_version"
    update_go_mod_version "$next_version"
    generate_changelog "$next_version"
    
    print_success "Version bumped to $next_version"
}

# Function to release
release() {
    local current_version=$(get_current_version)
    local next_version=$(get_next_version "$current_version")
    
    if [ "$current_version" = "$next_version" ]; then
        print_warning "No version bump needed. Current version: $current_version"
        return 0
    fi
    
    print_info "Releasing version $next_version"
    
    bump_version
    create_git_tag "$next_version"
    push_changes "$next_version"
    
    print_success "Released version $next_version"
}

# Function to show help
show_help() {
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  version     Show current version"
    echo "  next        Show next version based on commits"
    echo "  bump        Bump version based on commits"
    echo "  release     Bump version, create tag, and push"
    echo "  help        Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 version"
    echo "  $0 next"
    echo "  $0 bump"
    echo "  $0 release"
}

# Main script logic
case "${1:-help}" in
    "version")
        show_version
        ;;
    "next")
        show_next_version
        ;;
    "bump")
        bump_version
        ;;
    "release")
        release
        ;;
    "help"|"--help"|"-h")
        show_help
        ;;
    *)
        print_error "Unknown command: $1"
        show_help
        exit 1
        ;;
esac 