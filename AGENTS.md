# AGENTS.md

## Build, Lint, and Test Commands

### Go Backend (cmd/api/)

```bash
# Build the Go application
cd cmd/api && go build .

# Format code (required before commit)
cd cmd/api && go fmt .

# Run go vet for static analysis
cd cmd/api && go vet ./...

# Run all checks (fmt, vet, build)
cd cmd/api && go fmt . && go vet ./... && go build .
```

There are no automated tests. Manual verification via API testing commands below.

### Docker

Two deployment modes share a common Go build stage:

#### Development (Docker Compose)
Use `compose.yaml` for local development.

```bash
# Build and start all services
make up

# View logs
make logs

# Stop all services
make down
```

#### Distribution (Single Container)
Use `Dockerfile-dist` for production (Postgres + PostgREST + Nginx + API).

```bash
# Build and run single container
make run
```

**Important:** Changes must work in both modes. Test in single-container mode before committing.

### API Testing

```bash
# Health check
curl http://localhost:8080/health

# Trigger full import
curl -X POST http://localhost:8080/api/imports/create

# Check current import status (returns array)
curl http://localhost:8080/api/imports/current

# List import history
curl http://localhost:8080/api/imports
```

## Architecture

### Services

| Service | Port | Role |
|---------|------|------|
| PostgreSQL | 5432 | Database |
| PostgREST | 3000 | Auto-generated REST API |
| Go API | 8888 | Custom import/control logic |
| Nginx | 8080 | Reverse proxy + static files |

### Key Files

| File | Purpose |
|------|---------|
| `compose.yaml` | Docker Compose config |
| `Dockerfile-dist` | Single container Dockerfile |
| `cmd/api/Dockerfile` | API image for compose (uses builder) |
| `cmd/api/Dockerfile-builder` | Shared Go build stage |
| `Makefile` | Build orchestration |
| `build_multi.sh` | Multi-arch build & push to Hub |
| `nginx.conf.template` | Nginx config with placeholders |
| `config/pg_hba.conf` | PostgreSQL auth config (trust for Docker) |
| `cmd/api/main.go` | Server setup, routes |
| `cmd/api/db.go` | DB connection, retry |
| `cmd/api/handlers.go` | HTTP handlers |
| `cmd/api/importer.go` | Download, extract, COPY logic |
| `cmd/api/types.go` | Structs for JSON/DB |
| `cmd/api/utils.go` | Helpers (null conversions, HTTP errors) |
| `sql/notes_ddl.sql` | note table schema |
| `sql/import_history_ddl.sql` | import_history table schema |

## Code Style Guidelines

### General Principles

- **Simplicity**: Prefer simple, readable code over clever abstractions
- **No comments**: Avoid adding comments unless explaining non-obvious business logic
- **Early returns**: Use early returns to reduce nesting
- **Consistency**: Follow existing patterns in the codebase

### Go Conventions

#### Imports
Standard Go import grouping (enforced by `goimports`):
1. Standard library
2. Third-party packages (`github.com/...`)

#### Naming
- Variables/unexported: `camelCase` (`dbHost`, `jobID`)
- Exported types/functions: `PascalCase` (`HistoryEntry`, `ImportStatus`)
- Files: `snake_case` (`db.go`, `handlers.go`)

#### Types
- Nullable JSON: pointer types (`*string`, `*int`, `*time.Time`)
- DB scanning: `sql.Null*` types + helpers in `utils.go`:
  - `nullStringToStrPtr`, `nullInt64ToIntPtr`, `nullTimeToTimePtr`, etc.

#### Error Handling
- Wrap errors: `fmt.Errorf("failed to ...: %w", err)`
- HTTP errors: use `writeProblem(w, status, title, detail)`
- DB errors from `Exec`: check when result matters; fire-and-forget is acceptable

#### Database
- Parameterized queries (`$1`, `$2`, ...) — never string-format SQL
- Use `context.Background()` for background goroutines; use request `ctx` for handlers

#### Concurrency
- Import jobs: goroutines with `sync.Mutex` for shared counters
- Use `done chan struct{}` to stop background goroutines; close exactly once
- Check `isImportAborted(jobID)` at checkpoints for graceful cancellation

### Frontend (www/)
- Plain HTML + vanilla JS with [AlpineJS](https://alpinejs.dev/)
- No build step, no npm
- State in AlpineJS `x-data` object
- Polling: 2s for active import, 5s for status/health

### Database Initialization
- DDL scripts in `sql/*.sql` are executed on first database init
- Both compose and single-container mount `sql/` to `/docker-entrypoint-initdb.d/`
- `sql/notes_ddl.sql` — note table
- `sql/import_history_ddl.sql` — import_history table

### Docker
- Multi-stage builds for Go; pin versions (`golang:1.26-alpine`, `postgres:17-alpine`)
- In `Dockerfile-dist`, use `127.0.0.1` not `localhost` in nginx proxy_pass
- Use `pg_isready` to wait for Postgres — not `sleep`

### Authentication
- PostgreSQL uses `trust` authentication for internal container communication
- Custom `config/pg_hba.conf` enables trust for Docker networks (172.16.0.0/12, 192.168.0.0/16)
- External connections require password (scram-sha-256)
- Volume `x-notes-db` is shared between compose and single-container deployments

## Notes

- Data volume: `x-notes-db` (shared between both deployment modes)
- Full-text search: `summary_ts` column with PostgREST `wfts.` operator
- Importer looks back up to 7 days for latest data file from Twitter/X
- Downloaded zips cached in `/home/data/` — re-runs skip download if exists
- Import aborted by setting `status = 'failed'` in DB; goroutine polls at checkpoints
