#!/usr/bin/env bash

# Test script to verify docker image versioning logic

echo "Testing Docker Image Versioning Logic"
echo "======================================"
echo ""

# Simulate the versioning logic from build_multi.sh
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "")
if [ -z "$VERSION" ]; then
  echo "✓ No git release tag found, will only push 'latest' tag"
  PUSH_LATEST_ONLY=true
else
  VERSION="${VERSION#v}"
  echo "✓ Using git tag version: $VERSION"
  PUSH_LATEST_ONLY=false
fi

echo ""
if [ "$PUSH_LATEST_ONLY" = true ]; then
  echo "Image tags that would be pushed:"
  echo "  - ogerardin/x-notes:latest"
else
  echo "Image tags that would be pushed:"
  echo "  - ogerardin/x-notes:$VERSION"
  echo "  - ogerardin/x-notes:latest"
  echo ""
  echo "Version label:"
  echo "  - org.opencontainers.image.version=$VERSION"
fi


