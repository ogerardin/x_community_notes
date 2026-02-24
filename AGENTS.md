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

# Run all checks (fmt, vet, build) — do this before committing
cd cmd/api && go fmt . && go vet ./... && go build .
```

There are no automated tests. Verification is done manually via the API testing commands below.

### Docker

This project has two deployment modes:

#### Development (Docker Compose)
Use `compose.yaml` for local development. Runs each service as a separate container.

```bash
# Build and start all services
docker compose up -d --build

# View API logs
docker logs x-notes-api

# Stop all services
docker compose down
```

#### Distribution (Single Container)
Use `Dockerfile-dist` to build a single self-contained image (Postgres + PostgREST + Nginx + API).

```bash
# Build and run single container
./build_and_run.sh

# Build multi-architecture images and push
./build_multi.sh
```

**Important:** Changes must work in both modes. Always test in single-container mode before committing — it is the production distribution format.

### API Testing

```bash
# Health check
curl http://localhost:8080/health

# Trigger full import
curl -X POST http://localhost:8080/api/imports/create

# Trigger test import (limited rows per file)
curl -X POST "http://localhost:8080/api/imports/create?limit=1000"

# Check current import status (returns array, use .[0])
curl http://localhost:8080/api/imports/current

# List import history
curl http://localhost:8080/api/imports

# Get specific import by job ID
curl http://localhost:8080/api/imports/{job_id}

# Abort an import
curl -X DELETE http://localhost:8080/api/imports/{job_id}
```

## Architecture

### Services

| Service | Port | Role |
|---------|------|------|
| PostgreSQL | 5432 | Database |
| PostgREST | 3000 | Auto-generated REST API for DB tables |
| Go API | 8888 (internal) | Custom import/control logic |
| Nginx | 8080 | Reverse proxy + serves static files |

### URL Routing (Nginx)

Nginx routes requests between PostgREST and the Go API:

| Path | Backend | Notes |
|------|---------|-------|
| `GET /api/imports/current` | PostgREST | `import_history?order=started_at.desc&limit=1` |
| `GET /api/imports` | PostgREST | `import_history?order=started_at.desc&limit=50` |
| `POST /api/imports/create` | Go API | Triggers new import job |
| `GET /api/imports/{job_id}` | Go API | Get specific job |
| `DELETE /api/imports/{job_id}` | Go API | Abort job |
| `GET /api/*` | PostgREST | All other table queries |

**Important:** `/api/imports/current` returns a JSON **array** (PostgREST convention), not an object. Frontend handles this with `Array.isArray(data) ? data[0] : data`.

### Key Files

| File | Purpose |
|------|---------|
| `compose.yaml` | Docker Compose configuration |
| `Dockerfile-dist` | Single container Dockerfile |
| `nginx.conf` | Nginx config for Docker Compose mode |
| `www/index.html` | Web search interface |
| `www/admin.html` | Admin interface (AlpineJS) |
| `cmd/api/main.go` | Server setup, route registration, startup |
| `cmd/api/db.go` | DB connection, retry logic, PostgREST schema reload |
| `cmd/api/handlers.go` | HTTP handlers |
| `cmd/api/importer.go` | Download, extract, COPY import logic |
| `cmd/api/types.go` | Structs for JSON and DB scanning |
| `cmd/api/utils.go` | Helper functions (null conversions, formatting, HTTP errors) |
| `cmd/api/migrations.go` | golang-migrate runner |
| `cmd/api/migrations/` | SQL migration files (numbered, up only) |
| `sql/notes_ddl.sql` | Initial DB schema (used by single-container Postgres init) |

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

- Variables and unexported functions: `camelCase` (`dbHost`, `jobID`, `totalFiles`)
- Exported types and functions: `PascalCase` (`HistoryEntry`, `ImportStatus`)
- Files: `snake_case` (`db.go`, `handlers.go`)

#### Types

- Nullable JSON fields: use pointer types (`*string`, `*int`, `*int64`, `*time.Time`)
- DB scanning: use `sql.Null*` types, then convert with helpers in `utils.go`:
  - `nullStringToStrPtr`, `nullInt64ToIntPtr`, `nullInt64ToInt64Ptr`, `nullTimeToTimePtr`, `nullBoolToBoolPtr`

#### Error Handling

- Wrap errors: `fmt.Errorf("failed to ...: %w", err)`
- HTTP errors: always use `writeProblem(w, status, title, detail)` — never write raw error strings
- DB errors from `Exec`: check when the result matters; fire-and-forget is acceptable for best-effort progress updates
- Do not treat transient DB errors as business logic failures (e.g. do not abort an import on a DB query error unless it's in the critical path)

#### Database

- Use parameterized queries (`$1`, `$2`, ...) — never string-format SQL values
- Use `context.Background()` for background goroutine DB calls; use the request `ctx` for handler calls
- After running migrations, send `NOTIFY pgrst, 'reload schema'` to trigger PostgREST schema cache refresh

#### Concurrency

- Import jobs run in goroutines; use `sync.Mutex` to protect shared counters
- Use a `done chan struct{}` to stop background polling goroutines; close it exactly once
- Check `isImportAborted(jobID)` at key checkpoints in long-running goroutines to allow graceful cancellation

### Frontend (www/)

- Plain HTML + vanilla JavaScript with [AlpineJS](https://alpinejs.dev/)
- No build step, no npm — keep it simple
- Follow existing patterns in `admin.html` and `index.html`
- All state lives in the AlpineJS `x-data` object
- Polling: use `setInterval` for active import polling (2s); always-on background poll at 5s for status/health

### Migrations

- Migration files are embedded in the binary via `//go:embed migrations`
- Files are numbered sequentially: `002_...up.sql`, `013_...up.sql`
- Only `.up.sql` files are used (no down migrations in practice)
- **Do not renumber or delete existing migration files** — `golang-migrate` tracks applied versions in the DB. Adding new columns/changes requires a new numbered migration file, not editing existing ones.
- The `sql/notes_ddl.sql` file is used only by the single-container Postgres init (`/docker-entrypoint-initdb.d/`) for fresh installs — it must stay in sync with the cumulative result of all migrations.

### Docker

- Keep Dockerfiles minimal; use multi-stage builds for Go
- Pin specific versions (`golang:1.26-alpine`, `postgres:17-alpine`)
- In `Dockerfile-dist`, use `127.0.0.1` not `localhost` in nginx proxy_pass to avoid IPv6 resolution issues
- Use `pg_isready` to wait for Postgres before starting PostgREST — do not use `sleep`

## Notes

- Data volume: `x-notes-db` (compose) / `x-notes-data` (single container)
- Full-text search: `summary_ts` column with PostgREST `wfts.` operator
- Importer looks back up to 7 days to find the latest available data file from Twitter/X
- Downloaded zip files are cached in `/home/data/` — re-runs skip download if file exists
- Import is aborted by setting `status = 'failed'` in the DB; the goroutine polls this at checkpoints
