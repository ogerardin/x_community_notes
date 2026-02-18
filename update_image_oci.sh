#!/usr/bin/env bash

set -euo pipefail

IMAGE_NAME="ogerardin/x-notes"
CONTAINER_NAME="x-notes"
SSH_USER="ubuntu"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/.instance_ocid"

TAG="${1:-latest}"
INSTANCE_OCID="${2:-}"

if [ -z "$INSTANCE_OCID" ] && [ -f "$CONFIG_FILE" ]; then
  INSTANCE_OCID=$(cat "$CONFIG_FILE")
fi

if [ -z "$INSTANCE_OCID" ]; then
  echo "Usage: $0 [tag] [instance-ocid]" >&2
  echo "  tag: Image tag (default: latest)" >&2
  echo "  instance-ocid: Oracle Cloud instance OCID (or store in $CONFIG_FILE)" >&2
  exit 1
fi

if ! command -v oci &>/dev/null; then
  echo "Error: OCI CLI is not installed" >&2
  exit 1
fi

echo "Fetching instance public IP..."
IP=$(oci compute instance list-vnics --instance-id "$INSTANCE_OCID" \
  --query 'data[0]."public-ip"' --raw-output 2>/dev/null)

if [ -z "$IP" ]; then
  echo "Error: Could not get instance IP" >&2
  exit 1
fi

echo "Connecting to $SSH_USER@$IP"
echo "Updating to $IMAGE_NAME:$TAG"

ssh -o StrictHostKeyChecking=no "$SSH_USER@$IP" << EOF
set -euo pipefail

OLD_DIGEST=\$(sudo docker inspect --format='{{index .RepoDigests 0}}' "$IMAGE_NAME:$TAG" 2>/dev/null || echo "")

echo "Pulling image $IMAGE_NAME:$TAG..."
sudo docker pull "$IMAGE_NAME:$TAG"

NEW_DIGEST=\$(sudo docker inspect --format='{{index .RepoDigests 0}}' "$IMAGE_NAME:$TAG" 2>/dev/null || echo "")

if [ "\$OLD_DIGEST" = "\$NEW_DIGEST" ] && [ -n "\$OLD_DIGEST" ]; then
  echo "Image unchanged, skipping container recreation."
  echo "Container status:"
  sudo docker ps --filter name="$CONTAINER_NAME"
else
  if [ -n "\$OLD_DIGEST" ]; then
    echo "Image updated, recreating container..."
  else
    echo "No existing image, creating container..."
  fi
  sudo docker stop "$CONTAINER_NAME" 2>/dev/null || true
  sudo docker rm "$CONTAINER_NAME" 2>/dev/null || true
  sudo docker run --detach --name "$CONTAINER_NAME" \
    --publish 8080:80 \
    --mount type=volume,source=x-notes-db,target=/var/lib/postgresql/data \
    "$IMAGE_NAME:\$TAG"
  echo "Container status:"
  sudo docker ps --filter name="$CONTAINER_NAME"
fi
EOF

echo "Update complete!"
