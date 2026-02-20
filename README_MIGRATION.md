# Oracle → MariaDB Migration

This document covers the one-time ETL script that migrates all data from the Oracle XE source
database into the optimised MariaDB target database. It describes how the script works, how to
run it, how to verify the results, and how to diagnose failures.

---

## Table of Contents

1. [Background](#background)
2. [Schema transformation summary](#schema-transformation-summary)
3. [Prerequisites](#prerequisites)
4. [Environment variables](#environment-variables)
5. [How to run](#how-to-run)
6. [How the script works](#how-the-script-works)
   - [Startup](#startup)
   - [EAV aggregation](#eav-aggregation)
   - [Deploy-status folding](#deploy-status-folding)
   - [Batched processing](#batched-processing)
   - [Checkpoint and resume](#checkpoint-and-resume)
   - [Idempotency via INSERT IGNORE](#idempotency-via-insert-ignore)
   - [Migration order](#migration-order)
7. [The `_migration_log` table](#the-_migration_log-table)
8. [Verifying the results](#verifying-the-results)
9. [Debugging errors](#debugging-errors)

---

## Background

The Oracle source database has **14 tables** following a product suite → product → release unit →
deployment unit hierarchy, including three separate EAV (Entity-Attribute-Value) config tables and
two parallel release-grouping entities.

The MariaDB target has been optimised down to **8 tables** by:

- Collapsing the three EAV config tables into `JSON` columns on their parent tables
- Merging `release_product_info` and `release_packages` into a single `release_group` table
- Merging `rp_map` and `rp_ru_mapping` into a single `release_group_ru_map` table
- Merging `paas_deploy_status` history rows into a `status_history JSON` column on `paas_deploy_unit`

The full rationale for these changes is in [`docs/OptimizedDatabase.md`](docs/OptimizedDatabase.md).

The migration is implemented as a standalone Go binary at
[`backend/cmd/migrate/main.go`](backend/cmd/migrate/main.go). It connects to both databases
simultaneously, reads from Oracle, transforms in Go, and writes to MariaDB.

---

## Schema transformation summary

| Oracle source table(s) | MariaDB target table | Transformation |
|---|---|---|
| `product_suites_info` | `product_suites_info` | Direct column copy |
| `product_suite_config` (EAV) | `product_suites_info.config` | All rows aggregated into a single JSON object keyed by `prod_suite_config_param` |
| `product_info` | `product_info` | Direct column copy |
| `product_config` (EAV) | `product_info.config` | All rows aggregated into a single JSON object keyed by `prod_config_param` |
| `role_map` | `role_map` | Direct column copy |
| `release_unit_info` | `release_unit_info` | Direct column copy |
| `release_unit_config` (EAV) | `release_unit_info.config` | All rows aggregated into a single JSON object keyed by `ap_config_param` |
| `release_product_info` | `release_group` | `rp_id→group_id`, `rp_name→group_name`, `rp_description→group_description`, `group_type='product'` |
| `release_packages` | `release_group` | `package_id→group_id`, `package_name→group_name`, `package_description→group_description`, `group_type='package'` |
| `rp_map` | `release_group_ru_map` | `rp_id→group_id`, `ru_id→ap_id` |
| `rp_ru_mapping` | `release_group_ru_map` | `package_id→group_id`, `ap_id` direct |
| `paas_deploy_unit` | `paas_deploy_unit` | `unit_id`, `ap_id` direct copy; latest `paas_deploy_status` row → `deploy_status` / `deploy_message` |
| `paas_deploy_status` | `paas_deploy_unit.status_history` | All rows ordered by `created_at ASC` → JSON array `[{status, message, at}]` |
| `paas_rlse_info` | `paas_rlse_info` | Direct column copy |

All Oracle `TIMESTAMP` columns are normalised to UTC before being written as MariaDB `DATETIME`
values.

---

## Prerequisites

1. **Both containers must be running** and healthy before the script is executed:

   ```bash
   docker-compose up -d
   docker-compose ps   # oracle-xe and mariadb should show "healthy"
   ```

   Oracle XE can take 2–3 minutes to fully initialise on first boot. The `healthcheck.sh` probe
   inside the container confirms the database is accepting connections before the status changes.

2. **The MariaDB schema must already exist.** The `docker-compose.yml` mounts
   `db_mariaDB/schema-mariadb.sql` as an init script, so the 8 target tables are created
   automatically when the MariaDB container first starts.

3. **Oracle Instant Client libraries** must be available on the machine running the script (they
   are already installed inside the `backend` Docker image, but are also required when running the
   script locally via `go run`). See the `backend/Dockerfile` for the exact package names.

4. **Go 1.22+** must be installed locally if running with `go run`. The binary can alternatively
   be built once and copied into the running container.

---

## Environment variables

All variables have defaults that match the `docker-compose.yml` service configuration.

| Variable | Default | Description |
|---|---|---|
| `ORACLE_HOST` | `localhost` | Oracle hostname or container name |
| `ORACLE_PORT` | `1521` | Oracle listener port |
| `ORACLE_SERVICE` | `XEPDB1` | Oracle pluggable database service name |
| `ORACLE_USER` | `system` | Oracle username |
| `ORACLE_PASSWORD` | `oracle` | Oracle password |
| `MARIADB_HOST` | `localhost` | MariaDB hostname or container name |
| `MARIADB_PORT` | `3306` | MariaDB port |
| `MARIADB_DATABASE` | `migration_db` | Target database name |
| `MARIADB_USER` | `mariadb` | MariaDB username |
| `MARIADB_PASSWORD` | `mariadb` | MariaDB password |
| `BATCH_SIZE` | `1000` | Number of rows fetched from Oracle per round-trip |

Override any variable by setting it in the shell before running the script:

```bash
ORACLE_HOST=oracle-xe BATCH_SIZE=500 go run ./cmd/migrate/main.go
```

---

## How to run

### Via Docker Compose (recommended)

The migration binary is built into the same Docker image as the backend, which already contains
the Oracle Instant Client libraries. The `migrate` service is declared with a
[Compose profile](https://docs.docker.com/compose/profiles/) so it does not start automatically
with `docker-compose up`.

**First, ensure the other services are running and healthy:**

```bash
docker-compose up -d
docker-compose ps   # oracle-xe and mariadb must show "healthy" before proceeding
```

**Then run the migration:**

```bash
docker-compose --profile migrate run --rm migrate
```

`--rm` removes the stopped container afterwards. The service connects to `oracle-xe` and `mariadb`
by their Docker Compose service names, which resolve automatically inside the Docker network.

To override `BATCH_SIZE` or any other variable for this run:

```bash
BATCH_SIZE=500 docker-compose --profile migrate run --rm migrate
```

### Locally (requires Oracle Instant Client on macOS/Linux)

The godror driver links against `libclntsh.dylib` (macOS) or `libclntsh.so` (Linux) at runtime.
Running `go run` locally without those libraries installed produces:

```
ORA-00000: DPI-1047: Cannot locate a 64-bit Oracle Client library
```

If you do have Oracle Instant Client installed locally, set the library path and point the script
at the exposed container ports:

```bash
# macOS — adjust the version path to match your Instant Client installation
export DYLD_LIBRARY_PATH=/usr/local/lib/oracle/23/client64/lib

cd backend
ORACLE_HOST=localhost MARIADB_HOST=localhost go run ./cmd/migrate/main.go
```

For installation instructions see:
https://oracle.github.io/odpi/doc/installation.html#macos

### Expected output

```
[1/8] product_suites_info: loading EAV config...
[1/8] product_suites_info: batch offset=0 rows=1000
[1/8] product_suites_info: batch offset=1000 rows=1000
[1/8] product_suites_info: batch offset=2000 rows=243
[1/8] product_suites_info: done (2243 total rows)
[2/8] product_info: loading EAV config...
[2/8] product_info: batch offset=0 rows=1000
...
[8/8] paas_rlse_info: done (187 total rows)
Migration complete.
```

On a re-run after a completed migration:

```
[1/8] product_suites_info: loading EAV config...
[1/8] product_suites_info: SKIP (already completed)
[2/8] product_info: loading EAV config...
[2/8] product_info: SKIP (already completed)
...
Migration complete.
```

On a re-run after an interrupted migration (e.g. crash at offset 3000):

```
[1/8] product_suites_info: loading EAV config...
[1/8] product_suites_info: SKIP (already completed)
[2/8] product_info: loading EAV config...
[2/8] product_info: resuming from offset 3000
[2/8] product_info: batch offset=3000 rows=1000
...
```

---

## How the script works

### Startup

`main()` performs these steps before any data movement:

1. Opens an Oracle connection using the godror driver (DSN: `user/pass@host:port/service`).
2. Opens a MariaDB connection using the go-sql-driver/mysql driver (DSN:
   `user:pass@tcp(host:port)/db?parseTime=true&loc=UTC`). The `parseTime=true` parameter is
   critical — without it, `DATETIME` columns would be returned as raw `[]byte` instead of
   `time.Time`.
3. Calls `ensureMigrationLog()` which issues a `CREATE TABLE IF NOT EXISTS _migration_log ...`
   on MariaDB. This is idempotent and safe to run on every invocation.
4. Executes the eight `migrate*()` functions in dependency order.

Any connection failure at startup is fatal — `log.Fatalf` prints the error and exits with a
non-zero code.

### EAV aggregation

Three Oracle tables store key-value configuration using the EAV (Entity-Attribute-Value) pattern:
`product_suite_config`, `product_config`, and `release_unit_config`. Each row in these tables
represents one config parameter for one parent entity.

Because the entire EAV table is small relative to the parent table (typically tens of rows per
parent, not millions of rows overall), the script loads each EAV table fully into memory as a
`map[parentID]map[paramName]paramValue` before the batched loop over the parent table begins.

```
Oracle: product_suite_config
  (suite-A, timeout, 30)
  (suite-A, retry, 3)
  (suite-B, timeout, 60)

→ in-memory map:
  "suite-A" → {"timeout": "30", "retry": "3"}
  "suite-B" → {"timeout": "60"}

→ written to MariaDB product_suites_info.config:
  suite-A: {"retry":"3","timeout":"30"}
  suite-B: {"timeout":"60"}
```

NULL values in the Oracle `_val` column are stored as empty strings `""` in the JSON object.
Parent rows with no config entries at all receive `{}` (the MariaDB column default).

The `fetchEAVConfig(db, table, fkCol, paramCol, valCol string)` helper is generic and reused for
all three config tables.

### Deploy-status folding

`paas_deploy_status` in Oracle is a child table of `paas_deploy_unit` where each row records one
status event (status code, message, and timestamp). The MariaDB schema eliminates this separate
table by:

- Storing the **most recent** status row's values in the `deploy_status` and `deploy_message`
  columns on `paas_deploy_unit`.
- Storing the **full ordered history** as a JSON array in the `status_history` column.

Like the EAV tables, `paas_deploy_status` is expected to be small enough to hold entirely in
memory. The `fetchDeployStatusMap()` function queries all rows ordered by `unit_id, created_at ASC`
and builds a `map[unitID]deployStatusData`. For each unit:

- The last element in the ordered slice becomes `latestStatus` / `latestMessage`.
- All elements are serialised to a JSON array of `{status, message, at}` objects where `at` is an
  RFC 3339 UTC timestamp string.

A `paas_deploy_unit` row with no corresponding status rows in `paas_deploy_status` gets NULL for
`deploy_status` / `deploy_message` and `[]` for `status_history`.

Example output for a unit with two status events:

```json
[
  {"status": "PENDING",  "message": "Awaiting approval", "at": "2025-06-01T10:00:00Z"},
  {"status": "DEPLOYED", "message": "Rollout complete",  "at": "2025-06-01T12:30:00Z"}
]
```

### Batched processing

Oracle is queried in pages using the `OFFSET / FETCH NEXT` row-limiting syntax (available in
Oracle 12c+):

```sql
SELECT col1, col2, ...
FROM some_table
ORDER BY primary_key_col
OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY
```

The `:1` and `:2` are Oracle positional bind variables, bound to the current offset and the
configured `BATCH_SIZE`. The `ORDER BY` clause on the primary key is mandatory — without it,
Oracle does not guarantee a stable row order across pages and rows would be missed or duplicated.

The loop terminates when the returned batch is smaller than `BATCH_SIZE`, which indicates the last
page has been reached.

Steps within each batch iteration:

1. Query Oracle for the next page of rows.
2. Scan all rows into a local `[]structType` slice (held in Go heap memory).
3. For each row, look up any pre-loaded EAV config or deploy status data.
4. Execute one `INSERT IGNORE` per row against MariaDB.
5. Update `_migration_log` with the new offset and running total.
6. If `len(batch) < BATCH_SIZE`, break the loop.

### Checkpoint and resume

The `_migration_log` table in MariaDB tracks progress per logical source table. The `last_offset`
column records the number of rows that have been **successfully committed** to MariaDB. On startup,
`checkLog()` reads this value and the loop starts from that offset rather than zero.

This means:

- If the script is interrupted (Ctrl+C, OOM kill, network drop) after committing offset 5000, the
  next run starts reading from Oracle at `OFFSET 5000 ROWS` and continues from there.
- At most one batch of up to `BATCH_SIZE` rows could be re-processed on resume (the batch that was
  in flight when the crash occurred). The `INSERT IGNORE` strategy (see below) makes this safe.

The two tables that combine multiple Oracle sources (`release_group` and `release_group_ru_map`)
use separate log entries per Oracle source table — `release_product_info` and `release_packages`
are tracked independently, as are `rp_map` and `rp_ru_mapping`. This means a crash mid-way through
loading `release_packages` will resume from the right place without re-running the already-completed
`release_product_info` pass.

### Idempotency via INSERT IGNORE

Every MariaDB insert uses `INSERT IGNORE INTO ...`. If a row with the same primary key already
exists in the target table, MariaDB silently skips that row rather than returning an error. This
provides two guarantees:

1. **Safe re-run of a partially committed batch:** rows inserted before the crash are skipped;
   only genuinely missing rows are inserted.
2. **Safe full re-run:** delete the `_migration_log` table (or reset specific rows to
   `status='in_progress', last_offset=0`) and the script will re-run from scratch without
   duplicating data already in the target tables. FK violations in child tables can also be avoided
   this way.

### Migration order

The eight `migrate*()` functions are called in FK dependency order so that parent rows always exist
before child rows reference them:

| Step | Function | Target table | FK dependency |
|---|---|---|---|
| 1 | `migrateProductSuites` | `product_suites_info` | None (root) |
| 2 | `migrateProducts` | `product_info` | `product_suites_info` |
| 3 | `migrateRoleMap` | `role_map` | `product_info` |
| 4 | `migrateReleaseUnits` | `release_unit_info` | `product_info` |
| 5 | `migrateReleaseGroups` | `release_group` | None (no FK to other tables) |
| 6 | `migrateReleaseGroupRUMap` | `release_group_ru_map` | `release_group`, `release_unit_info` |
| 7 | `migrateDeployUnits` | `paas_deploy_unit` | `release_unit_info` |
| 8 | `migratePaasRlseInfo` | `paas_rlse_info` | `release_unit_info` |

---

## The `_migration_log` table

The script auto-creates this table in MariaDB on every run (the `IF NOT EXISTS` guard makes it
safe to call repeatedly):

```sql
CREATE TABLE IF NOT EXISTS _migration_log (
    table_name    VARCHAR(64)  NOT NULL,
    status        ENUM('in_progress', 'completed') NOT NULL DEFAULT 'in_progress',
    rows_migrated INT          NOT NULL DEFAULT 0,
    last_offset   INT          NOT NULL DEFAULT 0,
    started_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (table_name)
) ENGINE=InnoDB;
```

Each row tracks one logical source table:

| `table_name` | Corresponds to |
|---|---|
| `product_suites_info` | Oracle `product_suites_info` |
| `product_info` | Oracle `product_info` |
| `role_map` | Oracle `role_map` |
| `release_unit_info` | Oracle `release_unit_info` |
| `release_product_info` | Oracle `release_product_info` (feeds `release_group`) |
| `release_packages` | Oracle `release_packages` (feeds `release_group`) |
| `rp_map` | Oracle `rp_map` (feeds `release_group_ru_map`) |
| `rp_ru_mapping` | Oracle `rp_ru_mapping` (feeds `release_group_ru_map`) |
| `paas_deploy_unit` | Oracle `paas_deploy_unit` |
| `paas_rlse_info` | Oracle `paas_rlse_info` |

Inspect the log at any time:

```bash
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT table_name, status, rows_migrated, last_offset, updated_at
      FROM _migration_log ORDER BY updated_at;"
```

### Resetting a single table for re-migration

If a specific table needs to be re-migrated (e.g. the Oracle source was corrected after an initial
run), reset its log entry and truncate the target:

```bash
# Reset the log entry
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "UPDATE _migration_log
      SET status='in_progress', rows_migrated=0, last_offset=0
      WHERE table_name='product_info';"

# Truncate the target table (disable FK checks temporarily)
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SET FOREIGN_KEY_CHECKS=0; TRUNCATE TABLE product_info; SET FOREIGN_KEY_CHECKS=1;"
```

Then re-run the script — all completed steps are skipped and only the reset table is re-processed.

### Starting completely from scratch

```bash
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "DROP TABLE IF EXISTS _migration_log;"

docker exec mariadb mariadb -u mariadb -pmariadb migration_db <<'SQL'
SET FOREIGN_KEY_CHECKS=0;
TRUNCATE TABLE paas_rlse_info;
TRUNCATE TABLE paas_deploy_unit;
TRUNCATE TABLE release_group_ru_map;
TRUNCATE TABLE release_group;
TRUNCATE TABLE release_unit_info;
TRUNCATE TABLE role_map;
TRUNCATE TABLE product_info;
TRUNCATE TABLE product_suites_info;
SET FOREIGN_KEY_CHECKS=1;
SQL
```

---

## Verifying the results

### 1. Check migration log status

```bash
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT table_name, status, rows_migrated FROM _migration_log;"
```

All rows should show `status = completed`.

### 2. Compare row counts against Oracle

```bash
# Oracle counts
docker exec oracle-xe sqlplus -s system/oracle@XEPDB1 <<'SQL'
SELECT 'product_suites_info', COUNT(*) FROM product_suites_info
UNION ALL SELECT 'product_info', COUNT(*) FROM product_info
UNION ALL SELECT 'role_map', COUNT(*) FROM role_map
UNION ALL SELECT 'release_unit_info', COUNT(*) FROM release_unit_info
UNION ALL SELECT 'release_product_info', COUNT(*) FROM release_product_info
UNION ALL SELECT 'release_packages', COUNT(*) FROM release_packages
UNION ALL SELECT 'rp_map', COUNT(*) FROM rp_map
UNION ALL SELECT 'rp_ru_mapping', COUNT(*) FROM rp_ru_mapping
UNION ALL SELECT 'paas_deploy_unit', COUNT(*) FROM paas_deploy_unit
UNION ALL SELECT 'paas_rlse_info', COUNT(*) FROM paas_rlse_info;
SQL

# MariaDB counts
docker exec mariadb mariadb -u mariadb -pmariadb migration_db -e "
SELECT 'product_suites_info', COUNT(*) FROM product_suites_info
UNION ALL SELECT 'product_info',         COUNT(*) FROM product_info
UNION ALL SELECT 'role_map',             COUNT(*) FROM role_map
UNION ALL SELECT 'release_unit_info',    COUNT(*) FROM release_unit_info
UNION ALL SELECT 'release_group',        COUNT(*) FROM release_group
UNION ALL SELECT 'release_group_ru_map', COUNT(*) FROM release_group_ru_map
UNION ALL SELECT 'paas_deploy_unit',     COUNT(*) FROM paas_deploy_unit
UNION ALL SELECT 'paas_rlse_info',       COUNT(*) FROM paas_rlse_info;"
```

Expected: the `release_group` count equals `release_product_info` count + `release_packages`
count, and `release_group_ru_map` count equals `rp_map` count + `rp_ru_mapping` count.

### 3. Spot-check JSON config columns

```bash
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT prod_suite_id, config FROM product_suites_info LIMIT 3\G"

docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT prod_id, config FROM product_info LIMIT 3\G"

docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT ap_id, config FROM release_unit_info LIMIT 3\G"
```

Config should be a non-empty JSON object (e.g. `{"param1":"val1","param2":"val2"}`) for rows that
had EAV entries in Oracle, and `{}` for rows that had none.

Cross-check a specific row:

```bash
# Oracle: count config params for a known suite
docker exec oracle-xe sqlplus -s system/oracle@XEPDB1 \
  -e "SELECT COUNT(*) FROM product_suite_config WHERE prod_suite_id = 'YOUR-ID';"

# MariaDB: count keys in the JSON config for the same suite
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT JSON_LENGTH(config) FROM product_suites_info WHERE prod_suite_id = 'YOUR-ID';"
```

Both counts should match.

### 4. Spot-check deploy status and history

```bash
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT unit_id, deploy_status, deploy_message, status_history FROM paas_deploy_unit LIMIT 3\G"
```

- `deploy_status` and `deploy_message` should match the most recent row for that `unit_id` in
  Oracle's `paas_deploy_status` table.
- `status_history` should be a JSON array. Its length should equal the total number of
  `paas_deploy_status` rows for that `unit_id` in Oracle.

Cross-check:

```bash
# Oracle: how many status events does a unit have?
docker exec oracle-xe sqlplus -s system/oracle@XEPDB1 \
  -e "SELECT COUNT(*) FROM paas_deploy_status WHERE unit_id = 'YOUR-UNIT-ID';"

# MariaDB: how many entries are in the history array?
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT JSON_LENGTH(status_history) FROM paas_deploy_unit WHERE unit_id = 'YOUR-UNIT-ID';"
```

### 5. Check release_group type split

```bash
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SELECT group_type, COUNT(*) FROM release_group GROUP BY group_type;"
```

The `product` count must equal Oracle's `SELECT COUNT(*) FROM release_product_info` and the
`package` count must equal Oracle's `SELECT COUNT(*) FROM release_packages`.

---

## Debugging errors

### Oracle Instant Client not found (macOS / Linux)

**Symptom:**
```
Oracle connection failed: ping oracle: ORA-00000: DPI-1047: Cannot locate a 64-bit Oracle Client library:
"dlopen(libclntsh.dylib, 0x0001): tried: 'libclntsh.dylib' (no such file) ..."
```

**Cause:** The godror driver requires Oracle Instant Client libraries to be present on the machine
running the binary. They are included in the Docker image but are not installed on your Mac by default.

**Fix:** Use the Docker Compose approach instead of running `go run` locally:

```bash
docker-compose --profile migrate run --rm migrate
```

This runs the binary inside the container where the libraries are already installed.

---

### Connection failures at startup

**Symptom:**
```
Oracle connection failed: ping oracle: ...
```
or
```
MariaDB connection failed: ping mariadb: ...
```

**Checks:**
- Confirm both containers are running and healthy: `docker-compose ps`
- Oracle XE takes 2–3 minutes on first boot. Watch its logs: `docker-compose logs -f oracle-xe`
- Verify port exposure: `docker port oracle-xe 1521` and `docker port mariadb 3306`
- Test connectivity manually:
  ```bash
  # MariaDB
  docker exec mariadb mariadb -u mariadb -pmariadb migration_db -e "SELECT 1;"

  # Oracle
  docker exec oracle-xe sqlplus system/oracle@XEPDB1 <<< "SELECT 1 FROM dual;"
  ```
- Check that `ORACLE_HOST` and `MARIADB_HOST` match the container names or exposed addresses.
  When running the script via `docker-compose run migrate`, both default to the service names
  (`oracle-xe`, `mariadb`) which resolve inside the Docker network.
  When running the script locally against exposed ports, override both to `localhost`.

---

### `ORA-00942: table or view does not exist`

**Symptom:**
```
Step N failed: product_suites_info query: ORA-00942: table or view does not exist
```

**Cause:** The Oracle schema has not been initialised, or the script is connecting to the wrong
schema/service.

**Checks:**
- Verify that `db_oracle/schema-oracle.sql` was loaded:
  ```bash
  docker exec oracle-xe sqlplus system/oracle@XEPDB1 \
    <<< "SELECT table_name FROM user_tables ORDER BY table_name;"
  ```
  All 14 tables should appear.
- If the table list is empty, the init script did not run. Destroy and recreate the volume:
  ```bash
  docker-compose down -v
  docker-compose up -d
  ```

---

### `Error 1452: Cannot add or update a child row: a foreign key constraint fails`

**Symptom:**
```
Step 3 failed: role_map insert: Error 1452 (23000): Cannot add or update a child row: ...
```

**Cause:** A child row references a parent ID that does not yet exist in MariaDB. This should not
happen if the script runs in the correct order, but can occur if:

- The target MariaDB tables already contain partial data from a previous incomplete run that was
  not properly reset.
- Oracle source data itself has orphaned rows (referential integrity was not enforced in Oracle).

**Fix:**
1. Check whether Oracle has orphaned rows:
   ```sql
   -- Example: role_map rows referencing non-existent products
   SELECT r.id, r.prod_id FROM role_map r
   WHERE NOT EXISTS (SELECT 1 FROM product_info p WHERE p.prod_id = r.prod_id);
   ```
2. If the orphaned rows are a data quality issue, remove them from Oracle before re-running.
3. If the target MariaDB tables have stale partial data, truncate them (see
   [Starting completely from scratch](#starting-completely-from-scratch)).

---

### `Error 1062: Duplicate entry` (despite INSERT IGNORE)

This error cannot be raised by the migration script itself — `INSERT IGNORE` suppresses duplicate
PK violations silently. If you see error 1062, it is coming from a different process writing to
the same MariaDB instance concurrently.

---

### EAV config looks empty or missing keys

**Symptom:** A row's `config` column in MariaDB is `{}` but the corresponding Oracle parent had
config entries in the EAV table.

**Checks:**
1. Confirm EAV rows exist in Oracle:
   ```sql
   SELECT * FROM product_suite_config WHERE prod_suite_id = 'YOUR-ID';
   ```
2. Check whether the EAV query failed silently. Look for error messages early in the script output
   — an EAV fetch failure causes `log.Fatalf` and terminates the run, so the table would not show
   `completed` in `_migration_log`.
3. Confirm the `prod_suite_id` values in `product_suite_config` match the `prod_suite_id` values
   in `product_suites_info` exactly (case-sensitive string comparison). Orphaned EAV rows with no
   matching parent are loaded into the map but never written anywhere — this is intentional.

---

### Deploy status history is `[]` for units that should have history

**Symptom:** `status_history` is `[]` and `deploy_status` is NULL, but `paas_deploy_status` rows
exist in Oracle for that unit.

**Checks:**
1. Verify the `unit_id` values match exactly:
   ```sql
   -- Oracle
   SELECT DISTINCT unit_id FROM paas_deploy_status WHERE unit_id = 'YOUR-UNIT-ID';
   ```
2. Confirm `fetchDeployStatusMap` did not fail. As with EAV, a failure here is fatal and logged
   to stderr before the script exits.

---

### Script appears to hang with no output

**Cause:** The Oracle `OFFSET / FETCH NEXT` query is executing but taking a long time, usually
because there is no index on the `ORDER BY` column for a large table, or Oracle XE is under memory
pressure.

**Checks:**
- Reduce `BATCH_SIZE` to `100` to see if smaller pages proceed faster.
- Check Oracle XE container resource usage: `docker stats oracle-xe`
- Oracle XE has a 2 GB RAM cap. If it is swapping, performance degrades severely. Ensure Docker
  Desktop has at least 4 GB RAM allocated.

---

### `offset / FETCH NEXT` not supported

**Symptom:**
```
ORA-00933: SQL command not properly ended
```
or
```
ORA-00907: missing right parenthesis
```

**Cause:** The `OFFSET n ROWS FETCH NEXT m ROWS ONLY` syntax requires Oracle 12c or later. The
`docker-compose.yml` uses `gvenzl/oracle-xe:21-slim` (Oracle 21c XE), so this should not occur
with the standard setup. It would only happen if the image is replaced with an older Oracle version.

---

### Re-running a specific step manually

To force a single step to re-run without affecting others, reset only that step's log entry:

```bash
# Example: re-run release_unit_info
docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "UPDATE _migration_log
      SET status='in_progress', rows_migrated=0, last_offset=0
      WHERE table_name='release_unit_info';"

docker exec mariadb mariadb -u mariadb -pmariadb migration_db \
  -e "SET FOREIGN_KEY_CHECKS=0; TRUNCATE TABLE release_unit_info; SET FOREIGN_KEY_CHECKS=1;"
```

Note that truncating `release_unit_info` will cascade-delete rows in `release_group_ru_map`,
`paas_deploy_unit`, and `paas_rlse_info` (all of which have `ON DELETE CASCADE` FKs pointing to
it). Reset and truncate those tables' log entries too before re-running the script.
