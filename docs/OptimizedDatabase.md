# Schema Optimization: 14 → 8 Tables

## Overview

The original MariaDB schema for the Product / Release Unit / Deployment hierarchy contained 14 tables. Through a series of targeted optimizations — primarily leveraging MariaDB's JSON column support and merging structurally similar entities — the schema was consolidated to 8 tables without losing any functional capability.

---

## Optimization 1: Eliminate EAV Config Tables with JSON Columns

**Tables removed:** `product_suite_config`, `product_config`, `release_unit_config` (−3 tables)

The original schema used three separate Entity-Attribute-Value (EAV) tables to store arbitrary key-value configuration for product suites, products, and release units. Each followed an identical pattern: a foreign key to the parent, a param name column, and a param value column.

These were replaced by a single `config JSON DEFAULT '{}'` column on each parent table (`product_suites_info`, `product_info`, `release_unit_info`). MariaDB 10.2+ provides native JSON functions that make this practical:

- **Read** a param: `JSON_VALUE(config, '$.param_name')`
- **Write** a param: `JSON_SET(config, '$.param_name', 'value')`
- **Delete** a param: `JSON_REMOVE(config, '$.param_name')`

### Why this works well here

- Config data is almost always read and written in the context of its parent entity (e.g., "get the config for product X"), so co-locating it on the parent row eliminates a JOIN.
- EAV tables sacrifice type safety and referential integrity by design; JSON columns have the same trade-off but with simpler query patterns.
- Config rows are typically low cardinality per parent (tens of keys, not thousands), so the JSON document stays small.

### When to reconsider

If the application frequently needs cross-entity config queries (e.g., "find all products where `feature_flag_x = true`"), a generated virtual column with an index on the specific key is recommended. High write-concurrency on different config keys for the same entity will experience row-level lock contention with the JSON approach, whereas the EAV table allowed independent row locks per key.

---

## Optimization 2: Merge Release Grouping Tables

**Tables removed:** `release_product_info`, `release_packages`, `rp_map`, `rp_ru_mapping` (−4 tables)
**Tables added:** `release_group`, `release_group_ru_map` (+2 tables)
**Net effect:** −2 tables

The original schema had two nearly identical concepts:

| Original Table | Purpose |
|---|---|
| `release_product_info` (id, name, description) | A named release product grouping |
| `release_packages` (id, name, description) | A named release package grouping |

Each had its own many-to-many mapping table linking it to release units (`rp_map` and `rp_ru_mapping` respectively). Since both entities share the same structure and the same relationship pattern, they were unified into a single `release_group` table with a `group_type ENUM('product', 'package')` discriminator column, and a single `release_group_ru_map` join table.

### Benefits

- One set of CRUD operations handles both concepts.
- Queries that need "all groups for a release unit" no longer require a UNION across two mapping tables.
- The `group_type` column is indexed for efficient filtering when only one type is needed.

### When to reconsider

If the two concepts diverge in the future (e.g., packages gain versioning columns that products don't need), splitting them back out is straightforward. The ENUM column can be extended with additional types if new grouping concepts emerge.

---

## Optimization 3: Merge Deploy Status into Deploy Unit

**Tables removed:** `paas_deploy_status` (−1 table)

The original `paas_deploy_unit` table contained only a unit ID and a foreign key to a release unit — effectively a thin join table. Its child table `paas_deploy_status` held the actual status and message fields.

These were merged into a single `paas_deploy_unit` table with `deploy_status` and `deploy_message` columns representing the current state, plus an optional `status_history JSON DEFAULT '[]'` column that maintains an ordered audit trail of previous statuses.

### Status history format

```json
[
  { "status": "PENDING",  "message": "Awaiting approval", "at": "2025-06-01T10:00:00" },
  { "status": "DEPLOYED", "message": "Rollout complete",   "at": "2025-06-01T12:30:00" }
]
```

New entries are appended with `JSON_ARRAY_APPEND`, and the current status is always readable directly from the top-level columns without parsing JSON.

### Benefits

- Fetching the current deploy status is a single-table read with no JOIN.
- The full history is still available when needed via the JSON column.
- An index on `deploy_status` supports efficient filtering (e.g., "find all units currently in FAILED state").

### When to reconsider

If status history rows need to be independently queried with complex filters (e.g., "find all units that were in PENDING state for more than 24 hours"), a dedicated history table with proper datetime columns and indexes would be more appropriate than JSON array scanning.

---

## Optimization 4: Add Missing Indexes

**No table changes** — additive improvement only.

The original schema relied solely on primary keys and foreign keys. Several indexes were added to support common access patterns:

| Table | Index | Supports |
|---|---|---|
| `role_map` | `idx_role_map_acct (acct)` | Lookup by account (e.g., "what roles does user X have?") |
| `role_map` | `idx_role_map_prod_role (prod_id, role_name)` | Lookup by product + role combination |
| `product_info` | `idx_product_suite (prod_suite_id)` | FK join optimization |
| `release_unit_info` | `idx_release_unit_prod (prod_id)` | FK join optimization |
| `release_group_ru_map` | `idx_rg_ru_map_ap (ap_id)` | Reverse lookup from release unit to groups |
| `paas_deploy_unit` | `idx_deploy_unit_ap (ap_id)` | FK join optimization |
| `paas_deploy_unit` | `idx_deploy_unit_status (deploy_status)` | Filter by current status |
| `paas_rlse_info` | `idx_paas_rlse_ap (ap_id)` | FK join optimization |

---

## Final Schema Summary

| # | Table | Role |
|---|---|---|
| 1 | `product_suites_info` | Top-level grouping of products (with inline JSON config) |
| 2 | `product_info` | Individual product (with inline JSON config) |
| 3 | `role_map` | User-to-product role assignments |
| 4 | `release_unit_info` | Deployable release unit within a product (with inline JSON config) |
| 5 | `release_group` | Named grouping (product or package) for release units |
| 6 | `release_group_ru_map` | Many-to-many: release groups ↔ release units |
| 7 | `paas_deploy_unit` | Deploy unit with current status and status history |
| 8 | `paas_rlse_info` | PaaS release metadata tied to a release unit |

## Normalization Level

The optimized schema maintains **third normal form (3NF)** for all relational columns. The JSON columns (`config`, `status_history`) are intentionally denormalized for developer ergonomics and read performance, following the same trade-offs that the original EAV tables made but with simpler access patterns.