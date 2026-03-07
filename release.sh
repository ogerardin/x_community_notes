#!/usr/bin/env bash
set -euo pipefail

REPOSITORY="ogerardin/x-notes"
CONTAINER_NAME="x-notes"

VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
echo "Current version: $VERSION"

read -p "Enter version (or press Enter for suggested next patch): " NEW_VERSION

if [ -z "$NEW_VERSION" ]; then
    MAJOR=$(echo "$VERSION" | cut -d. -f1)
    MINOR=$(echo "$VERSION" | cut -d. -f2)
    PATCH=$(echo "$VERSION" | cut -d. -f3)
    NEW_VERSION="${MAJOR}.${MINOR}.$((PATCH + 1))"
fi

if ! echo "$NEW_VERSION" | grep -qE "^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$"; then
    echo "Error: Invalid semver format. Use e.g., 1.0.0 or 1.0.0-rc.1"
    exit 1
fi

echo "Tagging v$NEW_VERSION..."
git tag "v$NEW_VERSION"
git push origin "v$NEW_VERSION"

echo "Building and pushing multi-arch image..."
make push

echo ""
read -p "Deploy to OCI? [y/N] " DEPLOY

if [ "$DEPLOY" = "y" ] || [ "$DEPLOY" = "Y" ]; then
    INSTANCE_OCID=$(cat .instance_ocid)
    IP=$(oci compute instance list-vnics --instance-id "$INSTANCE_OCID" --query 'data[0]."public-ip"' --raw-output)

    echo "Pulling on OCI..."
    ssh -o StrictHostKeyChecking=no ubuntu@$IP "sudo docker pull $REPOSITORY:latest"

    JOB_ID=$(ssh -o StrictHostKeyChecking=no ubuntu@$IP "curl -s http://localhost:8080/api/imports/current" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)

    if [ -n "$JOB_ID" ]; then
        echo "Aborting running import $JOB_ID..."
        ssh -o StrictHostKeyChecking=no ubuntu@$IP "curl -s -X DELETE http://localhost:8080/api/imports/$JOB_ID"
        sleep 3
    fi

    ssh -o StrictHostKeyChecking=no ubuntu@$IP "sudo docker rm -f $CONTAINER_NAME 2>/dev/null || true"
    ssh -o StrictHostKeyChecking=no ubuntu@$IP "sudo docker run -d --name $CONTAINER_NAME --publish 8080:80 --mount type=volume,source=x-notes-db,target=/var/lib/postgresql --mount type=volume,source=x-notes-data,target=/home/data $REPOSITORY:latest"

    echo "Deployed $REPOSITORY:$NEW_VERSION to OCI"
fi
