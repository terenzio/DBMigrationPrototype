# MariaDB vs Oracle Schema Differences

This document details the specific differences between the MariaDB schema (`db_mariaDB/schema-mariadb.sql`) and the Oracle schema (`db_oracle/schema-oracle.sql`), and provides guidance on building a Go repository layer for each.

---

## 1. Schema Differences

### 1.1 Data Types

| Feature | MariaDB | Oracle |
|---|---|---|
| String columns | `VARCHAR(n)` | `VARCHAR2(n)` |
| Timestamps | `DATETIME` | `TIMESTAMP` |

`VARCHAR` is valid in Oracle but `VARCHAR2` is the recommended and industry-standard type. Oracle's `TIMESTAMP` provides fractional-second precision that `DATETIME` does not.

### 1.2 Timestamp Defaults

| Feature | MariaDB | Oracle |
|---|---|---|
| Default value | `DEFAULT CURRENT_TIMESTAMP` | `DEFAULT SYSTIMESTAMP` |
| Auto-update on row change | `ON UPDATE CURRENT_TIMESTAMP` | Not supported natively |

MariaDB's `ON UPDATE CURRENT_TIMESTAMP` clause automatically refreshes `updated_at` whenever any column in the row is modified. Oracle has no equivalent DDL clause. To replicate this behaviour in Oracle you must use a **trigger**:

```sql
CREATE OR REPLACE TRIGGER trg_product_info_updated_at
    BEFORE UPDATE ON product_info
    FOR EACH ROW
BEGIN
    :NEW.updated_at := SYSTIMESTAMP;
END;
/
```

This trigger would need to be created for **every table** that requires automatic `updated_at` tracking. Alternatively, the application layer (Go code) can set `updated_at` explicitly on every UPDATE call, which avoids the trigger overhead entirely.

### 1.3 Foreign Key Behaviour

| Feature | MariaDB | Oracle |
|---|---|---|
| `ON DELETE CASCADE` | Supported | Supported |
| `ON UPDATE CASCADE` | Supported | **Not supported** |

All foreign keys in the MariaDB schema use `ON DELETE CASCADE ON UPDATE CASCADE`. Oracle only supports `ON DELETE CASCADE` (and `ON DELETE SET NULL`). There is no `ON UPDATE CASCADE` in Oracle.

**Impact:** If a primary key value is updated in a parent table in MariaDB, all child rows automatically update their foreign key values. In Oracle, primary key values in parent tables should be treated as immutable. If you must change a PK value, you need to manually update all child tables within a transaction, or delete and re-insert.

### 1.4 Primary Key Declaration

| Feature | MariaDB | Oracle |
|---|---|---|
| Syntax | `PRIMARY KEY (col)` (inline, unnamed) | `CONSTRAINT pk_table_name PRIMARY KEY (col)` (named) |

The Oracle schema uses explicitly named primary key constraints (e.g., `pk_product_info`), which makes it easier to reference them in error messages, migration scripts, and administrative queries. MariaDB's inline `PRIMARY KEY` syntax generates a system name.

### 1.5 Storage Engine & Character Set

| Feature | MariaDB | Oracle |
|---|---|---|
| Storage engine | `ENGINE=InnoDB` | N/A (managed at tablespace level) |
| Character set | `DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci` | N/A (set at database/instance level via `NLS_CHARACTERSET`) |

MariaDB requires per-table engine and charset declarations. Oracle handles these concerns at the database or tablespace level, so individual `CREATE TABLE` statements do not include them.

### 1.6 Summary Table (All Differences at a Glance)

| Aspect | MariaDB | Oracle |
|---|---|---|
| String type | `VARCHAR` | `VARCHAR2` |
| Timestamp type | `DATETIME` | `TIMESTAMP` |
| Timestamp default | `CURRENT_TIMESTAMP` | `SYSTIMESTAMP` |
| Auto-update timestamp | `ON UPDATE CURRENT_TIMESTAMP` | Trigger or application logic |
| FK on update cascade | `ON UPDATE CASCADE` | Not available |
| PK naming | Anonymous / inline | Named constraints |
| Storage engine | `ENGINE=InnoDB` per table | Tablespace-level |
| Character set | Per-table `CHARSET`/`COLLATE` | Database-level `NLS_CHARACTERSET` |

---

## 2. Golang Repository Layer

This section shows how the repository layer differs when connecting to MariaDB vs Oracle from a Go backend. We use the `product_info` table as the representative example; the pattern applies identically to all other tables.

### 2.1 Shared Model

Both databases share the same Go struct:

```go
package model

import "time"

type ProductInfo struct {
    ProdID           string    `db:"prod_id"`
    ProdName         string    `db:"prod_name"`
    ProdShortName    *string   `db:"prod_short_name"`
    MgrNtAcct        *string   `db:"mgr_nt_acct"`
    ProdOwnerNtAcct  *string   `db:"prod_owner_nt_acct"`
    ProdPlatName     *string   `db:"prod_plat_name"`
    ProdNameAlias    *string   `db:"prod_name_alias"`
    ProdSuiteID      string    `db:"prod_suite_id"`
    CreatedAt        time.Time `db:"created_at"`
    UpdatedAt        time.Time `db:"updated_at"`
}
```

### 2.2 Repository Interface

A shared interface keeps both implementations interchangeable:

```go
package repository

import (
    "context"
    "myapp/model"
)

type ProductInfoRepository interface {
    GetByID(ctx context.Context, prodID string) (*model.ProductInfo, error)
    List(ctx context.Context) ([]model.ProductInfo, error)
    Create(ctx context.Context, p *model.ProductInfo) error
    Update(ctx context.Context, p *model.ProductInfo) error
    Delete(ctx context.Context, prodID string) error
}
```

### 2.3 MariaDB Repository

**Driver:** `github.com/go-sql-driver/mysql`

```go
package mariadb

import (
    "context"
    "database/sql"
    "myapp/model"
    "time"

    _ "github.com/go-sql-driver/mysql"
)

type ProductInfoRepo struct {
    db *sql.DB
}

func NewProductInfoRepo(db *sql.DB) *ProductInfoRepo {
    return &ProductInfoRepo{db: db}
}

func (r *ProductInfoRepo) GetByID(ctx context.Context, prodID string) (*model.ProductInfo, error) {
    query := `SELECT prod_id, prod_name, prod_short_name, mgr_nt_acct,
                     prod_owner_nt_acct, prod_plat_name, prod_name_alias,
                     prod_suite_id, created_at, updated_at
              FROM product_info
              WHERE prod_id = ?`

    row := r.db.QueryRowContext(ctx, query, prodID)

    var p model.ProductInfo
    err := row.Scan(
        &p.ProdID, &p.ProdName, &p.ProdShortName, &p.MgrNtAcct,
        &p.ProdOwnerNtAcct, &p.ProdPlatName, &p.ProdNameAlias,
        &p.ProdSuiteID, &p.CreatedAt, &p.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &p, nil
}

func (r *ProductInfoRepo) List(ctx context.Context) ([]model.ProductInfo, error) {
    query := `SELECT prod_id, prod_name, prod_short_name, mgr_nt_acct,
                     prod_owner_nt_acct, prod_plat_name, prod_name_alias,
                     prod_suite_id, created_at, updated_at
              FROM product_info`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []model.ProductInfo
    for rows.Next() {
        var p model.ProductInfo
        if err := rows.Scan(
            &p.ProdID, &p.ProdName, &p.ProdShortName, &p.MgrNtAcct,
            &p.ProdOwnerNtAcct, &p.ProdPlatName, &p.ProdNameAlias,
            &p.ProdSuiteID, &p.CreatedAt, &p.UpdatedAt,
        ); err != nil {
            return nil, err
        }
        results = append(results, p)
    }
    return results, rows.Err()
}

func (r *ProductInfoRepo) Create(ctx context.Context, p *model.ProductInfo) error {
    // MariaDB: created_at and updated_at default to CURRENT_TIMESTAMP automatically
    query := `INSERT INTO product_info
              (prod_id, prod_name, prod_short_name, mgr_nt_acct,
               prod_owner_nt_acct, prod_plat_name, prod_name_alias, prod_suite_id)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

    _, err := r.db.ExecContext(ctx, query,
        p.ProdID, p.ProdName, p.ProdShortName, p.MgrNtAcct,
        p.ProdOwnerNtAcct, p.ProdPlatName, p.ProdNameAlias, p.ProdSuiteID,
    )
    return err
}

func (r *ProductInfoRepo) Update(ctx context.Context, p *model.ProductInfo) error {
    // MariaDB: updated_at refreshes automatically via ON UPDATE CURRENT_TIMESTAMP
    query := `UPDATE product_info
              SET prod_name = ?, prod_short_name = ?, mgr_nt_acct = ?,
                  prod_owner_nt_acct = ?, prod_plat_name = ?,
                  prod_name_alias = ?, prod_suite_id = ?
              WHERE prod_id = ?`

    _, err := r.db.ExecContext(ctx, query,
        p.ProdName, p.ProdShortName, p.MgrNtAcct,
        p.ProdOwnerNtAcct, p.ProdPlatName,
        p.ProdNameAlias, p.ProdSuiteID,
        p.ProdID,
    )
    return err
}

func (r *ProductInfoRepo) Delete(ctx context.Context, prodID string) error {
    _, err := r.db.ExecContext(ctx, `DELETE FROM product_info WHERE prod_id = ?`, prodID)
    return err
}
```

**Key MariaDB traits:**
- Placeholder syntax: `?`
- `ON UPDATE CURRENT_TIMESTAMP` handles `updated_at` automatically -- no need to set it in the UPDATE query
- `created_at` defaults are handled by the DDL
- Driver: `github.com/go-sql-driver/mysql` (works for both MySQL and MariaDB)

### 2.4 Oracle Repository

**Driver:** `github.com/godror/godror`

```go
package oracle

import (
    "context"
    "database/sql"
    "myapp/model"
    "time"

    _ "github.com/godror/godror"
)

type ProductInfoRepo struct {
    db *sql.DB
}

func NewProductInfoRepo(db *sql.DB) *ProductInfoRepo {
    return &ProductInfoRepo{db: db}
}

func (r *ProductInfoRepo) GetByID(ctx context.Context, prodID string) (*model.ProductInfo, error) {
    query := `SELECT prod_id, prod_name, prod_short_name, mgr_nt_acct,
                     prod_owner_nt_acct, prod_plat_name, prod_name_alias,
                     prod_suite_id, created_at, updated_at
              FROM product_info
              WHERE prod_id = :1`

    row := r.db.QueryRowContext(ctx, query, prodID)

    var p model.ProductInfo
    err := row.Scan(
        &p.ProdID, &p.ProdName, &p.ProdShortName, &p.MgrNtAcct,
        &p.ProdOwnerNtAcct, &p.ProdPlatName, &p.ProdNameAlias,
        &p.ProdSuiteID, &p.CreatedAt, &p.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &p, nil
}

func (r *ProductInfoRepo) List(ctx context.Context) ([]model.ProductInfo, error) {
    query := `SELECT prod_id, prod_name, prod_short_name, mgr_nt_acct,
                     prod_owner_nt_acct, prod_plat_name, prod_name_alias,
                     prod_suite_id, created_at, updated_at
              FROM product_info`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []model.ProductInfo
    for rows.Next() {
        var p model.ProductInfo
        if err := rows.Scan(
            &p.ProdID, &p.ProdName, &p.ProdShortName, &p.MgrNtAcct,
            &p.ProdOwnerNtAcct, &p.ProdPlatName, &p.ProdNameAlias,
            &p.ProdSuiteID, &p.CreatedAt, &p.UpdatedAt,
        ); err != nil {
            return nil, err
        }
        results = append(results, p)
    }
    return results, rows.Err()
}

func (r *ProductInfoRepo) Create(ctx context.Context, p *model.ProductInfo) error {
    // Oracle: created_at and updated_at default to SYSTIMESTAMP automatically
    query := `INSERT INTO product_info
              (prod_id, prod_name, prod_short_name, mgr_nt_acct,
               prod_owner_nt_acct, prod_plat_name, prod_name_alias, prod_suite_id)
              VALUES (:1, :2, :3, :4, :5, :6, :7, :8)`

    _, err := r.db.ExecContext(ctx, query,
        p.ProdID, p.ProdName, p.ProdShortName, p.MgrNtAcct,
        p.ProdOwnerNtAcct, p.ProdPlatName, p.ProdNameAlias, p.ProdSuiteID,
    )
    return err
}

func (r *ProductInfoRepo) Update(ctx context.Context, p *model.ProductInfo) error {
    // Oracle: No ON UPDATE trigger in DDL, so we must set updated_at explicitly
    query := `UPDATE product_info
              SET prod_name = :1, prod_short_name = :2, mgr_nt_acct = :3,
                  prod_owner_nt_acct = :4, prod_plat_name = :5,
                  prod_name_alias = :6, prod_suite_id = :7,
                  updated_at = SYSTIMESTAMP
              WHERE prod_id = :8`

    _, err := r.db.ExecContext(ctx, query,
        p.ProdName, p.ProdShortName, p.MgrNtAcct,
        p.ProdOwnerNtAcct, p.ProdPlatName,
        p.ProdNameAlias, p.ProdSuiteID,
        p.ProdID,
    )
    return err
}

func (r *ProductInfoRepo) Delete(ctx context.Context, prodID string) error {
    _, err := r.db.ExecContext(ctx, `DELETE FROM product_info WHERE prod_id = :1`, prodID)
    return err
}
```

**Key Oracle traits:**
- Placeholder syntax: `:1, :2, :3, ...` (numbered bind variables) instead of `?`
- `updated_at = SYSTIMESTAMP` must be set **explicitly** in every UPDATE statement (Oracle has no `ON UPDATE` DDL clause)
- Driver: `github.com/godror/godror` (requires Oracle Instant Client)

### 2.5 Side-by-Side Comparison

| Aspect | MariaDB Repo | Oracle Repo |
|---|---|---|
| Driver import | `github.com/go-sql-driver/mysql` | `github.com/godror/godror` |
| Bind parameters | `?` | `:1, :2, :3, ...` |
| `updated_at` on UPDATE | Automatic (`ON UPDATE CURRENT_TIMESTAMP`) | Explicit `updated_at = SYSTIMESTAMP` in query |
| `created_at` on INSERT | Automatic (DDL default) | Automatic (DDL default) |
| DSN format | `user:pass@tcp(host:3306)/dbname?parseTime=true` | `user/pass@host:1521/service_name` |
| External dependency | None | Oracle Instant Client required |
| `sql.ErrNoRows` handling | Same | Same |
| `database/sql` interface | Same | Same |

### 2.6 Database Connection Setup

**MariaDB:**

```go
import "database/sql"
import _ "github.com/go-sql-driver/mysql"

dsn := "user:password@tcp(127.0.0.1:3306)/mydb?parseTime=true&loc=UTC"
db, err := sql.Open("mysql", dsn)
```

The `parseTime=true` parameter is critical -- it ensures `DATETIME` values are scanned into `time.Time` rather than `[]byte`.

**Oracle:**

```go
import "database/sql"
import _ "github.com/godror/godror"

dsn := `user="myuser" password="mypassword" connectString="127.0.0.1:1521/ORCLPDB1"`
db, err := sql.Open("godror", dsn)
```

The `godror` driver requires the Oracle Instant Client libraries to be installed and available in `LD_LIBRARY_PATH` (Linux) or `DYLD_LIBRARY_PATH` (macOS).

### 2.7 Swapping at Runtime

Since both implementations satisfy the same `ProductInfoRepository` interface, you can select the backend at startup:

```go
func NewProductInfoRepo(driver string, db *sql.DB) repository.ProductInfoRepository {
    switch driver {
    case "mysql":
        return mariadb.NewProductInfoRepo(db)
    case "godror":
        return oracle.NewProductInfoRepo(db)
    default:
        panic("unsupported driver: " + driver)
    }
}
```

This pattern applies to all 14 tables in the schema. Each table gets its own interface, MariaDB implementation, and Oracle implementation, with the only differences being bind parameter syntax and explicit `updated_at` handling.
