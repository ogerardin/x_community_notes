.PHONY: help build-builder build-api build-dist up down logs run push clean status

# Variables
CONTAINER_NAME := x-notes
DOCKER_BUILDER := x-notes-builder
DOCKER_DIST := x-notes-dist
DOCKER_API := x-notes-api
PORTS := -p 8080:80 -p 5432:5432
VOLUMES := -v $(shell pwd)/data:/home/data -v x-notes-db:/var/lib/postgresql/data

# Detect running mode: compose or single
MODE := $(shell if docker ps --format '{{.Names}}' | grep -q '^x-notes$$'; then echo "single"; else echo "compose"; fi)

help:
	@echo "Available targets:"
	@echo "  build-builder  - Build shared Go builder image"
	@echo "  build-api      - Build API image for compose"
	@echo "  build-dist     - Build single-container image"
	@echo "  up             - Start compose services"
	@echo "  down           - Stop compose services"
	@echo "  logs           - Follow compose logs"
	@echo "  run            - Build and run single container"
	@echo "  push           - Build and push to Docker Hub"
	@echo "  clean          - Remove all containers"
	@echo "  status         - Show running containers"

build-builder:
	@docker build -t $(DOCKER_BUILDER) -f cmd/api/Dockerfile-builder .

build-api: build-builder
	@docker build -t $(DOCKER_API) -f cmd/api/Dockerfile .

build-dist: build-builder
	@docker build -t $(DOCKER_DIST) -f Dockerfile-dist .

up: build-builder
	@docker volume create x-notes-db 2>/dev/null || true
	@docker stop $(CONTAINER_NAME) 2>/dev/null && docker rm $(CONTAINER_NAME) || true
	@docker compose up -d --build

down:
	@docker compose down

logs:
	@$(if $(filter single,$(MODE)),docker logs -f $(CONTAINER_NAME),docker compose logs -f)

run: build-dist
	@docker compose down 2>/dev/null || true
	@docker rm -f $(CONTAINER_NAME) 2>/dev/null || true
	@docker run -d $(PORTS) $(VOLUMES) --name $(CONTAINER_NAME) $(DOCKER_DIST)

push: build-builder
	@./build_multi.sh

clean:
	@docker compose down 2>/dev/null || true
	@docker stop $(CONTAINER_NAME) 2>/dev/null || true
	@docker rm $(CONTAINER_NAME) 2>/dev/null || true
	@echo "All containers stopped"

status:
	@echo "Mode: $(MODE)"
	@docker ps --format "table {{.Names}}\t{{.Status}}" | grep -E "x-notes" || echo "No containers running"
