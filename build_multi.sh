#!/usr/bin/env bash

# This script builds and pushes a multi-architecture Docker image using Docker Buildx.
# It ensures the required buildx builder exists, installs buildx if necessary, and targets several platforms.
#
# Versioning:
# - If on an exact git tag (e.g., v1.0.0): use 1.0.0, push as :1.0.0 and :latest
# - If not on a tag: use "dev", warn user, push as :latest only

set -euo pipefail

# Configurable variables
PLATFORMS="linux/amd64,linux/arm64,linux/arm/v7"
REPOSITORY="ogerardin/x-notes"
REPOSITORY_BUILDER="ogerardin/x-notes-builder"
DOCKERFILE="Dockerfile-dist"
BUILDER_NAME="builder-multi"

# Extract version from git tag - check for exact match
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "")
if [ -z "$VERSION" ]; then
  echo "WARNING: No exact git tag found on current commit."
  echo "         Building with version 'dev' - image will NOT be tagged with a version."
  echo "         Only 'latest' tag will be pushed."
  echo ""
  echo "         To create a proper release:"
  echo "           1. Run: make release"
  echo "           2. Or manually: git tag v1.0.0 && git push origin v1.0.0"
  echo ""
  VERSION="dev"
else
  # Remove 'v' prefix (e.g., v1.0.0 -> 1.0.0)
  VERSION="${VERSION#v}"
  echo "Using git tag version: $VERSION"
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

# Build and push builder image
echo "Building and pushing builder image..."
docker buildx build --platform "$PLATFORMS" \
  --build-arg VERSION="${VERSION}" \
  --build-arg GIT_SHA="${GIT_SHA}" \
  --build-arg BUILD_TIME="${BUILD_TIME}" \
  -t "$REPOSITORY_BUILDER:latest" \
  -f cmd/api/Dockerfile-builder . --push

# Build and push main image - always push :latest, also :VERSION if not dev
TAGS="--tag $REPOSITORY:latest"
if [ "$VERSION" != "dev" ]; then
  TAGS="$TAGS --tag $REPOSITORY:${VERSION}"
  echo "Building and pushing multi-arch image: $REPOSITORY:$VERSION and $REPOSITORY:latest"
else
  echo "Building and pushing multi-arch image: $REPOSITORY:latest (dev build)"
fi

docker buildx build --platform "$PLATFORMS" \
  $BUILD_ARGS $LABELS \
  $TAGS \
  --file "$DOCKERFILE" . --push
