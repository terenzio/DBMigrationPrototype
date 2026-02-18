-- Sample seed data for all 14 tables
-- Runs after schema-oracle.sql to populate tables with 2 rows each

ALTER SESSION SET CONTAINER = XEPDB1;
ALTER SESSION SET CURRENT_SCHEMA = SYSTEM;

-- ============================================================
-- Product Suites
-- ============================================================

INSERT INTO product_suites_info (prod_suite_id, prod_suite_name, prod_suite_owner_nt_acct, prod_suite_site_owner_acct, division)
VALUES ('ps-001', 'Cloud Platform Suite', 'alice', 'alice_site', 'Engineering');

INSERT INTO product_suites_info (prod_suite_id, prod_suite_name, prod_suite_owner_nt_acct, prod_suite_site_owner_acct, division)
VALUES ('ps-002', 'Data Analytics Suite', 'bob', 'bob_site', 'Data Science');

-- ============================================================
-- Product Suite Config
-- ============================================================

INSERT INTO product_suite_config (prod_suite_config_id, prod_suite_id, prod_suite_config_param, prod_suite_config_val)
VALUES ('psc-001', 'ps-001', 'max_products', '50');

INSERT INTO product_suite_config (prod_suite_config_id, prod_suite_id, prod_suite_config_param, prod_suite_config_val)
VALUES ('psc-002', 'ps-002', 'max_products', '25');

-- ============================================================
-- Products
-- ============================================================

INSERT INTO product_info (prod_id, prod_name, prod_short_name, mgr_nt_acct, prod_owner_nt_acct, prod_plat_name, prod_name_alias, prod_suite_id)
VALUES ('prod-001', 'API Gateway', 'APIGW', 'charlie', 'alice', 'Linux', 'Gateway', 'ps-001');

INSERT INTO product_info (prod_id, prod_name, prod_short_name, mgr_nt_acct, prod_owner_nt_acct, prod_plat_name, prod_name_alias, prod_suite_id)
VALUES ('prod-002', 'Data Pipeline', 'DPIPE', 'diana', 'bob', 'Linux', 'Pipeline', 'ps-002');

-- ============================================================
-- Product Config
-- ============================================================

INSERT INTO product_config (config_id, prod_id, prod_config_param, prod_config_val)
VALUES ('pc-001', 'prod-001', 'rate_limit', '1000');

INSERT INTO product_config (config_id, prod_id, prod_config_param, prod_config_val)
VALUES ('pc-002', 'prod-002', 'batch_size', '500');

-- ============================================================
-- Role Map
-- ============================================================

INSERT INTO role_map (id, prod_id, site, acct, type, org_code, role_name)
VALUES ('role-001', 'prod-001', 'US-West', 'charlie', 'admin', 'ENG-01', 'Product Admin');

INSERT INTO role_map (id, prod_id, site, acct, type, org_code, role_name)
VALUES ('role-002', 'prod-002', 'US-East', 'diana', 'developer', 'DS-01', 'Lead Developer');

-- ============================================================
-- Release Units
-- ============================================================

INSERT INTO release_unit_info (ap_id, prod_id, ap_name, dev_nt_acct)
VALUES ('ru-001', 'prod-001', 'apigw-core', 'eve');

INSERT INTO release_unit_info (ap_id, prod_id, ap_name, dev_nt_acct)
VALUES ('ru-002', 'prod-002', 'dpipe-ingest', 'frank');

-- ============================================================
-- Release Unit Config
-- ============================================================

INSERT INTO release_unit_config (ap_config_id, ap_id, ap_config_param, ap_config_val)
VALUES ('ruc-001', 'ru-001', 'deploy_target', 'kubernetes');

INSERT INTO release_unit_config (ap_config_id, ap_id, ap_config_param, ap_config_val)
VALUES ('ruc-002', 'ru-002', 'deploy_target', 'docker-compose');

-- ============================================================
-- Release Products
-- ============================================================

INSERT INTO release_product_info (rp_id, rp_name, rp_description)
VALUES ('rp-001', 'Cloud Platform v2.0', 'Major release with API Gateway improvements');

INSERT INTO release_product_info (rp_id, rp_name, rp_description)
VALUES ('rp-002', 'Data Analytics v1.5', 'Minor release with pipeline optimizations');

-- ============================================================
-- RP Map (Release Unit <-> Release Product)
-- ============================================================

INSERT INTO rp_map (ru_id, rp_id) VALUES ('ru-001', 'rp-001');
INSERT INTO rp_map (ru_id, rp_id) VALUES ('ru-002', 'rp-002');

-- ============================================================
-- PaaS Deploy Units
-- ============================================================

INSERT INTO paas_deploy_unit (unit_id, ap_id) VALUES ('pdu-001', 'ru-001');
INSERT INTO paas_deploy_unit (unit_id, ap_id) VALUES ('pdu-002', 'ru-002');

-- ============================================================
-- PaaS Deploy Status
-- ============================================================

INSERT INTO paas_deploy_status (status_id, unit_id, deploy_status, deploy_message)
VALUES ('ds-001', 'pdu-001', 'SUCCESS', 'Deployed to production cluster');

INSERT INTO paas_deploy_status (status_id, unit_id, deploy_status, deploy_message)
VALUES ('ds-002', 'pdu-002', 'PENDING', 'Awaiting staging approval');

-- ============================================================
-- PaaS Release Info
-- ============================================================

INSERT INTO paas_rlse_info (rlse_id, ap_id, rlse_name, rlse_description)
VALUES ('rl-001', 'ru-001', 'APIGW Release 2.0.1', 'Hotfix for rate limiting bug');

INSERT INTO paas_rlse_info (rlse_id, ap_id, rlse_name, rlse_description)
VALUES ('rl-002', 'ru-002', 'Pipeline Release 1.5.0', 'New Kafka connector support');

-- ============================================================
-- Release Packages
-- ============================================================

INSERT INTO release_packages (package_id, package_name, package_description)
VALUES ('pkg-001', 'cloud-platform-bundle', 'Full Cloud Platform deployment package');

INSERT INTO release_packages (package_id, package_name, package_description)
VALUES ('pkg-002', 'data-analytics-bundle', 'Full Data Analytics deployment package');

-- ============================================================
-- RP-RU Mapping (Release Package <-> Release Unit)
-- ============================================================

INSERT INTO rp_ru_mapping (package_id, ap_id) VALUES ('pkg-001', 'ru-001');
INSERT INTO rp_ru_mapping (package_id, ap_id) VALUES ('pkg-002', 'ru-002');

COMMIT;
