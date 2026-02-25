-- ============================================================
-- Optimized MariaDB Schema
-- Product / Release Unit / Deployment hierarchy
-- Requires MariaDB 10.2+ with InnoDB engine
--
-- Changes from original:
--   1. Eliminated 3 EAV config tables → JSON columns on parent tables
--   2. release_packages (enriched; release_product_info removed) → release_group
--   3. Merged rp_map + rp_ru_mapping (both from release_packages) → release_group_ru_map
--   4. Merged paas_deploy_status into paas_deploy_unit
--   5. Added indexes on role_map for common query patterns
--   6. Result: 13 Oracle tables → 8 MariaDB tables
-- ============================================================

-- ============================================================
-- Product Suite
-- ============================================================

CREATE TABLE product_suites_info (
    prod_suite_id              VARCHAR(64)   NOT NULL,
    prod_suite_name            VARCHAR(255)  NOT NULL,
    prod_suite_owner_nt_acct   VARCHAR(128)  DEFAULT NULL,
    prod_suite_site_owner_acct VARCHAR(128)  DEFAULT NULL,
    division                   VARCHAR(128)  DEFAULT NULL,
    config                     JSON          DEFAULT '{}' COMMENT 'Key-value config params (replaces product_suite_config table)',
    created_at                 DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (prod_suite_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Product
-- ============================================================

CREATE TABLE product_info (
    prod_id            VARCHAR(64)   NOT NULL,
    prod_name          VARCHAR(255)  NOT NULL,
    prod_short_name    VARCHAR(64)   DEFAULT NULL,
    mgr_nt_acct        VARCHAR(128)  DEFAULT NULL,
    prod_owner_nt_acct VARCHAR(128)  DEFAULT NULL,
    prod_plat_name     VARCHAR(255)  DEFAULT NULL,
    prod_name_alias    VARCHAR(255)  DEFAULT NULL,
    prod_suite_id      VARCHAR(64)   NOT NULL,
    config             JSON          DEFAULT '{}' COMMENT 'Key-value config params (replaces product_config table)',
    created_at         DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (prod_id),
    INDEX idx_product_suite (prod_suite_id),
    CONSTRAINT fk_product_suite
        FOREIGN KEY (prod_suite_id) REFERENCES product_suites_info (prod_suite_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Role Map
-- ============================================================

CREATE TABLE role_map (
    id        VARCHAR(64)  NOT NULL,
    prod_id   VARCHAR(64)  NOT NULL,
    site      VARCHAR(128) DEFAULT NULL,
    acct      VARCHAR(128) DEFAULT NULL,
    type      VARCHAR(64)  DEFAULT NULL,
    org_code  VARCHAR(64)  DEFAULT NULL,
    role_name VARCHAR(128) DEFAULT NULL,
    created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    INDEX idx_role_map_prod_id (prod_id),
    INDEX idx_role_map_acct (acct),
    INDEX idx_role_map_prod_role (prod_id, role_name),
    CONSTRAINT fk_role_map_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Release Unit
-- ============================================================

CREATE TABLE release_unit_info (
    ap_id       VARCHAR(64)  NOT NULL,
    prod_id     VARCHAR(64)  NOT NULL,
    ap_name     VARCHAR(255) NOT NULL,
    dev_nt_acct VARCHAR(128) DEFAULT NULL,
    config      JSON         DEFAULT '{}' COMMENT 'Key-value config params (replaces release_unit_config table)',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (ap_id),
    INDEX idx_release_unit_prod (prod_id),
    CONSTRAINT fk_release_unit_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Release Group  (sourced from enriched release_packages only)
-- ============================================================

CREATE TABLE release_group (
    group_id          VARCHAR(64)   NOT NULL,
    prod_id           VARCHAR(64)   DEFAULT NULL COMMENT 'References product_info',
    group_name        VARCHAR(255)  NOT NULL,
    group_description VARCHAR(1024) DEFAULT NULL,
    acronym           VARCHAR(128)  DEFAULT NULL,
    ap_level          VARCHAR(128)  DEFAULT NULL,
    owner             JSON          DEFAULT NULL COMMENT 'Owner metadata from release_packages',
    cd_details        TEXT          DEFAULT NULL,
    old_rp_id         VARCHAR(64)   DEFAULT NULL COMMENT 'Legacy release_product_info.rp_id reference',
    change_level      VARCHAR(128)  DEFAULT NULL,
    version           INT           DEFAULT NULL,
    is_deleted        TINYINT(1)    NOT NULL DEFAULT 0,
    created_at        DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id),
    INDEX idx_release_group_prod (prod_id),
    INDEX idx_release_group_deleted (is_deleted),
    CONSTRAINT fk_release_group_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Release Group ↔ Release Unit Mapping  (merged: rp_map + rp_ru_mapping)
-- ============================================================

CREATE TABLE release_group_ru_map (
    group_id   VARCHAR(64) NOT NULL,
    ap_id      VARCHAR(64) NOT NULL,
    created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, ap_id),
    INDEX idx_rg_ru_map_ap (ap_id),
    CONSTRAINT fk_rg_ru_map_group
        FOREIGN KEY (group_id) REFERENCES release_group (group_id)
        ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_rg_ru_map_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- PaaS Deploy Unit  (merged: paas_deploy_unit + paas_deploy_status)
-- ============================================================

CREATE TABLE paas_deploy_unit (
    unit_id        VARCHAR(64)   NOT NULL,
    ap_id          VARCHAR(64)   NOT NULL,
    deploy_status  VARCHAR(64)   DEFAULT NULL  COMMENT 'Current deployment status',
    deploy_message VARCHAR(1024) DEFAULT NULL  COMMENT 'Current status message',
    status_history JSON          DEFAULT '[]'  COMMENT 'Array of {status, message, at} for audit trail',
    created_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (unit_id),
    INDEX idx_deploy_unit_ap (ap_id),
    INDEX idx_deploy_unit_status (deploy_status),
    CONSTRAINT fk_paas_deploy_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- PaaS Release Info
-- ============================================================

CREATE TABLE paas_rlse_info (
    rlse_id          VARCHAR(64)   NOT NULL,
    ap_id            VARCHAR(64)   NOT NULL,
    rlse_name        VARCHAR(255)  DEFAULT NULL,
    rlse_description VARCHAR(1024) DEFAULT NULL,
    created_at       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (rlse_id),
    INDEX idx_paas_rlse_ap (ap_id),
    CONSTRAINT fk_paas_rlse_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- ============================================================
-- Example: JSON config usage
-- ============================================================

-- Insert with config
-- INSERT INTO product_info (prod_id, prod_name, prod_suite_id, config)
-- VALUES ('P001', 'MyProduct', 'S001', '{"timeout": "30", "retry_count": "3", "feature_flag_x": "true"}');

-- Read a single config param
-- SELECT JSON_VALUE(config, '$.timeout') AS timeout
-- FROM product_info WHERE prod_id = 'P001';

-- Update a single config param (without touching others)
-- UPDATE product_info
-- SET config = JSON_SET(config, '$.timeout', '60')
-- WHERE prod_id = 'P001';

-- Remove a config param
-- UPDATE product_info
-- SET config = JSON_REMOVE(config, '$.feature_flag_x')
-- WHERE prod_id = 'P001';

-- Append to deploy status history
-- UPDATE paas_deploy_unit
-- SET deploy_status  = 'DEPLOYED',
--     deploy_message = 'Rollout complete',
--     status_history = JSON_ARRAY_APPEND(
--         status_history, '$',
--         JSON_OBJECT('status', 'DEPLOYED', 'message', 'Rollout complete', 'at', NOW())
--     )
-- WHERE unit_id = 'U001';