-- MariaDB Schema for Product / Release Unit / Deployment hierarchy
-- Requires MariaDB 10.2+ with InnoDB engine
-- Tables are created in dependency order (parents before children)

-- ============================================================
-- Product Suite
-- ============================================================

CREATE TABLE product_suites_info (
    prod_suite_id            VARCHAR(64)  NOT NULL,
    prod_suite_name          VARCHAR(255) NOT NULL,
    prod_suite_owner_nt_acct VARCHAR(128) DEFAULT NULL,
    prod_suite_site_owner_acct VARCHAR(128) DEFAULT NULL,
    division                 VARCHAR(128) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (prod_suite_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE product_suite_config (
    prod_suite_config_id     VARCHAR(64)  NOT NULL,
    prod_suite_id            VARCHAR(64)  NOT NULL,
    prod_suite_config_param  VARCHAR(255) NOT NULL,
    prod_suite_config_val    VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (prod_suite_config_id),
    CONSTRAINT fk_suite_config_suite
        FOREIGN KEY (prod_suite_id) REFERENCES product_suites_info (prod_suite_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Product
-- ============================================================

CREATE TABLE product_info (
    prod_id                  VARCHAR(64)  NOT NULL,
    prod_name                VARCHAR(255) NOT NULL,
    prod_short_name          VARCHAR(64)  DEFAULT NULL,
    mgr_nt_acct              VARCHAR(128) DEFAULT NULL,
    prod_owner_nt_acct       VARCHAR(128) DEFAULT NULL,
    prod_plat_name           VARCHAR(255) DEFAULT NULL,
    prod_name_alias          VARCHAR(255) DEFAULT NULL,
    prod_suite_id            VARCHAR(64)  NOT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (prod_id),
    CONSTRAINT fk_product_suite
        FOREIGN KEY (prod_suite_id) REFERENCES product_suites_info (prod_suite_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE product_config (
    config_id                VARCHAR(64)  NOT NULL,
    prod_id                  VARCHAR(64)  NOT NULL,
    prod_config_param        VARCHAR(255) NOT NULL,
    prod_config_val          VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (config_id),
    CONSTRAINT fk_prod_config_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE role_map (
    id                       VARCHAR(64)  NOT NULL,
    prod_id                  VARCHAR(64)  NOT NULL,
    site                     VARCHAR(128) DEFAULT NULL,
    acct                     VARCHAR(128) DEFAULT NULL,
    type                     VARCHAR(64)  DEFAULT NULL,
    org_code                 VARCHAR(64)  DEFAULT NULL,
    role_name                VARCHAR(128) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_role_map_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Release Unit
-- ============================================================

CREATE TABLE release_unit_info (
    ap_id                    VARCHAR(64)  NOT NULL,
    prod_id                  VARCHAR(64)  NOT NULL,
    ap_name                  VARCHAR(255) NOT NULL,
    dev_nt_acct              VARCHAR(128) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (ap_id),
    CONSTRAINT fk_release_unit_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE release_unit_config (
    ap_config_id             VARCHAR(64)  NOT NULL,
    ap_id                    VARCHAR(64)  NOT NULL,
    ap_config_param          VARCHAR(255) NOT NULL,
    ap_config_val            VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (ap_config_id),
    CONSTRAINT fk_ru_config_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Release Product
-- ============================================================

CREATE TABLE release_product_info (
    rp_id                    VARCHAR(64)  NOT NULL,
    rp_name                  VARCHAR(255) NOT NULL,
    rp_description           VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (rp_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Deployment & Mapping
-- ============================================================

CREATE TABLE rp_map (
    ru_id                    VARCHAR(64)  NOT NULL,
    rp_id                    VARCHAR(64)  NOT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (ru_id, rp_id),
    CONSTRAINT fk_rp_map_release_unit
        FOREIGN KEY (ru_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_rp_map_release_product
        FOREIGN KEY (rp_id) REFERENCES release_product_info (rp_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE paas_deploy_unit (
    unit_id                  VARCHAR(64)  NOT NULL,
    ap_id                    VARCHAR(64)  NOT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (unit_id),
    CONSTRAINT fk_paas_deploy_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- PaaS Deploy Status
-- ============================================================

CREATE TABLE paas_deploy_status (
    status_id                VARCHAR(64)  NOT NULL,
    unit_id                  VARCHAR(64)  NOT NULL,
    deploy_status            VARCHAR(64)  DEFAULT NULL,
    deploy_message           VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (status_id),
    CONSTRAINT fk_deploy_status_deploy_unit
        FOREIGN KEY (unit_id) REFERENCES paas_deploy_unit (unit_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- PaaS Release Info
-- ============================================================

CREATE TABLE paas_rlse_info (
    rlse_id                  VARCHAR(64)  NOT NULL,
    ap_id                    VARCHAR(64)  NOT NULL,
    rlse_name                VARCHAR(255) DEFAULT NULL,
    rlse_description         VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (rlse_id),
    CONSTRAINT fk_paas_rlse_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Release Packages & RU Mapping
-- ============================================================

CREATE TABLE release_packages (
    package_id               VARCHAR(64)  NOT NULL,
    package_name             VARCHAR(255) NOT NULL,
    package_description      VARCHAR(1024) DEFAULT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (package_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE rp_ru_mapping (
    package_id               VARCHAR(64)  NOT NULL,
    ap_id                    VARCHAR(64)  NOT NULL,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (package_id, ap_id),
    CONSTRAINT fk_rp_ru_mapping_package
        FOREIGN KEY (package_id) REFERENCES release_packages (package_id)
        ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_rp_ru_mapping_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
