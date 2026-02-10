#!/usr/bin/env bash

# Builds a Docker image for the x-notes application using Dockerfile-dist, then runs the container with appropriate
# volumes and ports for data and PostgreSQL persistence. Cleans up any previous container instance and ensures Docker
# Buildx is available.

# Exit on error
set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Configurable variables
DOCKERFILE="Dockerfile-dist"
IMAGE="ogerardin/x-notes:latest"
CONTAINER_NAME="x-notes"
HOST_PORT="8080"
DATA_PATH="$SCRIPT_DIR/data"
DB_VOLUME="x-notes-db"

# Check for docker buildx
if ! docker buildx version >/dev/null 2>&1; then
  echo "Error: docker buildx is not available. Please install Docker Buildx." >&2
  exit 1
fi

echo "Building Docker image $IMAGE using $DOCKERFILE..."
docker buildx build -f "$DOCKERFILE" -t "$IMAGE" . --load

echo "Cleaning up any existing container named $CONTAINER_NAME..."
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

echo "Running container $CONTAINER_NAME..."
docker run -p "$HOST_PORT":80 -p 5432:5432 \
  -v "$DATA_PATH":/home/data \
  -v "$DB_VOLUME":/var/lib/postgresql/data \
  --name "$CONTAINER_NAME" "$IMAGE"
