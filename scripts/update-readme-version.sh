#!/bin/bash

# Script to update version references in README.md
# Usage: ./scripts/update-readme-version.sh <version>

set -e

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.2.1"
    exit 1
fi

VERSION=$1
README_FILE="README.md"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Check if README.md exists
if [ ! -f "$README_FILE" ]; then
    echo "Error: $README_FILE not found"
    exit 1
fi

print_info "Updating version references in $README_FILE to $VERSION"

# Create a backup
cp "$README_FILE" "${README_FILE}.bak"

# Update go get command (add version if not present, update if present)
sed -i.bak "s/go get github.com\/ignatij\/goflow\(@v[0-9]\+\.[0-9]\+\.[0-9]\+\)\?/go get github.com\/ignatij\/goflow@v$VERSION/g" "$README_FILE"

# Update require statement in go.mod example
sed -i.bak "s/require github.com\/ignatij\/goflow v[0-9]\+\.[0-9]\+\.[0-9]\+/require github.com\/ignatij\/goflow v$VERSION/g" "$README_FILE"

# Update any version badges (if they exist)
sed -i.bak "s/badge\/version-[0-9]\+\.[0-9]\+\.[0-9]\+/badge\/version-$VERSION/g" "$README_FILE"

# Remove backup files
rm -f "${README_FILE}.bak"

print_success "Updated $README_FILE with version $VERSION"

# Show what was changed
print_info "Changes made:"
git diff --no-index "${README_FILE}.bak" "$README_FILE" || true

# Clean up backup
rm -f "${README_FILE}.bak" 