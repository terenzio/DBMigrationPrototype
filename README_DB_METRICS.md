# 🏗️ The Four In-Memory Loads

These are the specific functions and tables that need monitoring to prevent OOM (Out of Memory) issues during migration.

| Load Function | Table Name | Migration Context |
| --- | --- | --- |
| `fetchEAVConfig` | `product_suite_config` | `migrateProductSuites` |
| `fetchEAVConfig` | `product_config` | `migrateProducts` |
| `fetchEAVConfig` | `release_unit_config` | `migrateReleaseUnits` |
| `fetchDeployStatusMap` | `paas_deploy_status` | `migrateDeployUnits` |

---

## 📊 Metrics to Collect: EAV Config Tables

Run this query against Oracle for each of the three config tables to determine the shape of your data.

```sql
SELECT
    COUNT(*)                          AS total_rows,
    COUNT(DISTINCT <fk_col>)          AS distinct_parents,
    ROUND(AVG(cnt))                   AS avg_params_per_parent,
    MAX(cnt)                          AS max_params_per_parent,
    ROUND(AVG(LENGTH(<param_col>)))   AS avg_param_name_bytes,
    MAX(LENGTH(<param_col>))          AS max_param_name_bytes,
    ROUND(AVG(LENGTH(<val_col>)))     AS avg_value_bytes,
    MAX(LENGTH(<val_col>))            AS max_value_bytes
FROM (
    SELECT <fk_col>, COUNT(*) AS cnt
    FROM <table>
    GROUP BY <fk_col>
) t
JOIN <table> USING (<fk_col>);

```

### Go Memory Estimate (EAV)

The approximate memory footprint in Go can be calculated as:

> **Note:** The **32** accounts for Go string headers (16 bytes each for key + value pointer/length).

---

## 📈 Metrics to Collect: `fetchDeployStatusMap`

This load is heavier because `historyJSON` is a serialized array of all status entries per unit.

```sql
SELECT
    COUNT(DISTINCT unit_id)                        AS distinct_units,
    COUNT(*)                                       AS total_rows,
    ROUND(AVG(cnt))                                AS avg_history_entries_per_unit,
    MAX(cnt)                                       AS max_history_entries_per_unit,
    ROUND(AVG(LENGTH(deploy_status)))              AS avg_status_bytes,
    ROUND(AVG(LENGTH(deploy_message)))             AS avg_message_bytes,
    MAX(LENGTH(deploy_message))                    AS max_message_bytes
FROM (
    SELECT unit_id, COUNT(*) AS cnt
    FROM paas_deploy_status
    GROUP BY unit_id
) t
JOIN paas_deploy_status USING (unit_id);

```

### Go Memory Estimate (Deploy Status)

* **JSON Overhead:** 
* **Total Bytes:** 

---

## ⚡ Oracle Data Dictionary Shortcut

If you need a "quick and dirty" ballpark estimate without complex joins, check the average row lengths tracked by Oracle:

```sql
SELECT table_name, num_rows, avg_row_len,
       ROUND(num_rows * avg_row_len / 1024 / 1024, 2) AS estimated_mb
FROM all_tables
WHERE table_name IN (
    'PRODUCT_SUITE_CONFIG', 'PRODUCT_CONFIG',
    'RELEASE_UNIT_CONFIG', 'PAAS_DEPLOY_STATUS'
);

```

### The Rule of Thumb

The Oracle estimate is a **floor**. Because of map metadata, bucket padding, and string headers, the actual Go memory usage is typically **2–3x** the raw data size.

* **Safe:** `estimated_mb × 3` fits comfortably in container limits.
* **Risky:** `estimated_mb × 3` is close to the limit. **Recommendation:** Implement a row-count guard.

---

**Would you like me to draft the Go code for that row-count guard or a utility to log these memory stats during runtime?**