#!/usr/bin/env bash

DOCKERFILE=Dockerfile-alpine
IMAGE=ogerardin/x-notes:latest

docker buildx build -f "$DOCKERFILE" -t "$IMAGE" .

docker run -p 8080:80 -p 5432:5432 \
  -v /Users/olivier/Documents/NAFO/community_notes/data:/home/data \
  -v x-notes-db:/var/lib/postgresql/data \
  --name x-notes "$IMAGE"
