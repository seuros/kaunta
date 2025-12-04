-- Add user and authentication tables (Umami-compatible schema)
-- Table name is "user" (singular) to match Umami v2/v3 schema
-- Migration 000020 will rename to "users" for Kaunta

-- Password helper functions (using pgcrypto extension)
CREATE OR REPLACE FUNCTION hash_password(password TEXT)
RETURNS TEXT AS $$
BEGIN
    RETURN crypt(password, gen_salt('bf', 10));
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION verify_password(password TEXT, password_hash TEXT)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN password_hash = crypt(password, password_hash);
END;
$$ LANGUAGE plpgsql;

-- User table (Umami-compatible: singular name, "password" column)
CREATE TABLE IF NOT EXISTS "user" (
    user_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(60) NOT NULL,
    role VARCHAR(50) DEFAULT 'admin',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_user_username ON "user"(username);

-- User sessions table for authentication tokens
CREATE TABLE IF NOT EXISTS user_sessions (
    session_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    user_agent VARCHAR(500),
    ip_address inet
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);

-- Add user_id to website table
ALTER TABLE website ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES "user"(user_id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_website_user_id ON website(user_id);

-- Function to cleanup expired sessions
CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM user_sessions WHERE expires_at < NOW();
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to validate session token
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
        FROM "user" u
        WHERE u.user_id = validate_session.user_id;

        RETURN NEXT;
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE "user" IS 'Application users with login credentials (Umami-compatible)';
COMMENT ON TABLE user_sessions IS 'Active user sessions with token-based authentication';
COMMENT ON COLUMN website.user_id IS 'Owner of the website. NULL for legacy/public websites.';
