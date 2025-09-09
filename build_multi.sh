#!/usr/bin/env bash

# This script builds and pushes a multi-architecture Docker image using Docker Buildx.
# It ensures the required buildx builder exists, installs buildx if necessary, and targets several platforms.
# The image is built from Dockerfile-alpine and pushed to the specified repository.

set -euo pipefail

# Configurable variables
PLATFORMS="linux/amd64,linux/arm64,linux/i386,linux/arm/v7"
IMAGE="ogerardin/x-notes:latest"
DOCKERFILE="Dockerfile-alpine"
BUILDER_NAME="mybuilder"

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

echo "Building and pushing multi-arch image: $IMAGE"
docker buildx build --platform "$PLATFORMS" \
  --tag "$IMAGE" \
  --file "$DOCKERFILE" . --push
