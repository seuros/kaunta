-- Rename "user" table to "users" and normalize columns for Kaunta
-- Handles multiple scenarios:
-- 1. Fresh install: "user" table exists from migration 000005
-- 2. Existing Kaunta: "users" table already exists (skip rename)
-- 3. Umami v2/v3 migration: "user" table exists with Umami columns

-- Step 1: Rename table if needed
DO $$
BEGIN
    -- If 'users' already exists (existing Kaunta install), skip rename
    IF EXISTS (SELECT 1 FROM information_schema.tables
               WHERE table_schema = 'public' AND table_name = 'users') THEN
        RAISE NOTICE 'Table "users" already exists, skipping rename';
    -- If 'user' exists (Umami migration or fresh install), rename it
    ELSIF EXISTS (SELECT 1 FROM information_schema.tables
                  WHERE table_schema = 'public' AND table_name = 'user') THEN
        -- Drop FK constraints first
        ALTER TABLE user_sessions DROP CONSTRAINT IF EXISTS user_sessions_user_id_fkey;
        ALTER TABLE website DROP CONSTRAINT IF EXISTS website_user_id_fkey;

        -- Rename the table
        ALTER TABLE "user" RENAME TO users;
        RAISE NOTICE 'Renamed "user" to "users"';

        -- Recreate FK constraints
        ALTER TABLE user_sessions
            ADD CONSTRAINT user_sessions_user_id_fkey
            FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE;
        ALTER TABLE website
            ADD CONSTRAINT website_user_id_fkey
            FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE SET NULL;
    END IF;
END $$;

-- Step 2: Rename password → password_hash if needed
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'users' AND column_name = 'password') THEN
        ALTER TABLE users RENAME COLUMN password TO password_hash;
        RAISE NOTICE 'Renamed column "password" to "password_hash"';
    END IF;
END $$;

-- Step 3: Add name column if missing
ALTER TABLE users ADD COLUMN IF NOT EXISTS name VARCHAR(255);

-- Step 4: Handle Umami v3 display_name → name
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'users' AND column_name = 'display_name') THEN
        UPDATE users SET name = display_name WHERE name IS NULL AND display_name IS NOT NULL;
        ALTER TABLE users DROP COLUMN display_name;
        RAISE NOTICE 'Migrated display_name to name';
    END IF;
END $$;

-- Step 5: Drop columns not used by Kaunta
ALTER TABLE users DROP COLUMN IF EXISTS role;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE users DROP COLUMN IF EXISTS logo_url;

-- Step 6: Rename index if exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_user_username') THEN
        ALTER INDEX idx_user_username RENAME TO idx_users_username;
        RAISE NOTICE 'Renamed index to idx_users_username';
    END IF;
END $$;

-- Step 7: Update validate_session function to use "users" table
CREATE OR REPLACE FUNCTION validate_session(p_token_hash VARCHAR)
RETURNS TABLE (user_id UUID, username VARCHAR, session_id UUID) AS $$
BEGIN
    -- Update last_used_at and return user info
    UPDATE user_sessions
    SET last_used_at = NOW()
    WHERE token_hash = p_token_hash
      AND expires_at > NOW()
    RETURNING user_sessions.user_id, user_sessions.session_id
    INTO validate_session.user_id, validate_session.session_id;

    IF FOUND THEN
        SELECT u.username INTO validate_session.username
        FROM users u
        WHERE u.user_id = validate_session.user_id;

        RETURN NEXT;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Update comments
COMMENT ON TABLE users IS 'Application users with login credentials';
