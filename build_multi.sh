#!/usr/bin/env bash

# This script builds and pushes a multi-architecture Docker image using Docker Buildx.
# It ensures the required buildx builder exists, installs buildx if necessary, and targets several platforms.
# The image is built from Dockerfile-alpine and pushed to the specified repository.
#
# Versioning:
# - If the current commit is tagged with a semantic version (e.g., v1.0.0), both the version-specific
#   and "latest" tags are pushed
# - If no release tag exists, only the "latest" tag is pushed

set -euo pipefail

# Configurable variables
PLATFORMS="linux/amd64,linux/arm64,linux/arm/v7"
REPOSITORY="ogerardin/x-notes"
REPOSITORY_BUILDER="ogerardin/x-notes-builder"
DOCKERFILE="Dockerfile-dist"
BUILDER_NAME="builder-multi"

# Extract version from git tag
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "")
if [ -z "$VERSION" ]; then
  # No exact tag match, only push 'latest'
  echo "No git release tag found, will only push 'latest' tag"
  VERSION="dev"
  PUSH_LATEST_ONLY=true
else
  # Remove 'v' prefix if present (e.g., v1.0.0 -> 1.0.0)
  VERSION="${VERSION#v}"
  echo "Using git tag version: $VERSION"
  PUSH_LATEST_ONLY=false
fi

# Extract git SHA and build time
GIT_SHA=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")

echo "Git SHA: $GIT_SHA"
echo "Build time: $BUILD_TIME"

# Build common arguments
BUILD_ARGS="--build-arg VERSION=${VERSION} --build-arg GIT_SHA=${GIT_SHA} --build-arg BUILD_TIME=${BUILD_TIME} --build-arg REPOSITORY=${REPOSITORY}"
LABELS="--label org.opencontainers.image.version=${VERSION} --label org.opencontainers.image.source=${REPOSITORY} --label org.opencontainers.image.revision=${GIT_SHA} --label org.opencontainers.image.created=${BUILD_TIME}"

# Check if buildx builder exists, create if not, otherwise use it
if ! docker buildx inspect "$BUILDER_NAME" &>/dev/null; then
  echo "Creating buildx builder '$BUILDER_NAME' with docker-container driver..."
  docker buildx create --name "$BUILDER_NAME" --driver docker-container --use
else
  echo "Using existing buildx builder '$BUILDER_NAME'..."
  docker buildx use "$BUILDER_NAME"
fi

# Install buildx if not already installed
if ! docker buildx version &>/dev/null; then
  echo "Installing docker buildx..."
  docker buildx install
fi

# Build and push builder image first (required for multi-arch build)
echo "Building and pushing builder image..."
docker buildx build --platform "$PLATFORMS" \
  --build-arg VERSION="${VERSION}" \
  --build-arg GIT_SHA="${GIT_SHA}" \
  --build-arg BUILD_TIME="${BUILD_TIME}" \
  -t "$REPOSITORY_BUILDER:latest" \
  -f cmd/api/Dockerfile-builder . --push

if [ "$PUSH_LATEST_ONLY" = false ]; then
  docker buildx build --platform "$PLATFORMS" \
    --build-arg VERSION="${VERSION}" \
    --build-arg GIT_SHA="${GIT_SHA}" \
    --build-arg BUILD_TIME="${BUILD_TIME}" \
    -t "$REPOSITORY_BUILDER:${VERSION}" \
    -f cmd/api/Dockerfile-builder . --push
fi

if [ "$PUSH_LATEST_ONLY" = true ]; then
  echo "Building and pushing multi-arch image: $REPOSITORY:latest"
  docker buildx build --platform "$PLATFORMS" \
    $BUILD_ARGS $LABELS \
    --tag "$REPOSITORY:latest" \
    --file "$DOCKERFILE" . --push
else
  echo "Building and pushing multi-arch image: $REPOSITORY:$VERSION and $REPOSITORY:latest"
  docker buildx build --platform "$PLATFORMS" \
    $BUILD_ARGS $LABELS \
    --tag "$REPOSITORY:$VERSION" \
    --tag "$REPOSITORY:latest" \
    --file "$DOCKERFILE" . --push
fi
