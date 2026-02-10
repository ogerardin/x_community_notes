#!/usr/bin/env bash

# This script builds and pushes a multi-architecture Docker image using Docker Buildx.
# It ensures the required buildx builder exists, installs buildx if necessary, and targets several platforms.
# The image is built from Dockerfile-alpine and pushed to the specified repository.
#
# Versioning:
# - If the current commit is tagged with a semantic version (e.g., v1.0.0), that version is used
# - Otherwise, the short commit hash is used as the version
# - Both version-specific and "latest" tags are pushed

set -euo pipefail

# Configurable variables
PLATFORMS="linux/amd64,linux/arm64,linux/i386,linux/arm/v7"
REPOSITORY="ogerardin/x-notes"
DOCKERFILE="Dockerfile-alpine"
BUILDER_NAME="mybuilder"

# Extract version from git tag or use commit hash as fallback
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "")
if [ -z "$VERSION" ]; then
  # No exact tag match, use short commit hash
  VERSION=$(git rev-parse --short HEAD)
  echo "No git tag found, using commit hash as version: $VERSION"
else
  # Remove 'v' prefix if present (e.g., v1.0.0 -> 1.0.0)
  VERSION="${VERSION#v}"
  echo "Using git tag version: $VERSION"
fi

# Check if buildx builder exists, create if not, otherwise use it
if ! docker buildx inspect "$BUILDER_NAME" &>/dev/null; then
  echo "Creating buildx builder '$BUILDER_NAME'..."
  docker buildx create --name "$BUILDER_NAME" --use
else
  echo "Using existing buildx builder '$BUILDER_NAME'..."
  docker buildx use "$BUILDER_NAME"
fi

# Install buildx if not already installed
if ! docker buildx version &>/dev/null; then
  echo "Installing docker buildx..."
  docker buildx install
fi

echo "Building and pushing multi-arch image: $REPOSITORY:$VERSION and $REPOSITORY:latest"
docker buildx build --platform "$PLATFORMS" \
  --tag "$REPOSITORY:$VERSION" \
  --tag "$REPOSITORY:latest" \
  --label "org.opencontainers.image.version=$VERSION" \
  --file "$DOCKERFILE" . --push
