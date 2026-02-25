package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/godror/godror"
)

// ── Env helpers ───────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var batchSize int

func init() {
	bs, err := strconv.Atoi(getEnv("BATCH_SIZE", "1000"))
	if err != nil || bs <= 0 {
		bs = 1000
	}
	batchSize = bs
}

// ── Database connections ──────────────────────────────────────────────────────

func connectOracle() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s/%s@%s:%s/%s",
		getEnv("ORACLE_USER", "system"),
		getEnv("ORACLE_PASSWORD", "oracle"),
		getEnv("ORACLE_HOST", "localhost"),
		getEnv("ORACLE_PORT", "1521"),
		getEnv("ORACLE_SERVICE", "XEPDB1"),
	)
	db, err := sql.Open("godror", dsn)
	if err != nil {
		return nil, fmt.Errorf("open oracle: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping oracle: %w", err)
	}
	return db, nil
}

func connectMariaDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=UTC",
		getEnv("MARIADB_USER", "mariadb"),
		getEnv("MARIADB_PASSWORD", "mariadb"),
		getEnv("MARIADB_HOST", "localhost"),
		getEnv("MARIADB_PORT", "3306"),
		getEnv("MARIADB_DATABASE", "migration_db"),
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mariadb: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping mariadb: %w", err)
	}
	return db, nil
}

// ── Migration log ─────────────────────────────────────────────────────────────

const createLogTable = `
CREATE TABLE IF NOT EXISTS _migration_log (
    table_name    VARCHAR(64)  NOT NULL,
    status        ENUM('in_progress', 'completed') NOT NULL DEFAULT 'in_progress',
    rows_migrated INT          NOT NULL DEFAULT 0,
    last_offset   INT          NOT NULL DEFAULT 0,
    started_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (table_name)
) ENGINE=InnoDB`

func ensureMigrationLog(db *sql.DB) error {
	_, err := db.Exec(createLogTable)
	return err
}

type logEntry struct {
	status       string
	rowsMigrated int
	lastOffset   int
}

// checkLog returns the current log entry for tableName, creating one if absent.
func checkLog(db *sql.DB, tableName string) (*logEntry, error) {
	var e logEntry
	err := db.QueryRow(
		`SELECT status, rows_migrated, last_offset FROM _migration_log WHERE table_name = ?`,
		tableName,
	).Scan(&e.status, &e.rowsMigrated, &e.lastOffset)
	if err == sql.ErrNoRows {
		_, err2 := db.Exec(
			`INSERT INTO _migration_log (table_name, status, rows_migrated, last_offset) VALUES (?, 'in_progress', 0, 0)`,
			tableName,
		)
		if err2 != nil {
			return nil, fmt.Errorf("insert log entry for %s: %w", tableName, err2)
		}
		return &logEntry{status: "in_progress"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query log for %s: %w", tableName, err)
	}
	return &e, nil
}

func updateLog(db *sql.DB, tableName string, newOffset, totalRows int) error {
	_, err := db.Exec(
		`UPDATE _migration_log SET rows_migrated = ?, last_offset = ? WHERE table_name = ?`,
		totalRows, newOffset, tableName,
	)
	return err
}

func markCompleted(db *sql.DB, tableName string, totalRows int) error {
	_, err := db.Exec(
		`UPDATE _migration_log SET status = 'completed', rows_migrated = ?, last_offset = ? WHERE table_name = ?`,
		totalRows, totalRows, tableName,
	)
	return err
}

// ── EAV aggregation helper ────────────────────────────────────────────────────

// fetchEAVConfig reads an entire EAV config table from Oracle into memory and
// returns map[parentID]map[paramName]paramValue. EAV tables are expected to be
// small, so loading them fully upfront is safe.
func fetchEAVConfig(db *sql.DB, table, fkCol, paramCol, valCol string) (map[string]map[string]string, error) {
	query := fmt.Sprintf("SELECT %s, %s, %s FROM %s", fkCol, paramCol, valCol, table)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("fetchEAVConfig %s: %w", table, err)
	}
	defer rows.Close()

	result := make(map[string]map[string]string)
	for rows.Next() {
		var fk, param string
		var val sql.NullString
		if err := rows.Scan(&fk, &param, &val); err != nil {
			return nil, fmt.Errorf("fetchEAVConfig %s scan: %w", table, err)
		}
		if result[fk] == nil {
			result[fk] = make(map[string]string)
		}
		if val.Valid {
			result[fk][param] = val.String
		} else {
			result[fk][param] = ""
		}
	}
	return result, rows.Err()
}

// configJSON serialises a param map to a JSON object string. Returns "{}" when
// the map is empty or nil, matching the MariaDB column default.
func configJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// ── Deploy-status helper ──────────────────────────────────────────────────────

type deployStatusEntry struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	At      string `json:"at"`
}

type deployStatusData struct {
	latestStatus  sql.NullString
	latestMessage sql.NullString
	historyJSON   string
}

// fetchDeployStatusMap loads all paas_deploy_status rows from Oracle, groups
// them by unit_id and returns per-unit summary + JSON history array.
func fetchDeployStatusMap(db *sql.DB) (map[string]deployStatusData, error) {
	rows, err := db.Query(`
		SELECT unit_id, deploy_status, deploy_message, created_at
		FROM paas_deploy_status
		ORDER BY unit_id, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("fetchDeployStatusMap: %w", err)
	}
	defer rows.Close()

	type rawRow struct {
		status    sql.NullString
		message   sql.NullString
		createdAt time.Time
	}
	grouped := make(map[string][]rawRow)
	var orderedKeys []string
	seen := make(map[string]bool)

	for rows.Next() {
		var unitID string
		var r rawRow
		if err := rows.Scan(&unitID, &r.status, &r.message, &r.createdAt); err != nil {
			return nil, fmt.Errorf("fetchDeployStatusMap scan: %w", err)
		}
		if !seen[unitID] {
			orderedKeys = append(orderedKeys, unitID)
			seen[unitID] = true
		}
		grouped[unitID] = append(grouped[unitID], r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string]deployStatusData, len(orderedKeys))
	for _, unitID := range orderedKeys {
		statuses := grouped[unitID]
		history := make([]deployStatusEntry, 0, len(statuses))
		for _, s := range statuses {
			e := deployStatusEntry{At: s.createdAt.UTC().Format(time.RFC3339)}
			if s.status.Valid {
				e.Status = s.status.String
			}
			if s.message.Valid {
				e.Message = s.message.String
			}
			history = append(history, e)
		}
		histJSON, _ := json.Marshal(history)
		latest := statuses[len(statuses)-1]
		result[unitID] = deployStatusData{
			latestStatus:  latest.status,
			latestMessage: latest.message,
			historyJSON:   string(histJSON),
		}
	}
	return result, nil
}

// ── Migration functions ───────────────────────────────────────────────────────

func migrateProductSuites(oracle, maria *sql.DB, step int) error {
	const tableName = "product_suites_info"
	fmt.Printf("[%d/8] %s: loading EAV config...\n", step, tableName)

	cfgMap, err := fetchEAVConfig(oracle,
		"product_suite_config", "prod_suite_id", "prod_suite_config_param", "prod_suite_config_val")
	if err != nil {
		return err
	}

	entry, err := checkLog(maria, tableName)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] %s: SKIP (already completed)\n", step, tableName)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] %s: resuming from offset %d\n", step, tableName, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT prod_suite_id, prod_suite_name, prod_suite_owner_nt_acct,
			       prod_suite_site_owner_acct, division, created_at, updated_at
			FROM product_suites_info
			ORDER BY prod_suite_id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("%s query: %w", tableName, err)
		}

		type psRow struct {
			id, name             string
			ownerNT, siteOwner   sql.NullString
			division             sql.NullString
			createdAt, updatedAt time.Time
		}
		var batch []psRow
		for rows.Next() {
			var r psRow
			if err := rows.Scan(&r.id, &r.name, &r.ownerNT, &r.siteOwner, &r.division,
				&r.createdAt, &r.updatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("%s scan: %w", tableName, err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] %s: batch offset=%d rows=%d\n", step, tableName, offset, len(batch))
		for _, r := range batch {
			_, err := maria.Exec(`
				INSERT IGNORE INTO product_suites_info
				    (prod_suite_id, prod_suite_name, prod_suite_owner_nt_acct,
				     prod_suite_site_owner_acct, division, config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				r.id, r.name, r.ownerNT, r.siteOwner, r.division,
				configJSON(cfgMap[r.id]),
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("%s insert: %w", tableName, err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, tableName, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, tableName, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] %s: done (%d total rows)\n", step, tableName, total)
	return nil
}

func migrateProducts(oracle, maria *sql.DB, step int) error {
	const tableName = "product_info"
	fmt.Printf("[%d/8] %s: loading EAV config...\n", step, tableName)

	cfgMap, err := fetchEAVConfig(oracle,
		"product_config", "prod_id", "prod_config_param", "prod_config_val")
	if err != nil {
		return err
	}

	entry, err := checkLog(maria, tableName)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] %s: SKIP (already completed)\n", step, tableName)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] %s: resuming from offset %d\n", step, tableName, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT prod_id, prod_name, prod_short_name, mgr_nt_acct, prod_owner_nt_acct,
			       prod_plat_name, prod_name_alias, prod_suite_id, created_at, updated_at
			FROM product_info
			ORDER BY prod_id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("%s query: %w", tableName, err)
		}

		type piRow struct {
			id, name               string
			shortName, mgrNT       sql.NullString
			ownerNT, platName      sql.NullString
			alias                  sql.NullString
			suiteID                string
			createdAt, updatedAt   time.Time
		}
		var batch []piRow
		for rows.Next() {
			var r piRow
			if err := rows.Scan(&r.id, &r.name, &r.shortName, &r.mgrNT, &r.ownerNT,
				&r.platName, &r.alias, &r.suiteID, &r.createdAt, &r.updatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("%s scan: %w", tableName, err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] %s: batch offset=%d rows=%d\n", step, tableName, offset, len(batch))
		for _, r := range batch {
			_, err := maria.Exec(`
				INSERT IGNORE INTO product_info
				    (prod_id, prod_name, prod_short_name, mgr_nt_acct, prod_owner_nt_acct,
				     prod_plat_name, prod_name_alias, prod_suite_id, config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				r.id, r.name, r.shortName, r.mgrNT, r.ownerNT,
				r.platName, r.alias, r.suiteID,
				configJSON(cfgMap[r.id]),
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("%s insert: %w", tableName, err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, tableName, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, tableName, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] %s: done (%d total rows)\n", step, tableName, total)
	return nil
}

func migrateRoleMap(oracle, maria *sql.DB, step int) error {
	const tableName = "role_map"

	entry, err := checkLog(maria, tableName)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] %s: SKIP (already completed)\n", step, tableName)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] %s: resuming from offset %d\n", step, tableName, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT id, prod_id, site, acct, type, org_code, role_name, created_at, updated_at
			FROM role_map
			ORDER BY id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("%s query: %w", tableName, err)
		}

		type rmRow struct {
			id, prodID           string
			site, acct, typ      sql.NullString
			orgCode, roleName    sql.NullString
			createdAt, updatedAt time.Time
		}
		var batch []rmRow
		for rows.Next() {
			var r rmRow
			if err := rows.Scan(&r.id, &r.prodID, &r.site, &r.acct, &r.typ,
				&r.orgCode, &r.roleName, &r.createdAt, &r.updatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("%s scan: %w", tableName, err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] %s: batch offset=%d rows=%d\n", step, tableName, offset, len(batch))
		for _, r := range batch {
			_, err := maria.Exec(`
				INSERT IGNORE INTO role_map
				    (id, prod_id, site, acct, type, org_code, role_name, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				r.id, r.prodID, r.site, r.acct, r.typ,
				r.orgCode, r.roleName,
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("%s insert: %w", tableName, err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, tableName, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, tableName, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] %s: done (%d total rows)\n", step, tableName, total)
	return nil
}

func migrateReleaseUnits(oracle, maria *sql.DB, step int) error {
	const tableName = "release_unit_info"
	fmt.Printf("[%d/8] %s: loading EAV config...\n", step, tableName)

	cfgMap, err := fetchEAVConfig(oracle,
		"release_unit_config", "ap_id", "ap_config_param", "ap_config_val")
	if err != nil {
		return err
	}

	entry, err := checkLog(maria, tableName)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] %s: SKIP (already completed)\n", step, tableName)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] %s: resuming from offset %d\n", step, tableName, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT ap_id, prod_id, ap_name, dev_nt_acct, created_at, updated_at
			FROM release_unit_info
			ORDER BY ap_id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("%s query: %w", tableName, err)
		}

		type ruRow struct {
			apID, prodID         string
			apName               string
			devNT                sql.NullString
			createdAt, updatedAt time.Time
		}
		var batch []ruRow
		for rows.Next() {
			var r ruRow
			if err := rows.Scan(&r.apID, &r.prodID, &r.apName, &r.devNT,
				&r.createdAt, &r.updatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("%s scan: %w", tableName, err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] %s: batch offset=%d rows=%d\n", step, tableName, offset, len(batch))
		for _, r := range batch {
			_, err := maria.Exec(`
				INSERT IGNORE INTO release_unit_info
				    (ap_id, prod_id, ap_name, dev_nt_acct, config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				r.apID, r.prodID, r.apName, r.devNT,
				configJSON(cfgMap[r.apID]),
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("%s insert: %w", tableName, err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, tableName, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, tableName, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] %s: done (%d total rows)\n", step, tableName, total)
	return nil
}

// migrateReleaseGroups loads Oracle release_packages into the MariaDB release_group
// table. release_product_info has been removed from Oracle; its data was folded
// into release_packages (old_rp_id stores the legacy reference).
func migrateReleaseGroups(oracle, maria *sql.DB, step int) error {
	const logKey = "release_packages"

	entry, err := checkLog(maria, logKey)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] release_group: SKIP (already completed)\n", step)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] release_group: resuming from offset %d\n", step, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT package_id, prod_id, name, description,
			       acronym, ap_level, owner, cd_details,
			       old_rp_id, change_level, version, is_deleted,
			       created_at, updated_at
			FROM release_packages
			ORDER BY package_id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("release_group query: %w", err)
		}

		type rgRow struct {
			id                   string
			prodID               sql.NullString
			name                 string
			description          sql.NullString
			acronym, apLevel     sql.NullString
			owner, cdDetails     sql.NullString
			oldRpID, changeLevel sql.NullString
			version              sql.NullInt64
			isDeleted            int64
			createdAt, updatedAt time.Time
		}
		var batch []rgRow
		for rows.Next() {
			var r rgRow
			if err := rows.Scan(
				&r.id, &r.prodID, &r.name, &r.description,
				&r.acronym, &r.apLevel, &r.owner, &r.cdDetails,
				&r.oldRpID, &r.changeLevel, &r.version, &r.isDeleted,
				&r.createdAt, &r.updatedAt,
			); err != nil {
				rows.Close()
				return fmt.Errorf("release_group scan: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] release_group: batch offset=%d rows=%d\n", step, offset, len(batch))
		for _, r := range batch {
			_, err := maria.Exec(`
				INSERT IGNORE INTO release_group
				    (group_id, prod_id, group_name, group_description,
				     acronym, ap_level, owner, cd_details,
				     old_rp_id, change_level, version, is_deleted,
				     created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				r.id, r.prodID, r.name, r.description,
				r.acronym, r.apLevel, r.owner, r.cdDetails,
				r.oldRpID, r.changeLevel, r.version, r.isDeleted,
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("release_group insert: %w", err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, logKey, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, logKey, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] release_group: done (%d total rows)\n", step, total)
	return nil
}

// migrateReleaseGroupRUMap loads rp_map (rp_id→group_id, ru_id→ap_id) and
// rp_ru_mapping (release_package_id→group_id, release_unit_id→ap_id) into
// release_group_ru_map. Both Oracle tables now reference release_packages.
func migrateReleaseGroupRUMap(oracle, maria *sql.DB, step int) error {
	type sourceSpec struct {
		logKey string
		query  string
	}
	// Column order: group_id first, ap_id second — matches INSERT below.
	sources := []sourceSpec{
		{
			logKey: "rp_map",
			// rp_id → group_id, ru_id → ap_id
			query: `SELECT rp_id, ru_id, created_at, updated_at
			        FROM rp_map ORDER BY rp_id, ru_id
			        OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
		},
		{
			logKey: "rp_ru_mapping",
			// release_package_id → group_id, release_unit_id → ap_id
			query: `SELECT release_package_id, release_unit_id, created_at, updated_at
			        FROM rp_ru_mapping ORDER BY release_package_id, release_unit_id
			        OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
		},
	}

	for _, src := range sources {
		entry, err := checkLog(maria, src.logKey)
		if err != nil {
			return err
		}
		if entry.status == "completed" {
			fmt.Printf("[%d/8] release_group_ru_map (%s): SKIP (already completed)\n", step, src.logKey)
			continue
		}

		offset := entry.lastOffset
		total := entry.rowsMigrated
		if offset > 0 {
			fmt.Printf("[%d/8] release_group_ru_map (%s): resuming from offset %d\n",
				step, src.logKey, offset)
		}

		for {
			rows, err := oracle.Query(src.query, offset, batchSize)
			if err != nil {
				return fmt.Errorf("release_group_ru_map(%s) query: %w", src.logKey, err)
			}

			type mapRow struct {
				groupID, apID        string
				createdAt, updatedAt time.Time
			}
			var batch []mapRow
			for rows.Next() {
				var r mapRow
				if err := rows.Scan(&r.groupID, &r.apID,
					&r.createdAt, &r.updatedAt); err != nil {
					rows.Close()
					return fmt.Errorf("release_group_ru_map(%s) scan: %w", src.logKey, err)
				}
				batch = append(batch, r)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			if len(batch) == 0 {
				break
			}

			fmt.Printf("[%d/8] release_group_ru_map (%s): batch offset=%d rows=%d\n",
				step, src.logKey, offset, len(batch))
			for _, r := range batch {
				_, err := maria.Exec(`
					INSERT IGNORE INTO release_group_ru_map
					    (group_id, ap_id, created_at, updated_at)
					VALUES (?, ?, ?, ?)`,
					r.groupID, r.apID, r.createdAt.UTC(), r.updatedAt.UTC())
				if err != nil {
					return fmt.Errorf("release_group_ru_map(%s) insert: %w", src.logKey, err)
				}
			}

			total += len(batch)
			offset += len(batch)
			if err := updateLog(maria, src.logKey, offset, total); err != nil {
				return err
			}
			if len(batch) < batchSize {
				break
			}
		}

		if err := markCompleted(maria, src.logKey, total); err != nil {
			return err
		}
		fmt.Printf("[%d/8] release_group_ru_map (%s): done (%d total rows)\n",
			step, src.logKey, total)
	}
	return nil
}

func migrateDeployUnits(oracle, maria *sql.DB, step int) error {
	const tableName = "paas_deploy_unit"
	fmt.Printf("[%d/8] %s: loading deploy status history...\n", step, tableName)

	statusMap, err := fetchDeployStatusMap(oracle)
	if err != nil {
		return err
	}

	entry, err := checkLog(maria, tableName)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] %s: SKIP (already completed)\n", step, tableName)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] %s: resuming from offset %d\n", step, tableName, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT unit_id, ap_id, created_at, updated_at
			FROM paas_deploy_unit
			ORDER BY unit_id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("%s query: %w", tableName, err)
		}

		type duRow struct {
			unitID, apID         string
			createdAt, updatedAt time.Time
		}
		var batch []duRow
		for rows.Next() {
			var r duRow
			if err := rows.Scan(&r.unitID, &r.apID,
				&r.createdAt, &r.updatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("%s scan: %w", tableName, err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] %s: batch offset=%d rows=%d\n", step, tableName, offset, len(batch))
		for _, r := range batch {
			sd := statusMap[r.unitID]
			histJSON := sd.historyJSON
			if histJSON == "" {
				histJSON = "[]"
			}
			_, err := maria.Exec(`
				INSERT IGNORE INTO paas_deploy_unit
				    (unit_id, ap_id, deploy_status, deploy_message, status_history,
				     created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				r.unitID, r.apID,
				sd.latestStatus, sd.latestMessage, histJSON,
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("%s insert: %w", tableName, err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, tableName, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, tableName, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] %s: done (%d total rows)\n", step, tableName, total)
	return nil
}

func migratePaasRlseInfo(oracle, maria *sql.DB, step int) error {
	const tableName = "paas_rlse_info"

	entry, err := checkLog(maria, tableName)
	if err != nil {
		return err
	}
	if entry.status == "completed" {
		fmt.Printf("[%d/8] %s: SKIP (already completed)\n", step, tableName)
		return nil
	}

	offset := entry.lastOffset
	total := entry.rowsMigrated
	if offset > 0 {
		fmt.Printf("[%d/8] %s: resuming from offset %d\n", step, tableName, offset)
	}

	for {
		rows, err := oracle.Query(`
			SELECT rlse_id, ap_id, rlse_name, rlse_description, created_at, updated_at
			FROM paas_rlse_info
			ORDER BY rlse_id
			OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			offset, batchSize)
		if err != nil {
			return fmt.Errorf("%s query: %w", tableName, err)
		}

		type rlseRow struct {
			rlseID, apID         string
			name, description    sql.NullString
			createdAt, updatedAt time.Time
		}
		var batch []rlseRow
		for rows.Next() {
			var r rlseRow
			if err := rows.Scan(&r.rlseID, &r.apID, &r.name, &r.description,
				&r.createdAt, &r.updatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("%s scan: %w", tableName, err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		fmt.Printf("[%d/8] %s: batch offset=%d rows=%d\n", step, tableName, offset, len(batch))
		for _, r := range batch {
			_, err := maria.Exec(`
				INSERT IGNORE INTO paas_rlse_info
				    (rlse_id, ap_id, rlse_name, rlse_description, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
				r.rlseID, r.apID, r.name, r.description,
				r.createdAt.UTC(), r.updatedAt.UTC())
			if err != nil {
				return fmt.Errorf("%s insert: %w", tableName, err)
			}
		}

		total += len(batch)
		offset += len(batch)
		if err := updateLog(maria, tableName, offset, total); err != nil {
			return err
		}
		if len(batch) < batchSize {
			break
		}
	}

	if err := markCompleted(maria, tableName, total); err != nil {
		return err
	}
	fmt.Printf("[%d/8] %s: done (%d total rows)\n", step, tableName, total)
	return nil
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	oracle, err := connectOracle()
	if err != nil {
		log.Fatalf("Oracle connection failed: %v", err)
	}
	defer oracle.Close()

	maria, err := connectMariaDB()
	if err != nil {
		log.Fatalf("MariaDB connection failed: %v", err)
	}
	defer maria.Close()

	if err := ensureMigrationLog(maria); err != nil {
		log.Fatalf("Failed to create migration log table: %v", err)
	}

	type step struct {
		fn func(*sql.DB, *sql.DB, int) error
	}
	steps := []step{
		{migrateProductSuites},
		{migrateProducts},
		{migrateRoleMap},
		{migrateReleaseUnits},
		{migrateReleaseGroups},
		{migrateReleaseGroupRUMap},
		{migrateDeployUnits},
		{migratePaasRlseInfo},
	}

	for i, s := range steps {
		if err := s.fn(oracle, maria, i+1); err != nil {
			log.Fatalf("Step %d failed: %v", i+1, err)
		}
	}

	fmt.Println("Migration complete.")
}
