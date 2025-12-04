-- Reverse: users → user, password_hash → password (restore Umami-compatible schema)

-- Step 1: Drop FK constraints
ALTER TABLE user_sessions DROP CONSTRAINT IF EXISTS user_sessions_user_id_fkey;
ALTER TABLE website DROP CONSTRAINT IF EXISTS website_user_id_fkey;

-- Step 2: Rename table
ALTER TABLE users RENAME TO "user";

-- Step 3: Recreate FK constraints
ALTER TABLE user_sessions
    ADD CONSTRAINT user_sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES "user"(user_id) ON DELETE CASCADE;
ALTER TABLE website
    ADD CONSTRAINT website_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES "user"(user_id) ON DELETE SET NULL;

-- Step 4: Rename column
ALTER TABLE "user" RENAME COLUMN password_hash TO password;

-- Step 5: Restore Umami columns
ALTER TABLE "user" ADD COLUMN IF NOT EXISTS role VARCHAR(50) DEFAULT 'admin';
ALTER TABLE "user" ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

-- Step 6: Rename index
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_users_username') THEN
        ALTER INDEX idx_users_username RENAME TO idx_user_username;
    END IF;
END $$;

-- Step 7: Update validate_session function to use "user" table
CREATE OR REPLACE FUNCTION validate_session(p_token_hash VARCHAR)
RETURNS TABLE (user_id UUID, username VARCHAR, session_id UUID) AS $$
BEGIN
    UPDATE user_sessions
    SET last_used_at = NOW()
    WHERE token_hash = p_token_hash
      AND expires_at > NOW()
    RETURNING user_sessions.user_id, user_sessions.session_id
    INTO validate_session.user_id, validate_session.session_id;

    IF FOUND THEN
        SELECT u.username INTO validate_session.username
        FROM "user" u
        WHERE u.user_id = validate_session.user_id;

        RETURN NEXT;
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE "user" IS 'Application users with login credentials (Umami-compatible)';
