#!/usr/bin/env bash

# Test script to verify docker image versioning logic

echo "Testing Docker Image Versioning Logic"
echo "======================================"
echo ""

# Simulate the versioning logic from build_multi.sh
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "")
if [ -z "$VERSION" ]; then
  VERSION=$(git rev-parse --short HEAD)
  echo "✓ No git tag found, using commit hash as version: $VERSION"
else
  VERSION="${VERSION#v}"
  echo "✓ Using git tag version: $VERSION"
fi

echo ""
echo "Image tags that would be pushed:"
echo "  - ogerardin/x-notes:$VERSION"
echo "  - ogerardin/x-notes:latest"
echo ""
echo "Version label:"
echo "  - org.opencontainers.image.version=$VERSION"

