-- Rollback user and authentication (Umami-compatible schema)

DROP FUNCTION IF EXISTS validate_session(VARCHAR);
DROP FUNCTION IF EXISTS verify_password(TEXT, TEXT);
DROP FUNCTION IF EXISTS hash_password(TEXT);
DROP FUNCTION IF EXISTS cleanup_expired_sessions();

DROP INDEX IF EXISTS idx_website_user_id;
ALTER TABLE website DROP COLUMN IF EXISTS user_id;

DROP INDEX IF EXISTS idx_user_sessions_expires;
DROP INDEX IF EXISTS idx_user_sessions_user_id;
DROP INDEX IF EXISTS idx_user_sessions_token;
DROP TABLE IF EXISTS user_sessions;

DROP INDEX IF EXISTS idx_user_username;
DROP TABLE IF EXISTS "user";
