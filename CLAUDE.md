# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Oracle-to-MariaDB database migration prototype with a Go backend service. Currently implements an Oracle-backed API as proof-of-concept, with a MariaDB service running in Docker as the migration target.

## Build & Run Commands

```bash
# Start everything (Oracle XE + MariaDB + Go backend) — --build compiles both server and migrate binaries
docker-compose up --build

# Run the ETL migration (profile-gated; requires oracle-xe and mariadb to be healthy)
docker-compose --profile migrate run --rm migrate

# Stop services
docker-compose down

# Stop and wipe database volumes
docker-compose down -v

# View logs
docker-compose logs -f backend
docker-compose logs -f oracle-xe
docker-compose logs -f mariadb

# Health check
curl http://localhost:8080/health
```

There are no tests currently. The Go module is at `backend/go.mod`. To work with Go dependencies:

```bash
cd backend && go mod tidy
```

## Architecture

The backend (`backend/`) follows a 3-layer pattern:

- **Handler** (`internal/handler/`) — HTTP request binding, validation, JSON responses
- **Service** (`internal/service/`) — Business logic, UUID generation
- **Repository** (`internal/repository/`) — SQL execution via godror (Oracle driver)

Wiring: `main.go` creates Config → DB connection → Repository → Service → Handler → Server (Gin).

**Key flow:** Gin routes (`internal/server/server.go`) dispatch to handlers, which call services, which call repositories that execute SQL against Oracle XE.

**Migration binary:** `backend/cmd/migrate/main.go` is a standalone ETL binary (separate from the API server) that reads from Oracle and writes to MariaDB.

## Database

Two parallel schemas exist for the same 14-table hierarchy (product suites → products → release units → deployment units):

- `db_oracle/schema-oracle.sql` — Active schema, auto-loaded by Docker on first boot
- `db_mariaDB/schema-mariadb.sql` — Migration target schema, auto-loaded into the live MariaDB Docker service

Schema differences are documented in `docs/mariadb-vs-oracle.md` (data types, constraint syntax, timestamp handling, cascade behavior).

See `README_MIGRATION.md` for full migration documentation.

## Environment Variables

Configured in `backend/internal/config/config.go` with defaults matching docker-compose:

| Variable | Default | Used by |
|----------|---------|---------|
| ORACLE_HOST | oracle-xe | server, migrate |
| ORACLE_PORT | 1521 | server, migrate |
| ORACLE_SERVICE | XEPDB1 | server, migrate |
| ORACLE_USER | system | server, migrate |
| ORACLE_PASSWORD | oracle | server, migrate |
| APP_PORT | 8080 | server |
| MARIADB_HOST | mariadb | migrate |
| MARIADB_PORT | 3306 | migrate |
| MARIADB_DATABASE | migration_db | migrate |
| MARIADB_USER | mariadb | migrate |
| MARIADB_PASSWORD | mariadb | migrate |
| BATCH_SIZE | 1000 | migrate |

## API

Base path: `/api/v1`

Currently implemented: `POST /api/v1/product-suites` and `GET /health`. Only the `product_suites_info` table has CRUD wired up — the remaining 13 tables need handler/service/repository layers following the same pattern.

## Migration

The ETL binary at `backend/cmd/migrate/main.go` migrates data from Oracle XE to MariaDB.

**Run:**
```bash
docker-compose --profile migrate run --rm migrate
```

**Key behaviours:**
- Batched Oracle reads (controlled by `BATCH_SIZE`)
- EAV → JSON aggregation for flexible attribute storage
- Deploy-status folding into structured columns
- Checkpoint/resume via `_migration_log` table — reruns are safe
- `INSERT IGNORE` idempotency — duplicate rows are skipped, not errored

See `README_MIGRATION.md` for full detail on the migration workflow, table mapping, and troubleshooting.

## Docker Setup

The backend Dockerfile (`backend/Dockerfile`) uses a multi-stage build on `oraclelinux:8` because the godror driver requires Oracle Instant Client libraries at both compile and runtime. The build stage compiles **two** binaries: `server` (the API) and `migrate` (the ETL tool).

docker-compose defines four services:

| Service | Description |
|---------|-------------|
| `oracle-xe` | Oracle XE 21 slim; schema and seed data auto-loaded on first boot |
| `mariadb` | MariaDB 11; target schema auto-loaded on first boot; healthcheck uses `mariadb -uroot -pmariadb -e "SELECT 1"` |
| `backend` | Go API server; starts after `oracle-xe` is healthy |
| `migrate` | ETL binary; profile-gated (`--profile migrate`), runs once and exits; starts after both databases are healthy |
