# AGENTS.md

## Project Overview

Searchable X/Twitter Community Notes database - a tool for searching and querying Community Notes (formerly Birdwatch) that X/Twitter doesn't provide natively.

## Tech Stack

- **Database**: PostgreSQL
- **API**: PostgREST
- **Reverse Proxy**: Nginx
- **Frontend**: Plain HTML/JS with AlpineJS
- **Container**: Docker & Docker Compose

## Commands

```bash
# Start the stack (Docker Compose method)
docker compose up -d

# Build and run single container
./build_and_run.sh

# Build multi-architecture images
./build_multi.sh

# Trigger full import
curl -X POST http://localhost:8080/api/import/trigger

# Trigger test import (limited rows per file)
curl -X POST "http://localhost:8080/api/import/trigger?limit=1000"
```

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
| `cmd/api/main.go` | Go API server (import logic) |
| `cmd/api/migrations/` | Database migrations |

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
