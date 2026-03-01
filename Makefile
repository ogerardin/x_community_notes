.PHONY: help build-builder build-api build-dist up down logs run push clean status tag-release update-oci oci-status oci-start oci-stop oci-restart oci-logs oci-pull

# Variables
CONTAINER_NAME := x-notes
DOCKER_BUILDER := x-notes-builder
DOCKER_DIST := x-notes-dist
DOCKER_API := x-notes-api
PORTS := -p 8080:80 -p 5432:5432
VOLUMES := -v $(shell pwd)/data:/home/data -v x-notes-db:/var/lib/postgresql/data
REPOSITORY := ogerardin/x-notes

# Detect running mode: compose or single
MODE := $(shell if docker ps --format '{{.Names}}' | grep -q '^x-notes$$'; then echo "single"; else echo "compose"; fi)

# Version variables
VERSION := $(shell VERSION=$$(git describe --tags --exact-match 2>/dev/null | sed 's/^v//'); [ -n "$$VERSION" ] && echo "$$VERSION" || echo "dev")
GIT_SHA := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")

# Build arguments
BUILD_ARGS := --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) --build-arg BUILD_TIME=$(BUILD_TIME) --build-arg REPOSITORY=$(REPOSITORY)
LABELS := --label org.opencontainers.image.version=$(VERSION) --label org.opencontainers.image.source=$(REPOSITORY) --label org.opencontainers.image.revision=$(GIT_SHA) --label org.opencontainers.image.created=$(BUILD_TIME)

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
	@echo "  tag-release   - Prompt for version, git tag, push, and build"
	@echo "  update-oci   - Update image on OCI instance (uses OCI_TAG or VERSION)"
	@echo "  oci-status   - Check container status on OCI"
	@echo "  oci-start    - Start container on OCI"
	@echo "  oci-stop     - Stop container on OCI"
	@echo "  oci-restart  - Restart container on OCI"
	@echo "  oci-logs     - View container logs on OCI"
	@echo "  oci-pull     - Pull latest image on OCI"
	@echo ""
	@echo "Version: $(VERSION)"
	@echo "Git SHA: $(GIT_SHA)"
	@echo "Build time: $(BUILD_TIME)"

build-builder:
	@docker build -t $(DOCKER_BUILDER) -f cmd/api/Dockerfile-builder $(BUILD_ARGS) .

build-api: build-builder
	@docker build -t $(DOCKER_API) -f cmd/api/Dockerfile $(BUILD_ARGS) .

build-dist: build-builder
	@docker build -t $(DOCKER_DIST) -f Dockerfile-dist $(BUILD_ARGS) $(LABELS) .

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

tag-release:
	@if [ -n "$$(git status --porcelain)" ] && [ "$(FORCE)" != "true" ]; then \
		echo "Error: Working directory not clean. Commit or stash changes, or use FORCE=true"; \
		exit 1; \
	fi
	@echo "Current version: $(VERSION)"
	@echo "This will tag, push, and build the dist image."
	@echo ""
	@$(SHELL) -c 'read -p "Enter version (or press Enter for suggested next patch): " NEW_VERSION; \
		if [ -z "$$NEW_VERSION" ]; then \
			if [ "$(VERSION)" = "dev" ]; then \
				NEW_VERSION="0.0.1"; \
			else \
				NEW_VERSION=$$(echo "$(VERSION)" | awk -F. "{$$3++; print}"); \
			fi; \
		fi; \
		if ! echo "$$NEW_VERSION" | grep -qE "^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$$"; then \
			echo "Error: Invalid semver format. Use e.g., 1.0.0 or 1.0.0-rc.1"; \
			exit 1; \
		fi; \
		echo ""; \
		echo "Tagging v$$NEW_VERSION..."; \
		git tag "v$$NEW_VERSION"; \
		echo "Pushing tag..."; \
		git push origin "v$$NEW_VERSION"; \
		echo ""; \
		echo "Building..."; \
		make build-dist'

update-oci:
	@TAG="$(VERSION)"; \
	if [ -n "$(OCI_TAG)" ]; then TAG="$(OCI_TAG)"; fi; \
	echo "Updating OCI instance to $(REPOSITORY):$$TAG"; \
	./update_image_oci.sh "$$TAG"

oci-status:
	@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
	ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker ps --filter name=$(CONTAINER_NAME)"

oci-start:
	@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
	ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker run -d --name $(CONTAINER_NAME) --publish 8080:80 --mount type=volume,source=x-notes-db,target=/var/lib/postgresql/data $(REPOSITORY):latest"

oci-stop:
	@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
	ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker stop $(CONTAINER_NAME)"

oci-restart: oci-stop oci-start

oci-logs:
	@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
	ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker logs -f $(CONTAINER_NAME)"

oci-pull:
	@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
	ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker pull $(REPOSITORY):latest"
