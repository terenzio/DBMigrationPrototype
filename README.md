# DBMigrationPrototype

A prototype project demonstrating an Oracle-to-MariaDB database migration strategy with a Go backend service using the GIN framework.

## Project Structure

```text
DBMigrationPrototype/
├── docker-compose.yml              # Orchestrates Oracle XE + MariaDB + Go backend
├── backend/                        # Go service (GIN framework)
│   ├── Dockerfile                  # Multi-stage build with Oracle Instant Client
│   ├── go.mod / go.sum
│   ├── main.go                     # Entry point and dependency wiring
│   └── internal/
│       ├── config/config.go        # Environment variable configuration
│       ├── model/product_suite.go  # Domain model and request DTOs
│       ├── handler/                # HTTP handlers (GIN)
│       ├── service/                # Business logic
│       ├── repository/             # Data access (Oracle via godror)
│       └── router/router.go       # Route definitions
├── db_oracle/
│   ├── schema-oracle.sql          # Oracle DDL (auto-loaded by Docker)
│   └── seed-data.sql              # Sample data (2 rows per table, auto-loaded)
├── db_mariaDB/
│   └── schema-mariadb.sql         # MariaDB DDL
└── docs/
    ├── erDiagram.md               # Entity-relationship diagram (Mermaid)
    ├── mariadb-vs-oracle.md       # Schema comparison + Go repo layer guide
    └── Improvements.md
```

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
- ~3 GB of free disk space (Oracle XE image is ~2 GB)
- No local Go or Oracle installation required — everything runs in containers

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/terence/DBMigrationPrototype.git
cd DBMigrationPrototype
```

### 2. Start the services

```bash
docker-compose up --build
```

This will:

- Pull the `gvenzl/oracle-xe:21-slim` and `mariadb:11` images (first run only)
- Initialize the Oracle XE database and execute `db_oracle/schema-oracle.sql` to create all tables
- Populate all 14 Oracle tables with sample data via `db_oracle/seed-data.sql` (2 rows per table)
- Initialize the MariaDB database and execute `db_mariaDB/schema-mariadb.sql` to create all 14 empty tables
- Build the Go backend with Oracle Instant Client
- Start the backend on port `8080` once Oracle is healthy

> **Note:** Oracle XE takes 60–90 seconds to initialize on the first run. The Go backend will wait until the database health check passes before starting.

### 3. Verify the service is running

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status": "healthy"}
```

## API Usage

### Create a Product Suite

**POST** `/api/v1/product-suites`

```bash
curl -X POST http://localhost:8080/api/v1/product-suites \
  -H "Content-Type: application/json" \
  -d '{
    "prod_suite_name": "My Product Suite",
    "prod_suite_owner_nt_acct": "jdoe",
    "prod_suite_site_owner_acct": "jdoe_site",
    "division": "Engineering"
  }'
```

**Request body:**

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `prod_suite_name` | string | Yes | Name of the product suite |
| `prod_suite_owner_nt_acct` | string | No | Owner NT account |
| `prod_suite_site_owner_acct` | string | No | Site owner account |
| `division` | string | No | Division name |

**Response (201 Created):**

```json
{
  "prod_suite_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "prod_suite_name": "My Product Suite",
  "prod_suite_owner_nt_acct": "jdoe",
  "prod_suite_site_owner_acct": "jdoe_site",
  "division": "Engineering",
  "created_at": "0001-01-01T00:00:00Z",
  "updated_at": "0001-01-01T00:00:00Z"
}
```

The `prod_suite_id` is a UUID generated automatically by the service. The `created_at` and `updated_at` timestamps are set by Oracle via `DEFAULT SYSTIMESTAMP`.

## Architecture

The backend follows a three-layer architecture:

```text
HTTP Request
    │
    ▼
┌──────────┐     Binds JSON, validates input, returns HTTP response
│  Handler │
└────┬─────┘
     │
     ▼
┌──────────┐     Generates UUID, maps DTO to domain model
│  Service │
└────┬─────┘
     │
     ▼
┌──────────┐     Executes SQL against Oracle via godror driver
│Repository│
└────┬─────┘
     │
     ▼
  Oracle XE
```

## Database Connections

### Oracle XE

| Field    | Value     |
| -------- | --------- |
| Host     | `localhost` |
| Port     | `1521`    |
| Service  | `XEPDB1`  |
| Username | `system`  |
| Password | `oracle`  |

### MariaDB

| Field    | Value          |
| -------- | -------------- |
| Host     | `localhost`    |
| Port     | `3306`         |
| Database | `migration_db` |
| Username | `mariadb`      |
| Password | `mariadb`      |

> Root password for MariaDB is also `mariadb`.

## Environment Variables

The backend reads these environment variables (all have defaults for Docker Compose):

| Variable | Default | Description |
| --- | --- | --- |
| `ORACLE_HOST` | `oracle-xe` | Oracle database hostname |
| `ORACLE_PORT` | `1521` | Oracle listener port |
| `ORACLE_SERVICE` | `XEPDB1` | Oracle pluggable database service name |
| `ORACLE_USER` | `system` | Database user |
| `ORACLE_PASSWORD` | `oracle` | Database password |
| `APP_PORT` | `8080` | Port the Go server listens on |

## Common Operations

### Stop the services

```bash
docker-compose down
```

### Stop and reset the databases (wipe all data)

```bash
docker-compose down -v
```

The `-v` flag removes the `oracle-data` and `mariadb-data` volumes. On the next `docker-compose up`, both databases will reinitialize and re-run their schema SQL.

### Rebuild after code changes

```bash
docker-compose up --build
```

### View logs

```bash
# All services
docker-compose logs -f

# Backend only
docker-compose logs -f backend

# Oracle only
docker-compose logs -f oracle-xe

# MariaDB only
docker-compose logs -f mariadb
```

## Documentation

- [MariaDB vs Oracle Schema Differences](docs/mariadb-vs-oracle.md) — detailed comparison of both schemas and Go repository layer examples
- [ER Diagram](docs/erDiagram.md) — entity-relationship diagram for the full schema
- [Improvements](docs/Improvements.md) — planned schema improvements
