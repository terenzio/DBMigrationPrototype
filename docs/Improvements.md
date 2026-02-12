# Schema Improvements Summary

## Overview

This document summarizes the improvements made to the product/deployment hierarchy schema during the Oracle to MariaDB migration.

## Changes

### 1. Removed Denormalized Columns from PRODUCT_INFO

**Problem:** `PROD_SUITE_NAME` and `PROD_SUITE_OWNER_NT_ACCT` were duplicated from `PRODUCT_SUITES_INFO`. Since `PROD_SUITE_ID` (FK) already existed, these columns were redundant and risked data inconsistency.

**Fix:** Removed both columns. The values should be resolved via JOIN on `PROD_SUITE_ID`.

### 2. Added Missing RELEASE_PRODUCT_INFO Table

**Problem:** `RP_MAP` referenced an `RP_ID` foreign key, but no corresponding table existed in the schema. This was an orphaned reference.

**Fix:** Added `RELEASE_PRODUCT_INFO` table with `RP_ID` (PK), `RP_NAME`, and `RP_DESCRIPTION`.

### 3. Fixed RP_MAP Relationship Direction

**Problem:** The original diagram defined `RP_MAP ||--o{ RELEASE_UNIT_INFO`, implying one `RP_MAP` row has many Release Units. Since `RP_MAP` is a junction table with a composite PK of `(RU_ID, RP_ID)`, the relationship was backwards.

**Fix:** Changed to `RELEASE_UNIT_INFO ||--o{ RP_MAP` and `RELEASE_PRODUCT_INFO ||--o{ RP_MAP`, correctly showing both parent tables feeding into the junction table.

### 4. Added Audit Columns

**Problem:** No tables had timestamp columns for tracking when records were created or modified.

**Fix:** Added `CREATED_AT` and `UPDATED_AT` columns to every table. In the MariaDB DDL, these use `DEFAULT CURRENT_TIMESTAMP` and `ON UPDATE CURRENT_TIMESTAMP` for automatic management.

## MariaDB Migration Notes

| Topic | Detail |
|---|---|
| Engine | All tables use `InnoDB` for foreign key support |
| Charset | `utf8mb4` with `utf8mb4_unicode_ci` collation for full Unicode support |
| Primary keys | Kept as `VARCHAR` business keys per design decision |
| Sequences | Not needed — MariaDB uses `AUTO_INCREMENT` (unused here since PKs are VARCHAR) |
| FK constraints | Explicitly defined with `ON DELETE CASCADE ON UPDATE CASCADE` |
| Table creation order | Parents before children to satisfy FK dependencies |
| NULL vs empty string | Oracle treats empty string as NULL; MariaDB does not — keep this in mind during data migration |

## Files Modified

- `docs/claude.md` — Updated Mermaid ER diagram with all improvements
- `docs/schema-mariadb.sql` — New MariaDB-compatible DDL script
