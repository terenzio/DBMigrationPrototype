# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Oracle-to-MariaDB database migration prototype with a Go backend service. Currently implements an Oracle-backed API as proof-of-concept, with a MariaDB schema ready for a parallel repository layer.

## Build & Run Commands

```bash
# Start everything (Oracle XE + Go backend)
docker-compose up --build

# Stop services
docker-compose down

# Stop and wipe database volume
docker-compose down -v

# View logs
docker-compose logs -f backend
docker-compose logs -f oracle-xe

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

Wiring: `main.go` creates Config → DB connection → Repository → Service → Handler → Router (Gin).

**Key flow:** Gin routes (`internal/router/router.go`) dispatch to handlers, which call services, which call repositories that execute SQL against Oracle XE.

## Database

Two parallel schemas exist for the same 14-table hierarchy (product suites → products → release units → deployment units):

- `db_oracle/schema-oracle.sql` — Active schema, auto-loaded by Docker on first boot
- `db_mariadb/schema-mariadb.sql` — Target migration schema (not yet wired to backend)

Schema differences are documented in `docs/mariadb-vs-oracle.md` (data types, constraint syntax, timestamp handling, cascade behavior).

## Environment Variables

Configured in `backend/internal/config/config.go` with defaults matching docker-compose:

| Variable | Default |
|----------|---------|
| ORACLE_HOST | oracle-xe |
| ORACLE_PORT | 1521 |
| ORACLE_SERVICE | XEPDB1 |
| ORACLE_USER | system |
| ORACLE_PASSWORD | oracle |
| APP_PORT | 8080 |

## API

Base path: `/api/v1`

Currently implemented: `POST /api/v1/product-suites` and `GET /health`. Only the `product_suites_info` table has CRUD wired up — the remaining 13 tables need handler/service/repository layers following the same pattern.

## Docker Setup

The backend Dockerfile (`backend/Dockerfile`) uses a multi-stage build on `oraclelinux:8` because the godror driver requires Oracle Instant Client libraries at both compile and runtime. The docker-compose health check ensures Oracle is ready before the backend starts.
