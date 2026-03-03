// Package main demonstrates READ and INSERT patterns for a MariaDB table that
// combines product_info (relational columns) with the former product_config EAV
// table collapsed into a single JSON column.
//
// Run from the backend/ directory:
//
//	go run ./cmd/examples
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ── DDL reference ─────────────────────────────────────────────────────────────
//
// Oracle source — two separate tables:
//
//	product_info (
//	    prod_id            VARCHAR2(64)  PK,
//	    prod_name          VARCHAR2(255) NOT NULL,
//	    prod_short_name    VARCHAR2(64),
//	    mgr_nt_acct        VARCHAR2(128),
//	    prod_owner_nt_acct VARCHAR2(128),
//	    prod_plat_name     VARCHAR2(255),
//	    prod_name_alias    VARCHAR2(255),
//	    prod_suite_id      VARCHAR2(64)  FK → product_suites_info
//	)
//
//	product_config (
//	    config_id          VARCHAR2(64)   PK,
//	    prod_id            VARCHAR2(64)   FK → product_info,
//	    prod_config_param  VARCHAR2(255)  NOT NULL,   ← the key
//	    prod_config_val    VARCHAR2(1024) DEFAULT NULL ← the value
//	)
//	-- With many params per product, a single product can generate
//	-- hundreds of rows in product_config.
//
// ─────────────────────────────────────────────────────────────────────────────
//
// MariaDB target — single combined table:
//
//	CREATE TABLE product_info (
//	    prod_id             VARCHAR(64)  NOT NULL,
//	    prod_name           VARCHAR(255) NOT NULL,
//	    prod_short_name     VARCHAR(64)  DEFAULT NULL,
//	    mgr_nt_acct         VARCHAR(128) DEFAULT NULL,
//	    prod_owner_nt_acct  VARCHAR(128) DEFAULT NULL,
//	    prod_plat_name      VARCHAR(255) DEFAULT NULL,
//	    prod_name_alias     VARCHAR(255) DEFAULT NULL,
//	    prod_suite_id       VARCHAR(64)  NOT NULL,
//	    config              JSON         NOT NULL DEFAULT '{}',
//	    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
//	    updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
//	    PRIMARY KEY (prod_id),
//	    INDEX idx_product_suite (prod_suite_id),
//	    CONSTRAINT fk_product_suite
//	        FOREIGN KEY (prod_suite_id) REFERENCES product_suites_info (prod_suite_id)
//	        ON DELETE CASCADE ON UPDATE CASCADE
//	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
//
// ─────────────────────────────────────────────────────────────────────────────
//
// Optimization: when config keys are frequently used in WHERE / ORDER BY clauses,
// add generated virtual columns so the DB can index them without full JSON scans.
// Only create these for keys that appear in real query predicates — each virtual
// column adds a small overhead on every write.
//
//	ALTER TABLE product_info
//	    ADD COLUMN cfg_region       VARCHAR(64)
//	        GENERATED ALWAYS AS (JSON_VALUE(config, '$.region')) VIRTUAL,
//	    ADD COLUMN cfg_feature_flag VARCHAR(8)
//	        GENERATED ALWAYS AS (JSON_VALUE(config, '$.feature_flag_x')) VIRTUAL,
//	    ADD COLUMN cfg_timeout      INT UNSIGNED
//	        GENERATED ALWAYS AS (CAST(JSON_VALUE(config, '$.timeout') AS UNSIGNED)) VIRTUAL,
//	    ADD INDEX idx_cfg_region       (cfg_region),
//	    ADD INDEX idx_cfg_feature_flag (cfg_feature_flag),
//	    ADD INDEX idx_cfg_timeout      (cfg_timeout);

// ── Model ─────────────────────────────────────────────────────────────────────

// ProductInfo is the in-memory representation of one product_info row.
// Config holds all merged EAV entries — every value is a string, matching
// Oracle's product_config.prod_config_val VARCHAR2(1024) type.
type ProductInfo struct {
	ProdID          string            `json:"prod_id"`
	ProdName        string            `json:"prod_name"`
	ProdShortName   *string           `json:"prod_short_name,omitempty"`
	MgrNTAcct       *string           `json:"mgr_nt_acct,omitempty"`
	ProdOwnerNTAcct *string           `json:"prod_owner_nt_acct,omitempty"`
	ProdPlatName    *string           `json:"prod_plat_name,omitempty"`
	ProdNameAlias   *string           `json:"prod_name_alias,omitempty"`
	ProdSuiteID     string            `json:"prod_suite_id"`
	Config          map[string]string `json:"config"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// ── Repository ────────────────────────────────────────────────────────────────

type ProductInfoRepo struct {
	db *sql.DB
}

func NewProductInfoRepo(db *sql.DB) *ProductInfoRepo {
	return &ProductInfoRepo{db: db}
}

// ── READ: full product + config ───────────────────────────────────────────────

// GetByID fetches one product row and deserialises the full JSON config into
// a map. Use this when you need all config keys, e.g. a settings page.
//
// Performance note: for large configs (100+ keys, each up to 1 KB), the config
// column may be stored off-page in InnoDB. The entire JSON document crosses
// the network and is unmarshalled in Go. If you only need a single key, use
// GetConfigParam instead.
func (r *ProductInfoRepo) GetByID(ctx context.Context, prodID string) (*ProductInfo, error) {
	var p ProductInfo
	var rawConfig string

	err := r.db.QueryRowContext(ctx, `
		SELECT prod_id, prod_name, prod_short_name, mgr_nt_acct,
		       prod_owner_nt_acct, prod_plat_name, prod_name_alias,
		       prod_suite_id, config, created_at, updated_at
		FROM product_info
		WHERE prod_id = ?`, prodID,
	).Scan(
		&p.ProdID, &p.ProdName, &p.ProdShortName, &p.MgrNTAcct,
		&p.ProdOwnerNTAcct, &p.ProdPlatName, &p.ProdNameAlias,
		&p.ProdSuiteID, &rawConfig, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetByID %s: %w", prodID, err)
	}
	if err := json.Unmarshal([]byte(rawConfig), &p.Config); err != nil {
		return nil, fmt.Errorf("GetByID %s unmarshal config: %w", prodID, err)
	}
	return &p, nil
}

// ── READ: single config key — server-side extraction ─────────────────────────

// GetConfigParam extracts one key from the config JSON column using
// JSON_VALUE, evaluated entirely inside MariaDB. Only the scalar result
// crosses the network — the full JSON blob is never sent to the Go process.
//
// Benchmarks typically show 5–10× lower network bytes vs GetByID when the
// config has 100+ keys.
//
// IMPORTANT: paramKey must be a known application constant, not user input.
// There is no bind-variable equivalent for JSON path strings in MariaDB.
// Never interpolate user-supplied strings into the path.
//
// Returns ("", false, nil) when the key does not exist in the config.
func (r *ProductInfoRepo) GetConfigParam(ctx context.Context, prodID, paramKey string) (string, bool, error) {
	path := "$." + paramKey
	var val sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT JSON_VALUE(config, ?) FROM product_info WHERE prod_id = ?`,
		path, prodID,
	).Scan(&val)
	if err != nil {
		return "", false, fmt.Errorf("GetConfigParam %s.%s: %w", prodID, paramKey, err)
	}
	if !val.Valid {
		return "", false, nil
	}
	return val.String, true, nil
}

// ── READ: filter by config key via generated virtual column ──────────────────

// ListByRegion returns products whose config contains "region": <region>.
//
// Requires the virtual column and index created by the ALTER TABLE above:
//
//	cfg_region VARCHAR(64) GENERATED ALWAYS AS (JSON_VALUE(config, '$.region')) VIRTUAL
//	INDEX idx_cfg_region (cfg_region)
//
// Without the virtual column, this query degrades to a full table scan where
// MariaDB parses every row's JSON blob. With the index it is an index range
// scan — O(log N) instead of O(N).
//
// Note: config is intentionally omitted from the SELECT list. When only
// identity columns are needed, skipping the JSON column avoids loading
// potentially large off-page data for every matched row.
func (r *ProductInfoRepo) ListByRegion(ctx context.Context, region string) ([]*ProductInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT prod_id, prod_name, prod_suite_id, created_at, updated_at
		FROM product_info
		WHERE cfg_region = ?`, region,
	)
	if err != nil {
		return nil, fmt.Errorf("ListByRegion %q: %w", region, err)
	}
	defer rows.Close()

	var results []*ProductInfo
	for rows.Next() {
		var p ProductInfo
		if err := rows.Scan(
			&p.ProdID, &p.ProdName, &p.ProdSuiteID,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ListByRegion scan: %w", err)
		}
		results = append(results, &p)
	}
	return results, rows.Err()
}

// ── INSERT: single row ────────────────────────────────────────────────────────

// Insert writes one product row. Suitable for low-volume, interactive writes.
// For bulk imports, use BulkInsert instead.
func (r *ProductInfoRepo) Insert(ctx context.Context, p *ProductInfo) error {
	configBytes, err := json.Marshal(p.Config)
	if err != nil {
		return fmt.Errorf("Insert marshal config for %s: %w", p.ProdID, err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT IGNORE INTO product_info
		    (prod_id, prod_name, prod_short_name, mgr_nt_acct,
		     prod_owner_nt_acct, prod_plat_name, prod_name_alias,
		     prod_suite_id, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ProdID, p.ProdName, p.ProdShortName, p.MgrNTAcct,
		p.ProdOwnerNTAcct, p.ProdPlatName, p.ProdNameAlias,
		p.ProdSuiteID, string(configBytes),
		p.CreatedAt.UTC(), p.UpdatedAt.UTC(),
	)
	return err
}

// ── INSERT: batch (multi-row VALUES) — primary optimization for large imports ─
//
// Why BulkInsert is faster than calling Insert in a loop:
//
//  1. Round-trips: each individual INSERT requires client → server → parse →
//     plan → execute → response. At 1 000 rows that is 1 000 round-trips.
//     One multi-row INSERT collapses all of that to a single round-trip.
//
//  2. Parse/plan overhead: the query is parsed and planned once regardless of
//     how many value tuples are appended.
//
//  3. Typical result: 10–30× faster for bulk loads of large-JSON rows compared
//     to a loop of single-row INSERT statements.
//
// Capacity guidance for large configs:
//
//   - Each config entry: up to 255 bytes (key) + 1 024 bytes (val) ≈ 1.3 KB.
//   - 100 config keys per product ≈ 130 KB of JSON per row.
//   - 1 000 rows/batch × 130 KB ≈ 130 MB per round-trip.
//   - MariaDB 11 default max_allowed_packet = 64 MB.
//   - Recommendation: keep batchSize × avg_config_size < 32 MB to stay safely
//     below the default limit. With 130 KB/row, use batchSize ≤ 200.
//   - Override server-side: SET GLOBAL max_allowed_packet = 134217728; (128 MB)
//
// INSERT IGNORE semantics on multi-row statements:
//   Duplicate PK rows are silently skipped for the entire batch, identical to
//   the per-row behaviour. Safe to re-run on a partially-committed batch.

// BulkInsert writes up to len(products) rows in a single SQL statement.
// Callers are responsible for splitting into appropriately-sized batches.
func (r *ProductInfoRepo) BulkInsert(ctx context.Context, products []*ProductInfo) error {
	if len(products) == 0 {
		return nil
	}

	const colsPerRow = 11
	placeholders := make([]string, len(products))
	args := make([]any, 0, len(products)*colsPerRow)

	for i, p := range products {
		configBytes, err := json.Marshal(p.Config)
		if err != nil {
			return fmt.Errorf("BulkInsert marshal config[%d] %s: %w", i, p.ProdID, err)
		}
		placeholders[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
		args = append(args,
			p.ProdID, p.ProdName, p.ProdShortName, p.MgrNTAcct,
			p.ProdOwnerNTAcct, p.ProdPlatName, p.ProdNameAlias,
			p.ProdSuiteID, string(configBytes),
			p.CreatedAt.UTC(), p.UpdatedAt.UTC(),
		)
	}

	query := `INSERT IGNORE INTO product_info
		(prod_id, prod_name, prod_short_name, mgr_nt_acct,
		 prod_owner_nt_acct, prod_plat_name, prod_name_alias,
		 prod_suite_id, config, created_at, updated_at)
		VALUES ` + strings.Join(placeholders, ", ")

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// ── UPDATE: patch one config key without rewriting the full map ───────────────

// SetConfigParam updates a single key inside the config JSON column.
// MariaDB evaluates JSON_SET server-side: the full document is never sent
// over the wire, only the path string and the new scalar value.
//
// Row-level locking still covers the entire row for the duration of the write.
// If many goroutines concurrently update different keys on the same product,
// they serialize behind that lock. At very high concurrent-write rates per
// entity, consider promoting hot keys to dedicated relational columns instead.
func (r *ProductInfoRepo) SetConfigParam(ctx context.Context, prodID, paramKey, value string) error {
	path := "$." + paramKey
	_, err := r.db.ExecContext(ctx, `
		UPDATE product_info
		SET config = JSON_SET(config, ?, ?)
		WHERE prod_id = ?`,
		path, value, prodID,
	)
	return err
}

// RemoveConfigParam deletes a key from the config JSON.
func (r *ProductInfoRepo) RemoveConfigParam(ctx context.Context, prodID, paramKey string) error {
	path := "$." + paramKey
	_, err := r.db.ExecContext(ctx, `
		UPDATE product_info
		SET config = JSON_REMOVE(config, ?)
		WHERE prod_id = ?`,
		path, prodID,
	)
	return err
}

// ── Connection setup ──────────────────────────────────────────────────────────

func connectMariaDB() (*sql.DB, error) {
	dsn := "mariadb:mariadb@tcp(localhost:3306)/migration_db?parseTime=true&loc=UTC"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	// Pool sizing: with large JSON rows, each connection may hold more memory
	// in its read/write buffers. Keep MaxOpenConns conservative.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// makeLargeConfig returns a config map with n synthetic key-value pairs,
// simulating a product that has accumulated many EAV rows in Oracle.
// Each value is ~50 bytes, so n=100 ≈ 8 KB of JSON; n=1000 ≈ 80 KB of JSON.
func makeLargeConfig(n int) map[string]string {
	m := make(map[string]string, n)
	// Common query-pattern keys that would get virtual columns + indexes
	m["region"] = "ap-northeast-1"
	m["feature_flag_x"] = "true"
	m["timeout"] = "30"
	// Simulate the remainder as generic config params
	for i := 3; i < n; i++ {
		key := fmt.Sprintf("param_%04d", i)
		val := fmt.Sprintf("value_for_param_%04d_with_some_padding_to_simulate_real_data", i)
		m[key] = val
	}
	return m
}

func ptr(s string) *string { return &s }

// ── Demo ──────────────────────────────────────────────────────────────────────

func main() {
	db, err := connectMariaDB()
	if err != nil {
		log.Fatalf("MariaDB connection failed: %v", err)
	}
	defer db.Close()

	repo := NewProductInfoRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()

	// ── 1. Single INSERT with a large config (100 keys) ──────────────────────
	fmt.Println("=== Single INSERT ===")
	prod := &ProductInfo{
		ProdID:      "PROD-EXAMPLE-001",
		ProdName:    "Example Product",
		ProdSuiteID: "SUITE-001",
		Config:      makeLargeConfig(100), // 100 config params from Oracle EAV
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := repo.Insert(ctx, prod); err != nil {
		log.Fatalf("Insert failed: %v", err)
	}
	fmt.Printf("Inserted %s with %d config keys\n", prod.ProdID, len(prod.Config))

	// ── 2. Bulk INSERT — 500 products, each with 100 config keys ─────────────
	//    With avg config ≈ 8 KB, 500 rows ≈ 4 MB per batch — well within the
	//    64 MB default max_allowed_packet.
	fmt.Println("\n=== Bulk INSERT (500 rows × 100 config keys) ===")
	const bulkCount = 500
	batch := make([]*ProductInfo, bulkCount)
	for i := range batch {
		batch[i] = &ProductInfo{
			ProdID:      fmt.Sprintf("PROD-BULK-%04d", i),
			ProdName:    fmt.Sprintf("Bulk Product %d", i),
			ProdSuiteID: "SUITE-001",
			Config:      makeLargeConfig(100),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	start := time.Now()
	if err := repo.BulkInsert(ctx, batch); err != nil {
		log.Fatalf("BulkInsert failed: %v", err)
	}
	fmt.Printf("BulkInsert %d rows in %v\n", bulkCount, time.Since(start))

	// ── 3. READ full product — deserialises entire JSON config ────────────────
	fmt.Println("\n=== READ full product (all config keys) ===")
	start = time.Now()
	full, err := repo.GetByID(ctx, "PROD-EXAMPLE-001")
	if err != nil {
		log.Fatalf("GetByID failed: %v", err)
	}
	fmt.Printf("GetByID fetched %d config keys in %v\n", len(full.Config), time.Since(start))
	fmt.Printf("  config[\"region\"]  = %q\n", full.Config["region"])
	fmt.Printf("  config[\"timeout\"] = %q\n", full.Config["timeout"])

	// ── 4. READ single config key — JSON_VALUE, no full deserialization ───────
	fmt.Println("\n=== READ single config key (JSON_VALUE, server-side extraction) ===")
	start = time.Now()
	region, ok, err := repo.GetConfigParam(ctx, "PROD-EXAMPLE-001", "region")
	if err != nil {
		log.Fatalf("GetConfigParam failed: %v", err)
	}
	fmt.Printf("GetConfigParam \"region\" = %q (found=%v) in %v\n", region, ok, time.Since(start))
	// Only the scalar "ap-northeast-1" crossed the network — not the full 8 KB JSON.

	// ── 5. READ: filter by config key using virtual column index ──────────────
	//    Requires: ADD COLUMN cfg_region ... GENERATED ALWAYS AS (...) VIRTUAL
	//              ADD INDEX idx_cfg_region (cfg_region)
	fmt.Println("\n=== READ by config key (virtual column index) ===")
	products, err := repo.ListByRegion(ctx, "ap-northeast-1")
	if err != nil {
		log.Fatalf("ListByRegion failed: %v", err)
	}
	fmt.Printf("ListByRegion found %d products (config column not loaded)\n", len(products))

	// ── 6. Partial config update — JSON_SET (single key, no full rewrite) ─────
	fmt.Println("\n=== Partial config UPDATE (JSON_SET) ===")
	if err := repo.SetConfigParam(ctx, "PROD-EXAMPLE-001", "timeout", "60"); err != nil {
		log.Fatalf("SetConfigParam failed: %v", err)
	}
	updated, _, _ := repo.GetConfigParam(ctx, "PROD-EXAMPLE-001", "timeout")
	fmt.Printf("After SetConfigParam: config[\"timeout\"] = %q\n", updated)

	// ── 7. Remove a config key ────────────────────────────────────────────────
	fmt.Println("\n=== Remove config key (JSON_REMOVE) ===")
	if err := repo.RemoveConfigParam(ctx, "PROD-EXAMPLE-001", "feature_flag_x"); err != nil {
		log.Fatalf("RemoveConfigParam failed: %v", err)
	}
	_, exists, err := repo.GetConfigParam(ctx, "PROD-EXAMPLE-001", "feature_flag_x")
	if err != nil {
		log.Fatalf("GetConfigParam after remove failed: %v", err)
	}
	fmt.Printf("After RemoveConfigParam: \"feature_flag_x\" exists = %v\n", exists)

	fmt.Println("\nDone.")
}

// ── Optimization summary ──────────────────────────────────────────────────────
//
// When the combined EAV config is large, apply these in order of impact:
//
//  1. BulkInsert (multi-row VALUES)
//     Use for any batch load > 10 rows. Eliminates per-row round-trip overhead.
//     Watch max_allowed_packet: keep batchSize × avg_json_bytes < 32 MB.
//
//  2. Selective SELECT — omit config when not needed
//     ListByRegion above never loads the config column. This matters most when
//     InnoDB stores the JSON off-page (row + JSON > ~8 KB combined): the
//     off-page read is skipped entirely if the column is absent from SELECT.
//
//  3. JSON_VALUE for single-key reads
//     GetConfigParam sends only one scalar over the wire. At 100 keys × 1 KB
//     each, this reduces network bytes by ~100× compared to GetByID.
//
//  4. Generated virtual columns + indexes for filter keys
//     Without them, WHERE config->'$.region' = ? is a full table scan.
//     The ALTER TABLE above converts that to an index range scan.
//     Add a virtual column only for keys that genuinely appear in WHERE clauses.
//
//  5. JSON_SET / JSON_REMOVE for partial updates
//     Avoids read-modify-write round-trips. MariaDB rewrites the full JSON
//     column server-side, but only one network round-trip is needed.
//     At very high concurrent-write rates on the same row, row-level locking
//     serialises writers. If that becomes a bottleneck, promote the hot key
//     to a dedicated relational column.
//
//  6. Connection pool tuning (SetMaxOpenConns)
//     Large JSON rows inflate per-connection buffer usage. Keep MaxOpenConns
//     lower than you would for narrow-row tables (10–20 is usually sufficient).
//
//  7. Hybrid schema: promote ultra-hot keys to relational columns
//     If a small set of keys (e.g. "status", "version") are read/written on
//     nearly every query, adding them as proper columns eliminates JSON
//     parsing entirely for those accesses. Use JSON only for the long tail of
//     rarely-accessed params.
