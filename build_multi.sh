#!/usr/bin/env bash

docker buildx create --name mybuilder --use
docker buildx install

docker build --platform linux/amd64,linux/arm64,linux/i386,linux/arm/v7 \
  --tag ogerardin/x-notes:latest \
  --file Dockerfile-alpine . --push