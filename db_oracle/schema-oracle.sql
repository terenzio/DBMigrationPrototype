-- Oracle Schema for Product / Release Unit / Deployment hierarchy
-- Compatible with Oracle 12c+
-- Tables are created in dependency order (parents before children)

-- Switch to the pluggable database and create tables under SYSTEM schema
-- (init scripts run as SYS, but the backend connects as SYSTEM)
ALTER SESSION SET CONTAINER = XEPDB1;
ALTER SESSION SET CURRENT_SCHEMA = SYSTEM;

-- ============================================================
-- Product Suite
-- ============================================================

CREATE TABLE product_suites_info (
    prod_suite_id              VARCHAR2(64)   NOT NULL,
    prod_suite_name            VARCHAR2(255)  NOT NULL,
    prod_suite_owner_nt_acct   VARCHAR2(128)  DEFAULT NULL,
    prod_suite_site_owner_acct VARCHAR2(128)  DEFAULT NULL,
    division                   VARCHAR2(128)  DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_product_suites_info PRIMARY KEY (prod_suite_id)
);

CREATE TABLE product_suite_config (
    prod_suite_config_id       VARCHAR2(64)   NOT NULL,
    prod_suite_id              VARCHAR2(64)   NOT NULL,
    prod_suite_config_param    VARCHAR2(255)  NOT NULL,
    prod_suite_config_val      VARCHAR2(1024) DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_product_suite_config PRIMARY KEY (prod_suite_config_id),
    CONSTRAINT fk_suite_config_suite
        FOREIGN KEY (prod_suite_id) REFERENCES product_suites_info (prod_suite_id)
        ON DELETE CASCADE
);

-- ============================================================
-- Product
-- ============================================================

CREATE TABLE product_info (
    prod_id                    VARCHAR2(64)   NOT NULL,
    prod_name                  VARCHAR2(255)  NOT NULL,
    prod_short_name            VARCHAR2(64)   DEFAULT NULL,
    mgr_nt_acct                VARCHAR2(128)  DEFAULT NULL,
    prod_owner_nt_acct         VARCHAR2(128)  DEFAULT NULL,
    prod_plat_name             VARCHAR2(255)  DEFAULT NULL,
    prod_name_alias            VARCHAR2(255)  DEFAULT NULL,
    prod_suite_id              VARCHAR2(64)   NOT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_product_info PRIMARY KEY (prod_id),
    CONSTRAINT fk_product_suite
        FOREIGN KEY (prod_suite_id) REFERENCES product_suites_info (prod_suite_id)
        ON DELETE CASCADE
);

CREATE TABLE product_config (
    config_id                  VARCHAR2(64)   NOT NULL,
    prod_id                    VARCHAR2(64)   NOT NULL,
    prod_config_param          VARCHAR2(255)  NOT NULL,
    prod_config_val            VARCHAR2(1024) DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_product_config PRIMARY KEY (config_id),
    CONSTRAINT fk_prod_config_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE
);

CREATE TABLE role_map (
    id                         VARCHAR2(64)   NOT NULL,
    prod_id                    VARCHAR2(64)   NOT NULL,
    site                       VARCHAR2(128)  DEFAULT NULL,
    acct                       VARCHAR2(128)  DEFAULT NULL,
    type                       VARCHAR2(64)   DEFAULT NULL,
    org_code                   VARCHAR2(64)   DEFAULT NULL,
    role_name                  VARCHAR2(128)  DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_role_map PRIMARY KEY (id),
    CONSTRAINT fk_role_map_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE
);

-- ============================================================
-- Release Unit
-- ============================================================

CREATE TABLE release_unit_info (
    ap_id                      VARCHAR2(64)   NOT NULL,
    prod_id                    VARCHAR2(64)   NOT NULL,
    ap_name                    VARCHAR2(255)  NOT NULL,
    dev_nt_acct                VARCHAR2(128)  DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_release_unit_info PRIMARY KEY (ap_id),
    CONSTRAINT fk_release_unit_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE CASCADE
);

CREATE TABLE release_unit_config (
    ap_config_id               VARCHAR2(64)   NOT NULL,
    ap_id                      VARCHAR2(64)   NOT NULL,
    ap_config_param            VARCHAR2(255)  NOT NULL,
    ap_config_val              VARCHAR2(1024) DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_release_unit_config PRIMARY KEY (ap_config_id),
    CONSTRAINT fk_ru_config_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE
);

-- ============================================================
-- Release Packages & RU Mapping
-- (release_product_info removed; its data folded into release_packages)
-- ============================================================

CREATE TABLE release_packages (
    package_id                 VARCHAR2(64)   NOT NULL,
    prod_id                    VARCHAR2(64)   DEFAULT NULL,
    name                       VARCHAR2(255)  NOT NULL,
    description                VARCHAR2(1024) DEFAULT NULL,
    acronym                    VARCHAR2(128)  DEFAULT NULL,
    ap_level                   VARCHAR2(128)  DEFAULT NULL,
    owner                      CLOB           DEFAULT NULL,
    cd_details                 CLOB           DEFAULT NULL,
    old_rp_id                  VARCHAR2(64)   DEFAULT NULL,
    change_level               VARCHAR2(128)  DEFAULT NULL,
    version                    NUMBER(10)     DEFAULT NULL,
    is_deleted                 NUMBER(1)      DEFAULT 0 NOT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_release_packages PRIMARY KEY (package_id),
    CONSTRAINT fk_release_packages_product
        FOREIGN KEY (prod_id) REFERENCES product_info (prod_id)
        ON DELETE SET NULL
);

CREATE TABLE rp_map (
    ru_id                      VARCHAR2(64)   NOT NULL,
    rp_id                      VARCHAR2(64)   NOT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_rp_map PRIMARY KEY (ru_id, rp_id),
    CONSTRAINT fk_rp_map_release_unit
        FOREIGN KEY (ru_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_rp_map_release_package
        FOREIGN KEY (rp_id) REFERENCES release_packages (package_id)
        ON DELETE CASCADE
);

CREATE TABLE rp_ru_mapping (
    release_package_id         VARCHAR2(64)   NOT NULL,
    release_unit_id            VARCHAR2(64)   NOT NULL,
    is_deleted                 NUMBER(1)      DEFAULT 0 NOT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_rp_ru_mapping PRIMARY KEY (release_package_id, release_unit_id),
    CONSTRAINT fk_rp_ru_mapping_package
        FOREIGN KEY (release_package_id) REFERENCES release_packages (package_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_rp_ru_mapping_release_unit
        FOREIGN KEY (release_unit_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE
);

-- ============================================================
-- Deployment
-- ============================================================

CREATE TABLE paas_deploy_unit (
    unit_id                    VARCHAR2(64)   NOT NULL,
    ap_id                      VARCHAR2(64)   NOT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_paas_deploy_unit PRIMARY KEY (unit_id),
    CONSTRAINT fk_paas_deploy_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE
);

-- ============================================================
-- PaaS Deploy Status
-- ============================================================

CREATE TABLE paas_deploy_status (
    status_id                  VARCHAR2(64)   NOT NULL,
    unit_id                    VARCHAR2(64)   NOT NULL,
    deploy_status              VARCHAR2(64)   DEFAULT NULL,
    deploy_message             VARCHAR2(1024) DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_paas_deploy_status PRIMARY KEY (status_id),
    CONSTRAINT fk_deploy_status_deploy_unit
        FOREIGN KEY (unit_id) REFERENCES paas_deploy_unit (unit_id)
        ON DELETE CASCADE
);

-- ============================================================
-- PaaS Release Info
-- ============================================================

CREATE TABLE paas_rlse_info (
    rlse_id                    VARCHAR2(64)   NOT NULL,
    ap_id                      VARCHAR2(64)   NOT NULL,
    rlse_name                  VARCHAR2(255)  DEFAULT NULL,
    rlse_description           VARCHAR2(1024) DEFAULT NULL,
    created_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP      DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT pk_paas_rlse_info PRIMARY KEY (rlse_id),
    CONSTRAINT fk_paas_rlse_release_unit
        FOREIGN KEY (ap_id) REFERENCES release_unit_info (ap_id)
        ON DELETE CASCADE
);
