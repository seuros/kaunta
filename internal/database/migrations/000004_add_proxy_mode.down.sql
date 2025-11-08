-- Rollback: Remove proxy_mode column and related constraints

DROP INDEX IF EXISTS idx_website_proxy_mode;
ALTER TABLE website DROP CONSTRAINT check_proxy_mode;
ALTER TABLE website DROP COLUMN proxy_mode;
