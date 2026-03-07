.PHONY: help build-builder build-api build-dist compose-up compose-down compose-logs run push clean status release oci-update oci-status oci-start oci-stop oci-restart oci-logs oci-pull oci-prune list-releases up down logs stop

# Variables
CONTAINER_NAME := x-notes
DOCKER_BUILDER := x-notes-builder
DOCKER_DIST := x-notes-dist
DOCKER_API := x-notes-api
PORTS := -p 8080:80 -p 5432:5432
VOLUMES := -v $(shell pwd)/data:/home/data -v x-notes-db:/var/lib/postgresql
REPOSITORY := ogerardin/x-notes

# Detect running mode: compose or single
MODE ?= $(shell if docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^x-notes$$'; then echo "single"; else echo "compose"; fi)

# Version variables
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
GIT_SHA := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")

# OCI helper: executes docker command on OCI instance
define oci_exec
@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker $(1)"
endef

# Build arguments
BUILD_ARGS := --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) --build-arg BUILD_TIME=$(BUILD_TIME) --build-arg REPOSITORY=$(REPOSITORY)
LABELS := --label org.opencontainers.image.version=$(VERSION) --label org.opencontainers.image.source=$(REPOSITORY) --label org.opencontainers.image.revision=$(GIT_SHA) --label org.opencontainers.image.created=$(BUILD_TIME)

help:
	@echo "Available targets:"
	@echo "  build-builder   - Build shared Go builder image"
	@echo "  build-api      - Build API image for compose"
	@echo "  build-dist     - Build single-container image"
	@echo "  compose-up     - Start compose services"
	@echo "  compose-down   - Stop compose services"
	@echo "  stop           - Stop single container or compose"
	@echo "  compose-logs   - Follow compose logs"
	@echo "  run            - Build and run single container"
	@echo "  push           - Build and push to Docker Hub"
	@echo "  clean          - Remove all containers"
	@echo "  status         - Show running containers"
	@echo "  release        - Version, build, push, and optionally deploy to OCI"
	@echo "  oci-update    - Update image on OCI instance (uses OCI_TAG or VERSION)"
	@echo "  oci-status    - Check container status on OCI"
	@echo "  oci-start     - Start container on OCI"
	@echo "  oci-stop      - Stop container on OCI"
	@echo "  oci-restart   - Restart container on OCI"
	@echo "  oci-logs      - View container logs on OCI"
	@echo "  oci-pull      - Pull latest image on OCI"
	@echo "  list-releases - List release tags"
	@echo ""
	@echo "Version: $(VERSION)"
	@echo "Git SHA: $(GIT_SHA)"
	@echo "Build time: $(BUILD_TIME)"

build-builder:
	@docker build -t $(DOCKER_BUILDER) -f cmd/api/Dockerfile-builder $(BUILD_ARGS) .

build-api: build-builder
	@docker build -t $(DOCKER_API) -f cmd/api/Dockerfile $(BUILD_ARGS) .

build-dist: build-builder
	@docker buildx build -t $(DOCKER_DIST) -f Dockerfile-dist $(BUILD_ARGS) $(LABELS) --load .

compose-up: build-builder
	@docker volume create x-notes-db 2>/dev/null || true
	@docker stop $(CONTAINER_NAME) 2>/dev/null && docker rm $(CONTAINER_NAME) || true
	@BUILDX_BUILDER=default docker compose up -d --build

compose-down:
	@docker compose down

stop:
	@$(if $(filter single,$(MODE)),docker stop $(CONTAINER_NAME),docker compose down)

compose-logs:
	@$(if $(filter single,$(MODE)),docker logs -f $(CONTAINER_NAME),docker compose logs -f)

# Aliases for backwards compatibility
up: compose-up
down: compose-down
logs: compose-logs

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

release:
	@if [ -n "$$(git status --porcelain)" ] && [ "$(FORCE)" != "true" ]; then \
		echo "Error: Working directory not clean. Commit or stash changes, or use FORCE=true"; \
		exit 1; \
	fi
	@./release.sh

oci-update:
	@if [ -n "$(OCI_TAG)" ]; then \
		TAG="$(OCI_TAG)"; \
	else \
		TAG="latest"; \
	fi; \
	echo "Updating OCI instance to $(REPOSITORY):$$TAG"; \
	./update_image_oci.sh "$$TAG"

oci-status:
	$(call oci_exec,ps --filter name=$(CONTAINER_NAME))

oci-start:
	@IP=$$(oci compute instance list-vnics --instance-id "$$(cat .instance_ocid)" --query 'data[0]."public-ip"' --raw-output) && \
	ssh -o StrictHostKeyChecking=no ubuntu@$$IP "sudo docker pull $(REPOSITORY):latest && sudo docker rm -f $(CONTAINER_NAME) 2>/dev/null || true; sudo docker run -d --name $(CONTAINER_NAME) --publish 8080:80 --mount type=volume,source=x-notes-db,target=/var/lib/postgresql --mount type=volume,source=x-notes-data,target=/home/data -e ADMIN_CONTROLS_DISABLED=true $(REPOSITORY):latest"

oci-stop:
	$(call oci_exec,stop $(CONTAINER_NAME))

oci-restart: oci-stop oci-start

oci-logs:
	$(call oci_exec,logs -f $(CONTAINER_NAME))

oci-pull:
	@if [ -n "$(OCI_TAG)" ]; then \
		TAG="$(OCI_TAG)"; \
	else \
		TAG="latest"; \
	fi; \
	$(call oci_exec,pull $(REPOSITORY):$$TAG)

oci-prune:
	$(call oci_exec,container prune -f)
	$(call oci_exec,image prune -a -f)
	$(call oci_exec,volume prune -f)

list-releases:
	@git tag -l 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+' | head -20
