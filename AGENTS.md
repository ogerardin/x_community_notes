# AGENTS.md

## Build, Lint, and Test Commands

### Go Backend (cmd/api/)

```bash
# Build the Go application
cd cmd/api && go build .

# Run the Go application
cd cmd/api && go run .

# Format code (required before commit)
cd cmd/api && go fmt .

# Organize imports (required before commit)
go install golang.org/x/tools/cmd/goimports@latest
~/go/bin/goimports -w .

# Run go vet for static analysis
cd cmd/api && go vet ./...

# Run all checks (fmt, vet, build)
cd cmd/api && go fmt . && go vet ./... && go build .
```

### Docker

This project has two deployment modes:

#### Development & Testing (Docker Compose)
Use `compose.yaml` for local development and testing. This runs each service as a separate container.

```bash
# Build and start all services
docker compose up -d --build

# View logs
docker logs x-notes-api

# Stop all services
docker compose down
```

#### Distribution (Single Container)
Use `Dockerfile-dist` to build a single self-contained image that includes Postgres, PostgREST, Nginx, and the API server.

```bash
# Build and run single container
./build_and_run.sh

# Build multi-architecture images
./build_multi.sh
```

**Important:** When making changes in dev mode, always reflect them in the single-image distribution and test in that mode before committing. The single container is the production distribution format.

### API Testing

```bash
# Health check
curl http://localhost:8080/health

# Trigger full import
curl -X POST http://localhost:8080/api/imports

# Trigger test import (limited rows per file)
curl -X POST "http://localhost:8080/api/imports?limit=1000"

# Check import status
curl http://localhost:8080/api/imports/current

# List import history
curl http://localhost:8080/api/imports
```

## Code Style Guidelines

### General Principles

- **Simplicity**: Prefer simple, readable code over clever abstractions
- **Consistency**: Follow existing patterns in the codebase
- **No Comments**: Avoid adding comments unless explaining complex business logic
- **Early Returns**: Use early returns to reduce nesting

### Go Conventions

#### Imports

- Use the standard Go import organization:
  1. Standard library (`context`, `database/sql`, `fmt`, etc.)
  2. Third-party packages (`github.com/...`)
  3. Local packages (none in this project)
- Always run `goimports` before committing to organize imports

#### Formatting

- Use `gofmt` or `goimports` for automatic formatting
- Keep lines under 100 characters when practical
- Group related constants or variables together

#### Naming

- **Variables**: Use camelCase (`dbHost`, `currentJobID`)
- **Constants**: Use PascalCase for exported, camelCase for unexported
- **Functions**: Use PascalCase for exported, camelCase for unexported
- **Files**: Use lowercase with underscores (`db.go`, `handlers.go`)

#### Types

- Use meaningful type names (`ImportStatus`, `HistoryEntry`)
- Use pointers (`*string`, `*int`) for nullable fields in JSON structs
- Use `sql.Null*` types when scanning from database, convert to pointers with helper functions

#### Error Handling

- Always check and handle errors from `db.QueryRow`, `db.Exec`, etc.
- Return meaningful error messages: `fmt.Errorf("failed to ...: %w", err)`
- Use `writeProblem()` helper for HTTP error responses

#### Concurrency

- Use goroutines for background tasks (e.g., import progress polling)
- Always use proper synchronization (mutexes, channels) for shared data
- Close channels when done to prevent leaks

#### Database

- Use parameterized queries to prevent SQL injection
- Close database resources with `defer`
- Use context (`context.Background()`, `ctx`) for all DB operations

### Frontend (www/)

- Plain HTML/JavaScript with AlpineJS
- Follow existing patterns in `index.html` and `admin.html`
- Use semantic HTML elements

### Docker

- Keep Dockerfiles simple and minimal
- Use multi-stage builds for Go applications
- Pin specific versions in production

## Project Overview

Searchable X/Twitter Community Notes database - a tool for searching and querying Community Notes (formerly Birdwatch) that X/Twitter doesn't provide natively.

## Tech Stack

- **Database**: PostgreSQL
- **API**: PostgREST and custom Go API
- **Reverse Proxy**: Nginx
- **Frontend**: Plain HTML/JS with AlpineJS
- **Container**: Docker & Docker Compose

## Key Files

| File | Purpose |
|------|---------|
| `compose.yaml` | Docker Compose configuration |
| `Dockerfile-dist` | Single container Dockerfile |
| `build_and_run.sh` | Builds and runs single container |
| `build_multi.sh` | Builds/pushes multi-arch Docker images with versioning |
| `nginx.conf` | Nginx reverse proxy configuration |
| `www/index.html` | Web search interface |
| `www/admin.html` | Admin interface for triggering imports |
| `cmd/api/main.go` | Go API entry point, server setup |
| `cmd/api/db.go` | Database connection and notice handler |
| `cmd/api/handlers.go` | HTTP handlers (API endpoints) |
| `cmd/api/importer.go` | Core import logic (download, extract, COPY) |
| `cmd/api/types.go` | Data structures (JSON structs, models) |
| `cmd/api/utils.go` | Helper functions |
| `cmd/api/migrations.go` | Database migration logic |
| `cmd/api/migrations/` | SQL migration files |

## Architecture

- **db**: PostgreSQL database (port 5432)
- **postgrest**: REST API server (port 3000)
- **nginx**: Reverse proxy + web UI (port 8080)
- **adminer**: Optional database admin UI (port 8082)
- **swagger**: Optional API docs (port 8081)

## URLs

- Web UI: http://localhost:8080
- API: http://localhost:8080/api/note
- PostgREST direct: http://localhost:3000

## Notes

- Data is persisted in Docker volume `x-notes-db`
- Full-text search uses `summary_ts` column with PostgREST `wfts.` operator
- Loader fetches from `https://ton.twimg.com/birdwatch-public-data/%Y/%m/%d/notes/` and discovers all available notes-XXXXX.zip files
- Multi-file support: downloads and imports all available files sequentially
- Test mode: use `?limit=N` to limit rows per file during import

## Other Useful URLs

| Service | URL |
|---------|-----|
| PostgREST sample query | http://localhost:3000/note?limit=50&summary_ts.fts.Nigeria&select=summary |
| PostgREST through nginx | http://localhost:8080/api/note?limit=50 |
| Adminer | http://localhost:8082 |
| Swagger UI | http://localhost:8081 |
| PostgreSQL | localhost:5432 |

## TODO

- Schedule the loader to run periodically
